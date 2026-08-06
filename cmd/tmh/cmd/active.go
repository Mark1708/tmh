package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newActiveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "active",
		Short: i18n.T("cli.active.short"),
	}
	c.AddCommand(
		newActiveStatusCmd(),
		newActivePruneCmd(),
		newActiveRemoveCmd(),
		newActiveRecoverCmd(),
		newActiveTouchCmd(),
	)
	return c
}

func newActiveStatusCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "status",
		Short: i18n.T("cli.active.status.short"),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := loadConfig(true)
			if err != nil {
				return err
			}
			db, err := state.Open(xdg.StateDBPath())
			if err != nil {
				fmt.Fprintln(c.OutOrStderr(), i18n.Tf("cli.print.state_db_warn", map[string]any{"err": err}))
				db = nil
			}
			if db != nil {
				defer db.Close()
			}

			r := newRunner()
			ctx := context.Background()

			report, err := actions.ActiveStatus(ctx, r, db, cfg.Defaults.TmuxIntegration.Active, time.Now())
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}

			renderActiveStatus(c, report)
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, i18n.T("cli.flag.json"))
	return c
}

func newActivePruneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "prune",
		Short: i18n.T("cli.active.prune.short"),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := loadConfig(true)
			if err != nil {
				return err
			}
			db, err := state.Open(xdg.StateDBPath())
			if err != nil {
				return fmt.Errorf("open state db: %w", err)
			}
			defer db.Close()

			r := newRunner()
			ctx := context.Background()

			report, err := actions.PruneActiveWindows(ctx, r, db, cfg.Defaults.TmuxIntegration.Active, time.Now())
			if err != nil {
				return err
			}

			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.prune.report",
				map[string]any{"unlinked": report.Unlinked, "deleted": report.Deleted, "orphaned": report.Orphaned}))
			return nil
		},
	}
	return c
}

func newActiveRemoveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "remove @ID",
		Short: i18n.T("cli.active.remove.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := loadConfig(true)
			if err != nil {
				return err
			}
			db, err := state.Open(xdg.StateDBPath())
			if err != nil {
				return fmt.Errorf("open state db: %w", err)
			}
			defer db.Close()

			r := newRunner()
			ctx := context.Background()
			windowID := args[0]

			if err := actions.RemoveActiveWindow(ctx, r, db, cfg.Defaults.TmuxIntegration.Active, windowID); err != nil {
				return err
			}

			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.removed", map[string]any{"window_id": windowID}))
			return nil
		},
	}
	return c
}

func newActiveRecoverCmd() *cobra.Command {
	var destination string
	c := &cobra.Command{
		Use:   "recover @ID --to session",
		Short: i18n.T("cli.active.recover.short"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if destination == "" {
				return fmt.Errorf("--to is required")
			}

			cfg, err := loadConfig(true)
			if err != nil {
				return err
			}
			db, err := state.Open(xdg.StateDBPath())
			if err != nil {
				return fmt.Errorf("open state db: %w", err)
			}
			defer db.Close()

			r := newRunner()
			ctx := context.Background()
			windowID := args[0]

			if err := actions.RecoverActiveWindow(ctx, r, db, cfg.Defaults.TmuxIntegration.Active, windowID, destination); err != nil {
				return err
			}

			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.recovered",
				map[string]any{"window_id": windowID, "destination": destination}))
			return nil
		},
	}
	c.Flags().StringVar(&destination, "to", "", i18n.T("cli.flag.active.recover.to"))
	return c
}

func newActiveTouchCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "touch @ID",
		Short:  "Internal hook handler - do not call directly",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			// Use short timeout so hook cannot stall tmux
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			cfg, err := loadConfig(true)
			if err != nil {
				// Feature disabled or missing config: treat as success no-op
				return nil
			}
			if !cfg.Defaults.TmuxIntegration.Active.Enabled {
				// Feature disabled: treat as success no-op
				return nil
			}

			db, err := state.Open(xdg.StateDBPath())
			if err != nil {
				// State DB unavailable: treat as success no-op
				return nil
			}
			defer db.Close()

			r := newRunner()
			windowID := args[0]

			if _, touchErr := actions.TouchActiveWindow(ctx, r, db, cfg.Defaults.TmuxIntegration.Active, windowID, time.Now()); touchErr != nil {
				slog.Debug("active window touch failed", "window_id", windowID, "error", touchErr)
			}
			// Touch returning false for absent tracked ID is not an error.
			return nil
		},
	}
	return c
}

func renderActiveStatus(c *cobra.Command, report actions.ActiveStatusReport) {
	if !report.Enabled {
		fmt.Fprintln(c.OutOrStdout(), i18n.T("cli.active.status.disabled"))
		return
	}

	fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.session", map[string]any{"session": report.Session}))
	fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.enabled", map[string]any{"enabled": report.Enabled}))
	fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.owned", map[string]any{"owned": report.Owned}))
	if report.Collision {
		fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.collision", map[string]any{"collision": report.Collision, "reason": report.CollisionReason}))
	}
	if report.TTL != "" {
		fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.ttl", map[string]any{"ttl": report.TTL}))
	}
	if report.ServerEpoch != "" {
		fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.server_epoch", map[string]any{"epoch": report.ServerEpoch}))
	}
	fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.tracked", map[string]any{"tracked": report.Tracked}))
	fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.orphaned", map[string]any{"orphaned": report.Orphaned}))

	if len(report.Entries) > 0 {
		fmt.Fprintln(c.OutOrStdout(), i18n.T("cli.active.status.entries_header"))
		for _, entry := range report.Entries {
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.entry_window", map[string]any{"window_id": entry.WindowID, "window_name": entry.WindowName}))
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.entry_state", map[string]any{"state": entry.State}))
			if entry.ActiveIndex != nil {
				fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.entry_active_index", map[string]any{"index": *entry.ActiveIndex}))
			}
			for _, link := range entry.SourceLinks {
				fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.entry_source", map[string]any{"session": link.SessionName, "window_index": link.WindowIndex}))
			}
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.entry_promoted_at", map[string]any{"time": entry.PromotedAt.Format(time.RFC3339)}))
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.entry_last_selected", map[string]any{"time": entry.LastSelectedAt.Format(time.RFC3339)}))
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.entry_expires_at", map[string]any{"time": entry.ExpiresAt.Format(time.RFC3339)}))
			if !entry.OrphanedAt.IsZero() {
				fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.active.status.entry_orphaned_at", map[string]any{"time": entry.OrphanedAt.Format(time.RFC3339)}))
			}
			if entry.Expired {
				fmt.Fprintln(c.OutOrStdout(), i18n.T("cli.active.status.entry_expired"))
			}
		}
	}
}
