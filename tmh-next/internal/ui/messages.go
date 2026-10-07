package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
)

// Page → root intent messages. They live in ui so pages and the root shell
// share one vocabulary without import cycles.

// ExecuteActionMsg requests a user mutation through the serialized lane.
type ExecuteActionMsg struct{ Action domain.Action }

// PushRouteMsg navigates to a location (no-op when equal to the top).
type PushRouteMsg struct {
	Loc     Location
	Replace bool
}

// PopRouteMsg pops the route stack (Dashboard is the lower bound).
type PopRouteMsg struct{}

// OpenPaletteMsg opens the command palette overlay.
type OpenPaletteMsg struct{}

// OpenQuickSwitchMsg opens the quick switch overlay.
type OpenQuickSwitchMsg struct{}

// OpenHelpMsg opens the help overlay.
type OpenHelpMsg struct{}

// OpenSelectorMsg opens a contextual selector overlay.
type OpenSelectorMsg struct {
	Title   string
	Options []SelectorOption
}

// OpenPromptMsg opens the root-owned text prompt.
type OpenPromptMsg struct {
	Title       string
	Placeholder string
	Initial     string
	Validate    func(string) error
	Submit      func(string) tea.Cmd
}

// ConfirmActionMsg opens the confirmation overlay for a destructive action.
// The action dispatches with Confirmed=true only after y/enter.
type ConfirmActionMsg struct {
	Title    string
	Detail   string
	Action   domain.Action
	OnAccept func(domain.Action) tea.Cmd
}

// OpenConfigFormMsg opens the root-owned Huh configuration draft form.
type OpenConfigFormMsg struct{ Draft domain.Config }

// ShowToastMsg posts a transient notification.
type ShowToastMsg struct {
	Text string
	Kind string
}

// SearchRequestMsg asks the root search lane to query the backend.
type SearchRequestMsg struct {
	Text  string
	Scope string
}

// SearchResultsMsg delivers the latest search lane results to the search page.
type SearchResultsMsg struct {
	Seq      int
	Revision uint64
	Hits     []domain.SearchHit
}

// QuitNowMsg terminates the program.
type QuitNowMsg struct{}
