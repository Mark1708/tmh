package cmd

import (
	"context"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"

	"github.com/spf13/cobra"
)

func newAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach [name|name:window]",
		Short: i18n.T("cli.attach.short"),
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
