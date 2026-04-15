package cmd

import (
	"context"

	"git.mark1708.ru/me/tmh/internal/actions"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var (
		only []string
	)
	c := &cobra.Command{
		Use:   "init",
		Short: "Create all configured sessions that aren't already running",
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
