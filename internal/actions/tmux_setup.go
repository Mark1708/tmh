package actions

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux"
)

// setupHeader is written before the tmh-managed block when --append writes
// into ~/.tmux.conf. Used to detect an existing block and to group tmh
// lines together for clean removal.
const setupHeader = "# ---- tmh integration (added by `tmh tmux setup --append`) ----"

// setupFooter closes the managed block.
const setupFooter = "# ---- end tmh integration ----"

// Snippet is one line of ~/.tmux.conf the setup command suggests or writes.
type Snippet struct {
	Line    string // the literal tmux directive
	Reason  string // one-line comment above it in --append mode
	Already bool   // true when audit shows the line is already effective
}

// Setup computes the list of tmux.conf snippets needed for ideal tmh
// integration given current server state. Lines whose effect is already
// applied (via --audit finding OK) are marked Already=true so callers can
// filter them out in suggestion output.
func Setup(ctx context.Context, r tmux.Runner) []Snippet {
	return SetupWithActiveConfig(ctx, r, nil)
}

// SetupWithActiveConfig computes the list of tmux.conf snippets needed for
// ideal tmh integration given current server state and configuration. The
// active hook is only included when cfg is non-nil and active integration
// is enabled.
func SetupWithActiveConfig(ctx context.Context, r tmux.Runner, cfg *config.Config) []Snippet {
	findings := AuditTmuxConfigWithActiveConfig(ctx, r, cfg)
	findingByCheck := make(map[string]AuditFinding, len(findings))
	for _, f := range findings {
		findingByCheck[f.Check] = f
	}
	ok := func(check string) bool { return findingByCheck[check].Level == AuditOK }

	snippets := []Snippet{
		{`set -g default-terminal "tmux-256color"`, "truecolor for lipgloss", ok("default-terminal")},
		{`set -as terminal-features ",xterm-256color:RGB"`, "RGB capability", false},
		{`set -g mouse on`, "required by bubbletea mouse mode", ok("mouse")},
		{`set -sg escape-time 0`, "tmux intercepts esc by default", ok("escape-time")},
		{`set -s extended-keys on`, "Shift+Tab / Ctrl+Enter in TUI", ok("extended-keys")},
		{`set -g base-index 1`, "match tmh window numbering", ok("base-index")},
		{`setw -g pane-base-index 1`, "consistent with base-index", ok("pane-base-index")},
		{`set -g renumber-windows on`, "keep indices contiguous after kill", ok("renumber-windows")},
		{`set -ag status-right ' #(tmh status)'`, "drift / reload / zshrc badges", ok("status-right includes tmh status")},
		{`unbind R`, "prefix R → tmh reload --all", false},
		{`bind R run-shell "tmh reload --all"`, "", false},
	}

	// Include active hook when:
	// 1. cfg is non-nil AND enabled (config-aware path), OR
	// 2. cfg is nil AND slot is unset (legacy behavior)
	var includeActiveHook bool
	if cfg != nil {
		includeActiveHook = cfg.Defaults.TmuxIntegration.Active.Enabled
	} else {
		// Legacy behavior: include hook when slot is unset
		includeActiveHook = true
	}

	if includeActiveHook {
		if check, exists := findingByCheck["active session hook"]; exists && (check.Level == AuditOK || check.Current == "(unset)") {
			snippets = append(snippets, Snippet{
				Line:    fmt.Sprintf("set-hook -g '%s' '%s'", ActiveHookSlot, ActiveHookCommand),
				Reason:  "active session window tracking",
				Already: check.Current != "(unset)",
			})
		}
	}

	return snippets
}

// PrintSetup renders the snippet list to the writer; skips lines that are
// already effective when onlyMissing is true.
func PrintSetup(snippets []Snippet, w *os.File, onlyMissing bool) {
	fmt.Fprintln(w, "# Рекомендованные строки для ~/.tmux.conf (добавь в конец):")
	fmt.Fprintln(w, "# tmh tmux setup --append — записать автоматически.")
	fmt.Fprintln(w, "")
	for _, s := range snippets {
		if onlyMissing && s.Already {
			continue
		}
		if s.Reason != "" {
			fmt.Fprintln(w, "# "+s.Reason)
		}
		fmt.Fprintln(w, s.Line)
	}
}

// AppendToConfig writes or updates the managed block in path. It only
// replaces content between setupHeader and setupFooter, preserving all other
// content. Returns the number of lines added/modified.
func AppendToConfig(path string, snippets []Snippet) (int, error) {
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return 0, err
	}

	// Find or create managed block
	before, managed, after := splitManagedBlock(existing)

	// Build new managed block content
	var newBlock strings.Builder
	for _, s := range snippets {
		// Skip duplicates within snippets list
		blockContent := newBlock.String()
		if strings.Contains(blockContent, s.Line) {
			continue
		}
		if s.Reason != "" {
			newBlock.WriteString("# " + s.Reason + "\n")
		}
		newBlock.WriteString(s.Line + "\n")
	}

	// If block content unchanged, no modifications needed
	if strings.TrimSpace(managed) == strings.TrimSpace(newBlock.String()) {
		return 0, nil
	}

	// Rebuild full file content
	var result strings.Builder
	result.WriteString(before)
	if existing != "" && !strings.HasSuffix(before, "\n") {
		result.WriteString("\n")
	}
	result.WriteString("\n")
	result.WriteString(setupHeader + "\n")
	result.WriteString(newBlock.String())
	result.WriteString(setupFooter + "\n")
	result.WriteString(after)

	// Write atomically
	if err := os.WriteFile(path+".tmp", []byte(result.String()), 0o644); err != nil {
		return 0, err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return 0, err
	}

	return strings.Count(newBlock.String(), "\n"), nil
}

// splitManagedBlock splits existing content into before, managed block, and after.
// If no managed block exists, returns (existing, "", "").
func splitManagedBlock(content string) (before, managed, after string) {
	beforeIdx := strings.Index(content, setupHeader)
	if beforeIdx == -1 {
		return content, "", ""
	}
	afterHeader := content[beforeIdx:]

	footerIdx := strings.Index(afterHeader, setupFooter)
	if footerIdx == -1 {
		// Malformed block - treat as no block
		return content, "", ""
	}

	footerEndIdx := footerIdx + len(setupFooter) + 1
	// Find the end of the line containing footer
	if footerEndIdx < len(afterHeader) {
		newlineIdx := strings.Index(afterHeader[footerEndIdx:], "\n")
		if newlineIdx != -1 {
			footerEndIdx += newlineIdx + 1
		} else {
			footerEndIdx = len(afterHeader)
		}
	}

	return content[:beforeIdx], strings.TrimSpace(content[beforeIdx+len(setupHeader) : footerIdx]), afterHeader[footerEndIdx:]
}

// ApplyRuntime invokes the Apply hook for every non-OK finding that has one.
// Used by the Settings screen's "apply recommended tmux options" button.
// Returns the list of applied check names for feedback.
func ApplyRuntime(ctx context.Context, r tmux.Runner, findings []AuditFinding) ([]string, error) {
	var applied []string
	for _, f := range findings {
		if f.Level == AuditOK || f.Apply == nil {
			continue
		}
		if err := f.Apply(ctx, r); err != nil {
			return applied, fmt.Errorf("%s: %w", f.Check, err)
		}
		applied = append(applied, f.Check)
	}
	return applied, nil
}
