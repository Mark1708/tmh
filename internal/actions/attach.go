package actions

import (
	"context"

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
