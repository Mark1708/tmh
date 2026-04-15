package cmd

import (
	"context"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var (
		push, pull, bootstrap, applyAll, dryRun bool
	)
	c := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile live tmux and config.yml",
		Long: `Default direction is --push (live matches config). Use --pull to update
config from live, --bootstrap to import all live sessions into an empty config.`,
		RunE: func(c *cobra.Command, args []string) error {
			if moreThanOne(push, pull, bootstrap) {
				return cmdErr("choose at most one of --push | --pull | --bootstrap")
			}
			if !push && !pull && !bootstrap {
				push = true
			}
			cfg, err := loadConfig(bootstrap) // bootstrap may start from missing config
			if err != nil {
				return err
			}
			r := newRunner()
			opts := actions.SyncOptions{Profile: flags.Profile, DryRun: dryRun, ApplyAll: applyAll}

			var rep *actions.SyncReport
			switch {
			case bootstrap:
				rep, err = actions.Bootstrap(context.Background(), r, cfg)
			case pull:
				rep, err = actions.Pull(context.Background(), r, cfg, opts)
			default:
				rep, err = actions.Push(context.Background(), r, cfg, opts)
			}
			if err != nil {
				return err
			}
			printReport(c, rep)
			// Persist config changes for pull/bootstrap.
			if !dryRun && (pull || bootstrap) {
				path := resolveConfigPath()
				if err := config.Write(cfg, path, config.WriteOptions{
					BackupDir:      xdg.BackupsDir(),
					MaxBackups:     20,
					PreserveBlanks: true,
				}); err != nil {
					return err
				}
				fmt.Fprintln(c.OutOrStdout(), "config written:", path)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&push, "push", false, "live ← config (create missing sessions/windows)")
	c.Flags().BoolVar(&pull, "pull", false, "config ← live (import new, rewrite drifted dirs)")
	c.Flags().BoolVar(&bootstrap, "bootstrap", false, "import all live sessions into empty config")
	c.Flags().BoolVar(&applyAll, "all", false, "apply drifted/gone entries (default: skipped)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print actions without executing")
	return c
}

func printReport(c *cobra.Command, rep *actions.SyncReport) {
	if rep == nil {
		return
	}
	for _, s := range rep.Created {
		fmt.Fprintln(c.OutOrStdout(), "+", s)
	}
	for _, s := range rep.Updated {
		fmt.Fprintln(c.OutOrStdout(), "~", s)
	}
	for _, s := range rep.Deleted {
		fmt.Fprintln(c.OutOrStdout(), "-", s)
	}
	for _, s := range rep.Skipped {
		fmt.Fprintln(c.OutOrStdout(), " ", s)
	}
	if len(rep.Created)+len(rep.Updated)+len(rep.Deleted)+len(rep.Skipped) == 0 {
		fmt.Fprintln(c.OutOrStdout(), "nothing to do")
	}
}

func moreThanOne(bs ...bool) bool {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n > 1
}
