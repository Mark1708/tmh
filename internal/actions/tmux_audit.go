package actions

import (
	"context"
	"strings"

	"git.mark1708.ru/me/tmh/internal/tmux"
)

// AuditLevel classifies findings from AuditTmuxConfig.
type AuditLevel string

const (
	AuditOK    AuditLevel = "ok"
	AuditWarn  AuditLevel = "warn"
	AuditError AuditLevel = "error"
)

// AuditCategory groups checks by their relationship to tmh.
type AuditCategory string

const (
	CatBaseline    AuditCategory = "baseline"     // tmh depends on this
	CatRecommended AuditCategory = "recommended"  // UX nicety
	CatConflict    AuditCategory = "conflict"     // creates races with tmh
	CatIntegration AuditCategory = "integration"  // tmh bindings / status segment
)

// AuditFinding is one row in the audit report.
type AuditFinding struct {
	Level    AuditLevel    `json:"level"`
	Category AuditCategory `json:"category"`
	Check    string        `json:"check"`
	Current  string        `json:"current,omitempty"`
	Expected string        `json:"expected,omitempty"`
	Message  string        `json:"message"`
	FixHint  string        `json:"fix_hint,omitempty"`

	// Apply, when non-nil, performs the remediation in-process against the
	// supplied Runner (e.g. SetOption). UI layer wires this to an "apply"
	// button. For checks that must live in ~/.tmux.conf (bindings,
	// status-right) Apply stays nil — suggest user edits the file.
	Apply func(ctx context.Context, r tmux.Runner) error `json:"-"`
}

// AuditTmuxConfig runs every baseline/recommended/conflict/integration check
// against the live server state accessed through r. The result is a flat
// list suitable for rendering as a table.
func AuditTmuxConfig(ctx context.Context, r tmux.Runner) []AuditFinding {
	var out []AuditFinding
	out = append(out, auditBaseline(ctx, r)...)
	out = append(out, auditRecommended(ctx, r)...)
	out = append(out, auditConflicts(ctx, r)...)
	out = append(out, auditIntegration(ctx, r)...)
	return out
}

// --- baseline ---

func auditBaseline(ctx context.Context, r tmux.Runner) []AuditFinding {
	return []AuditFinding{
		auditOption(ctx, r, AuditError, CatBaseline, "default-terminal",
			"tmux-256color", contains("tmux-256color"),
			"colour themes need truecolor; old xterm-256color breaks catppuccin",
			`set -g default-terminal "tmux-256color"`),

		auditOption(ctx, r, AuditError, CatBaseline, "mouse", "on", equals("on"),
			"mouse needed for bubbletea scroll/hover in the dashboard",
			`set -g mouse on`),

		auditOption(ctx, r, AuditError, CatBaseline, "escape-time", "0", numLE(10),
			"bubbletea sees esc ~500ms late with the default 500ms escape-time",
			`set -sg escape-time 0`),

		auditOption(ctx, r, AuditWarn, CatBaseline, "extended-keys", "on", equals("on"),
			"extended-keys=on enables Shift+Tab / Ctrl+Enter in the TUI",
			`set -s extended-keys on`),
	}
}

// --- recommended ---

func auditRecommended(ctx context.Context, r tmux.Runner) []AuditFinding {
	f1 := auditOption(ctx, r, AuditWarn, CatRecommended, "base-index", "1", equals("1"),
		"1-based index matches the convention used by tmh attach epcp:1",
		`set -g base-index 1`)
	f1.Apply = applySetOption("base-index", "1", false)

	f2 := auditOption(ctx, r, AuditWarn, CatRecommended, "pane-base-index", "1", equals("1"),
		"consistent with base-index",
		`setw -g pane-base-index 1`)
	f2.Apply = applySetOption("pane-base-index", "1", true)

	f3 := auditOption(ctx, r, AuditWarn, CatRecommended, "renumber-windows", "on", equals("on"),
		"keeps window indices contiguous after kill; drift view stays clean",
		`set -g renumber-windows on`)
	f3.Apply = applySetOption("renumber-windows", "on", false)

	return []AuditFinding{f1, f2, f3}
}

// --- conflicts ---

func auditConflicts(ctx context.Context, r tmux.Runner) []AuditFinding {
	var out []AuditFinding

	if hook, _ := r.ShowHook(ctx, "after-new-window"); hook != "" {
		f := AuditFinding{
			Level:    AuditWarn,
			Category: CatConflict,
			Check:    "after-new-window hook",
			Current:  hook,
			Expected: "(unset)",
			Message:  "hook races with tmh window creation and can overwrite the chosen name",
			FixHint:  "tmux set-hook -gu after-new-window",
			Apply: func(ctx context.Context, r tmux.Runner) error {
				return r.UnsetHook(ctx, "after-new-window")
			},
		}
		out = append(out, f)
	}

	if autoRename, _ := r.ShowOption(ctx, "automatic-rename"); strings.EqualFold(strings.TrimSpace(autoRename), "on") {
		out = append(out, AuditFinding{
			Level:    AuditWarn,
			Category: CatConflict,
			Check:    "automatic-rename",
			Current:  autoRename,
			Expected: "off",
			Message:  "shell commands would overwrite tmh window names when on",
			FixHint:  "set -g automatic-rename off",
			Apply:    applySetOption("automatic-rename", "off", false),
		})
	}

	return out
}

// --- integration ---

func auditIntegration(ctx context.Context, r tmux.Runner) []AuditFinding {
	var out []AuditFinding

	statusRight, _ := r.ShowOption(ctx, "status-right")
	if !strings.Contains(statusRight, "tmh status") {
		out = append(out, AuditFinding{
			Level:    AuditWarn,
			Category: CatIntegration,
			Check:    "status-right includes tmh status",
			Current:  truncateStr(statusRight, 40),
			Expected: `contains "#(tmh status)"`,
			Message:  "drift / reload / zshrc badges stay hidden without the status segment",
			FixHint:  `set -ag status-right ' #(tmh status)'`,
		})
	} else {
		out = append(out, AuditFinding{
			Level: AuditOK, Category: CatIntegration,
			Check:   "status-right includes tmh status",
			Current: "present",
			Message: "tmh status segment visible in status bar",
		})
	}

	return out
}

// --- option-check helpers ---

func auditOption(ctx context.Context, r tmux.Runner, failLevel AuditLevel, cat AuditCategory, opt, expected string, match func(string) bool, message, fix string) AuditFinding {
	cur, _ := r.ShowOption(ctx, opt)
	cur = strings.TrimSpace(cur)
	f := AuditFinding{
		Category: cat, Check: opt, Expected: expected, Current: cur,
		Message: message, FixHint: fix,
	}
	if match(cur) {
		f.Level = AuditOK
	} else {
		f.Level = failLevel
	}
	return f
}

func equals(want string) func(string) bool {
	return func(s string) bool { return strings.EqualFold(strings.TrimSpace(s), want) }
}

func contains(sub string) func(string) bool {
	return func(s string) bool { return strings.Contains(s, sub) }
}

func numLE(limit int) func(string) bool {
	return func(s string) bool {
		var n int
		if _, err := fmtSscan(s, &n); err != nil {
			return false
		}
		return n <= limit
	}
}

func applySetOption(name, value string, window bool) func(context.Context, tmux.Runner) error {
	return func(ctx context.Context, r tmux.Runner) error {
		return r.SetOption(ctx, name, value, window)
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// fmtSscan is a tiny shim so we don't pull fmt into this file (keeps the
// audit package minimal and easier to test in isolation).
func fmtSscan(s string, n *int) (int, error) {
	// strconv.Atoi is simpler but returns count-less errors; we mimic
	// fmt.Sscanf's signature so the helper reads naturally at call sites.
	var sign int = 1
	i := 0
	if len(s) == 0 {
		return 0, errParseInt
	}
	if s[0] == '-' {
		sign = -1
		i++
	}
	val := 0
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errParseInt
		}
		val = val*10 + int(c-'0')
	}
	*n = sign * val
	return 1, nil
}

// errParseInt is returned by fmtSscan on bad input. Defined here to keep the
// helper dependency-free.
var errParseInt = parseErr("parse int")

type parseErr string

func (e parseErr) Error() string { return string(e) }
