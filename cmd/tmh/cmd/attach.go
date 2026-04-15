package cmd

import (
	"context"

	"git.mark1708.ru/me/tmh/internal/actions"

	"github.com/spf13/cobra"
)

func newAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach [name|name:window]",
		Short: "Attach to a session (or switch client if already inside tmux)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			r := newRunner()
			if target == "" {
				// Without a target, list live sessions for fuzzy pickup by the
				// TUI. Until the TUI lands, error out so scripts get a clear
				// signal rather than a silent hang.
				return cmdErr("attach requires a session target until TUI is implemented")
			}
			return actions.Attach(context.Background(), r, target)
		},
	}
}
