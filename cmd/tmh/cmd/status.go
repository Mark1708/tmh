package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

// newStatusCmd prints a single-glyph tmux statusbar segment. Used by
// `set-option -ag status-right ' #(tmh status)'` in ~/.tmux.conf.
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: i18n.T("cli.status.short"),
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Fprint(c.OutOrStdout(), renderStatus())
			return nil
		},
	}
}

func renderStatus() string {
	// Drift — cheap: parse config, collect live, count tracked.
	cfg, err := config.Load(resolveConfigPath())
	drift := 0
	if err == nil {
		if resolved, err := config.Resolve(cfg, ""); err == nil {
			r := newRunner()
			snap, err := collectLiveForDiff(context.Background(), r)
			if err == nil {
				for _, d := range config.Diff(resolved, snap) {
					if d.Status != config.StatusOK {
						drift++
					}
				}
			}
		}
	}

	pending := 0
	if db, err := state.Open(xdg.StateDBPath()); err == nil {
		defer db.Close()
		entries, _ := actions.PendingReloads(context.Background(), db)
		pending = len(entries)
	}

	zsh := dotfileStale(filepath.Join(os.Getenv("HOME"), ".zshrc"))
	tmuxConf := dotfileStale(filepath.Join(os.Getenv("HOME"), ".tmux.conf"))

	switch {
	case drift > 0:
		return fmt.Sprintf("⚠drift:%d", drift)
	case zsh:
		return "⚠zsh"
	case tmuxConf:
		return "⚠tmux"
	case pending > 0:
		return fmt.Sprintf("⏳%d", pending)
	default:
		return "·"
	}
}

// dotfileStale is a cheap placeholder: returns false for now. A proper check
// compares mtime of the dotfile vs the start time of the oldest shell pane,
// which requires ps(1) parsing — left as follow-up.
func dotfileStale(_ string) bool { return false }
