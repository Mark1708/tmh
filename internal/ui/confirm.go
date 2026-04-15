package ui

import (
	"strings"

	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
)

// confirmModel is a yes/no modal. OnConfirm is fired when the user accepts.
type confirmModel struct {
	keys      Keys
	st        theme.Styles
	str       UIStrings
	width     int
	height    int
	title     string
	body      string
	OnConfirm func() tea.Cmd
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
		case "n", "esc":
			return c, func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} }
		}
	}
	return c, nil
}

func (c *confirmModel) View() string {
	body := c.st.Title.Render(c.title) + "\n\n" + c.body + "\n\n" +
		c.str.Modal.ConfirmYes + "   " + c.str.Modal.ConfirmNo
	return placeMiddle(c.width, c.height, c.st.Modal.Render(padBlock(body)), c.st.Palette)
}
