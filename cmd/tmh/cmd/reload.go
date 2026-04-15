package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newReloadCmd() *cobra.Command {
	var (
		shell, tmuxOnly, all, busy, respawn, status bool
		rcFile, tmuxConf                            string
	)
	c := &cobra.Command{
		Use:   "reload",
		Short: i18n.T("cli.reload.short"),
		RunE: func(c *cobra.Command, args []string) error {
			if status {
				return printReloadStatus(c)
			}

			if !shell && !tmuxOnly && !all {
				all = true
			}
			if all {
				shell = true
				tmuxOnly = true
			}
			if rcFile == "" {
				rcFile = filepath.Join(os.Getenv("HOME"), ".zshrc")
			}
			if tmuxConf == "" {
				tmuxConf = filepath.Join(os.Getenv("HOME"), ".tmux.conf")
			}

			db, err := state.Open(xdg.StateDBPath())
			if err != nil {
				fmt.Fprintln(c.OutOrStdout(), "warn: state.db unavailable —", err)
				db = nil
			}
			if db != nil {
				defer db.Close()
			}

			r := newRunner()
			rep, err := actions.Reload(context.Background(), r, db, rcFile, actions.ReloadOptions{
				Shell: shell, Tmux: tmuxOnly, Busy: busy, Respawn: respawn,
				RcFile: rcFile, TmuxConf: tmuxConf,
			})
			if err != nil {
				return err
			}
			for _, p := range rep.ReloadedPanes {
				fmt.Fprintln(c.OutOrStdout(), "reloaded:", p)
			}
			for _, p := range rep.QueuedPanes {
				fmt.Fprintln(c.OutOrStdout(), "queued: ", p)
			}
			for _, p := range rep.SkippedBusy {
				fmt.Fprintln(c.OutOrStdout(), "skipped:", p)
			}
			if rep.TmuxSourced {
				fmt.Fprintln(c.OutOrStdout(), "tmux: source-file", tmuxConf)
			}
			if respawn {
				fmt.Fprintln(c.OutOrStdout(), "--respawn not yet implemented; use `tmux kill-server && tmh init`")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&shell, "shell", false, i18n.T("cli.flag.reload.shell"))
	c.Flags().BoolVar(&tmuxOnly, "tmux", false, i18n.T("cli.flag.reload.tmux"))
	c.Flags().BoolVar(&all, "all", false, i18n.T("cli.flag.reload.all"))
	c.Flags().BoolVar(&busy, "busy", false, i18n.T("cli.flag.reload.busy"))
	c.Flags().BoolVar(&respawn, "respawn", false, i18n.T("cli.flag.reload.respawn"))
	c.Flags().BoolVar(&status, "status", false, i18n.T("cli.flag.reload.status"))
	c.Flags().StringVar(&rcFile, "rc", "", i18n.T("cli.flag.reload.rc"))
	c.Flags().StringVar(&tmuxConf, "tmux-conf", "", i18n.T("cli.flag.reload.tmux_conf"))
	return c
}

func printReloadStatus(c *cobra.Command) error {
	db, err := state.Open(xdg.StateDBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	pending, err := db.PendingReloads(context.Background())
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		fmt.Fprintln(c.OutOrStdout(), "no pending reloads")
		return nil
	}
	for _, e := range pending {
		fmt.Fprintf(c.OutOrStdout(), "%-24s %-8s queued %s, expires %s\n",
			e.PaneTarget, e.Action, e.QueuedAt.Format("15:04:05"), e.ExpiresAt.Format("15:04:05"))
	}
	return nil
}
