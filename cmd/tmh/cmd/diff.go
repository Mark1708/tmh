package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/tmux"

	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "diff",
		Short: "Show drift between live tmux and config.yml",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := loadConfig(false)
			if err != nil {
				return err
			}
			r := newRunner()
			snap, err := collectLiveForDiff(context.Background(), r)
			if err != nil {
				return err
			}
			resolved, err := config.Resolve(cfg, flags.Profile)
			if err != nil {
				return err
			}
			entries := config.Diff(resolved, snap)

			if jsonOut {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}
			if len(entries) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "no drift")
				return nil
			}
			for _, e := range entries {
				fmt.Fprintf(c.OutOrStdout(), "%-6s %-30s %s\n", e.Status, e.ConfigEntry, e.Reason)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return c
}

// collectLiveForDiff mirrors actions.collectLive without pulling the actions
// package into cmd (would create a dependency cycle once TUI lands here).
func collectLiveForDiff(ctx context.Context, r tmux.Runner) (config.LiveSnapshot, error) {
	var snap config.LiveSnapshot
	sessions, err := r.ListSessions(ctx)
	if err != nil {
		return snap, err
	}
	for _, s := range sessions {
		wins, err := r.ListWindows(ctx, s.Name)
		if err != nil {
			return snap, err
		}
		panes, err := r.ListPanes(ctx, s.Name)
		if err != nil {
			return snap, err
		}
		dirByWin := make(map[int]string, len(wins))
		for _, p := range panes {
			if _, ok := dirByWin[p.Window]; !ok {
				dirByWin[p.Window] = p.Path
			}
		}
		ls := config.LiveSession{Name: s.Name}
		for _, w := range wins {
			ls.Windows = append(ls.Windows, config.LiveWindow{
				Name: w.Name, Dir: dirByWin[w.Index],
			})
		}
		snap.Sessions = append(snap.Sessions, ls)
	}
	return snap, nil
}
