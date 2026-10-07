// Package pages implements the fifteen resident tmh-next pages over the
// shared ui.Page contract. Pages project the read-only catalog, reconcile
// selection by stable ID, and emit typed intents only.
package pages

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/layout"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// base carries the shared page state and the contract plumbing.
type base struct {
	route ui.Route
	cat   *domain.Catalog
	loc   ui.Location

	width, height int
	styles        theme.Styles

	mode   ui.InputMode
	filter string

	cursor     int
	selectedID string

	notFound string // non-empty: explicit ID missing from catalog
	explicit bool   // selection came from an explicit Location primary

	region int // compact master/detail region: 0 master, 1 detail

	entered bool
}

func (b *base) Route() ui.Route { return b.route }

func (b *base) SetTheme(s theme.Styles) { b.styles = s }

func (b *base) SetSize(w, h int) { b.width, b.height = w, h }

func (b *base) InputMode() ui.InputMode { return b.mode }

// Enter stores the location and resets transient state.
func (b *base) enter(loc ui.Location) {
	b.loc = loc
	b.mode = ui.ModeNormal
	b.filter = ""
	if loc.Primary.ID != "" {
		b.selectedID = loc.Primary.ID
		b.cursor = 0
		b.explicit = true
	} else {
		b.explicit = false
	}
}

// clip truncates every line ANSI-aware to the page width so page output
// never overflows the terminal.
func (b *base) clip(s string) string {
	if b.width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipglossWidth(l) > b.width {
			lines[i] = truncate(l, b.width)
		}
	}
	return strings.Join(lines, "\n")
}

// setCatalog reconciles selection by stable ID: keep the same entity, else
// the item at the same stable-sorted ordinal, else the previous item. An
// empty selection over a populated catalog selects the first stable ID.
func (b *base) setCatalog(ids []string) {
	b.notFound = ""
	if b.selectedID == "" {
		if len(ids) > 0 {
			b.selectedID = ids[0]
			b.cursor = 0
		}
		return
	}
	idx := indexOf(ids, b.selectedID)
	if idx >= 0 {
		b.cursor = idx
		return
	}
	// An explicit Location ID that disappeared becomes Not found: it is
	// never silently replaced. UI selections fall back instead.
	if b.explicit {
		return
	}
	// vanished: same ordinal, else previous, else none
	switch {
	case b.cursor < len(ids):
		b.selectedID = ids[b.cursor]
	case b.cursor-1 >= 0 && b.cursor-1 < len(ids):
		b.cursor--
		b.selectedID = ids[b.cursor]
	default:
		b.cursor = 0
		b.selectedID = ""
	}
}

func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

// sortedIDs returns sorted string IDs.
func sortedIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

// layoutMode classifies the current size.
func (b *base) layoutMode() layout.Mode { return layout.Classify(b.width, b.height) }

// isWide reports wide layout.
func (b *base) isWide() bool { return b.layoutMode() == layout.ModeWide }

// bodyHeight is the usable inner height.
func (b *base) bodyHeight() int {
	h := b.height - 4 // page title + blank + footer padding
	if h < 1 {
		h = 1
	}
	return h
}

// titleBar renders the page title line with optional right-hand metadata.
func (b *base) titleBar(title string) string {
	right := []string{b.styles.Mock.Render("MOCK")}
	if b.mode == ui.ModeFilter {
		right = append(right, b.styles.Title.Render("filter: "+b.filter+"▏"))
	}
	if b.layoutMode() == layout.ModeCompact && b.region == 1 {
		right = append(right, b.styles.Dim.Render("[detail]"))
	}
	return b.styles.Title.Render(title) + "  " + strings.Join(right, "  ")
}

// notFoundView renders the in-app not-found state with disabled actions.
func (b *base) notFoundView(kind domain.ResourceKind, id string) string {
	return b.styles.Err.Render("Not found "+string(kind)+" "+id) + "\n" +
		b.styles.Dim.Render("the requested resource does not exist in this scenario — actions are disabled")
}

// emptyView renders an empty-collection state without phantom selection.
func (b *base) emptyView(what string) string {
	return b.styles.Dim.Render("No " + what + " in this scenario — nothing to select or act on")
}

// filterIds applies the substring filter over (id, label) pairs.
func (b *base) filterIds(pairs []idLabel) []idLabel {
	if b.filter == "" {
		return pairs
	}
	needle := strings.ToLower(b.filter)
	var out []idLabel
	for _, p := range pairs {
		if strings.Contains(strings.ToLower(p.label), needle) || strings.Contains(strings.ToLower(p.id), needle) {
			out = append(out, p)
		}
	}
	return out
}

type idLabel struct {
	id    string
	label string
}

// baseFilterKeys drives '/' filter entry, typing and Esc exit for list pages.
func (b *base) baseFilterKeys(msg tea.KeyPressMsg, rows []idLabel) (filtered []idLabel, handled bool) {
	filtered = b.filterIds(rows)
	switch b.mode {
	case ui.ModeFilter:
		switch msg.String() {
		case "esc":
			b.mode = ui.ModeNormal
			b.filter = ""
			filtered = rows
			return filtered, true
		case "backspace":
			if n := len(b.filter); n > 0 {
				b.filter = b.filter[:n-1]
			}
			filtered = b.filterIds(rows)
			b.clampCursor(filtered)
			return filtered, true
		case "enter":
			b.mode = ui.ModeNormal
			b.clampCursor(filtered)
			return filtered, true
		default:
			if msg.Text != "" {
				b.filter += msg.Text
				filtered = b.filterIds(rows)
				b.clampCursor(filtered)
			}
			return filtered, true
		}
	case ui.ModeNormal:
		if msg.String() == "/" {
			b.mode = ui.ModeFilter
			b.filter = ""
			filtered = rows
			return filtered, true
		}
	}
	return filtered, false
}

func (b *base) clampCursor(rows []idLabel) {
	if b.cursor >= len(rows) {
		b.cursor = len(rows) - 1
	}
	if b.cursor < 0 {
		b.cursor = 0
	}
	if len(rows) > 0 {
		b.selectedID = rows[b.cursor].id
	}
}

// cursorKeys moves the cursor over filtered rows.
func (b *base) cursorKeys(msg tea.KeyPressMsg, rows []idLabel) bool {
	switch msg.String() {
	case "down", "j":
		if b.cursor < len(rows)-1 {
			b.cursor++
			if len(rows) > 0 {
				b.selectedID = rows[b.cursor].id
			}
		}
		return true
	case "up", "k":
		if b.cursor > 0 {
			b.cursor--
			if len(rows) > 0 {
				b.selectedID = rows[b.cursor].id
			}
		}
		return true
	}
	return false
}

// row renders one list row with the cursor marker.
func (b *base) row(cursor int, text string) string {
	if cursor == b.cursor {
		return "› " + b.styles.Select(text)
	}
	return "  " + text
}

// regionTab toggles the compact master/detail region.
func (b *base) regionTab(msg tea.KeyPressMsg) bool {
	if msg.String() == "tab" {
		b.region = 1 - b.region
		return true
	}
	if msg.String() == "shift+tab" {
		b.region = 1 - b.region
		return true
	}
	return false
}

// status renders a status word with theme colors.
func (b *base) status(w string) string { return b.styles.Status(w) }

// help renders a key/desc pair binding list.
func pageHelp(pairs ...[2]string) []key.Binding {
	out := make([]key.Binding, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, key.NewBinding(key.WithKeys(p[0]), key.WithHelp(p[0], p[1])))
	}
	return out
}

// Intent helpers emit the shared ui message types (translated by the root).

func pushRoute(loc ui.Location, replace bool) tea.Cmd {
	return func() tea.Msg { return ui.PushRouteMsg{Loc: loc, Replace: replace} }
}

func openPrompt(title, placeholder string, submit func(string) tea.Cmd) tea.Cmd {
	return func() tea.Msg { return ui.OpenPromptMsg{Title: title, Placeholder: placeholder, Submit: submit} }
}

func confirm(title, detail string, a domain.Action) tea.Cmd {
	return func() tea.Msg { return ui.ConfirmActionMsg{Title: title, Detail: detail, Action: a} }
}

func toast(text, kind string) tea.Cmd {
	return func() tea.Msg { return ui.ShowToastMsg{Text: text, Kind: kind} }
}

func openSelector(title string, options []ui.SelectorOption) tea.Cmd {
	return func() tea.Msg { return ui.OpenSelectorMsg{Title: title, Options: options} }
}

func execute(a domain.Action) tea.Cmd {
	return func() tea.Msg { return ui.ExecuteActionMsg{Action: a} }
}

// labelOf returns a human label for a ref.
func labelOf(ref domain.ResourceRef) string { return string(ref.Kind) + ":" + ref.ID }

// selected returns the currently selected id.
func (b *base) selected() string { return b.selectedID }

func lipglossWidth(s string) int { return lipgloss.Width(s) }

func truncate(s string, width int) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
