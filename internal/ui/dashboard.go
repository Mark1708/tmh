package ui

import (
	"fmt"
	"strings"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/ui/pane"
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
	cursor     int             // index into rows when not filtered
	listing    *actions.Listing
	driftIndex map[string]config.DriftStatus // "session" or "session/window" → status

	// process visibility (Variant 4)
	paneProvider *pane.Provider

	// inline filter (Variant 6)
	filterText   string // current filter query
	filterActive bool   // filter input has focus
	filtered     []int  // display→original row indices; nil = no filter active
	filterCursor int    // cursor within filtered slice

	preview       string // cached capture-pane content for current row
	previewTarget string // row key the cached preview belongs to

	// pane cycling (Variant 10): preview is bound to a specific pane index
	previewPaneIdx     int    // pane being previewed within the current window (0-based)
	previewDefaultPane int    // reset value when row changes; set from Display settings
	lastPreviewRowID   string // detects row change so previewPaneIdx can reset
}

type dashboardRow struct {
	IsSession bool
	Session   string
	Window    string // window name; empty if IsSession
	WindowIdx int    // numeric window index used for pane cache lookups
	Indent    int
	Status    config.DriftStatus
	Live      bool
	Attached  bool
	Layout    string
	WindowCnt int
	// Commands holds the live process names for this row (Variant 4).
	// For sessions: all unique non-shell commands across all panes.
	// For windows: first non-shell command visible in the pane cache.
	Commands []string
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

// SetPaneProvider wires in the pane command cache for process visibility.
func (d *dashboardModel) SetPaneProvider(p *pane.Provider) { d.paneProvider = p }

// SetPreviewDefaultPane configures which pane index is selected when the
// cursor moves to a new window row. 0 is the default (first pane).
func (d *dashboardModel) SetPreviewDefaultPane(idx int) { d.previewDefaultPane = idx }

// maybeResetPreviewPane resets previewPaneIdx to the default when the
// selected row has changed since the last call.
func (d *dashboardModel) maybeResetPreviewPane() {
	id := d.currentTargetKey()
	if id != d.lastPreviewRowID {
		d.lastPreviewRowID = id
		d.previewPaneIdx = d.previewDefaultPane
		d.preview = ""
		d.previewTarget = ""
	}
}

// currentPreviewTarget returns the tmux target string for the preview pane.
// For session rows it uses the session name (tmux picks the active window).
// For window rows it encodes the specific pane: "session:windowIdx.paneIdx".
func (d *dashboardModel) currentPreviewTarget() string {
	r := d.currentRow()
	if r == nil {
		return ""
	}
	if r.IsSession {
		return r.Session
	}
	return fmt.Sprintf("%s:%d.%d", r.Session, r.WindowIdx, d.previewPaneIdx)
}

// SetPreview updates the cached preview text. Only accepted when the target
// matches the currently active preview target (pane-aware).
func (d *dashboardModel) SetPreview(target, text string) {
	if target != d.currentPreviewTarget() {
		return
	}
	d.preview = text
	d.previewTarget = target
}

// currentTargetKey returns a stable row identifier for cursor/selection
// purposes. Does NOT include the pane index — use currentPreviewTarget for
// that.
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

// PreviewStale reports true when the cached preview doesn't match the current
// pane target. Resets pane index when the selected row has changed.
func (d *dashboardModel) PreviewStale() (target string, stale bool) {
	d.maybeResetPreviewPane()
	target = d.currentPreviewTarget()
	if target == "" {
		return "", false
	}
	return target, d.previewTarget != target
}

// FilterActive reports whether the filter text-input has focus, used by the
// auto-refresh cadence to pause fetches during input.
func (d *dashboardModel) FilterActive() bool { return d.filterActive }

// SetData replaces the rendered tree with fresh data.
func (d *dashboardModel) SetData(l *actions.Listing, drift []config.Drift) {
	// Preserve selected ID across rebuilds.
	selectedID := d.currentTargetKey()

	d.listing = l
	d.driftIndex = make(map[string]config.DriftStatus, len(drift))
	for _, dr := range drift {
		d.driftIndex[dr.ConfigEntry] = dr.Status
	}
	d.rebuildRows()

	// Restore cursor by ID.
	d.restoreCursorByID(selectedID)
}

// InvalidatePanes clears process cache and rebuilds rows so process labels
// refresh after a kill/attach/sync action.
func (d *dashboardModel) InvalidatePanes() {
	if d.paneProvider != nil {
		d.paneProvider.Invalidate()
	}
}

// UpdateCommands refreshes the Commands field on existing rows from the pane
// cache without rebuilding the full row list.
func (d *dashboardModel) UpdateCommands() {
	if d.paneProvider == nil {
		return
	}
	for i := range d.rows {
		d.rows[i].Commands = d.commandsFor(&d.rows[i])
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
		row.Status = worstStatus(d.driftIndex, s.Name, s.Windows)
		row.Commands = d.commandsFor(&row)
		d.rows = append(d.rows, row)
		if d.collapsed[s.Name] {
			continue
		}
		for _, w := range s.Windows {
			entry := s.Name + "/" + w.Name
			wr := dashboardRow{
				Session:   s.Name,
				Window:    w.Name,
				WindowIdx: w.Index,
				Indent:    1,
				Status:    d.driftIndex[entry],
				Live:      w.Live,
				Layout:    w.Layout,
				WindowCnt: w.Panes,
			}
			wr.Commands = d.commandsFor(&wr)
			d.rows = append(d.rows, wr)
		}
	}
	// Rebuild filtered view if a filter is active.
	if d.filterText != "" {
		d.applyFilter()
	}
}

// commandsFor returns the process list for a row from the pane cache.
func (d *dashboardModel) commandsFor(r *dashboardRow) []string {
	if d.paneProvider == nil {
		return nil
	}
	if r.IsSession {
		return d.paneProvider.CommandsForSession(r.Session)
	}
	return d.paneProvider.CommandsForWindow(r.Session, r.WindowIdx)
}

// applyFilter rebuilds the filtered index from filterText.
func (d *dashboardModel) applyFilter() {
	query := strings.ToLower(d.filterText)
	if query == "" {
		d.filtered = nil
		return
	}
	d.filtered = d.filtered[:0]
	for i, r := range d.rows {
		if rowMatchesFilter(r, query) {
			d.filtered = append(d.filtered, i)
		}
	}
}

// rowMatchesFilter reports whether a row matches the query string.
func rowMatchesFilter(r dashboardRow, query string) bool {
	if strings.Contains(strings.ToLower(r.Session), query) {
		return true
	}
	if strings.Contains(strings.ToLower(r.Window), query) {
		return true
	}
	for _, cmd := range r.Commands {
		if strings.Contains(strings.ToLower(cmd), query) {
			return true
		}
	}
	return false
}

// restoreCursorByID re-positions the cursor on the row with the given ID
// after a data rebuild. Falls back to clamping.
func (d *dashboardModel) restoreCursorByID(id string) {
	if id == "" {
		d.clampCursor()
		return
	}
	eff := d.effectiveRows()
	for i, idx := range eff {
		r := d.rows[idx]
		var rowID string
		if r.IsSession {
			rowID = r.Session
		} else {
			rowID = r.Session + ":" + r.Window
		}
		if rowID == id {
			d.setCursor(i)
			return
		}
	}
	d.clampCursor()
}

func (d *dashboardModel) Update(msg tea.Msg) (*dashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Filter input mode: typed characters update the filter.
		if d.filterActive {
			return d.updateFilterInput(msg)
		}
		// Normal navigation.
		switch {
		case keyMatches(msg, d.keys.Down):
			d.moveCursor(+1)
		case keyMatches(msg, d.keys.Up):
			d.moveCursor(-1)
		case keyMatches(msg, d.keys.Top):
			d.setCursor(0)
		case keyMatches(msg, d.keys.Bottom):
			eff := d.effectiveRows()
			d.setCursor(maxInt(0, len(eff)-1))
		case keyMatches(msg, d.keys.PgDown):
			d.moveCursor(+10)
		case keyMatches(msg, d.keys.PgUp):
			d.moveCursor(-10)
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
		case keyMatches(msg, d.keys.Search):
			// `/` activates filter input.
			d.filterActive = true
			if d.filtered == nil {
				d.filtered = make([]int, 0, len(d.rows))
			}
		case keyMatches(msg, d.keys.Tab):
			// Tab cycles to the next pane within the current window (Variant 10).
			if r := d.currentRow(); r != nil && !r.IsSession && r.WindowCnt > 0 {
				d.previewPaneIdx = (d.previewPaneIdx + 1) % r.WindowCnt
				d.preview = ""
				d.previewTarget = ""
			}
		case msg.Type == tea.KeyShiftTab:
			// Shift+Tab cycles to the previous pane within the current window.
			if r := d.currentRow(); r != nil && !r.IsSession && r.WindowCnt > 0 {
				d.previewPaneIdx = (d.previewPaneIdx - 1 + r.WindowCnt) % r.WindowCnt
				d.preview = ""
				d.previewTarget = ""
			}
		case keyMatches(msg, d.keys.Esc):
			// Esc clears the filter entirely.
			if d.filterText != "" || d.filtered != nil {
				savedID := d.currentTargetKey()
				d.filterText = ""
				d.filtered = nil
				d.filterActive = false
				d.restoreCursorByID(savedID)
			}
		}
	}
	return d, nil
}

// updateFilterInput handles key presses when filter text-input is focused.
func (d *dashboardModel) updateFilterInput(k tea.KeyMsg) (*dashboardModel, tea.Cmd) {
	savedID := d.currentTargetKey()
	switch k.String() {
	case "esc":
		// Clear filter and return to normal navigation.
		d.filterText = ""
		d.filtered = nil
		d.filterActive = false
		d.restoreCursorByID(savedID)
	case "enter":
		// Blur the input but keep the filtered view.
		d.filterActive = false
	case "backspace", "ctrl+h":
		if len(d.filterText) > 0 {
			runes := []rune(d.filterText)
			d.filterText = string(runes[:len(runes)-1])
			d.applyFilter()
			d.restoreCursorByID(savedID)
		}
	default:
		// Accept printable characters.
		if k.Type == tea.KeyRunes && len(k.Runes) > 0 {
			d.filterText += string(k.Runes)
			d.applyFilter()
			d.restoreCursorByID(savedID)
		}
	}
	return d, nil
}

// ── cursor helpers ─────────────────────────────────────────────────────────

// effectiveRows returns indices of displayable rows. When a filter is active,
// returns filtered; otherwise generates [0, len(rows)).
func (d *dashboardModel) effectiveRows() []int {
	if d.filtered != nil {
		return d.filtered
	}
	all := make([]int, len(d.rows))
	for i := range all {
		all[i] = i
	}
	return all
}

func (d *dashboardModel) effectiveCursor() int {
	if d.filtered != nil {
		return d.filterCursor
	}
	return d.cursor
}

func (d *dashboardModel) setCursor(displayIdx int) {
	eff := d.effectiveRows()
	if len(eff) == 0 {
		d.filterCursor = 0
		d.cursor = 0
		return
	}
	if displayIdx < 0 {
		displayIdx = 0
	}
	if displayIdx >= len(eff) {
		displayIdx = len(eff) - 1
	}
	if d.filtered != nil {
		d.filterCursor = displayIdx
	} else {
		d.cursor = displayIdx
	}
}

func (d *dashboardModel) moveCursor(delta int) {
	eff := d.effectiveRows()
	if len(eff) == 0 {
		return
	}
	cur := d.effectiveCursor()
	d.setCursor(cur + delta)
}

func (d *dashboardModel) clampCursor() {
	eff := d.effectiveRows()
	cur := d.effectiveCursor()
	if cur < 0 {
		d.setCursor(0)
	} else if cur >= len(eff) {
		d.setCursor(maxInt(0, len(eff)-1))
	}
}

func (d *dashboardModel) currentRow() *dashboardRow {
	eff := d.effectiveRows()
	cur := d.effectiveCursor()
	if cur < 0 || cur >= len(eff) {
		return nil
	}
	idx := eff[cur]
	if idx < 0 || idx >= len(d.rows) {
		return nil
	}
	return &d.rows[idx]
}

// SelectedTarget returns "session" or "session:window" for the current row.
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

// ── rendering ─────────────────────────────────────────────────────────────

func (d *dashboardModel) View() string {
	if d.listing == nil {
		return d.st.Hint.Render(d.str.Loading)
	}
	if len(d.rows) == 0 {
		return d.st.Hint.Render(d.str.NoSessions)
	}

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
	// Filter header row (if filter is active or typed).
	var header string
	if d.filterText != "" || d.filterActive {
		cursor := ""
		if d.filterActive {
			cursor = "█"
		}
		eff := d.effectiveRows()
		total := len(d.rows)
		match := len(eff)
		count := fmt.Sprintf(" %d/%d", match, total)
		filterLine := "/" + d.filterText + cursor + count
		header = d.st.Hint.Render(truncate(filterLine, width)) + "\n"
	}

	// Compute the visible window.
	maxRows := d.height - 4
	if d.filterText != "" || d.filterActive {
		maxRows-- // header line takes a row
	}
	if maxRows < 5 {
		maxRows = 5
	}
	eff := d.effectiveRows()
	cur := d.effectiveCursor()
	start := 0
	if cur >= maxRows {
		start = cur - maxRows + 1
	}
	end := minInt(len(eff), start+maxRows)

	var b strings.Builder
	b.WriteString(header)
	for i := start; i < end; i++ {
		rowIdx := eff[i]
		if rowIdx < 0 || rowIdx >= len(d.rows) {
			continue
		}
		row := d.rows[rowIdx]
		line := d.formatRow(row, width)
		if i == cur {
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
		// Append process hints for sessions (dimmed).
		procs := d.procHint(r.Commands, width-lipgloss.Width(head)-lipgloss.Width(status)-3)
		base := head
		if procs != "" {
			base = head + " " + procs
		}
		return padRight(base, width-lipgloss.Width(status)-1) + " " + status
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

// procHint formats a short process hint (e.g. "nvim claude") for a session row.
// maxW is the available space; returns empty string if maxW < 4.
func (d *dashboardModel) procHint(cmds []string, maxW int) string {
	if len(cmds) == 0 || maxW < 4 {
		return ""
	}
	text := strings.Join(cmds, " ")
	text = truncate(text, maxW)
	return d.st.Hint.Render(text)
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
		if len(r.Commands) > 0 {
			procs := truncate(strings.Join(r.Commands, " · "), width-12)
			fmt.Fprintf(&b, "%-10s%s\n", "procs", d.st.Hint.Render(procs))
		}
	} else {
		title := i18n.Tf("tui.dashboard.window_label", map[string]any{"session": r.Session, "window": r.Window})
		b.WriteString(d.st.Title.Render(title) + "\n\n")
		fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.live"), d.boolGlyph(r.Live))
		if r.Layout != "" {
			fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.layout"), r.Layout)
		}
		fmt.Fprintf(&b, "%-8s%d\n", i18n.T("tui.dashboard.field.panes"), r.WindowCnt)
		fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.status"), d.statusLabel(r.Status))
		// Variant 9: per-pane process + cwd rows.
		if d.paneProvider != nil && r.WindowCnt > 0 {
			b.WriteString("\n")
			for pIdx := 0; pIdx < r.WindowCnt; pIdx++ {
				paneKey := fmt.Sprintf("%s:%d.%d", r.Session, r.WindowIdx, pIdx)
				info, ok := d.paneProvider.Get(paneKey)
				if !ok {
					continue
				}
				cmd := info.Command
				if cmd == "" {
					cmd = "—"
				}
				cwd := shortenPath(info.Path, width-16)
				marker := " "
				if pIdx == d.previewPaneIdx {
					marker = "▶"
				}
				line := fmt.Sprintf(" %s %d  %-10s  %s", marker, pIdx,
					truncate(cmd, 10), d.st.Hint.Render(cwd))
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(d.st.Hint.Render(d.str.AttachHint))
	}
	if d.preview != "" {
		b.WriteString("\n\n")
		b.WriteString(d.st.Subtitle.Render(d.previewPaneLabel(r)) + "\n")
		b.WriteString(d.renderPreview(width))
	}
	return b.String()
}

// previewPaneLabel builds the header line for the preview section.
// For session rows it returns the generic label. For window rows it includes
// the pane index and its running command: "preview [pane N: cmd]".
func (d *dashboardModel) previewPaneLabel(r *dashboardRow) string {
	if r == nil || r.IsSession {
		return i18n.T("tui.dashboard.preview_label")
	}
	label := fmt.Sprintf("preview [pane %d", d.previewPaneIdx)
	if d.paneProvider != nil {
		paneKey := fmt.Sprintf("%s:%d.%d", r.Session, r.WindowIdx, d.previewPaneIdx)
		if info, ok := d.paneProvider.Get(paneKey); ok && info.Command != "" && !pane.IsIdleShell(info.Command) {
			label += ": " + info.Command
		}
	}
	return label + "]"
}

func (d *dashboardModel) boolGlyph(v bool) string {
	if v {
		return d.st.StatusOK.Render("✓")
	}
	return d.st.StatusGone.Render("✗")
}

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

func (d *dashboardModel) previewRows() int {
	budget := d.height - 12
	if budget > 15 {
		budget = 15
	}
	if budget < 0 {
		return 0
	}
	return budget
}

// ── helpers ────────────────────────────────────────────────────────────────

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
