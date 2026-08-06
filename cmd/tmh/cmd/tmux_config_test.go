package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

func TestTmuxAuditCommandUsesActiveConfig(t *testing.T) {
	withActiveConfigFile(t, true)
	runner := tmuxtest.New()
	runner.Hooks()[actions.ActiveHookSlot] = ""

	findings, err := tmuxAuditFindings(context.Background(), runner)
	if err != nil {
		t.Fatalf("audit findings: %v", err)
	}
	if !hasActiveHookFinding(findings) {
		t.Fatalf("expected active hook finding when active enabled: %+v", findings)
	}
}

func TestTmuxSetupCommandUsesActiveConfig(t *testing.T) {
	withActiveConfigFile(t, true)
	runner := tmuxtest.New()
	runner.Hooks()[actions.ActiveHookSlot] = ""

	snippets, err := tmuxSetupSnippets(context.Background(), runner)
	if err != nil {
		t.Fatalf("setup snippets: %v", err)
	}
	for _, snippet := range snippets {
		if strings.Contains(snippet.Line, actions.ActiveHookSlot) {
			return
		}
	}
	t.Fatalf("expected setup snippets to include active hook: %+v", snippets)
}

func TestTmuxAuditCommandOmitsActiveHookWhenDisabled(t *testing.T) {
	withActiveConfigFile(t, false)
	runner := tmuxtest.New()
	runner.Hooks()[actions.ActiveHookSlot] = ""

	findings, err := tmuxAuditFindings(context.Background(), runner)
	if err != nil {
		t.Fatalf("audit findings: %v", err)
	}
	if hasActiveHookFinding(findings) {
		t.Fatalf("did not expect active hook finding when disabled: %+v", findings)
	}
}

func withActiveConfigFile(t *testing.T, enabled bool) {
	t.Helper()
	resetRootFlags(t)
	active := "false"
	if enabled {
		active = "true"
	}
	path := filepath.Join(t.TempDir(), "config.yml")
	content := "version: 1\ndefaults:\n  tmux_integration:\n    active:\n      enabled: " + active + "\n      ttl: 5h\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	flags.ConfigPath = path
}

func hasActiveHookFinding(findings []actions.AuditFinding) bool {
	for _, finding := range findings {
		if finding.Check == "active session hook" {
			return true
		}
	}
	return false
}
