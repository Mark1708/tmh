package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"
	"github.com/spf13/cobra"
)

// TestActiveCommandConstruction verifies the Cobra command structure.
func TestActiveCommandConstruction(t *testing.T) {
	cmd := newActiveCmd()

	// Verify main command structure
	if cmd.Use != "active" {
		t.Errorf("expected Use='active', got %q", cmd.Use)
	}

	// Verify subcommands
	expectedSubcmds := []string{"status", "prune", "remove", "recover", "touch"}
	subcmds := make(map[string]bool)
	for _, c := range cmd.Commands() {
		subcmds[c.Name()] = true
	}

	for _, name := range expectedSubcmds {
		if !subcmds[name] {
			t.Errorf("missing subcommand: %s", name)
		}
	}

	// Verify touch command is hidden
	var touchCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "touch" {
			touchCmd = c
			break
		}
	}

	if touchCmd == nil {
		t.Fatal("touch subcommand not found")
	}

	if !touchCmd.Hidden {
		t.Error("touch command should be hidden")
	}
}

// TestHelpDoesNotShowTouch verifies hidden touch command does not appear in help.
func TestHelpDoesNotShowTouch(t *testing.T) {
	cmd := newActiveCmd()

	// Get help output
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Test root help
	if err := cmd.Help(); err != nil {
		t.Fatalf("Help() failed: %v", err)
	}

	help := buf.String()

	// Check that "touch" is not listed as a command name (lines starting with spaces then command name)
	// The touch command itself will have "touch" in its help text, but not as a top-level command name
	lines := strings.Split(help, "\n")
	for _, line := range lines {
		// Check for command lines (not descriptions or other text)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "touch") && len(trimmed) > 5 && !strings.Contains(trimmed, "  ") {
			// This looks like a command name line, not the touch command's own help text
			t.Errorf("touch command should not appear as a top-level command in help output")
		}
	}
}

func TestRenderActiveStatusOnlyLabelsOrphanedRows(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("init i18n: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	orphanedAt := now.Add(time.Minute)
	report := actions.ActiveStatusReport{
		Enabled:  true,
		Session:  actions.ActiveSessionName,
		Owned:    true,
		Tracked:  1,
		Orphaned: 1,
		Entries: []actions.ActiveWindowStatus{
			{
				WindowID:       "@1",
				WindowName:     "tracked",
				State:          state.ActiveWindowTracked,
				PromotedAt:     now,
				LastSelectedAt: now,
				ExpiresAt:      now.Add(time.Hour),
			},
			{
				WindowID:       "@2",
				WindowName:     "orphaned",
				State:          state.ActiveWindowOrphaned,
				PromotedAt:     now,
				LastSelectedAt: now,
				ExpiresAt:      now.Add(time.Hour),
				OrphanedAt:     orphanedAt,
			},
		},
	}
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	renderActiveStatus(cmd, report)

	out := buf.String()
	tracked := sectionForWindow(out, "@1")
	if strings.Contains(tracked, "Orphaned") {
		t.Fatalf("tracked row was labeled orphaned:\n%s", tracked)
	}
	if !strings.Contains(sectionForWindow(out, "@2"), "Orphaned at: "+orphanedAt.Format(time.RFC3339)) {
		t.Fatalf("orphaned row missing orphaned timestamp:\n%s", sectionForWindow(out, "@2"))
	}
}

func sectionForWindow(out, windowID string) string {
	start := strings.Index(out, "  "+windowID+":")
	if start < 0 {
		return ""
	}
	next := strings.Index(out[start+1:], "\n  @")
	if next < 0 {
		return out[start:]
	}
	return out[start : start+1+next]
}
