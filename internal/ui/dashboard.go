package ui

import (
	"fmt"
	"strings"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// dashboardModel is a flattened tree (sessions + windows) with a preview
// pane on the right. j/k navigate the flat list; left/right collapse and
// expand sessions.
type dashboardModel struct {
	keys Keys
	st   theme.Styles

	width, height int

	rows       []dashboardRow
	collapsed  map[string]bool // session name → collapsed
	cursor     int
	listing    *actions.Listing
	driftIndex map[string]config.DriftStatus // "session" or "session/window" → status
}

type dashboardRow struct {
	IsSession bool
	Session   string
	Window    string // empty if IsSession
	Indent    int
	Status    config.DriftStatus
	Live      bool
	Attached  bool
	Layout    string
	WindowCnt int
}

func newDashboard(keys Keys, st theme.Styles) *dashboardModel {
	return &dashboardModel{keys: keys, st: st, collapsed: map[string]bool{}}
}

// Resize sets viewport dimensions.
func (d *dashboardModel) Resize(w, h int) { d.width, d.height = w, h }

// SetStyles updates the theme.
func (d *dashboardModel) SetStyles(st theme.Styles) { d.st = st }

// SetData replaces the rendered tree with fresh data.
func (d *dashboardModel) SetData(l *actions.Listing, drift []config.Drift) {
	d.listing = l
	d.driftIndex = make(map[string]config.DriftStatus, len(drift))
	for _, dr := range drift {
		d.driftIndex[dr.ConfigEntry] = dr.Status
	}
	d.rebuildRows()
	if d.cursor >= len(d.rows) {
		d.cursor = maxInt(0, len(d.rows)-1)
	}
}

func (d *dashboardModel) rebuildRows() {
	d.rows = nil
	if d.listing == nil {
		return
	}
	for _, s := range d.listing.Sessions {
		row := dashboardRow{
			IsSession: true,
			Session:   s.Name,
			Live:      s.Live,
			Attached:  s.Attached,
			WindowCnt: len(s.Windows),
		}
		// Session-level drift is rare; pick the worst window-level status as
		// a header summary.
		row.Status = worstStatus(d.driftIndex, s.Name, s.Windows)
		d.rows = append(d.rows, row)
		if d.collapsed[s.Name] {
			continue
		}
		for _, w := range s.Windows {
			entry := s.Name + "/" + w.Name
			d.rows = append(d.rows, dashboardRow{
				Session: s.Name, Window: w.Name, Indent: 1,
				Status:    d.driftIndex[entry],
				Live:      w.Live,
				Layout:    w.Layout,
				WindowCnt: w.Panes,
			})
		}
	}
}

func (d *dashboardModel) Update(msg tea.Msg) (*dashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, d.keys.Down):
			if d.cursor < len(d.rows)-1 {
				d.cursor++
			}
		case keyMatches(msg, d.keys.Up):
			if d.cursor > 0 {
				d.cursor--
			}
		case keyMatches(msg, d.keys.Top):
			d.cursor = 0
		case keyMatches(msg, d.keys.Bottom):
			d.cursor = maxInt(0, len(d.rows)-1)
		case keyMatches(msg, d.keys.PgDown):
			d.cursor = minInt(len(d.rows)-1, d.cursor+10)
		case keyMatches(msg, d.keys.PgUp):
			d.cursor = maxInt(0, d.cursor-10)
		case keyMatches(msg, d.keys.Left):
			if r := d.currentRow(); r != nil && r.IsSession {
				d.collapsed[r.Session] = true
				d.rebuildRows()
			}
		case keyMatches(msg, d.keys.Right):
			if r := d.currentRow(); r != nil && r.IsSession {
				delete(d.collapsed, r.Session)
				d.rebuildRows()
			}
		}
	}
	return d, nil
}

func (d *dashboardModel) currentRow() *dashboardRow {
	if d.cursor < 0 || d.cursor >= len(d.rows) {
		return nil
	}
	return &d.rows[d.cursor]
}

// SelectedTarget returns "session" or "session:window" depending on cursor.
func (d *dashboardModel) SelectedTarget() string {
	r := d.currentRow()
	if r == nil {
		return ""
	}
	if r.IsSession {
		return r.Session
	}
	return r.Session + ":" + r.Window
}

func (d *dashboardModel) View() string {
	if d.listing == nil {
		return d.st.Hint.Render("loading…")
	}
	if len(d.rows) == 0 {
		return d.st.Hint.Render("no sessions yet — press n to create one")
	}

	// Side-by-side: tree (45%) | detail (rest)
	treeWidth := d.width * 45 / 100
	if treeWidth < 30 {
		treeWidth = d.width
	}
	tree := d.renderTree(treeWidth)
	if treeWidth == d.width {
		return tree
	}
	detail := d.renderDetail(d.width - treeWidth - 4)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		d.st.PanelFocus.Width(treeWidth).Render(tree),
		d.st.Panel.Width(d.width-treeWidth-4).Render(detail),
	)
}

func (d *dashboardModel) renderTree(width int) string {
	maxRows := d.height - 4
	if maxRows < 5 {
		maxRows = 5
	}
	start := 0
	if d.cursor >= maxRows {
		start = d.cursor - maxRows + 1
	}
	end := minInt(len(d.rows), start+maxRows)

	var b strings.Builder
	for i := start; i < end; i++ {
		row := d.rows[i]
		line := d.formatRow(row, width)
		if i == d.cursor {
			line = d.st.Selected.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (d *dashboardModel) formatRow(r dashboardRow, width int) string {
	indent := strings.Repeat("  ", r.Indent)
	if r.IsSession {
		marker := " "
		switch {
		case r.Attached:
			marker = "*"
		case r.Live:
			marker = "●"
		}
		head := fmt.Sprintf("%s%s %-12s %dw", indent, marker, truncate(r.Session, 12), r.WindowCnt)
		status := d.statusLabel(r.Status)
		return padRight(head, width-lipgloss.Width(status)-1) + " " + status
	}
	prefix := indent + "├─ "
	statusGlyph := " "
	if r.Live {
		statusGlyph = "●"
	}
	main := fmt.Sprintf("%s%s %-14s", prefix, statusGlyph, truncate(r.Window, 14))
	right := d.statusLabel(r.Status)
	return padRight(main, width-lipgloss.Width(right)-1) + " " + right
}

func (d *dashboardModel) statusLabel(s config.DriftStatus) string {
	switch s {
	case config.StatusOK:
		return d.st.StatusOK.Render("ok")
	case config.StatusDrift:
		return d.st.StatusDrift.Render("drift")
	case config.StatusNew:
		return d.st.StatusNew.Render("new")
	case config.StatusGone:
		return d.st.StatusGone.Render("gone")
	}
	return ""
}

func (d *dashboardModel) renderDetail(width int) string {
	r := d.currentRow()
	if r == nil {
		return d.st.Hint.Render("nothing selected")
	}
	var b strings.Builder
	if r.IsSession {
		b.WriteString(d.st.Title.Render("session: "+r.Session) + "\n\n")
		fmt.Fprintf(&b, "live      %v\n", r.Live)
		fmt.Fprintf(&b, "attached  %v\n", r.Attached)
		fmt.Fprintf(&b, "windows   %d\n", r.WindowCnt)
		fmt.Fprintf(&b, "status    %s\n", d.statusLabel(r.Status))
	} else {
		b.WriteString(d.st.Title.Render(fmt.Sprintf("window: %s/%s", r.Session, r.Window)) + "\n\n")
		fmt.Fprintf(&b, "live    %v\n", r.Live)
		if r.Layout != "" {
			fmt.Fprintf(&b, "layout  %s\n", r.Layout)
		}
		fmt.Fprintf(&b, "panes   %d\n", r.WindowCnt)
		fmt.Fprintf(&b, "status  %s\n", d.statusLabel(r.Status))
		b.WriteString("\n")
		b.WriteString(d.st.Hint.Render("press a / enter to attach this window"))
	}
	_ = width
	return b.String()
}

// --- helpers ---

func worstStatus(idx map[string]config.DriftStatus, sess string, windows []actions.ListedWindow) config.DriftStatus {
	worst := config.StatusOK
	rank := func(s config.DriftStatus) int {
		switch s {
		case config.StatusGone:
			return 4
		case config.StatusDrift:
			return 3
		case config.StatusNew:
			return 2
		case config.StatusOK:
			return 1
		}
		return 0
	}
	for _, w := range windows {
		s := idx[sess+"/"+w.Name]
		if rank(s) > rank(worst) {
			worst = s
		}
	}
	return worst
}
