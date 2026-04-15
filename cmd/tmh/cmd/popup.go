package cmd

import (
	"context"
	"strings"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/i18n"

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
		Short: i18n.T("cli.popup.short"),
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
	c.Flags().StringVar(&width, "width", "", i18n.T("cli.flag.popup.width"))
	c.Flags().StringVar(&height, "height", "", i18n.T("cli.flag.popup.height"))
	c.Flags().BoolVar(&noEnv, "no-env", false, i18n.T("cli.flag.popup.no_env"))
	c.Flags().BoolVar(&noCwd, "no-cwd", false, i18n.T("cli.flag.popup.no_cwd"))
	c.Flags().StringVar(&sessionN, "session", "", i18n.T("cli.flag.popup.session"))
	c.Flags().StringVar(&windowN, "window", "", i18n.T("cli.flag.popup.window"))
	return c
}
