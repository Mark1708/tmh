package ui

import (
	"fmt"
	"sort"
	"strings"

	"git.mark1708.ru/me/tmh/internal/ui/theme"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// PaletteAction is one row in the command palette. Run is invoked when the
// user presses enter on this entry.
type PaletteAction struct {
	Title    string
	Subtitle string
	Run      func() tea.Cmd
}

// paletteModel is the fuzzy-search command palette overlay (`:` or ^P).
type paletteModel struct {
	keys    Keys
	st      theme.Styles
	str     UIStrings
	width   int
	height  int
	input   textinput.Model
	actions []PaletteAction
	matches []int // indexes into actions, fuzzy-ranked
	cursor  int
}

func newPalette(keys Keys, st theme.Styles, str UIStrings, actions []PaletteAction) *paletteModel {
	in := textinput.New()
	in.Placeholder = str.Palette.Placeholder
	in.Focus()
	in.Prompt = "» "
	in.CharLimit = 80
	p := &paletteModel{keys: keys, st: st, str: str, input: in, actions: actions}
	p.refresh()
	return p
}

func (p *paletteModel) Resize(w, h int) {
	p.width, p.height = w, h
	p.input.Width = minInt(w-12, 60)
}

func (p *paletteModel) Update(msg tea.Msg) (*paletteModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, p.keys.Down):
			if p.cursor < len(p.matches)-1 {
				p.cursor++
			}
			return p, nil
		case keyMatches(msg, p.keys.Up):
			if p.cursor > 0 {
				p.cursor--
			}
			return p, nil
		case keyMatches(msg, p.keys.Enter):
			if p.cursor >= 0 && p.cursor < len(p.matches) {
				idx := p.matches[p.cursor]
				if act := p.actions[idx].Run; act != nil {
					return p, act()
				}
			}
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.refresh()
	if p.cursor >= len(p.matches) {
		p.cursor = maxInt(0, len(p.matches)-1)
	}
	return p, cmd
}

func (p *paletteModel) refresh() {
	q := strings.TrimSpace(p.input.Value())
	if q == "" {
		p.matches = make([]int, len(p.actions))
		for i := range p.actions {
			p.matches[i] = i
		}
		sort.SliceStable(p.matches, func(i, j int) bool {
			return p.actions[p.matches[i]].Title < p.actions[p.matches[j]].Title
		})
		return
	}
	titles := make([]string, len(p.actions))
	for i, a := range p.actions {
		titles[i] = a.Title
	}
	results := fuzzy.Find(q, titles)
	p.matches = make([]int, 0, len(results))
	for _, r := range results {
		p.matches = append(p.matches, r.Index)
	}
}

func (p *paletteModel) View() string {
	// Page size derived from the modal height: reserve 6 rows for border,
	// padding, input row, and a trailing spacer so the cursor can't outrun
	// the viewport at the bottom.
	viewportRows := maxInt(5, minInt(12, p.height-10))
	total := len(p.matches)
	start := 0
	if p.cursor >= viewportRows {
		start = p.cursor - viewportRows + 1
	}
	end := minInt(total, start+viewportRows)

	mb := modalBg(p.st.Palette)
	title := p.st.Title.Inherit(mb)
	hint := p.st.Hint.Inherit(mb)
	width := minInt(80, p.width-8)
	if width < 40 {
		width = 40
	}

	var b strings.Builder
	b.WriteString(paintLine(p.st.Palette, p.input.View()) + "\n\n")
	for i := start; i < end; i++ {
		row := p.actions[p.matches[i]]
		marker := "  "
		if i == p.cursor {
			marker = "▸ "
		}
		// Compose line with inner styles that inherit modal bg, then pad the
		// whole thing and wrap once more so plain gaps (marker, separator,
		// trailing) also carry the bg.
		line := mb.Render(marker) + title.Render(row.Title)
		if row.Subtitle != "" {
			line += mb.Render("  ") + hint.Render(row.Subtitle)
		}
		line = padRight(line, width)
		if i == p.cursor {
			line = p.st.Selected.Render(line)
		} else {
			line = paintLine(p.st.Palette, line)
		}
		b.WriteString(line + "\n")
	}
	if total == 0 {
		b.WriteString(paintLine(p.st.Palette, hint.Render(p.str.Palette.NoMatches)) + "\n")
	}
	// Scroll indicator: "N/M" aligned right so the user sees position at a
	// glance when the list overflows.
	if total > viewportRows {
		scroll := fmt.Sprintf("%d/%d", p.cursor+1, total)
		b.WriteString("\n" + paintLine(p.st.Palette, hint.Render(padRight("", width-lipgloss.Width(scroll))+scroll)))
	}
	body := p.st.Modal.Render(padBlock(b.String()))
	return placeMiddle(p.width, p.height, body, p.st.Palette)
}
