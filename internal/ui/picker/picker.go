// Package picker renders a small interactive fuzzy-filter list of tmh
// session candidates (declared + live ad-hoc + discovered) so that bare
// `tmh` invocations behave like sesh — type-to-filter + Enter-to-attach
// — without forcing users through the full TUI dashboard.
//
// The full dashboard remains reachable via `tmh --dashboard`.
package picker

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Result is the single outcome returned after the picker exits.
type Result struct {
	// Target is the selected session name. Empty when the user aborted
	// (Esc / Ctrl-C) or pressed D to fall through to the dashboard.
	Target string
	// Dir, when non-empty, is the working directory tmh should use if
	// Target identifies a discovered (not-yet-created) session.
	Dir string
	// FallThroughToDashboard is true when the user explicitly requested
	// the full dashboard from the picker (`?` or `d`).
	FallThroughToDashboard bool
}

type item struct {
	name    string
	status  string // "attached", "live", "configured", "discovered"
	dir     string
	newSess bool
}

func (i item) Title() string       { return i.name }
func (i item) Description() string { return i.status + "  " + i.dir }
func (i item) FilterValue() string { return i.name }

type model struct {
	list   list.Model
	input  textinput.Model
	items  []item
	result Result
	quit   bool
}

// Run builds a fresh listing (+ discovered entries) and blocks until the
// user picks something or aborts. Returns Result.Target == "" for abort.
func Run(ctx context.Context, r tmux.Runner, cfg *config.Config, profile string) (Result, error) {
	listing, err := actions.BuildListing(ctx, r, cfg, profile)
	if err != nil {
		return Result{}, err
	}
	items := buildItems(listing)
	if len(items) == 0 {
		// No sessions and no discoverable candidates — fall through so
		// the dashboard can show empty-state onboarding.
		return Result{FallThroughToDashboard: true}, nil
	}

	listItems := make([]list.Item, len(items))
	for i := range items {
		listItems[i] = items[i]
	}

	delegate := list.NewDefaultDelegate()
	lm := list.New(listItems, delegate, 0, 0)
	lm.Title = "tmh — pick a session"
	lm.SetShowStatusBar(false)
	lm.SetFilteringEnabled(true)
	lm.SetShowHelp(false)

	input := textinput.New()
	input.CharLimit = 64
	input.Blur()

	m := &model{list: lm, input: input, items: items}
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	mm, _ := final.(*model)
	if mm == nil {
		return Result{}, nil
	}
	return mm.result, nil
}

func buildItems(l *actions.Listing) []item {
	if l == nil {
		return nil
	}
	var out []item
	for _, s := range l.Sessions {
		st := "configured"
		switch {
		case s.Attached:
			st = "attached"
		case s.Live:
			st = "live"
		case s.Discovered:
			st = "discovered"
		}
		out = append(out, item{
			name:    s.Name,
			status:  st,
			dir:     s.Dir,
			newSess: s.Discovered && !s.Live,
		})
	}
	return out
}

// Init satisfies tea.Model.
func (m *model) Init() tea.Cmd { return nil }

// Update handles keyboard input.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quit = true
			return m, tea.Quit
		case "?", "d":
			m.result.FallThroughToDashboard = true
			return m, tea.Quit
		case "enter":
			if sel, ok := m.list.SelectedItem().(item); ok {
				m.result.Target = sel.name
				m.result.Dir = sel.dir
			}
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the picker full-screen.
func (m *model) View() string {
	footer := lipgloss.NewStyle().
		Faint(true).
		Render("↑/↓ move · / filter · enter attach · d dashboard · esc cancel")
	return fmt.Sprintf("%s\n%s", m.list.View(), footer)
}

// IsEmpty reports whether the user aborted without choosing anything.
func (r Result) IsEmpty() bool { return r.Target == "" && !r.FallThroughToDashboard }
