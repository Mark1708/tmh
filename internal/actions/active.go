package actions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark1708/tmh/internal/config"
	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
)

const (
	ActiveSessionName           = "active"
	ActiveOwnerOption           = "@tmh-active-owner"
	ActiveOwnerValue            = "tmh/v1"
	activePlaceholderWindowName = "__tmh_active_placeholder__"
	activeLinkAttempts          = 3
)

var activeWindowIDPattern = regexp.MustCompile(`^@[0-9]+$`)

type ActiveWindowStore interface {
	UpsertActiveWindow(context.Context, state.ActiveWindowInput, time.Time, time.Duration) error
	TouchActiveWindow(context.Context, string, string, time.Time, time.Duration) (bool, error)
	MarkActiveWindowOrphaned(context.Context, string, string, time.Time) error
	DeleteActiveWindow(context.Context, string, string) error
	ListActiveWindows(context.Context, string) ([]state.ActiveWindow, error)
	ListExpiredActiveWindows(context.Context, string, time.Time) ([]state.ActiveWindow, error)
	DeleteStaleActiveEpochs(context.Context, string, string) (int64, error)
}

type ActiveSourceLink struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	WindowIndex int    `json:"window_index"`
}

type ActiveWindowStatus struct {
	WindowID       string                  `json:"window_id"`
	WindowName     string                  `json:"window_name"`
	State          state.ActiveWindowState `json:"state"`
	ActiveIndex    *int                    `json:"active_index,omitempty"`
	SourceLinks    []ActiveSourceLink      `json:"source_links"`
	PromotedAt     time.Time               `json:"promoted_at"`
	LastSelectedAt time.Time               `json:"last_selected_at"`
	ExpiresAt      time.Time               `json:"expires_at"`
	OrphanedAt     time.Time               `json:"orphaned_at,omitempty"`
	Expired        bool                    `json:"expired"`
}

type ActiveStatusReport struct {
	Enabled         bool                 `json:"enabled"`
	Session         string               `json:"session"`
	Owned           bool                 `json:"owned"`
	Collision       bool                 `json:"collision"`
	CollisionReason string               `json:"collision_reason,omitempty"`
	TTL             string               `json:"ttl,omitempty"`
	ServerEpoch     string               `json:"server_epoch,omitempty"`
	Tracked         int                  `json:"tracked"`
	Orphaned        int                  `json:"orphaned"`
	Entries         []ActiveWindowStatus `json:"entries,omitempty"`
}

type activeOwnership struct {
	exists    bool
	owned     bool
	collision bool
	reason    string
}

type activeLinkSet struct {
	active    tmux.WindowLink
	hasActive bool
	sources   []tmux.WindowLink
}

type activeSnapshot struct {
	links []tmux.WindowLink
}

func activeTTL(cfg config.ActiveSessionConfig) (time.Duration, error) {
	if !cfg.Enabled {
		return 0, errs.ErrActiveSessionDisabled
	}
	raw := cfg.EffectiveTTL()
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl <= 0 || ttl > config.MaxActiveTTL {
		return 0, fmt.Errorf("active windows TTL %q: %w", raw, errs.ErrInvalidTTL)
	}
	return ttl, nil
}

func validateActiveWindowID(windowID string) error {
	if !activeWindowIDPattern.MatchString(windowID) {
		return fmt.Errorf("invalid window_id %q: must match ^@[0-9]+$", windowID)
	}
	return nil
}

func validatePromotionTarget(target string) (string, error) {
	if target == "" || strings.TrimSpace(target) != target || strings.ContainsAny(target, "\x00\r\n") {
		return "", fmt.Errorf("invalid promotion target %q", target)
	}
	if activeWindowIDPattern.MatchString(target) {
		return "", nil
	}
	parts := strings.SplitN(target, ":", 2)
	if parts[0] == "" || (len(parts) == 2 && parts[1] == "") {
		return "", fmt.Errorf("invalid promotion target %q", target)
	}
	if parts[0] == ActiveSessionName {
		return "", fmt.Errorf("active alias is not a source target")
	}
	return parts[0], nil
}

func validateRecoveryDestination(destination string) error {
	if destination == "" || destination == ActiveSessionName || strings.Contains(destination, ":") {
		return fmt.Errorf("invalid recovery destination %q", destination)
	}
	if strings.TrimSpace(destination) != destination || strings.ContainsAny(destination, "\x00\r\n") {
		return fmt.Errorf("invalid recovery destination %q", destination)
	}
	return nil
}

func inspectActiveOwnership(ctx context.Context, runner tmux.Runner) (activeOwnership, error) {
	exists, err := runner.HasSession(ctx, ActiveSessionName)
	if err != nil {
		return activeOwnership{}, fmt.Errorf("inspect active session: %w", err)
	}
	if !exists {
		return activeOwnership{}, nil
	}
	marker, err := runner.ShowSessionOption(ctx, ActiveSessionName, ActiveOwnerOption)
	if err != nil {
		return activeOwnership{}, fmt.Errorf("inspect active ownership: %w", err)
	}
	if marker == ActiveOwnerValue {
		return activeOwnership{exists: true, owned: true}, nil
	}
	reason := "ownership marker is missing"
	if marker != "" {
		reason = fmt.Sprintf("ownership marker is %q", marker)
	}
	return activeOwnership{exists: true, collision: true, reason: reason}, nil
}

func activeCollisionError(ownership activeOwnership) error {
	return fmt.Errorf("active session %q: %s: %w", ActiveSessionName, ownership.reason, errs.ErrActiveSessionCollision)
}

func ensureOwnedActiveSession(ctx context.Context, runner tmux.Runner) (activeOwnership, error) {
	ownership, err := inspectActiveOwnership(ctx, runner)
	if err != nil || ownership.owned {
		return ownership, err
	}
	if ownership.collision {
		return ownership, activeCollisionError(ownership)
	}
	if err := contextError(ctx, "create active session"); err != nil {
		return activeOwnership{}, err
	}
	err = runner.NewSession(ctx, tmux.NewSessionOpts{
		Name:       ActiveSessionName,
		WindowName: activePlaceholderWindowName,
		Detached:   true,
		SessionOptions: map[string]string{
			ActiveOwnerOption: ActiveOwnerValue,
		},
	})
	if err != nil && !errors.Is(err, errs.ErrSessionExists) {
		return activeOwnership{}, fmt.Errorf("create active session: %w", err)
	}
	return requireOwnedActiveSession(ctx, runner)
}

func requireOwnedActiveSession(ctx context.Context, runner tmux.Runner) (activeOwnership, error) {
	ownership, err := inspectActiveOwnership(ctx, runner)
	if err != nil {
		return activeOwnership{}, err
	}
	if ownership.collision {
		return ownership, activeCollisionError(ownership)
	}
	return ownership, nil
}

func reconcileActiveEpoch(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
) (tmux.ServerEpoch, error) {
	epoch, err := runner.ServerEpoch(ctx)
	if err != nil {
		return tmux.ServerEpoch{}, fmt.Errorf("read active server epoch: %w", err)
	}
	if _, err := store.DeleteStaleActiveEpochs(ctx, epoch.ServerKey, epoch.Value); err != nil {
		return tmux.ServerEpoch{}, fmt.Errorf("reconcile active server epoch: %w", err)
	}
	return epoch, nil
}

func loadActiveSnapshot(ctx context.Context, runner tmux.Runner) (activeSnapshot, error) {
	links, err := runner.ListWindowLinks(ctx)
	if err != nil {
		return activeSnapshot{}, fmt.Errorf("list active window links: %w", err)
	}
	return activeSnapshot{links: append([]tmux.WindowLink(nil), links...)}, nil
}

func (snapshot activeSnapshot) forWindow(windowID string) activeLinkSet {
	set := activeLinkSet{sources: make([]tmux.WindowLink, 0)}
	for _, link := range snapshot.links {
		if link.WindowID != windowID {
			continue
		}
		if link.SessionName == ActiveSessionName {
			set.active = link
			set.hasActive = true
			continue
		}
		set.sources = append(set.sources, link)
	}
	sort.Slice(set.sources, func(i, j int) bool {
		if set.sources[i].SessionName != set.sources[j].SessionName {
			return set.sources[i].SessionName < set.sources[j].SessionName
		}
		return set.sources[i].WindowIndex < set.sources[j].WindowIndex
	})
	return set
}

func sourceLinkForTarget(set activeLinkSet, sourceSession, windowID string) (tmux.WindowLink, error) {
	for _, source := range set.sources {
		if sourceSession == "" || source.SessionName == sourceSession {
			return source, nil
		}
	}
	if len(set.sources) == 0 {
		return tmux.WindowLink{}, fmt.Errorf("window %s has no non-active source: %w", windowID, errs.ErrUnsafeActiveWindow)
	}
	return tmux.WindowLink{}, fmt.Errorf("window %s is not linked to source session %q: %w", windowID, sourceSession, errs.ErrWindowNotFound)
}

func actualBaseIndex(ctx context.Context, runner tmux.Runner) (int, error) {
	raw, err := runner.ShowOption(ctx, "base-index")
	if err != nil {
		return 0, fmt.Errorf("read tmux base-index: %w", err)
	}
	if raw == "" {
		return 0, nil
	}
	baseIndex, err := strconv.Atoi(raw)
	if err != nil || baseIndex < 0 {
		return 0, fmt.Errorf("invalid tmux base-index %q", raw)
	}
	return baseIndex, nil
}

func nextFreeIndex(snapshot activeSnapshot, session string, baseIndex int) int {
	used := make(map[int]struct{})
	for _, link := range snapshot.links {
		if link.SessionName == session {
			used[link.WindowIndex] = struct{}{}
		}
	}
	for index := baseIndex; ; index++ {
		if _, exists := used[index]; !exists {
			return index
		}
	}
}

func ensureLinkedWindow(
	ctx context.Context,
	runner tmux.Runner,
	windowID string,
	destination string,
	snapshot activeSnapshot,
) (activeSnapshot, bool, error) {
	if linkSet := snapshot.forWindow(windowID); hasSessionLink(linkSet, destination) {
		return snapshot, false, nil
	}
	baseIndex, err := actualBaseIndex(ctx, runner)
	if err != nil {
		return activeSnapshot{}, false, err
	}
	return linkWindowWithRetry(ctx, runner, windowID, destination, baseIndex, snapshot)
}

func linkWindowWithRetry(
	ctx context.Context,
	runner tmux.Runner,
	windowID string,
	destination string,
	baseIndex int,
	snapshot activeSnapshot,
) (activeSnapshot, bool, error) {
	var lastErr error
	for range activeLinkAttempts {
		if err := contextError(ctx, "link active window"); err != nil {
			return snapshot, false, err
		}
		index := nextFreeIndex(snapshot, destination, baseIndex)
		lastErr = runner.LinkWindow(ctx, windowID, fmt.Sprintf("%s:%d", destination, index))
		fresh, listErr := loadActiveSnapshot(ctx, runner)
		if listErr != nil {
			return snapshot, false, listErr
		}
		snapshot = fresh
		if hasSessionLink(snapshot.forWindow(windowID), destination) {
			return snapshot, true, nil
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("linked window %s is absent from %s", windowID, destination)
		}
	}
	return snapshot, false, fmt.Errorf("link window %s to %s after %d attempts: %w", windowID, destination, activeLinkAttempts, lastErr)
}

func hasSessionLink(set activeLinkSet, session string) bool {
	if session == ActiveSessionName {
		return set.hasActive
	}
	for _, source := range set.sources {
		if source.SessionName == session {
			return true
		}
	}
	return false
}

func contextError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
