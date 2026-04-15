package cmd

import (
	"context"
	"strings"

	"git.mark1708.ru/me/tmh/internal/actions"

	"github.com/spf13/cobra"
)

func newPopupCmd() *cobra.Command {
	var (
		width, height       string
		noEnv, noCwd        bool
		sessionN, windowN   string
	)
	c := &cobra.Command{
		Use:   "popup [-- command...]",
		Short: "Run a command in a tmux popup with env/cwd from config",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			cmdline := strings.Join(args, " ")
			if cmdline == "" {
				cmdline = "zsh"
			}
			cfg, _ := loadConfig(true)
			r := newRunner()
			return actions.Popup(context.Background(), r, cfg, flags.Profile, sessionN, windowN, actions.PopupOpts{
				Command: cmdline,
				Width:   width,
				Height:  height,
				NoEnv:   noEnv,
				NoCwd:   noCwd,
			})
		},
	}
	c.Flags().StringVar(&width, "width", "", "popup width (e.g. 80% or 120)")
	c.Flags().StringVar(&height, "height", "", "popup height")
	c.Flags().BoolVar(&noEnv, "no-env", false, "do not inherit env from config")
	c.Flags().BoolVar(&noCwd, "no-cwd", false, "do not inherit cwd from config")
	c.Flags().StringVar(&sessionN, "session", "", "session name to derive env/cwd from")
	c.Flags().StringVar(&windowN, "window", "", "window name to derive cwd from")
	return c
}
