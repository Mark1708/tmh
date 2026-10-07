// Package testutil provides deterministic Bubble Tea v2 test helpers shared by
// all tmh-next tests: key/size message builders and ANSI-aware layout
// assertions. Backend gating helpers live in backend.go (domain-aware).
package testutil

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Rune builds a printable key press.
func Rune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// Space builds the space key press ("space" per v2 String()).
func Space() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

// Special builds a non-printable key press (enter, esc, tab, up, ...).
func Special(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// Ctrl builds a ctrl+<rune> key press.
func Ctrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

// Type expands a literal string into printable key presses.
func Type(s string) []tea.Msg {
	msgs := make([]tea.Msg, 0, len(s))
	for _, r := range s {
		msgs = append(msgs, Rune(r))
	}
	return msgs
}

// Size builds a window size message.
func Size(w, h int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: w, Height: h}
}

// MaxLineWidth returns the widest ANSI-aware line width in a rendered view.
func MaxLineWidth(v string) int {
	max := 0
	for _, line := range strings.Split(v, "\n") {
		if w := StringWidth(line); w > max {
			max = w
		}
	}
	return max
}

// StringWidth is the ANSI-aware display width of a (possibly styled) string.
func StringWidth(s string) int {
	return ansi.StringWidth(s)
}

// Plain strips ANSI escapes from a rendered string.
func Plain(s string) string {
	return ansi.Strip(s)
}

// Contains reports whether the plain-text rendering of v contains want.
func Contains(v, want string) bool {
	return strings.Contains(Plain(v), want)
}

// ColumnOf returns the ANSI-aware display column of the first occurrence of
// substr in line, or -1 when absent. Never use byte offsets on styled text.
func ColumnOf(line, substr string) int {
	plain := Plain(line)
	idx := strings.Index(plain, substr)
	if idx < 0 {
		return -1
	}
	return StringWidth(plain[:idx])
}

// RunCmdFast executes a tea.Cmd with a short deadline so timer-driven
// commands (cursor blink, spinners) never stall a deterministic test. It
// reports ok=false when the command did not finish in time.
func RunCmdFast(cmd tea.Cmd, timeout time.Duration) (tea.Msg, bool) {
	if cmd == nil {
		return nil, false
	}
	type result struct{ msg tea.Msg }
	ch := make(chan result, 1)
	go func() { ch <- result{cmd()} }()
	select {
	case r := <-ch:
		return r.msg, true
	case <-time.After(timeout):
		return nil, false
	}
}
