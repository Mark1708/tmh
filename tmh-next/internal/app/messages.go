package app

import (
	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
)

// Page-intent messages are defined once in package ui; these aliases keep
// them addressable from the app package.
type (
	ExecuteActionMsg   = ui.ExecuteActionMsg
	PushRouteMsg       = ui.PushRouteMsg
	PopRouteMsg        = ui.PopRouteMsg
	OpenPaletteMsg     = ui.OpenPaletteMsg
	OpenQuickSwitchMsg = ui.OpenQuickSwitchMsg
	OpenHelpMsg        = ui.OpenHelpMsg
	OpenSelectorMsg    = ui.OpenSelectorMsg
	OpenPromptMsg      = ui.OpenPromptMsg
	ConfirmActionMsg   = ui.ConfirmActionMsg
	OpenConfigFormMsg  = ui.OpenConfigFormMsg
	ShowToastMsg       = ui.ShowToastMsg
	SearchRequestMsg   = ui.SearchRequestMsg
	SearchResultsMsg   = ui.SearchResultsMsg
	QuitNowMsg         = ui.QuitNowMsg
)

// --- async backend results (app-owned) --------------------------------------
// --- async backend results -------------------------------------------------

// LoadCatalogMsg carries the initial control-plane snapshot.
type LoadCatalogMsg struct {
	Catalog  *domain.Catalog
	EventSeq uint64
}

// LoadFailedMsg reports a failed initial load.
type LoadFailedMsg struct{ Err error }

// ReloadedMsg carries a conflict-recovery reload of the current backend state.
type ReloadedMsg struct{ Catalog *domain.Catalog }

// ReloadFailedMsg releases the mutation lane after conflict recovery could
// not fetch a valid replacement snapshot.
type ReloadFailedMsg struct{ Err error }
type RuntimeWatchMsg struct{ Event control.WatchEvent }

type RuntimeWatchFailedMsg struct{ Err error }

type RuntimeSnapshotMsg struct{ Snapshot control.Snapshot }

type RetryWatchMsg struct{}

type InteractiveDoneMsg struct{ Err error }

// MutationResultMsg carries one accepted Execute/Tick result.
type MutationResultMsg struct{ Result domain.MutationResult }

// MutationFailedMsg carries a rejected mutation with the in-flight identity.
type MutationFailedMsg struct {
	Kind     domain.ActionKind
	IsTick   bool
	Identity string
	Err      error
}

// TickTimerMsg is a due scheduler timer token.
type TickTimerMsg struct{ Token uint64 }

// ToastExpiredMsg retires one toast by sequence number.
type ToastExpiredMsg struct{ Seq uint64 }

// WalkthroughTickMsg advances the active guided-help animation.
type WalkthroughTickMsg struct{ Token uint64 }
