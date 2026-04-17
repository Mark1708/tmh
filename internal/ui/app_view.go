package ui

import (
	"fmt"
	"strings"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/ui/toast"

	"github.com/charmbracelet/lipgloss"
)

// View renders the active screen + persistent overlays (toast, help).
func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	var body string
	switch m.current {
	case ScreenDashboard:
		body = m.renderDashboardScreen()
	case ScreenError:
		body = m.renderErrorScreen()
	case ScreenEmpty:
		body = m.renderEmptyScreen()
	case ScreenPalette:
		if m.palette != nil {
			body = m.palette.View()
		}
	case ScreenConfirm:
		if m.confirm != nil {
			body = m.confirm.View()
		}
	case ScreenDiff:
		if m.diff != nil {
			body = lipgloss.JoinVertical(lipgloss.Left,
				m.renderHeader(), m.diff.View(), m.renderFooter())
		}
	case ScreenSettings:
		if m.settings != nil {
			body = m.settings.View()
		}
	case ScreenHistory:
		body = m.renderHistory()
	default:
		body = m.dashboard.View()
	}

	if m.helpVisible {
		body = m.overlayHelp(body)
	}
	return body
}

func (m *Model) renderDashboardScreen() string {
	header := m.renderHeader()
	footer := m.renderFooter()
	body := m.dashboard.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *Model) renderHeader() string {
	left := m.st.Header.Render(fmt.Sprintf("tmh · %s", m.deps.ConfigPath))
	driftCount := 0
	for _, d := range m.drift {
		if d.Status != config.StatusOK {
			driftCount++
		}
	}
	right := ""
	if driftCount > 0 {
		right = m.st.StatusDrift.Render(i18n.Tf("tui.header.drift_badge", map[string]any{"count": driftCount}))
	}
	gap := strings.Repeat(" ", maxInt(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)))
	return left + gap + right
}

func (m *Model) renderFooter() string {
	hints := []string{
		m.st.KeyBinding.Render("a") + " " + m.str.Footer.Attach,
		m.st.KeyBinding.Render("n") + " " + m.str.Footer.NewSession,
		m.st.KeyBinding.Render("d") + " " + m.str.Footer.Kill,
		m.st.KeyBinding.Render("R") + " " + m.str.Footer.Dotfiles,
		m.st.KeyBinding.Render("s") + " " + m.str.Footer.Sync,
		m.st.KeyBinding.Render("S") + " " + m.str.Footer.Settings,
		m.st.KeyBinding.Render(":") + " " + m.str.Footer.Palette,
		m.st.KeyBinding.Render("^L") + " " + m.str.Footer.History,
		m.st.KeyBinding.Render("?") + " " + m.str.Footer.Help,
		m.st.KeyBinding.Render("q") + " " + m.str.Footer.Quit,
	}
	hintsStr := strings.Join(hints, " · ")

	// Undo hint (Variant 4.3): "↶ kill session epcp" when undo stack is non-empty.
	if m.undoHint != "" {
		hintsStr += "  " + m.st.KeyBinding.Render("u") + " ↶ " + m.st.Hint.Render(m.undoHint)
	}

	// Last-location hint (Variant 4.1): "'' ← prev" when ring is non-empty.
	if m.marksStore != nil && m.marksStore.HasLastLocation() {
		hintsStr += "  " + m.st.KeyBinding.Render("''") + " " + m.st.Hint.Render("← prev")
	}

	// Footer heatmap (Variant 14): "live N · idle N" shown on the right when
	// enabled in Display settings.
	heatmap := ""
	if m.cfg != nil && m.cfg.Defaults.Display.ShowFooterHeatmap && m.paneProvider != nil {
		live, idle := m.paneProvider.Stats()
		heatmap = m.st.Hint.Render(fmt.Sprintf("live %d · idle %d", live, idle))
	}

	if m.toast == "" && heatmap == "" || m.width == 0 {
		return m.st.Footer.Render(hintsStr)
	}
	if m.toast == "" {
		// Only heatmap — right-align it.
		contentW := m.width - 2
		heatW := lipgloss.Width(heatmap)
		hintsW := contentW - heatW - 1
		if hintsW < 0 {
			hintsW = 0
		}
		line := truncate(hintsStr, hintsW) + strings.Repeat(" ", maxInt(1, contentW-lipgloss.Width(truncate(hintsStr, hintsW))-heatW)) + heatmap
		return m.st.Footer.Render(line)
	}
	// Inline toast right-aligned on the same footer row. We truncate hints
	// from the right so the toast always fits; the full hint set stays
	// visible in `?` help and in the palette.
	toastStyle := m.st.Toast
	switch m.toastKind {
	case toast.KindSuccess:
		toastStyle = m.st.ToastSuccess
	case toast.KindError:
		toastStyle = m.st.ToastError
	}
	toastRendered := toastStyle.Render(m.toast)
	contentW := m.width - 2 // Footer has Padding(0, 1)
	toastW := lipgloss.Width(toastRendered)
	hintsW := contentW - toastW - 1
	if hintsW < 0 {
		hintsW = 0
	}
	line := truncate(hintsStr, hintsW) + strings.Repeat(" ", maxInt(1, contentW-lipgloss.Width(truncate(hintsStr, hintsW))-toastW)) + toastRendered
	return m.st.Footer.Render(line)
}

func (m *Model) renderErrorScreen() string {
	mb := modalBg(m.st.Palette)
	rowW := maxInt(40, m.width-12)
	title := m.st.StatusGone.Inherit(mb).Render(m.str.Modal.ErrorTitle)
	var b strings.Builder
	b.WriteString(modalRow(m.st.Palette, rowW, title))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	for _, line := range strings.Split(m.errMsg, "\n") {
		b.WriteString(modalRow(m.st.Palette, rowW, mb.Render(line)))
		b.WriteString("\n")
	}
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, mb.Render(m.str.Modal.ErrorDismiss)))
	return placeMiddle(m.width, m.height, m.st.Modal.Render(b.String()), m.st.Palette)
}

func (m *Model) renderEmptyScreen() string {
	mb := modalBg(m.st.Palette)
	rowW := maxInt(40, m.width-12)
	var b strings.Builder
	b.WriteString(modalRow(m.st.Palette, rowW, mb.Render(m.str.Modal.EmptyTitle)))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, mb.Render(m.str.Modal.EmptyHint)))
	return placeMiddle(m.width, m.height, m.st.Modal.Render(b.String()), m.st.Palette)
}

func (m *Model) overlayHelp(body string) string {
	_ = body
	return placeMiddle(m.width, m.height, m.st.Modal.Render(m.modeHelpText()), m.st.Palette)
}

// modeHelpText returns help text appropriate for the current screen.
func (m *Model) modeHelpText() string {
	switch m.current {
	case ScreenSettings:
		return m.helpTextSettings()
	case ScreenHistory:
		return m.helpTextHistory()
	case ScreenPalette:
		return m.helpTextPalette()
	case ScreenConfirm:
		return m.helpTextConfirm()
	default:
		return m.helpText()
	}
}

func (m *Model) helpText() string {
	km := m.str.Keymap
	sections := []struct {
		title string
		rows  [][2]string
	}{
		{km.SectionNav, [][2]string{
			{"j / k / ↑↓", km.NavUpdown},
			{"h / l", km.NavCollapse},
			{"g / G", km.NavTopBottom},
			{"PgUp / PgDn", km.NavPage},
			{"tab", i18n.T("tui.help.action.tab_panes")},
		}},
		{km.SectionActions, [][2]string{
			{"enter / a", km.ActionAttach},
			{"n", km.ActionNew},
			{"d", km.ActionKill},
			{"u", km.ActionUndo},
			{"m<a>", i18n.T("tui.help.action.mark_set")},
			{"'<a>", i18n.T("tui.help.action.mark_jump")},
			{"''", i18n.T("tui.help.action.prev_location")},
		}},
		{km.SectionSync, [][2]string{
			{"r", km.SyncRefresh},
			{"R", km.SyncReload},
			{"s", km.SyncPush},
			{"D", km.SyncDiff},
		}},
		{km.SectionOther, [][2]string{
			{": / Ctrl+P", km.OtherPalette},
			{"S", km.OtherSettings},
			{"? / esc", km.OtherHelp},
			{"q / Ctrl+C", km.OtherQuit},
		}},
	}
	mb := modalBg(m.st.Palette)
	title := m.st.Title.Inherit(mb)
	subtitle := m.st.Subtitle.Inherit(mb)
	keyBind := m.st.KeyBinding.Inherit(mb)
	plain := mb

	// Width budget: modal content area (screen width minus border + padding).
	rowW := maxInt(40, m.width-12)

	var b strings.Builder
	b.WriteString(modalRow(m.st.Palette, rowW, title.Render(km.Title)))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	for si, sec := range sections {
		b.WriteString(modalRow(m.st.Palette, rowW, subtitle.Render(sec.title)))
		b.WriteString("\n")
		for _, row := range sec.rows {
			line := plain.Render("  ") + keyBind.Render(row[0]) + plain.Render("   "+row[1])
			b.WriteString(modalRow(m.st.Palette, rowW, line))
			b.WriteString("\n")
		}
		if si < len(sections)-1 {
			b.WriteString(modalRow(m.st.Palette, rowW, ""))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// helpTextSettings renders a minimal help overlay for the Settings screen.
func (m *Model) helpTextSettings() string {
	return m.buildModeHelp(m.str.Keymap.Title+" — settings", [][2]string{
		{"j / k", "navigate fields"},
		{"enter", "edit / activate"},
		{"esc", "cancel / back"},
		{"Ctrl+S", "save"},
		{"?", "close help"},
	})
}

// helpTextHistory renders a minimal help overlay for the History screen.
func (m *Model) helpTextHistory() string {
	return m.buildModeHelp(m.str.Keymap.Title+" — history", [][2]string{
		{"j / k", "scroll"},
		{"X", "clear history"},
		{"esc / Ctrl+L", "close"},
		{"?", "close help"},
	})
}

// helpTextPalette renders a minimal help overlay for the Palette screen.
func (m *Model) helpTextPalette() string {
	return m.buildModeHelp(m.str.Keymap.Title+" — palette", [][2]string{
		{"type", "filter actions"},
		{"j / k / ↑↓", "navigate"},
		{"enter", "execute"},
		{"esc", "close"},
		{"?", "close help"},
	})
}

// helpTextConfirm renders a minimal help overlay for the Confirm dialog.
func (m *Model) helpTextConfirm() string {
	return m.buildModeHelp(m.str.Keymap.Title+" — confirm", [][2]string{
		{"y / enter", "confirm"},
		{"n / esc", "cancel"},
		{"?", "close help"},
	})
}

// buildModeHelp constructs a modal help text with a title and key rows.
func (m *Model) buildModeHelp(title string, rows [][2]string) string {
	mb := modalBg(m.st.Palette)
	titleStyle := m.st.Title.Inherit(mb)
	keyBind := m.st.KeyBinding.Inherit(mb)
	plain := mb
	rowW := maxInt(40, m.width-12)

	var b strings.Builder
	b.WriteString(modalRow(m.st.Palette, rowW, titleStyle.Render(title)))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	for _, row := range rows {
		line := plain.Render("  ") + keyBind.Render(row[0]) + plain.Render("   "+row[1])
		b.WriteString(modalRow(m.st.Palette, rowW, line))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
