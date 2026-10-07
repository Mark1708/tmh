// Package components implements the root-owned overlay widgets: the
// bubbles-list selector, the text prompt, the confirm dialog and toasts.
package components

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// selectorItem adapts SelectorOption to the bubbles list.
type selectorItem struct {
	opt ui.SelectorOption
}

func (i selectorItem) FilterValue() string { return i.opt.Title + " " + i.opt.Desc }

type selectorDelegate struct {
	styles theme.Styles
}

func (d selectorDelegate) Height() int  { return 1 }
func (d selectorDelegate) Spacing() int { return 0 }

func (d selectorDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d selectorDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	opt := item.(selectorItem).opt
	cursor := " "
	title := opt.Title
	if index == m.Index() {
		cursor = "›"
		title = d.styles.Select(opt.Title)
	} else {
		title = d.styles.Base.Render(opt.Title)
	}
	line := fmt.Sprintf("%s %s", cursor, title)
	if opt.Shortcut != "" {
		line += "  " + d.styles.Dim.Render("("+opt.Shortcut+")")
	}
	if opt.Disabled {
		reason := opt.Reason
		if reason == "" {
			reason = "unavailable"
		}
		line += "  " + d.styles.Err.Render("✗ "+reason)
	}
	if len(opt.Desc) > 0 {
		line += "  " + d.styles.Dim.Render(opt.Desc)
	}
	fmt.Fprintln(w, line)
}

// Selector is a filterable one-choice overlay over the Bubbles v2 list.
type Selector struct {
	list      list.Model
	options   []ui.SelectorOption
	title     string
	styles    theme.Styles
	width     int
	chosen    int // index at Enter, -1 until chosen
	cancelled bool
}

// NewSelector builds a selector overlay sized for the given body area.
func NewSelector(title string, opts []ui.SelectorOption, width, height int, styles theme.Styles) *Selector {
	items := make([]list.Item, 0, len(opts))
	for _, o := range opts {
		items = append(items, selectorItem{opt: o})
	}
	// Keep overlays dense and bounded. The old two-lines-per-item budget made
	// palettes almost terminal-height on wide screens, obscuring context and
	// forcing the eye to scan large empty gaps.
	visibleRows := len(opts)
	if visibleRows > 18 {
		visibleRows = 18
	}
	if visibleRows < 4 {
		visibleRows = 4
	}
	visible := visibleRows + 2 // filter row + pagination breathing room
	if bound := height - 10; bound >= 6 && visible > bound {
		visible = bound
	}
	l := list.New(items, selectorDelegate{styles: styles}, width, visible)
	l.SetShowHelp(false)
	l.SetShowStatusBar(true)
	l.SetShowTitle(false)
	l.SetShowPagination(len(opts) > visibleRows)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	return &Selector{list: l, options: opts, title: title, styles: styles, width: width, chosen: -1}
}

// SetTheme applies a new style set.
func (s *Selector) SetTheme(styles theme.Styles) {
	s.styles = styles
	s.list.SetDelegate(selectorDelegate{styles: styles})
}

// Update drives navigation, filtering and selection. Printable keys
// auto-enter filtering so palettes narrow as you type.
func (s *Selector) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		if s.list.FilterState() == list.Unfiltered && k.Text != "" &&
			k.String() != "enter" && k.String() != "esc" && k.String() != "space" &&
			k.String() != "j" && k.String() != "k" && k.String() != "/" {
			var preCmd tea.Cmd
			s.list, preCmd = s.list.Update(tea.KeyPressMsg{Code: '/'})
			var runCmd tea.Cmd
			s.list, runCmd = s.list.Update(k)
			return tea.Batch(preCmd, runCmd)
		}
		switch k.String() {
		case "esc":
			if s.list.FilterState() == list.Filtering || s.list.FilterState() == list.FilterApplied {
				var cmd tea.Cmd
				s.list, cmd = s.list.Update(msg)
				return cmd
			}
			s.cancelled = true
			return nil
		case "enter":
			vis := s.list.VisibleItems()
			if len(vis) == 0 {
				return nil
			}
			idx := s.list.Index()
			if idx < 0 || idx >= len(vis) {
				return nil
			}
			// Map visible position back to the option list by title identity.
			chosenOpt := vis[idx].(selectorItem).opt
			for i := range s.options {
				if s.options[i].ID == chosenOpt.ID {
					if s.options[i].Disabled {
						return nil
					}
					s.chosen = i
					return nil
				}
			}
			return nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

// Choice returns the chosen option, if any.
func (s *Selector) Choice() (ui.SelectorOption, bool) {
	if s.chosen < 0 || s.chosen >= len(s.options) {
		return ui.SelectorOption{}, false
	}
	return s.options[s.chosen], true
}

// Cancelled reports whether the user dismissed the selector.
func (s *Selector) Cancelled() bool { return s.cancelled }

// Done reports whether the overlay has concluded.
func (s *Selector) Done() bool { return s.cancelled || s.chosen >= 0 }

// View renders the selector as one compact bordered panel with header and rows.
func (s *Selector) View() string {
	header := s.styles.Title.Render(s.title)
	hint := "type to filter  ·  ↑/↓ move  ·  enter select  ·  esc close"
	if s.list.FilterState() == list.Filtering {
		hint = "filtering  ·  enter apply  ·  esc clear"
	}
	body := s.list.View()
	content := header + "\n" + s.styles.Dim.Render(hint) + "\n\n" + body
	return s.styles.Overlay.Render(
		theme.PaintBackground(content, s.styles.Palette.Mantle),
	)
}

// Unused binding guard keeps key import referenced for future per-selector keys.
var _ = key.NewBinding
