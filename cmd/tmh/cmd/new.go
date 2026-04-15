package cmd

import (
	"context"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/actions"

	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	var (
		name   string
		dir    string
		layout string
	)
	c := &cobra.Command{
		Use:   "new",
		Short: "Create an ad-hoc session",
		RunE: func(c *cobra.Command, args []string) error {
			if name == "" {
				return cmdErr("--name is required until the wizard is implemented")
			}
			if dir == "" {
				return cmdErr("--dir is required until the wizard is implemented")
			}
			if layout == "" {
				layout = "3-pane"
			}
			r := newRunner()
			sess := config.ResolvedSession{
				Name: name, Dir: dir,
				Windows: []config.ResolvedWindow{{Name: name, Dir: dir, Layout: layout}},
			}
			return actions.CreateSession(context.Background(), r, sess, nil)
		},
	}
	c.Flags().StringVar(&name, "name", "", "session name")
	c.Flags().StringVar(&dir, "dir", "", "working directory")
	c.Flags().StringVar(&layout, "layout", "", "1-pane | 2-pane | 3-pane")
	return c
}
