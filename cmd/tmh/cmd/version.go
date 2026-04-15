package cmd

import (
	"fmt"

	"git.mark1708.ru/me/tmh/internal/i18n"

	"github.com/spf13/cobra"
)

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: i18n.T("cli.version.short"),
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println(version)
			return nil
		},
	}
}
