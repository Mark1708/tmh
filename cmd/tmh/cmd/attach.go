package cmd

import (
	"context"
	"time"

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
			if target == "" {
				return cmdErr("attach requires a session target until TUI is implemented")
			}

			runner := newRunner()
			cfg, err := loadConfig(true)
			if err != nil {
				return err
			}

			db, err := openStateForConfig(cfg)
			if err != nil {
				return err
			}
			var store actions.ActiveWindowStore
			if db != nil {
				store = db
				defer db.Close()
			}

			ctx := context.Background()
			now := time.Now()
			return actions.NavigateWithActive(ctx, runner, cfg, store, target, now)
		},
	}
}
