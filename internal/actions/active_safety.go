package actions

import (
	"context"
	"fmt"

	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/tmux"
)

func unlinkVerifiedActiveAlias(
	ctx context.Context,
	runner tmux.Runner,
	windowID string,
	snapshot activeSnapshot,
) (activeSnapshot, error) {
	before := snapshot.forWindow(windowID)
	if !before.hasActive {
		return snapshot, nil
	}
	if len(before.sources) == 0 {
		return snapshot, fmt.Errorf("refuse to unlink last link for %s: %w", windowID, errs.ErrUnsafeActiveWindow)
	}
	if err := contextError(ctx, "unlink active alias"); err != nil {
		return snapshot, err
	}
	if err := runner.UnlinkWindow(ctx, ActiveSessionName, windowID); err != nil {
		return snapshot, fmt.Errorf("unlink active alias %s: %w", windowID, err)
	}
	fresh, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return snapshot, err
	}
	after := fresh.forWindow(windowID)
	if after.hasActive || len(after.sources) == 0 {
		return fresh, fmt.Errorf("verify active alias removal for %s: source link invariant failed", windowID)
	}
	return fresh, nil
}
