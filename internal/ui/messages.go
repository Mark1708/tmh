package ui

import (
	"time"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/ui/toast"
)

// dataLoadedMsg arrives after the background poll fetches a new listing.
type dataLoadedMsg struct {
	Listing       *actions.Listing
	Drift         []config.Drift
	Cfg           *config.Config // populated so Update can assign m.cfg on the main goroutine
	PaneBaseIndex int            // actual pane-base-index read from tmux (0 when undetected)
	Err           error
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

// undoHintMsg sets or clears the footer undo hint ("↶ kill session atlas").
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

// pendingOpExpiredMsg fires when a two-step mark operation times out without
// the user pressing a second key.
type pendingOpExpiredMsg struct{ Op rune }

// gotoProcMsg is returned by gotoProcCmd when a matching pane is found.
// The model handles the cursor jump on the main goroutine.
type gotoProcMsg struct{ Target string }

// clearHistoryMsg requests a history wipe (with optional archive).
type clearHistoryMsg struct{}

// historyClearedMsg confirms the wipe completed.
type historyClearedMsg struct {
	ArchivePath string
	Err         error
}
