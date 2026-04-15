package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/i18n"

	"github.com/spf13/cobra"
)

func newLsCmd() *cobra.Command {
	var (
		jsonOut bool
	)
	c := &cobra.Command{
		Use:   "ls",
		Short: i18n.T("cli.ls.short"),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := loadConfig(true)
			if err != nil {
				return err
			}
			r := newRunner()
			listing, err := actions.BuildListing(context.Background(), r, cfg, flags.Profile)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(listing)
			}
			return renderListing(c, listing)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, i18n.T("cli.flag.json"))
	return c
}

func renderListing(c *cobra.Command, l *actions.Listing) error {
	if len(l.Sessions) == 0 {
		fmt.Fprintln(c.OutOrStdout(), "(no sessions)")
		return nil
	}
	for _, s := range l.Sessions {
		marker := "  "
		switch {
		case s.Attached:
			marker = "*"
		case s.Live:
			marker = "●"
		case s.Configured:
			marker = "-"
		}
		fmt.Fprintf(c.OutOrStdout(), "%s %s  %d windows\n", marker, s.Name, len(s.Windows))
		for _, w := range s.Windows {
			tag := ""
			switch {
			case w.Configured && w.Live:
				tag = "ok"
			case w.Configured:
				tag = "gone"
			case w.Live:
				tag = "new"
			}
			fmt.Fprintf(c.OutOrStdout(), "    %-20s %s\n", w.Name, tag)
		}
	}
	return nil
}
