package cmd

import (
	"context"
	"fmt"
	"os"

	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/tmux"

	"github.com/spf13/cobra"
)

func newWindowCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{
		Use:   "window",
		Short: i18n.T("cli.window.short"),
		RunE: func(c *cobra.Command, args []string) error {
			if dir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				dir = cwd
			}
			r := newRunner()
			// `tmh window` is meant to be invoked from inside tmux —
			// the session target uses the current session via empty target.
			win, err := r.NewWindow(context.Background(), tmux.NewWindowOpts{Dir: dir})
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "opened: %s:%d\n", win.Session, win.Index)
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", "", i18n.T("cli.flag.window.dir"))
	return c
}
