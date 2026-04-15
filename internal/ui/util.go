package ui

import (
	"context"
	"strings"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/ui/theme"

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

// placeMiddle centres content inside a width×height canvas. The whitespace
// around the content is painted with the palette's base background so the
// modal does not bleed through to whatever was behind (dashboard tree,
// etc.). Without WithWhitespaceBackground the gaps around a Modal.Render
// show the dashboard underneath — which reads like unfilled strings.
func placeMiddle(width, height int, content string, p theme.Palette) string {
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center, content,
		lipgloss.WithWhitespaceBackground(p.Bg),
	)
}

// modalBg returns a style that inherits the modal's background. Pass it to
// Inherit() on any inner style so the inner span keeps its fg/bold/italic
// but also paints its bg — which is required because lipgloss emits a full
// reset (\x1b[0m) at the end of each Render(), wiping any outer bg applied
// by a surrounding wrapper style.
func modalBg(p theme.Palette) lipgloss.Style {
	return lipgloss.NewStyle().Background(p.BgOverlay)
}

// paintLine wraps an arbitrary line (which may already contain inner styled
// spans) in the modal bg so trailing whitespace and plain separators also
// carry the bg. Inner spans must Inherit(modalBg(...)) themselves because
// their own reset sequences clobber the wrapper's bg for their own cells.
func paintLine(p theme.Palette, line string) string {
	return modalBg(p).Render(line)
}

// padBlock right-pads every line in a multi-line string to the block's
// widest visible width so lipgloss.Background fills the whole rectangle.
// Without this, a Modal style with Background leaves unfilled gaps on any
// line shorter than the longest one.
func padBlock(s string) string {
	lines := strings.Split(s, "\n")
	max := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > max {
			max = w
		}
	}
	for i, l := range lines {
		lines[i] = padRight(l, max)
	}
	return strings.Join(lines, "\n")
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
