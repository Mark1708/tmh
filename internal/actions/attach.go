package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux"
)

// Attach brings the caller into the named session. When invoked inside tmux
// ($TMUX set), the runner switches the client instead of nesting an attach.
//
// target may be "session" or "session:window".
func Attach(ctx context.Context, r tmux.Runner, target string) error {
	if r.InTmux() {
		return r.SwitchClient(ctx, target)
	}
	return r.AttachSession(ctx, target)
}

// NavigateWithActive prepares active-aware navigation and attaches/switches.
// When the active feature is enabled, it promotes the target window to the
// active session before attaching. When disabled, it behaves like Attach.
//
// This function preserves legacy Attach behavior when feature is disabled
// or config/state are nil (pass-through mode).
func NavigateWithActive(
	ctx context.Context,
	runner tmux.Runner,
	cfg *config.Config,
	store ActiveWindowStore,
	target string,
	now time.Time,
) error {
	if cfg == nil || !cfg.Defaults.TmuxIntegration.Active.Enabled {
		return Attach(ctx, runner, target)
	}

	// Defense-in-depth: nil store should cause zero tmux mutation
	if store == nil {
		return fmt.Errorf("active windows feature enabled but state store is nil")
	}

	activeTarget, err := PromoteActiveWindow(ctx, runner, store, cfg.Defaults.TmuxIntegration.Active, target, now)
	if err != nil {
		return err
	}

	return Attach(ctx, runner, activeTarget)
}
