package actions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mark1708/tmh/internal/config"
	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
)

type activePromotion struct {
	windowID string
	source   tmux.WindowLink
	epoch    tmux.ServerEpoch
	snapshot activeSnapshot
}

func PromoteActiveWindow(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	cfg config.ActiveSessionConfig,
	target string,
	now time.Time,
) (string, error) {
	if !cfg.Enabled {
		return target, nil
	}
	ttl, err := activeTTL(cfg)
	if err != nil {
		return "", err
	}

	// Defense-in-depth: nil store should cause zero tmux mutation
	if store == nil {
		return "", fmt.Errorf("active windows feature enabled but state store is nil")
	}

	promotion, err := prepareActivePromotion(ctx, runner, store, target)
	if err != nil {
		return "", err
	}
	snapshot, linked, err := ensureLinkedWindow(
		ctx, runner, promotion.windowID, ActiveSessionName, promotion.snapshot,
	)
	if err != nil {
		return "", err
	}

	snapshot, cleanupErr := cleanupActivePlaceholders(ctx, runner, promotion.windowID, snapshot)
	if cleanupErr != nil {
		return "", rollbackActivePromotion(ctx, runner, promotion.windowID, linked, snapshot, cleanupErr)
	}

	persistErr := persistActivePromotion(ctx, store, promotion, now, ttl)
	if persistErr != nil {
		return "", rollbackActivePromotion(ctx, runner, promotion.windowID, linked, snapshot, persistErr)
	}
	return ActiveSessionName + ":" + promotion.windowID, nil
}

func prepareActivePromotion(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	target string,
) (activePromotion, error) {
	sourceSession, err := validatePromotionTarget(target)
	if err != nil {
		return activePromotion{}, err
	}
	windowID, err := runner.WindowID(ctx, target)
	if err != nil {
		return activePromotion{}, fmt.Errorf("resolve promotion target %q: %w", target, err)
	}
	if err := validateActiveWindowID(windowID); err != nil {
		return activePromotion{}, err
	}
	initial, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return activePromotion{}, err
	}
	if _, err := sourceLinkForTarget(initial.forWindow(windowID), sourceSession, windowID); err != nil {
		return activePromotion{}, err
	}
	if _, err := ensureOwnedActiveSession(ctx, runner); err != nil {
		return activePromotion{}, err
	}
	epoch, err := reconcileActiveEpoch(ctx, runner, store)
	if err != nil {
		return activePromotion{}, err
	}
	fresh, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return activePromotion{}, err
	}
	source, err := sourceLinkForTarget(fresh.forWindow(windowID), sourceSession, windowID)
	if err != nil {
		return activePromotion{}, err
	}
	return activePromotion{windowID: windowID, source: source, epoch: epoch, snapshot: fresh}, nil
}

func persistActivePromotion(
	ctx context.Context,
	store ActiveWindowStore,
	promotion activePromotion,
	now time.Time,
	ttl time.Duration,
) error {
	input := state.ActiveWindowInput{
		ServerKey:         promotion.epoch.ServerKey,
		ServerEpoch:       promotion.epoch.Value,
		WindowID:          promotion.windowID,
		SourceSessionID:   promotion.source.SessionID,
		SourceSessionName: promotion.source.SessionName,
		WindowName:        promotion.source.WindowName,
	}
	if err := store.UpsertActiveWindow(ctx, input, now, ttl); err != nil {
		return fmt.Errorf("persist active promotion %s: %w", promotion.windowID, err)
	}
	return nil
}

func rollbackActivePromotion(
	ctx context.Context,
	runner tmux.Runner,
	windowID string,
	linked bool,
	snapshot activeSnapshot,
	persistErr error,
) error {
	if !linked {
		return persistErr
	}

	// Load fresh snapshot to verify the alias actually exists and can be safely unlinked
	fresh, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return errors.Join(persistErr, fmt.Errorf("rollback verification: load snapshot: %w", err))
	}

	// Check if alias exists and is safe to unlink
	before := fresh.forWindow(windowID)
	if !before.hasActive {
		// No alias to rollback, just return the persist error
		return persistErr
	}

	// Check if we can safely unlink (preserve last link safety)
	if len(before.sources) == 0 {
		return errors.Join(persistErr, fmt.Errorf("rollback unsafe: %s would become orphan: %w", windowID, errs.ErrUnsafeActiveWindow))
	}

	// Perform the rollback unlink
	if err := contextError(ctx, "rollback active alias"); err != nil {
		return errors.Join(persistErr, fmt.Errorf("rollback aborted: %w", err))
	}

	if err := runner.UnlinkWindow(ctx, ActiveSessionName, windowID); err != nil {
		return errors.Join(persistErr, fmt.Errorf("rollback failed: unlink active alias %s: %w", windowID, err))
	}

	// Return joined error with rollback context
	return errors.Join(persistErr, fmt.Errorf("rollback completed: %s unlinked from %s", windowID, ActiveSessionName))
}

func cleanupActivePlaceholders(
	ctx context.Context,
	runner tmux.Runner,
	protectedWindowID string,
	snapshot activeSnapshot,
) (activeSnapshot, error) {
	for _, placeholder := range activePlaceholders(snapshot, protectedWindowID) {
		if activeSessionLinkCount(snapshot) <= 1 {
			break
		}
		if err := contextError(ctx, "remove active placeholder"); err != nil {
			return snapshot, err
		}
		target := ActiveSessionName + ":" + placeholder.WindowID
		if err := runner.KillWindow(ctx, target); err != nil {
			return snapshot, fmt.Errorf("remove active placeholder %s: %w", placeholder.WindowID, err)
		}
		fresh, err := loadActiveSnapshot(ctx, runner)
		if err != nil {
			return snapshot, err
		}
		if fresh.forWindow(placeholder.WindowID).hasActive {
			return fresh, fmt.Errorf("verify active placeholder removal %s", placeholder.WindowID)
		}
		snapshot = fresh
	}
	return snapshot, nil
}

func activePlaceholders(snapshot activeSnapshot, protectedWindowID string) []tmux.WindowLink {
	placeholders := make([]tmux.WindowLink, 0)
	for _, link := range snapshot.links {
		links := snapshot.forWindow(link.WindowID)
		if link.SessionName == ActiveSessionName &&
			link.WindowName == activePlaceholderWindowName &&
			link.WindowID != protectedWindowID &&
			len(links.sources) == 0 {
			placeholders = append(placeholders, link)
		}
	}
	return placeholders
}

func activeSessionLinkCount(snapshot activeSnapshot) int {
	count := 0
	for _, link := range snapshot.links {
		if link.SessionName == ActiveSessionName {
			count++
		}
	}
	return count
}
