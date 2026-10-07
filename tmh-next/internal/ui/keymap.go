package ui

import (
	"charm.land/bubbles/v2/key"
)

// Global key bindings owned by the root shell. They only fire when the
// active page is in Normal input mode and no overlay is open.
var (
	KeyQuit    = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	KeyBack    = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	KeyPalette = key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "command palette"))
	KeySwitch  = key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "quick switch"))
	KeyHelp    = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
	KeyActions = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "actions"))
	KeyNavDown = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	KeyNavUp   = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	KeyOpen    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
)

// GlobalBindings returns the always-visible help set.
func GlobalBindings() []key.Binding {
	return []key.Binding{KeyPalette, KeySwitch, KeyActions, KeyHelp, KeyBack, KeyQuit}
}
