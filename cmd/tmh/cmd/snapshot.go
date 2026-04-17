package cmd

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newSnapshotCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "snapshot",
		Short: i18n.T("cli.snapshot.short"),
	}
	c.AddCommand(snapshotSaveCmd(), snapshotRestoreCmd(), snapshotLsCmd(), snapshotRmCmd())
	return c
}

func openStateOrExit(c *cobra.Command) (*state.DB, error) {
	db, err := state.Open(xdg.StateDBPath())
	if err != nil {
		return nil, err
	}
	return db, nil
}

func snapshotSaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "save <name>",
		Short: i18n.T("cli.snapshot.save.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			db, err := openStateOrExit(c)
			if err != nil {
				return err
			}
			defer db.Close()
			r := newRunner()
			if err := actions.SaveSnapshot(context.Background(), r, db, args[0]); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "saved:", args[0])
			return nil
		},
	}
}

func snapshotRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <name>",
		Short: i18n.T("cli.snapshot.restore.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			db, err := openStateOrExit(c)
			if err != nil {
				return err
			}
			defer db.Close()
			r := newRunner()
			restored, err := actions.RestoreSnapshot(context.Background(), r, db, args[0])
			if err != nil {
				return err
			}
			for _, s := range restored {
				fmt.Fprintf(c.OutOrStdout(), "restored: %s (%d windows)\n", s.Name, len(s.Windows))
				for _, w := range s.Windows {
					for _, p := range w.Panes {
						if p.Command != "" && p.Command != "zsh" && p.Command != "bash" {
							fmt.Fprintf(c.OutOrStdout(), "  hint: pane in %s/%s ran %q before — re-run manually\n",
								s.Name, w.Name, p.Command)
						}
					}
				}
			}
			return nil
		},
	}
}

func snapshotLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: i18n.T("cli.snapshot.list.short"),
		RunE: func(c *cobra.Command, args []string) error {
			db, err := openStateOrExit(c)
			if err != nil {
				return err
			}
			defer db.Close()
			snaps, err := db.ListSnapshots(context.Background())
			if err != nil {
				return err
			}
			if len(snaps) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "(no snapshots)")
				return nil
			}
			for _, s := range snaps {
				fmt.Fprintf(c.OutOrStdout(), "%s  %s\n", s.TS.Format("2006-01-02 15:04:05"), s.Name)
			}
			return nil
		},
	}
}

func snapshotRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: i18n.T("cli.snapshot.delete.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			db, err := openStateOrExit(c)
			if err != nil {
				return err
			}
			defer db.Close()
			return db.DeleteSnapshot(context.Background(), args[0])
		},
	}
}
