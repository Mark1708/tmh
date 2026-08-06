package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/tmux"

	"github.com/spf13/cobra"
)

func newTmuxCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tmux",
		Short: i18n.T("cli.tmux.short"),
	}
	c.AddCommand(newTmuxAuditCmd(), newTmuxSetupCmd())
	return c
}

func tmuxAuditFindings(ctx context.Context, r tmux.Runner) ([]actions.AuditFinding, error) {
	cfg, err := loadConfig(true)
	if err != nil {
		return nil, err
	}
	return actions.AuditTmuxConfigWithActiveConfig(ctx, r, cfg), nil
}

func tmuxSetupSnippets(ctx context.Context, r tmux.Runner) ([]actions.Snippet, error) {
	cfg, err := loadConfig(true)
	if err != nil {
		return nil, err
	}
	return actions.SetupWithActiveConfig(ctx, r, cfg), nil
}

func newTmuxAuditCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "audit",
		Short: i18n.T("cli.tmux.audit.short"),
		RunE: func(c *cobra.Command, args []string) error {
			r := newRunner()
			findings, err := tmuxAuditFindings(context.Background(), r)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(findings)
			}
			renderAudit(c, findings)
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, i18n.T("cli.flag.tmux.audit.json"))
	return c
}

func newTmuxSetupCmd() *cobra.Command {
	var appendToFile bool
	c := &cobra.Command{
		Use:   "setup",
		Short: i18n.T("cli.tmux.setup.short"),
		RunE: func(c *cobra.Command, args []string) error {
			r := newRunner()
			snippets, err := tmuxSetupSnippets(context.Background(), r)
			if err != nil {
				return err
			}
			if !appendToFile {
				actions.PrintSetup(snippets, os.Stdout, true)
				return nil
			}
			path := filepath.Join(os.Getenv("HOME"), ".tmux.conf")
			n, err := actions.AppendToConfig(path, snippets)
			if err != nil {
				return err
			}
			if n == 0 {
				fmt.Fprintln(c.OutOrStdout(), i18n.T("cli.print.nothing_to_append"))
				return nil
			}
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.print.appended_lines", map[string]any{"count": n, "path": path}))
			return nil
		},
	}
	c.Flags().BoolVar(&appendToFile, "append", false, i18n.T("cli.flag.tmux.setup.append"))
	return c
}

func renderAudit(c *cobra.Command, findings []actions.AuditFinding) {
	for _, f := range findings {
		marker := "  "
		switch f.Level {
		case actions.AuditOK:
			marker = "✓ "
		case actions.AuditWarn:
			marker = "⚠ "
		case actions.AuditError:
			marker = "✗ "
		}
		fmt.Fprintf(c.OutOrStdout(), "%s%-38s %s\n", marker, f.Check, f.Message)
		if f.Current != "" || f.Expected != "" {
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.print.current_expected", map[string]any{"current": f.Current, "expected": f.Expected}))
		}
		if f.Level != actions.AuditOK && f.FixHint != "" {
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.print.fix_hint", map[string]any{"hint": f.FixHint}))
		}
	}
}
