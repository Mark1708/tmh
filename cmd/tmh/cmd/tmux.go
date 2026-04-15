package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"git.mark1708.ru/me/tmh/internal/actions"

	"github.com/spf13/cobra"
)

func newTmuxCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tmux",
		Short: "Tmux integration audit and setup",
	}
	c.AddCommand(newTmuxAuditCmd(), newTmuxSetupCmd())
	return c
}

func newTmuxAuditCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "audit",
		Short: "Check tmux server settings against tmh baseline + recommended list",
		RunE: func(c *cobra.Command, args []string) error {
			r := newRunner()
			findings := actions.AuditTmuxConfig(context.Background(), r)
			if jsonOut {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(findings)
			}
			renderAudit(c, findings)
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "print findings as JSON")
	return c
}

func newTmuxSetupCmd() *cobra.Command {
	var appendToFile bool
	c := &cobra.Command{
		Use:   "setup",
		Short: "Generate a tmux.conf snippet for tmh; --append writes to ~/.tmux.conf",
		RunE: func(c *cobra.Command, args []string) error {
			r := newRunner()
			snippets := actions.Setup(context.Background(), r)
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
				fmt.Fprintln(c.OutOrStdout(), "nothing to append — all lines already present or block exists")
				return nil
			}
			fmt.Fprintf(c.OutOrStdout(), "appended %d line(s) to %s\n", n, path)
			return nil
		},
	}
	c.Flags().BoolVar(&appendToFile, "append", false, "append recommended block to ~/.tmux.conf")
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
			fmt.Fprintf(c.OutOrStdout(), "    current: %q  expected: %q\n", f.Current, f.Expected)
		}
		if f.Level != actions.AuditOK && f.FixHint != "" {
			fmt.Fprintf(c.OutOrStdout(), "    → %s\n", f.FixHint)
		}
	}
}
