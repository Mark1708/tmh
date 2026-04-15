package cmd

import (
	"context"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var (
		push, pull, bootstrap, applyAll, dryRun bool
	)
	c := &cobra.Command{
		Use:   "sync",
		Short: i18n.T("cli.sync.short"),
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
				fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.print.config_written", map[string]any{"path": path}))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&push, "push", false, i18n.T("cli.flag.sync.push"))
	c.Flags().BoolVar(&pull, "pull", false, i18n.T("cli.flag.sync.pull"))
	c.Flags().BoolVar(&bootstrap, "bootstrap", false, i18n.T("cli.flag.sync.bootstrap"))
	c.Flags().BoolVar(&applyAll, "all", false, i18n.T("cli.flag.sync.all"))
	c.Flags().BoolVar(&dryRun, "dry-run", false, i18n.T("cli.flag.sync.dry_run"))
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
		fmt.Fprintln(c.OutOrStdout(), i18n.T("cli.print.nothing_to_do"))
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
