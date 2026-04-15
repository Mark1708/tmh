package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print tmh version",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println(version)
			return nil
		},
	}
}
