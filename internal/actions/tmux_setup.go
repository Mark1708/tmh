package actions

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	findings := AuditTmuxConfig(ctx, r)
	findingByCheck := make(map[string]AuditFinding, len(findings))
	for _, f := range findings {
		findingByCheck[f.Check] = f
	}
	ok := func(check string) bool { return findingByCheck[check].Level == AuditOK }

	return []Snippet{
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

// AppendToConfig writes the managed block into path, skipping lines that
// already appear anywhere in the file verbatim. Returns the number of
// lines actually appended. If the block already exists (detected by
// setupHeader) the call is a no-op.
func AppendToConfig(path string, snippets []Snippet) (appended int, err error) {
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return 0, err
	}

	if strings.Contains(existing, setupHeader) {
		return 0, nil
	}

	var block strings.Builder
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		block.WriteString("\n")
	}
	block.WriteString("\n")
	block.WriteString(setupHeader + "\n")
	for _, s := range snippets {
		if strings.Contains(existing, s.Line) {
			continue // don't duplicate what user already has
		}
		if s.Reason != "" {
			block.WriteString("# " + s.Reason + "\n")
		}
		block.WriteString(s.Line + "\n")
		appended++
	}
	block.WriteString(setupFooter + "\n")

	if appended == 0 {
		return 0, nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.WriteString(block.String()); err != nil {
		return 0, err
	}
	return appended, nil
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
