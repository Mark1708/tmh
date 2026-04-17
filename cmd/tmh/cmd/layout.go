package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newLayoutCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "layout",
		Short: i18n.T("cli.layout.short"),
	}
	c.AddCommand(newLayoutSaveCmd())
	return c
}

func newLayoutSaveCmd() *cobra.Command {
	var description string
	c := &cobra.Command{
		Use:   "save <name>",
		Short: i18n.T("cli.layout.save.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			session, idx, err := currentTmuxTarget()
			if err != nil {
				return err
			}
			cfg, err := loadConfig(false)
			if err != nil {
				return err
			}
			r := newRunner()
			hash, err := actions.LayoutSave(context.Background(), r, cfg, session, idx, name, description)
			if err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.print.saved_layout", map[string]any{"name": name, "hash": hash}))
			return config.Write(cfg, resolveConfigPath(), config.WriteOptions{
				BackupDir:      xdg.BackupsDir(),
				MaxBackups:     20,
				PreserveBlanks: true,
			})
		},
	}
	c.Flags().StringVar(&description, "description", "", i18n.T("cli.flag.layout.description"))
	return c
}

// currentTmuxTarget asks tmux for the active session and window index.
// Used by `tmh layout save` since it needs to capture the *current* layout.
func currentTmuxTarget() (string, int, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}\t#{window_index}").Output()
	if err != nil {
		return "", 0, fmt.Errorf("tmux display-message: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("unexpected tmux output: %q", out)
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}
	return parts[0], idx, nil
}
