package ui

import (
	"time"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/ui/toast"
)

// dataLoadedMsg arrives after the background poll fetches a new listing.
type dataLoadedMsg struct {
	Listing *actions.Listing
	Drift   []config.Drift
	Err     error
}

// previewLoadedMsg carries a tmux capture-pane result for the focused window.
type previewLoadedMsg struct {
	Target string
	Data   string
	Err    error
}

// toastMsg requests a transient bottom-right notification.
type toastMsg struct {
	Kind toast.Kind
	Text string
	// TTL overrides the kind-default duration when non-zero.
	TTL time.Duration
}

// toastExpiredMsg fires after a toast TTL elapses.
// Seq is the tag-compare counter set at Show time; the dismiss handler only
// clears the toast when Seq matches the current counter.
type toastExpiredMsg struct{ Seq uint64 }

// tickMsg drives the polling cadence.
type tickMsg time.Time

// switchScreenMsg routes the app to a different screen.
type switchScreenMsg struct {
	Screen Screen
}

// errorMsg surfaces an error in the toast/error UI.
type errorMsg struct{ Err error }

// actionDoneMsg signals an action completed successfully.
type actionDoneMsg struct{ Text string }

// undoHintMsg sets or clears the footer undo hint ("↶ kill session epcp").
// An empty Text clears the hint.
type undoHintMsg struct{ Text string }

// paneRefreshTickMsg is the alias used internally; the actual type comes from
// the refresh package and is re-exported here for documentation clarity.
// (The root model switches on refresh.TickMsg and refresh.PaneDataMsg directly.)

// historyLoadedMsg carries the initial history load from disk.
type historyLoadedMsg struct {
	Entries []historyDiskEntry
	Err     error
}

// historyDiskEntry mirrors state.HistoryEntry for in-process use.
type historyDiskEntry struct {
	Ts      string
	Action  string
	Target  string
	Result  string
	Details string
}

// clearHistoryMsg requests a history wipe (with optional archive).
type clearHistoryMsg struct{}

// historyClearedMsg confirms the wipe completed.
type historyClearedMsg struct {
	ArchivePath string
	Err         error
}
