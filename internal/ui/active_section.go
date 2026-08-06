package ui

import (
	"fmt"
	"strings"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"

	"github.com/charmbracelet/lipgloss"
)

func (d *dashboardModel) SetActiveStatus(report actions.ActiveStatusReport) {
	selectedID := d.currentTargetKey()
	d.active = cloneActiveReport(report)
	d.rebuildRows()
	d.restoreCursorByID(selectedID)
}

func cloneActiveReport(report actions.ActiveStatusReport) actions.ActiveStatusReport {
	cloned := report
	cloned.Entries = make([]actions.ActiveWindowStatus, len(report.Entries))
	for i, entry := range report.Entries {
		cloned.Entries[i] = entry
		cloned.Entries[i].SourceLinks = append([]actions.ActiveSourceLink(nil), entry.SourceLinks...)
	}
	return cloned
}

func (d *dashboardModel) appendActiveRows() {
	if !d.active.Enabled {
		return
	}
	d.rows = append(d.rows, dashboardRow{Level: levelActiveHeader})
	for i := range d.active.Entries {
		entry := d.active.Entries[i]
		d.rows = append(d.rows, dashboardRow{Level: levelActiveWindow, Indent: 1, Active: &entry})
	}
}

func activeRowMatches(entry actions.ActiveWindowStatus, query string) bool {
	if strings.Contains(strings.ToLower(entry.WindowID+" "+entry.WindowName), query) {
		return true
	}
	for _, source := range entry.SourceLinks {
		if strings.Contains(strings.ToLower(source.SessionName), query) {
			return true
		}
	}
	return false
}

func (d *dashboardModel) formatActiveHeader(width int) string {
	title := i18n.T("tui.dashboard.active.title")
	if d.active.Collision {
		warningText := i18n.T("tui.dashboard.active.collision")
		if d.active.CollisionReason != "" {
			warningText += ": " + d.active.CollisionReason
		}
		warning := d.st.StatusDrift.Render(truncate(warningText, maxInt(8, width-lipgloss.Width(title)-1)))
		return padRight(d.st.Subtitle.Render(title), width-lipgloss.Width(warning)-1) + " " + warning
	}
	count := fmt.Sprintf("%d", len(d.active.Entries))
	return padRight(d.st.Subtitle.Render(title), width-lipgloss.Width(count)-1) + " " + count
}

func (d *dashboardModel) formatActiveWindow(r dashboardRow, width int) string {
	entry := r.Active
	if entry == nil {
		return ""
	}
	index := "#—"
	if entry.ActiveIndex != nil {
		index = fmt.Sprintf("#%d", *entry.ActiveIndex)
	}
	main := fmt.Sprintf("  ├─ %-5s %-10s %s", entry.WindowID, truncate(entry.WindowName, 10), index)
	right := d.activeStateLabel(*entry)
	sources := activeSourceHint(entry.SourceLinks)
	if sources != "" {
		main += " " + d.st.Hint.Render(sources)
	}
	return padRight(main, width-lipgloss.Width(right)-1) + " " + right
}

func (d *dashboardModel) activeStateLabel(entry actions.ActiveWindowStatus) string {
	label := i18n.T("tui.dashboard.active.tracked")
	style := d.st.StatusOK
	if entry.State == state.ActiveWindowOrphaned {
		label = i18n.T("tui.dashboard.active.orphaned")
		style = d.st.StatusGone
	}
	if entry.Expired {
		label += "/" + i18n.T("tui.dashboard.active.expired")
		style = d.st.StatusDrift
	}
	return style.Render(label)
}

func activeSourceHint(links []actions.ActiveSourceLink) string {
	parts := make([]string, 0, len(links))
	for _, link := range links {
		parts = append(parts, fmt.Sprintf("%s:%d", link.SessionName, link.WindowIndex))
	}
	return strings.Join(parts, ",")
}

func (d *dashboardModel) renderActiveDetail(r *dashboardRow, width int) string {
	if r.Level == levelActiveHeader {
		return d.renderActiveSummary(width)
	}
	entry := r.Active
	if entry == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(d.st.Title.Render(entry.WindowID+" · "+entry.WindowName) + "\n\n")
	fmt.Fprintf(&b, "%-10s%s\n", i18n.T("tui.dashboard.field.status"), d.activeStateLabel(*entry))
	if entry.ActiveIndex != nil {
		fmt.Fprintf(&b, "%-10s#%d\n", "active", *entry.ActiveIndex)
	}
	if sources := activeSourceHint(entry.SourceLinks); sources != "" {
		fmt.Fprintf(&b, "%-10s%s\n", "source", truncate(sources, width-10))
	}
	return b.String()
}

func (d *dashboardModel) renderActiveSummary(width int) string {
	var b strings.Builder
	b.WriteString(d.st.Title.Render(i18n.T("tui.dashboard.active.title")) + "\n\n")
	fmt.Fprintf(&b, "%-10s%d\n", i18n.T("tui.dashboard.active.tracked"), d.active.Tracked)
	fmt.Fprintf(&b, "%-10s%d\n", i18n.T("tui.dashboard.active.orphaned"), d.active.Orphaned)
	if d.active.Collision {
		b.WriteString("\n" + d.st.StatusDrift.Render(i18n.T("tui.dashboard.active.collision")) + "\n")
		b.WriteString(d.st.Hint.Render(truncate(d.active.CollisionReason, width)))
	}
	return b.String()
}
