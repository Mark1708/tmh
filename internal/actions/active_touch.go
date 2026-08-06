package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
)

func TouchActiveWindow(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	cfg config.ActiveSessionConfig,
	windowID string,
	now time.Time,
) (bool, error) {
	// Disabled is a no-op with explicit error
	if !cfg.Enabled {
		return false, errs.ErrActiveSessionDisabled
	}

	// DB nil when enabled is an error
	if store == nil {
		return false, fmt.Errorf("active integration enabled but state database unavailable")
	}

	ttl, err := activeTTL(cfg)
	if err != nil {
		return false, err
	}
	if err := validateActiveWindowID(windowID); err != nil {
		return false, err
	}
	if _, err := requireOwnedActiveSession(ctx, runner); err != nil {
		return false, err
	}
	epoch, err := reconcileActiveEpoch(ctx, runner, store)
	if err != nil {
		return false, err
	}
	row, found, err := findActiveRow(ctx, store, epoch.Value, windowID)
	if err != nil || !found {
		// Absent tracked ID is a successful no-op (not an error)
		return false, nil
	}
	snapshot, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return false, err
	}
	if !snapshot.forWindow(row.WindowID).hasActive {
		if err := store.DeleteActiveWindow(ctx, epoch.Value, windowID); err != nil {
			return false, fmt.Errorf("delete stale touched window %s: %w", windowID, err)
		}
		return false, nil
	}
	touched, err := store.TouchActiveWindow(ctx, epoch.Value, windowID, now, ttl)
	if err != nil {
		return false, fmt.Errorf("touch active window %s: %w", windowID, err)
	}
	return touched, nil
}

func findActiveRow(
	ctx context.Context,
	store ActiveWindowStore,
	epoch string,
	windowID string,
) (state.ActiveWindow, bool, error) {
	rows, err := store.ListActiveWindows(ctx, epoch)
	if err != nil {
		return state.ActiveWindow{}, false, fmt.Errorf("list active windows: %w", err)
	}
	for _, row := range rows {
		if row.WindowID == windowID {
			return row, true, nil
		}
	}
	return state.ActiveWindow{}, false, nil
}
