package ui

import (
	"fmt"
	"strings"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
)

// diffModel is the full-screen drift list (`D` from dashboard).
type diffModel struct {
	keys   Keys
	st     theme.Styles
	width  int
	height int
	items  []config.Drift
	cursor int
}

func newDiffScreen(keys Keys, st theme.Styles, items []config.Drift) *diffModel {
	return &diffModel{keys: keys, st: st, items: items}
}

func (d *diffModel) Resize(w, h int) { d.width, d.height = w, h }

func (d *diffModel) Update(msg tea.Msg) (*diffModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case keyMatches(k, d.keys.Down):
			if d.cursor < len(d.items)-1 {
				d.cursor++
			}
		case keyMatches(k, d.keys.Up):
			if d.cursor > 0 {
				d.cursor--
			}
		}
	}
	return d, nil
}

func (d *diffModel) View() string {
	if len(d.items) == 0 {
		return d.st.Title.Render("no drift") + "\n\n" + d.st.Hint.Render("press esc to return")
	}
	var b strings.Builder
	b.WriteString(d.st.Title.Render(fmt.Sprintf("drift · %d entries", len(d.items))) + "\n\n")
	maxRows := d.height - 6
	if maxRows < 5 {
		maxRows = 5
	}
	start := 0
	if d.cursor >= maxRows {
		start = d.cursor - maxRows + 1
	}
	end := minInt(len(d.items), start+maxRows)
	for i := start; i < end; i++ {
		it := d.items[i]
		status := d.statusBadge(it.Status)
		entry := fmt.Sprintf("%-6s  %-30s  %s", status, it.ConfigEntry, it.Reason)
		if i == d.cursor {
			entry = d.st.Selected.Render(padRight(entry, d.width-2))
		}
		b.WriteString(entry + "\n")
	}
	if it := d.items[d.cursor]; it.ConfigDir != "" || it.LiveDir != "" {
		b.WriteString("\n")
		if it.ConfigDir != "" {
			b.WriteString(d.st.Hint.Render("config: ") + it.ConfigDir + "\n")
		}
		if it.LiveDir != "" {
			b.WriteString(d.st.Hint.Render("live:   ") + it.LiveDir + "\n")
		}
	}
	b.WriteString("\n" + d.st.Hint.Render("esc back · ↑/↓ navigate · s sync push (use CLI for now)"))
	return b.String()
}

func (d *diffModel) statusBadge(s config.DriftStatus) string {
	switch s {
	case config.StatusOK:
		return d.st.StatusOK.Render(string(s))
	case config.StatusDrift:
		return d.st.StatusDrift.Render(string(s))
	case config.StatusNew:
		return d.st.StatusNew.Render(string(s))
	case config.StatusGone:
		return d.st.StatusGone.Render(string(s))
	}
	return string(s)
}
