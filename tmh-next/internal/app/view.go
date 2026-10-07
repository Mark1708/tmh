package app

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/components"
	"github.com/mark1708/tmh-next/internal/ui/layout"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// View renders the fixed header/body/footer chrome, page content, overlays
// and toasts into a declarative tea.View.
func (r *Root) View() tea.View {
	v := tea.NewView(theme.PaintBackground(r.frame(), r.styles.Palette.Base))
	v.AltScreen = true
	v.ReportFocus = true
	v.WindowTitle = "tmh-next — control panel"
	v.ForegroundColor = r.styles.Palette.Text
	v.BackgroundColor = r.styles.Palette.Base
	return v
}

// frame builds the full-screen content string.
func (r *Root) frame() string {
	if r.fatal != "" {
		return r.styles.Err.Render("tmh-next: " + r.fatal + "\n\npress q to quit")
	}
	if r.tooSmall {
		return layout.TooSmall(r.width, r.height, r.styles)
	}
	if !r.ready || r.cat == nil {
		return r.styles.Dim.Render("tmh-next: loading control-plane snapshot …")
	}

	header := r.headerView()
	footer := r.footerView()
	body := r.bodyView()

	frame := lipgloss.JoinVertical(lipgloss.Left,
		header,
		body,
		footer,
	)

	// Root-owned overlays composite on top of the whole frame.
	if r.ov != nil {
		if panel := r.overlayView(); panel != "" {
			frame = layout.ComposeOverlay(frame, panel, r.styles)
		}
	}
	return frame
}

// headerView renders route breadcrumb and live/demo runtime status.
func (r *Root) headerView() string {
	var crumbs []string
	for i, loc := range r.stack {
		title := ui.RouteTitle(loc.Route)
		if loc.Primary.ID != "" {
			title += " " + loc.Primary.ID
		}
		if i == len(r.stack)-1 {
			crumbs = append(crumbs, r.styles.Header.Render(title))
		} else {
			crumbs = append(crumbs, r.styles.Breadcrumb.Render(title))
		}
	}
	left := strings.Join(crumbs, " "+r.styles.Dim.Render("›")+" ")
	badge := r.runtimeBadge()
	right := strings.Join([]string{
		"rev " + fmt.Sprint(r.cat.Revision),
		string(r.cat.Scenario),
		r.cat.Now.UTC().Format("15:04:05"),
		badge,
	}, "  ")
	if r.mut.kind != mutNone {
		right += "  " + r.styles.Warn.Render("● "+r.busyReason())
	}

	innerWidth := max(1, r.width-2)
	header := padJoin(left, right, innerWidth)
	return r.styles.Chip.Width(innerWidth).Render(
		theme.PaintBackground(clipLine(header, innerWidth), r.styles.Palette.Crust),
	)
}

func revisionWord(r *Root) string { return "OK" }

// bodyView renders the active page with toasts layered on top.
func (r *Root) bodyView() string {
	p := r.activePage()
	content := ""
	if p != nil {
		content = p.View()
	}
	if toasts := components.Toasts(r.toasts, r.styles, max(1, r.width-8)); toasts != "" {
		content = lipgloss.JoinVertical(lipgloss.Right, toasts, content)
	}

	// Explicitly paint every terminal cell. A declarative background color does
	// not fill untouched cells in transparent terminals, which made most of the
	// screen show the desktop wallpaper.
	innerWidth := max(1, r.width-4)
	bodyHeight := max(1, r.height-2)
	contentLines := strings.Split(content, "\n")
	lines := make([]string, 0, bodyHeight)
	lines = append(lines, "")
	for _, line := range contentLines {
		if len(lines) >= bodyHeight-1 {
			break
		}
		lines = append(lines, "  "+clipLine(line, innerWidth))
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	bg := lipgloss.NewStyle().Width(r.width).Background(r.styles.Palette.Base)
	for i, line := range lines {
		lines[i] = bg.Render(clipLine(line, r.width))
	}
	return strings.Join(lines, "\n")
}

// footerView renders compact, high-contrast help instead of the low-contrast
// default Bubbles help renderer.
func (r *Root) footerView() string {
	bindings := ui.GlobalBindings()
	if p := r.activePage(); p != nil {
		bindings = append(append([]key.Binding(nil), bindings...), p.Help()...)
	}
	parts := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		help := binding.Help()
		if help.Key == "" || help.Desc == "" {
			continue
		}
		parts = append(parts,
			r.styles.Accent.Copy().Bold(true).Render(help.Key)+" "+r.styles.Footer.Render(help.Desc))
	}
	innerWidth := max(1, r.width-2)
	line := clipLine(strings.Join(parts, r.styles.Dim.Render("  ·  ")), innerWidth)
	return r.styles.Chip.Width(innerWidth).Render(
		theme.PaintBackground(line, r.styles.Palette.Crust),
	)
}

// overlayView renders the open root-owned overlay panel.
func (r *Root) overlayView() string {
	if r.ov == nil {
		return ""
	}
	switch r.ov.kind {
	case ovPalette, ovQuickSwitch, ovSelector:
		if r.ov.sel != nil {
			return r.ov.sel.View()
		}
	case ovPrompt:
		if r.ov.prompt != nil {
			return r.ov.prompt.View()
		}
	case ovConfirm:
		if r.ov.confirm != nil {
			return r.ov.confirm.View()
		}
	case ovHelp:
		return r.helpOverlayView()
	case ovConfig:
		if r.ov.form != nil {
			form := r.ov.form
			if wide := layout.Classify(r.width, r.height) == layout.ModeWide; wide {
				content := r.ov.title + " — " + r.runtimeBadge() + "\n\n" + form.View()
				return r.styles.Overlay.Render(theme.PaintBackground(content, r.styles.Palette.Mantle))
			}
			return form.View()
		}
	}
	return ""
}

// helpOverlayView renders the animated, use-case-oriented help panel.
func (r *Root) helpOverlayView() string {
	return r.walkthroughView()
}

func (r *Root) runtimeBadge() string {
	if r.isProduction() {
		return r.styles.OK.Render("LIVE")
	}
	return r.styles.Mock.Render("MOCK")
}

// --- small helpers ---------------------------------------------------------

type bindingKeyMap struct{ bindings []key.Binding }

func (b bindingKeyMap) ShortHelp() []key.Binding { return b.bindings }

func (b bindingKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{b.bindings} }

// padJoin joins left and right with padding to fill width (ANSI-aware).
func padJoin(left, right string, width int) string {
	lw := lineWidth(left)
	rw := lineWidth(right)
	gap := width - lw - rw - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func lineWidth(s string) int { return lipgloss.Width(s) }

func clipLine(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
