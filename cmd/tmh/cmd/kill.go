package cmd

import (
	"context"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/i18n"

	"github.com/spf13/cobra"
)

func newKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <pattern>",
		Short: i18n.T("cli.kill.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			r := newRunner()
			killed, err := actions.KillMatching(context.Background(), r, args[0])
			for _, name := range killed {
				fmt.Fprintln(c.OutOrStdout(), "killed:", name)
			}
			return err
		},
	}
}
