package cmd

import (
	"context"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/i18n"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var (
		only []string
	)
	c := &cobra.Command{
		Use:   "init",
		Short: i18n.T("cli.init.short"),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := loadConfig(false)
			if err != nil {
				return err
			}
			r := newRunner()
			return actions.Init(context.Background(), r, cfg, actions.InitOptions{
				Profile: flags.Profile,
				Only:    only,
			})
		},
	}
	c.Flags().StringSliceVar(&only, "only", nil, "restrict to these session names")
	return c
}
