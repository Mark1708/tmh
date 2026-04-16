package ui

import (
	"strings"

	"git.mark1708.ru/me/tmh/internal/ui/theme"
	"git.mark1708.ru/me/tmh/internal/ui/toast"

	tea "github.com/charmbracelet/bubbletea"
)

// confirmModel is a yes/no modal. OnConfirm is fired when the user accepts.
// If DryRunDesc is non-empty, pressing `t` shows a dry-run info toast instead
// of executing the action (Variant 4.6).
type confirmModel struct {
	keys       Keys
	st         theme.Styles
	str        UIStrings
	width      int
	height     int
	title      string
	body       string
	OnConfirm  func() tea.Cmd
	DryRunDesc string // human-readable description of what the action would do
}

func newConfirm(keys Keys, st theme.Styles, str UIStrings, title, body string, onConfirm func() tea.Cmd) *confirmModel {
	return &confirmModel{
		keys: keys, st: st, str: str, title: title, body: body, OnConfirm: onConfirm,
	}
}

func (c *confirmModel) Resize(w, h int) { c.width, c.height = w, h }

func (c *confirmModel) Update(msg tea.Msg) (*confirmModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch strings.ToLower(k.String()) {
		case "y", "enter":
			if c.OnConfirm != nil {
				return c, c.OnConfirm()
			}
			return c, nil
		case "t":
			// Dry-run: show what would happen without executing.
			desc := c.DryRunDesc
			if desc == "" {
				desc = c.title
			}
			text := "[dry-run] " + desc
			return c, tea.Sequence(
				func() tea.Msg { return toastMsg{Kind: toast.KindInfo, Text: text} },
				func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} },
			)
		case "n", "esc":
			return c, func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} }
		}
	}
	return c, nil
}

func (c *confirmModel) View() string {
	mb := modalBg(c.st.Palette)
	title := c.st.Title.Inherit(mb).Render(c.title)
	rowW := maxInt(40, c.width-12)

	var b strings.Builder
	b.WriteString(modalRow(c.st.Palette, rowW, title))
	b.WriteString("\n")
	b.WriteString(modalRow(c.st.Palette, rowW, ""))
	b.WriteString("\n")
	for _, line := range strings.Split(c.body, "\n") {
		b.WriteString(modalRow(c.st.Palette, rowW, mb.Render(line)))
		b.WriteString("\n")
	}
	b.WriteString(modalRow(c.st.Palette, rowW, ""))
	b.WriteString("\n")
	hint := c.str.Modal.ConfirmYes + "   " + c.str.Modal.ConfirmNo
	if c.DryRunDesc != "" {
		hint += "   t dry-run"
	}
	b.WriteString(modalRow(c.st.Palette, rowW, mb.Render(hint)))
	return placeMiddle(c.width, c.height, c.st.Modal.Render(b.String()), c.st.Palette)
}
