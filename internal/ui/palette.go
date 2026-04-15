package ui

import (
	"sort"
	"strings"

	"git.mark1708.ru/me/tmh/internal/ui/theme"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	keys      Keys
	st        theme.Styles
	width     int
	height    int
	input     textinput.Model
	actions   []PaletteAction
	matches   []int // indexes into actions, fuzzy-ranked
	cursor    int
}

func newPalette(keys Keys, st theme.Styles, actions []PaletteAction) *paletteModel {
	in := textinput.New()
	in.Placeholder = "type to filter…"
	in.Focus()
	in.Prompt = "» "
	in.CharLimit = 80
	p := &paletteModel{keys: keys, st: st, input: in, actions: actions}
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
	maxRows := minInt(10, len(p.matches))
	var b strings.Builder
	b.WriteString(p.input.View() + "\n\n")
	for i := 0; i < maxRows; i++ {
		row := p.actions[p.matches[i]]
		marker := "  "
		if i == p.cursor {
			marker = "▸ "
		}
		line := marker + p.st.Title.Render(row.Title)
		if row.Subtitle != "" {
			line += "  " + p.st.Hint.Render(row.Subtitle)
		}
		if i == p.cursor {
			line = p.st.Selected.Render(padRight(line, 70))
		}
		b.WriteString(line + "\n")
	}
	if len(p.matches) == 0 {
		b.WriteString(p.st.Hint.Render("(no matches)\n"))
	}
	body := p.st.Modal.Render(padBlock(b.String()))
	return placeMiddle(p.width, p.height, body, p.st.Palette)
}
