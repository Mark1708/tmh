package cmd

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newFreezeCmd() *cobra.Command {
	var (
		session string
		dryRun  bool
	)
	c := &cobra.Command{
		Use:   "freeze",
		Short: i18n.T("cli.freeze.short"),
		Long: `Capture the live tmux state and merge it into ~/.config/tmh/config.yml,
preserving comments, templates, and profiles. Sessions or windows absent
from the config are added (with an inferred root when possible); existing
entries with matching dirs are left untouched; mismatches are reported as
conflicts — review them and apply with "tmh sync --pull --all" or by hand.

This is the authoring complement to "tmh diff": build your layout live,
then freeze it into the config so drift detection becomes meaningful.`,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := loadConfig(true)
			if err != nil {
				return err
			}
			r := newRunner()
			rep, err := actions.Freeze(context.Background(), r, cfg, actions.FreezeOptions{
				Session: session,
				DryRun:  dryRun,
			})
			if err != nil {
				return err
			}
			printFreezeReport(c, rep, dryRun)
			if !dryRun && (len(rep.AddedSessions) > 0 || len(rep.AddedWindows) > 0) {
				path := resolveConfigPath()
				if err := config.Write(cfg, path, config.WriteOptions{
					BackupDir:      xdg.BackupsDir(),
					MaxBackups:     20,
					PreserveBlanks: true,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&session, "session", "", i18n.T("cli.flag.freeze.session"))
	c.Flags().BoolVar(&dryRun, "dry-run", false, i18n.T("cli.flag.freeze.dry_run"))
	return c
}

func printFreezeReport(c *cobra.Command, rep *actions.FreezeReport, dryRun bool) {
	if dryRun {
		fmt.Fprintln(c.OutOrStdout(), "(dry-run — nothing written)")
	}
	for _, s := range rep.AddedSessions {
		fmt.Fprintf(c.OutOrStdout(), "+ session %s\n", s)
	}
	for _, w := range rep.AddedWindows {
		fmt.Fprintf(c.OutOrStdout(), "+ window  %s\n", w)
	}
	for _, u := range rep.Unchanged {
		fmt.Fprintf(c.OutOrStdout(), "= %s\n", u)
	}
	for _, cf := range rep.Conflicts {
		fmt.Fprintf(c.OutOrStdout(), "! %s\n", cf)
	}
	if len(rep.Conflicts) > 0 {
		fmt.Fprintln(c.OutOrStdout(),
			`resolve conflicts with "tmh sync --pull --all" (overwrite config dirs) or edit the file manually`)
	}
}
