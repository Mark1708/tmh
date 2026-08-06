package actions

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/config"
	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/tmux"
)

func RemoveActiveWindow(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	cfg config.ActiveSessionConfig,
	windowID string,
) error {
	if _, err := activeTTL(cfg); err != nil {
		return err
	}
	if err := validateActiveWindowID(windowID); err != nil {
		return err
	}
	if _, err := requireOwnedActiveSession(ctx, runner); err != nil {
		return err
	}
	epoch, err := reconcileActiveEpoch(ctx, runner, store)
	if err != nil {
		return err
	}
	snapshot, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return err
	}
	links := snapshot.forWindow(windowID)
	if len(links.sources) == 0 {
		return fmt.Errorf("remove active window %s requires a non-active link: %w", windowID, errs.ErrUnsafeActiveWindow)
	}
	if links.hasActive {
		if _, err := unlinkVerifiedActiveAlias(ctx, runner, windowID, snapshot); err != nil {
			return err
		}
	}
	return deleteActiveRow(ctx, store, epoch.Value, windowID)
}
