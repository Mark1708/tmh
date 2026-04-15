package ui

import (
	"time"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
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
	Text string
	TTL  time.Duration
}

// toastExpiredMsg fires after a toast TTL elapses.
type toastExpiredMsg struct{}

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
