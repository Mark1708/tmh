package ui

import (
	"strings"

	"git.mark1708.ru/me/tmh/internal/i18n"
)

// renderHistory draws the action-history panel (`Ctrl+L`). Newest entries
// appear at the top; errors are tagged in red so a glance tells the user
// whether the reload / sync / kill they just triggered finished cleanly.
func (m *Model) renderHistory() string {
	mb := modalBg(m.st.Palette)
	title := m.st.Title.Inherit(mb)
	hint := m.st.Hint.Inherit(mb)
	ok := m.st.StatusOK.Inherit(mb).Bold(true)
	errS := m.st.StatusGone.Inherit(mb).Bold(true)

	var b strings.Builder
	b.WriteString(paintLine(m.st.Palette, title.Render(i18n.T("tui.history.title"))) + "\n\n")

	if len(m.history) == 0 {
		b.WriteString(paintLine(m.st.Palette, hint.Render(i18n.T("tui.history.empty"))))
	} else {
		// newest first
		for i := len(m.history) - 1; i >= 0; i-- {
			e := m.history[i]
			var badge string
			if e.Err {
				badge = errS.Render(padRight(i18n.T("tui.history.error_label"), 7))
			} else {
				badge = ok.Render(padRight(i18n.T("tui.history.ok_label"), 7))
			}
			ts := hint.Render(e.Stamp.Format("15:04:05"))
			line := mb.Render(" ") + badge + mb.Render(" ") + ts + mb.Render("  "+truncate(e.Text, maxInt(30, m.width-30)))
			b.WriteString(paintLine(m.st.Palette, line) + "\n")
		}
	}

	b.WriteString("\n" + paintLine(m.st.Palette, hint.Render(i18n.T("tui.history.back_hint"))))
	body := padBlock(b.String())
	return placeMiddle(m.width, m.height, m.st.Modal.Render(body), m.st.Palette)
}

