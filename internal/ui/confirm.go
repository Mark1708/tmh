package ui

import (
	"strings"

	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmModel is a yes/no modal. OnConfirm is fired when the user accepts.
type confirmModel struct {
	keys      Keys
	st        theme.Styles
	width     int
	height    int
	title     string
	body      string
	OnConfirm func() tea.Cmd
}

func newConfirm(keys Keys, st theme.Styles, title, body string, onConfirm func() tea.Cmd) *confirmModel {
	return &confirmModel{
		keys: keys, st: st, title: title, body: body, OnConfirm: onConfirm,
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
		c.st.KeyBinding.Render("y") + "/enter confirm   " +
		c.st.KeyBinding.Render("n") + "/esc cancel"
	return lipgloss.Place(c.width, c.height, lipgloss.Center, lipgloss.Center, c.st.Modal.Render(body))
}
