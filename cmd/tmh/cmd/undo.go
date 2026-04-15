package cmd

import (
	"context"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newUndoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "undo",
		Short: "Restore the most recent destructive action",
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
