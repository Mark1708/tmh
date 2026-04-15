package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newScratchCmd() *cobra.Command {
	var (
		dir string
		ttl time.Duration
	)
	c := &cobra.Command{
		Use:   "scratch",
		Short: i18n.T("cli.scratch.short"),
		RunE: func(c *cobra.Command, args []string) error {
			if dir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				dir = cwd
			}
			db, _ := state.Open(xdg.StateDBPath())
			if db != nil {
				defer db.Close()
			}
			r := newRunner()
			name, err := actions.CreateScratch(context.Background(), r, db, actions.ScratchOptions{
				Dir: dir, TTL: ttl,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "created:", name)
			return actions.Attach(context.Background(), r, name)
		},
	}
	c.Flags().StringVar(&dir, "dir", "", i18n.T("cli.flag.scratch.dir"))
	c.Flags().DurationVar(&ttl, "ttl", 0, "auto-kill after duration (e.g. 1h, 30m); zero = no expiry")
	return c
}
