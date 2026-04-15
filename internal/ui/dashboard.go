package ui

import (
	"fmt"
	"strings"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
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
	str  UIStrings

	width, height int

	rows       []dashboardRow
	collapsed  map[string]bool // session name → collapsed
	cursor     int
	listing    *actions.Listing
	driftIndex map[string]config.DriftStatus // "session" or "session/window" → status

	preview       string // cached capture-pane content for current row
	previewTarget string // row key the cached preview belongs to
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

func newDashboard(keys Keys, st theme.Styles, str UIStrings) *dashboardModel {
	return &dashboardModel{keys: keys, st: st, str: str, collapsed: map[string]bool{}}
}

// Resize sets viewport dimensions.
func (d *dashboardModel) Resize(w, h int) { d.width, d.height = w, h }

// SetStyles updates the theme.
func (d *dashboardModel) SetStyles(st theme.Styles) { d.st = st }

// SetStrings swaps the translated string bundle (used after a language change).
func (d *dashboardModel) SetStrings(str UIStrings) { d.str = str }

// SetPreview updates the cached preview text for the row key it was
// captured against. Noop if the selection has moved in the meantime, so an
// in-flight fetch for an old row never overwrites a fresh one.
func (d *dashboardModel) SetPreview(target, text string) {
	if target != d.currentTargetKey() {
		return
	}
	d.preview = text
	d.previewTarget = target
}

// currentTargetKey returns a stable identifier for the current selection
// used as preview cache key. Empty when nothing is selected.
func (d *dashboardModel) currentTargetKey() string {
	r := d.currentRow()
	if r == nil {
		return ""
	}
	if r.IsSession {
		return r.Session
	}
	return r.Session + ":" + r.Window
}

// PreviewStale reports true when the current selection lacks a matching
// capture in the cache; the root model triggers an async fetch in that case.
func (d *dashboardModel) PreviewStale() (target string, stale bool) {
	target = d.currentTargetKey()
	if target == "" {
		return "", false
	}
	return target, d.previewTarget != target
}

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
		return d.st.Hint.Render(d.str.Loading)
	}
	if len(d.rows) == 0 {
		return d.st.Hint.Render(d.str.NoSessions)
	}

	// Side-by-side: tree (45%) | detail (rest). Each panel adds a 1-cell
	// border + 1-cell horizontal padding on both sides, so subtract 4 from
	// the rendered width before formatting rows; otherwise content reaches
	// the panel's inner edge and lipgloss wraps it onto a second line.
	const panelChrome = 4
	treeOuter := d.width * 45 / 100
	if treeOuter < 30 {
		treeOuter = d.width
	}
	treeInner := maxInt(10, treeOuter-panelChrome)
	tree := d.renderTree(treeInner)
	if treeOuter >= d.width {
		return d.st.PanelFocus.Width(d.width).Render(tree)
	}
	detailOuter := d.width - treeOuter
	detailInner := maxInt(10, detailOuter-panelChrome)
	detail := d.renderDetail(detailInner)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		d.st.PanelFocus.Width(treeOuter).Render(tree),
		d.st.Panel.Width(detailOuter).Render(detail),
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
		return d.st.Hint.Render(d.str.NothingSelected)
	}
	var b strings.Builder
	if r.IsSession {
		title := i18n.Tf("tui.dashboard.session_label", map[string]any{"name": r.Session})
		b.WriteString(d.st.Title.Render(title) + "\n\n")
		fmt.Fprintf(&b, "%-10s%s\n", i18n.T("tui.dashboard.field.live"), d.boolGlyph(r.Live))
		fmt.Fprintf(&b, "%-10s%s\n", i18n.T("tui.dashboard.field.attached"), d.boolGlyph(r.Attached))
		fmt.Fprintf(&b, "%-10s%d\n", i18n.T("tui.dashboard.field.windows"), r.WindowCnt)
		fmt.Fprintf(&b, "%-10s%s\n", i18n.T("tui.dashboard.field.status"), d.statusLabel(r.Status))
	} else {
		title := i18n.Tf("tui.dashboard.window_label", map[string]any{"session": r.Session, "window": r.Window})
		b.WriteString(d.st.Title.Render(title) + "\n\n")
		fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.live"), d.boolGlyph(r.Live))
		if r.Layout != "" {
			fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.layout"), r.Layout)
		}
		fmt.Fprintf(&b, "%-8s%d\n", i18n.T("tui.dashboard.field.panes"), r.WindowCnt)
		fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.status"), d.statusLabel(r.Status))
		b.WriteString("\n")
		b.WriteString(d.st.Hint.Render(d.str.AttachHint))
	}
	if d.preview != "" {
		b.WriteString("\n\n")
		b.WriteString(d.st.Subtitle.Render(i18n.T("tui.dashboard.preview_label")) + "\n")
		b.WriteString(d.renderPreview(width))
	}
	_ = width
	return b.String()
}

// boolGlyph renders true/false as a themed check/cross so the detail panel
// reads at a glance.
func (d *dashboardModel) boolGlyph(v bool) string {
	if v {
		return d.st.StatusOK.Render("✓")
	}
	return d.st.StatusGone.Render("✗")
}

// renderPreview paints the cached tmux capture-pane output for the current
// row, truncated to fit the detail panel. Empty when no capture exists yet
// (it's fetched asynchronously by loadPreviewCmd).
func (d *dashboardModel) renderPreview(width int) string {
	if d.preview == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(d.preview, "\n"), "\n")
	maxLines := d.previewRows()
	if maxLines <= 0 {
		return ""
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	w := maxInt(10, width-2)
	for i, l := range lines {
		lines[i] = d.st.Hint.Render(truncate(l, w))
	}
	return strings.Join(lines, "\n")
}

// previewRows returns how many rows are left below the detail fields for
// the preview body.
func (d *dashboardModel) previewRows() int {
	// detail header + fields take ~8 rows; reserve a couple for preview label
	// and margins.
	budget := d.height - 12
	if budget > 15 {
		budget = 15
	}
	if budget < 0 {
		return 0
	}
	return budget
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
