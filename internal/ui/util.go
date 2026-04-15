package ui

import (
	"context"
	"strings"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/tmux"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// keyMatches returns true if the message matches the binding.
func keyMatches(msg tea.KeyMsg, b key.Binding) bool { return key.Matches(msg, b) }

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w-1]) + "…"
}

func padRight(s string, w int) string {
	if w <= lipgloss.Width(s) {
		return s
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

func placeMiddle(width, height int, content string) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func overlayBottomRight(base, overlay string, w, h int) string {
	// Simple approach: position overlay using lipgloss.Place's right/bottom
	// alignment over a transparent layer.
	bottom := lipgloss.Place(w, 1, lipgloss.Right, lipgloss.Bottom, overlay)
	_ = h
	return base + "\n" + bottom
}

// collectLive mirrors actions.collectLive but lives here to avoid a
// circular import. (actions imports config; ui imports actions.)
func collectLive(ctx context.Context, r tmux.Runner) (config.LiveSnapshot, error) {
	var snap config.LiveSnapshot
	sessions, err := r.ListSessions(ctx)
	if err != nil {
		return snap, err
	}
	all, err := r.ListPanes(ctx, "")
	if err != nil {
		return snap, err
	}
	type k struct {
		s string
		w int
	}
	first := make(map[k]string, len(all))
	for _, p := range all {
		key := k{p.Session, p.Window}
		if _, set := first[key]; !set {
			first[key] = p.Path
		}
	}
	for _, s := range sessions {
		wins, err := r.ListWindows(ctx, s.Name)
		if err != nil {
			return snap, err
		}
		ls := config.LiveSession{Name: s.Name}
		for _, w := range wins {
			ls.Windows = append(ls.Windows, config.LiveWindow{
				Name: w.Name,
				Dir:  first[k{s.Name, w.Index}],
			})
		}
		snap.Sessions = append(snap.Sessions, ls)
	}
	return snap, nil
}
