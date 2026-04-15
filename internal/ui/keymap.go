package ui

import "github.com/charmbracelet/bubbles/key"

// Keys defines every binding the TUI listens for. Centralised here so the
// help overlay can render them and so they can be overridden in the future
// from ~/.config/tmh/keys.yml.
type Keys struct {
	Up, Down, Left, Right key.Binding
	PgUp, PgDown          key.Binding
	Top, Bottom           key.Binding

	Enter, Esc, Quit key.Binding
	Tab              key.Binding

	Attach, NewSession, Kill, Sync, Reload key.Binding
	ConfigEditor, Diff, Snapshot, Undo     key.Binding
	Palette, Help, Theme, Search           key.Binding
	Refresh                                key.Binding
}

// DefaultKeys returns the default key bindings.
func DefaultKeys() Keys {
	return Keys{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:   key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse")),
		Right:  key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand")),
		PgUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("PgUp", "page up")),
		PgDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("PgDn", "page down")),
		Top:    key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom: key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),

		Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Esc:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Tab:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus")),

		Attach:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "attach")),
		NewSession:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		Kill:         key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "kill")),
		Sync:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Reload:       key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload")),
		ConfigEditor: key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "config")),
		Diff:         key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "diff")),
		Snapshot:     key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "snapshot")),
		Undo:         key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo")),
		Palette:      key.NewBinding(key.WithKeys(":", "ctrl+p"), key.WithHelp(":", "palette")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Theme:        key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("^T", "theme")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Refresh:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
}
