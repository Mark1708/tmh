package ui

import (
	"fmt"
	"strings"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/ui/pane"

	"github.com/charmbracelet/lipgloss"
)

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
	switch r.Level {
	case levelSession:
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

	case levelWindow:
		prefix := indent + "├─ "
		statusGlyph := " "
		if r.Live {
			statusGlyph = "●"
		}
		main := fmt.Sprintf("%s%s %-14s", prefix, statusGlyph, truncate(r.Window, 14))
		right := d.statusLabel(r.Status)
		return padRight(main, width-lipgloss.Width(right)-1) + " " + right

	case levelPane:
		// Pane row: "    │  N  cmd  cwd"
		prefix := indent + "│  "
		cmd := "—"
		cwd := ""
		if d.paneProvider != nil {
			paneKey := fmt.Sprintf("%s:%d.%d", r.Session, r.WindowIdx, r.PaneIdx)
			if info, ok := d.paneProvider.Get(paneKey); ok {
				if info.Command != "" {
					cmd = info.Command
				}
				cwd = shortenPath(info.Path, maxInt(0, width-30))
			}
		}
		main := fmt.Sprintf("%s%d  %-10s  %s", prefix, r.PaneIdx,
			truncate(cmd, 10), d.st.Hint.Render(cwd))
		return main
	}
	return ""
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
	switch r.Level {
	case levelSession:
		title := i18n.Tf("tui.dashboard.session_label", map[string]any{"name": r.Session})
		b.WriteString(d.st.Title.Render(title) + "\n\n")
		fmt.Fprintf(&b, "%-10s%s\n", i18n.T("tui.dashboard.field.live"), d.boolGlyph(r.Live))
		fmt.Fprintf(&b, "%-10s%s\n", i18n.T("tui.dashboard.field.attached"), d.boolGlyph(r.Attached))
		fmt.Fprintf(&b, "%-10s%d\n", i18n.T("tui.dashboard.field.windows"), r.WindowCnt)
		fmt.Fprintf(&b, "%-10s%s\n", i18n.T("tui.dashboard.field.status"), d.statusLabel(r.Status))
		if len(r.Commands) > 0 {
			procs := truncate(strings.Join(r.Commands, " · "), width-12)
			fmt.Fprintf(&b, "%-10s%s\n", i18n.T("tui.dashboard.field.procs"), d.st.Hint.Render(procs))
		}
	case levelPane:
		// Pane row detail: show command and cwd prominently.
		paneKey := fmt.Sprintf("%s:%d.%d", r.Session, r.WindowIdx, r.PaneIdx)
		title := fmt.Sprintf("pane %d — %s:%s", r.PaneIdx, r.Session, r.Window)
		b.WriteString(d.st.Title.Render(title) + "\n\n")
		if d.paneProvider != nil {
			if info, ok := d.paneProvider.Get(paneKey); ok {
				cmd := info.Command
				if cmd == "" {
					cmd = "—"
				}
				fmt.Fprintf(&b, "%-8s%s\n", "cmd", cmd)
				if info.Path != "" {
					fmt.Fprintf(&b, "%-8s%s\n", "cwd", shortenPath(info.Path, width-10))
				}
				fmt.Fprintf(&b, "%-8s%s\n", "active", d.boolGlyph(info.Active))
			}
		}
		b.WriteString("\n")
		b.WriteString(d.st.Hint.Render(d.str.AttachHint))
	default: // levelWindow
		title := i18n.Tf("tui.dashboard.window_label", map[string]any{"session": r.Session, "window": r.Window})
		b.WriteString(d.st.Title.Render(title) + "\n\n")
		fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.live"), d.boolGlyph(r.Live))
		if r.Layout != "" {
			fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.layout"), r.Layout)
		}
		// Variant 11: inline command drift indicator.
		entry := r.Session + "/" + r.Window
		if dr, ok := d.driftFull[entry]; ok && dr.ReasonCode == config.ReasonCommandDiffers {
			driftLine := fmt.Sprintf("%s ≠ expected: %s", dr.LiveCommand, dr.ConfigCommand)
			fmt.Fprintf(&b, "%-8s%s\n", "drift", d.st.StatusDrift.Render(driftLine))
		}
		fmt.Fprintf(&b, "%-8s%d\n", i18n.T("tui.dashboard.field.panes"), r.WindowCnt)
		fmt.Fprintf(&b, "%-8s%s\n", i18n.T("tui.dashboard.field.status"), d.statusLabel(r.Status))
		// Variant 9: per-pane process + cwd rows.
		if d.paneProvider != nil && r.WindowCnt > 0 {
			b.WriteString("\n")
			for pIdx := 0; pIdx < r.WindowCnt; pIdx++ {
				tmuxPaneIdx := pIdx + d.paneBaseOffset
				paneKey := fmt.Sprintf("%s:%d.%d", r.Session, r.WindowIdx, tmuxPaneIdx)
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
				line := fmt.Sprintf(" %s %d  %-10s  %s", marker, tmuxPaneIdx,
					truncate(cmd, 10), d.st.Hint.Render(cwd))
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(d.st.Hint.Render(d.str.AttachHint))
	} // end switch r.Level
	if d.preview != "" {
		b.WriteString("\n\n")
		b.WriteString(d.st.Subtitle.Render(d.previewPaneLabel(r)) + "\n")
		b.WriteString(d.renderPreview(width))
	}
	return b.String()
}

// previewPaneLabel builds the header line for the preview section.
// For session rows it returns the generic label. For window/pane rows it
// includes the pane index and its running command: "preview [pane N: cmd]".
func (d *dashboardModel) previewPaneLabel(r *dashboardRow) string {
	if r == nil || r.Level == levelSession {
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
