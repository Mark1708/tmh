package cmd

import (
	"context"
	"fmt"
	"os"

	"git.mark1708.ru/me/tmh/internal/tmux"

	"github.com/spf13/cobra"
)

func newWindowCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{
		Use:   "window",
		Short: "Open an ad-hoc window in the current session (not written to config)",
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
	c.Flags().StringVar(&dir, "dir", "", "working directory (default: $PWD)")
	return c
}
