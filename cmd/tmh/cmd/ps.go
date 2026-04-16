package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"git.mark1708.ru/me/tmh/internal/i18n"

	"github.com/spf13/cobra"
)

// psRow is a single row of tmh ps output.
type psRow struct {
	Session string `json:"session"`
	Window  int    `json:"window"`
	Pane    int    `json:"pane"`
	Cmd     string `json:"cmd"`
	PID     int    `json:"pid"`
	CWD     string `json:"cwd"`
}

func newPsCmd() *cobra.Command {
	var (
		session string
		format  string
	)
	c := &cobra.Command{
		Use:   "ps",
		Short: i18n.T("cli.ps.short"),
		RunE: func(c *cobra.Command, args []string) error {
			r := newRunner()
			ctx := context.Background()

			target := session // "" means -a (all sessions)
			panes, err := r.ListPanes(ctx, target)
			if err != nil {
				return err
			}

			var rows []psRow
			for _, p := range panes {
				if session != "" && !strings.EqualFold(p.Session, session) {
					continue
				}
				rows = append(rows, psRow{
					Session: p.Session,
					Window:  p.Window,
					Pane:    p.Index,
					Cmd:     p.Command,
					PID:     p.PID,
					CWD:     p.Path,
				})
			}

			switch format {
			case "json":
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)

			case "tsv":
				fmt.Fprintf(c.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\t%s\n",
					i18n.T("cli.ps.header.session"),
					i18n.T("cli.ps.header.window"),
					i18n.T("cli.ps.header.pane"),
					i18n.T("cli.ps.header.cmd"),
					i18n.T("cli.ps.header.pid"),
					i18n.T("cli.ps.header.cwd"),
				)
				for _, row := range rows {
					fmt.Fprintf(c.OutOrStdout(), "%s\t%d\t%d\t%s\t%d\t%s\n",
						row.Session, row.Window, row.Pane, row.Cmd, row.PID, row.CWD)
				}

			default: // table
				tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					i18n.T("cli.ps.header.session"),
					i18n.T("cli.ps.header.window"),
					i18n.T("cli.ps.header.pane"),
					i18n.T("cli.ps.header.cmd"),
					i18n.T("cli.ps.header.pid"),
					i18n.T("cli.ps.header.cwd"),
				)
				for _, row := range rows {
					fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%d\t%s\n",
						row.Session, row.Window, row.Pane, row.Cmd, row.PID, row.CWD)
				}
				tw.Flush()
			}
			return nil
		},
	}
	c.Flags().StringVar(&session, "session", "", i18n.T("cli.ps.flag.session"))
	c.Flags().StringVar(&format, "format", "table", i18n.T("cli.ps.flag.format"))
	return c
}
