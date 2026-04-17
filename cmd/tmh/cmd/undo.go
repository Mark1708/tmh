package cmd

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newUndoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "undo",
		Short: i18n.T("cli.undo.short"),
		RunE: func(c *cobra.Command, args []string) error {
			db, err := state.Open(xdg.StateDBPath())
			if err != nil {
				return err
			}
			defer db.Close()
			r := newRunner()
			target, err := actions.UndoLast(context.Background(), r, db)
			if err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "restored:", target)
			return nil
		},
	}
}
