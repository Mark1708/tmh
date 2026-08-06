package actions

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/config"
	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
)

func RecoverActiveWindow(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	cfg config.ActiveSessionConfig,
	windowID string,
	destination string,
) error {
	if _, err := activeTTL(cfg); err != nil {
		return err
	}
	if err := validateActiveWindowID(windowID); err != nil {
		return err
	}
	if err := validateRecoveryDestination(destination); err != nil {
		return err
	}
	if _, err := requireOwnedActiveSession(ctx, runner); err != nil {
		return err
	}
	exists, err := runner.HasSession(ctx, destination)
	if err != nil {
		return fmt.Errorf("inspect recovery destination %q: %w", destination, err)
	}
	if !exists {
		return fmt.Errorf("recovery destination %q: %w", destination, errs.ErrSessionNotFound)
	}
	epoch, err := reconcileActiveEpoch(ctx, runner, store)
	if err != nil {
		return err
	}
	row, found, err := findActiveRow(ctx, store, epoch.Value, windowID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("recover active window %s: %w", windowID, errs.ErrWindowNotFound)
	}
	if row.State != state.ActiveWindowOrphaned {
		return fmt.Errorf("recover active window %s in state %q: %w", windowID, row.State, errs.ErrUnsafeActiveWindow)
	}
	return recoverOrphan(ctx, runner, store, epoch.Value, windowID, destination)
}

func recoverOrphan(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	epoch string,
	windowID string,
	destination string,
) error {
	snapshot, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return err
	}
	links := snapshot.forWindow(windowID)
	if !links.hasActive && len(links.sources) == 0 {
		return fmt.Errorf("recover active window %s: %w", windowID, errs.ErrWindowNotFound)
	}
	verified, _, err := ensureLinkedWindow(ctx, runner, windowID, destination, snapshot)
	if err != nil {
		return err
	}
	if err := contextError(ctx, "recover active window"); err != nil {
		return err
	}
	if !hasSessionLink(verified.forWindow(windowID), destination) {
		return fmt.Errorf("verify recovery destination %q for %s", destination, windowID)
	}
	if verified.forWindow(windowID).hasActive {
		if _, err := unlinkVerifiedActiveAlias(ctx, runner, windowID, verified); err != nil {
			return err
		}
	}
	return deleteActiveRow(ctx, store, epoch, windowID)
}
