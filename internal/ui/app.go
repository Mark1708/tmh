// Package ui hosts the bubbletea application that powers `tmh` (no args).
//
// The model is a thin router: each screen is a sub-model with its own
// Update/View. Heavy work lives in internal/actions; the UI never calls
// tmux directly outside of polling tmux.Runner via the same actions.
package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	appstate "git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/ui/pane"
	"git.mark1708.ru/me/tmh/internal/ui/refresh"
	"git.mark1708.ru/me/tmh/internal/ui/theme"
	"git.mark1708.ru/me/tmh/internal/ui/toast"

	tea "github.com/charmbracelet/bubbletea"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// Deps wires the side-effect surface the UI needs. Tests pass a MockRunner
// here; production passes CLIRunner + the real config path.
type Deps struct {
	Runner     tmux.Runner
	State      *appstate.DB
	ConfigPath string
	Profile    string
	LoadConfig func() (*config.Config, error)
}

// Model is the root bubbletea model.
type Model struct {
	deps Deps
	keys Keys
	st   theme.Styles
	str  UIStrings

	width, height int

	cfg     *config.Config
	listing *actions.Listing
	drift   []config.Drift

	current     Screen
	prev        Screen
	dashboard   *dashboardModel
	palette     *paletteModel
	confirm     *confirmModel
	diff        *diffModel
	settings    *settingsModel
	helpVisible bool
	errMsg      string

	// toast is the current visible notification text; empty means no toast.
	toast    string
	toastEnd time.Time
	// toastSeq is a tag-compare counter. Every call to showToast increments it
	// and embeds the new value in the expiry Tick. The dismiss handler only
	// clears the toast when the incoming Seq matches toastSeq, preventing an
	// old Tick from dismissing a newer message.
	toastSeq  uint64
	toastKind toast.Kind
	// history is a ring-buffer of the last few toasts (including errors) so
	// the user can glance back at what finished and with what outcome via
	// ScreenHistory (`Ctrl+L`).
	history    []toastEntry
	historyMax int

	pollEvery time.Duration

	// historyStore persists the action log to disk (JSONL). May be nil if
	// the store could not be created (e.g. read-only FS).
	historyStore *appstate.HistoryStore

	// paneRefresher drives the periodic pane-command batch fetch.
	paneRefresher *refresh.Refresher
	// paneProvider is the in-memory cache of pane runtime data.
	paneProvider *pane.Provider

	// undoHint is the last undoable action description shown in the footer
	// (e.g. "kill session epcp"). Empty when there is nothing to undo.
	undoHint string

	// marksStore persists named marks and last-location ring to disk.
	marksStore *appstate.MarksStore

	// pendingOp tracks the first keystroke of a two-step mark operation:
	//   'm' → next key sets mark letter
	//   '\'' → next key jumps to mark letter
	// Zero means no pending operation.
	pendingOp       rune
	pendingOpExpiry time.Time
}

// toastEntry captures one entry in the toast history ring buffer.
type toastEntry struct {
	Text  string
	Err   bool
	Stamp time.Time
}

// historyOptsFromConfig converts config.HistoryConfig to appstate.HistoryOptions.
// Uses sensible defaults when fields are zero/nil.
func historyOptsFromConfig(c config.HistoryConfig) appstate.HistoryOptions {
	opts := appstate.HistoryOptions{
		MaxEntries:     c.MaxEntries,
		ArchiveOnClear: true, // default on
	}
	if c.ArchiveOnClear != nil {
		opts.ArchiveOnClear = *c.ArchiveOnClear
	}
	if c.Retention != "" && c.Retention != "forever" {
		if d, err := parseRetentionDuration(c.Retention); err == nil {
			opts.Retention = d
		}
	}
	if opts.Retention == 0 && c.Retention != "forever" {
		opts.Retention = 30 * 24 * time.Hour // default 30d
	}
	return opts
}

// parseRetentionDuration parses strings like "7d", "30d", "90d".
func parseRetentionDuration(s string) (time.Duration, error) {
	if len(s) > 1 && s[len(s)-1] == 'd' {
		days := strings.TrimSuffix(s, "d")
		var n int
		_, err := fmt.Sscanf(days, "%d", &n)
		if err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("unrecognised retention %q", s)
}

// New constructs the root model.
func New(deps Deps) *Model {
	keys := DefaultKeys()
	st := theme.New(theme.Mocha)
	str := LoadStrings()

	// Build a HistoryStore from the default options (config not yet loaded).
	// If the config specifies custom options, they're applied after the first
	// dataLoadedMsg arrives.
	hs := appstate.NewHistoryStore(appstate.HistoryOptions{
		Retention:      30 * 24 * time.Hour,
		ArchiveOnClear: true,
	})

	pr := refresh.New(refresh.DefaultInterval)
	pp := pane.New(2 * time.Second) // 2s TTL matches DefaultInterval

	m := &Model{
		deps:          deps,
		keys:          keys,
		st:            st,
		str:           str,
		current:       ScreenDashboard,
		dashboard:     newDashboard(keys, st, str),
		pollEvery:     2 * time.Second,
		historyMax:    30,
		historyStore:  hs,
		paneRefresher: pr,
		paneProvider:  pp,
		marksStore:    appstate.NewMarksStore(),
	}
	m.dashboard.SetPaneProvider(pp)
	return m
}

// pushHistory appends a message to the ring buffer and keeps the buffer
// capped at historyMax. Callers classify errors via isErr so the history
// screen can colour them distinctly.
func (m *Model) pushHistory(text string, isErr bool) {
	m.history = append(m.history, toastEntry{Text: text, Err: isErr, Stamp: time.Now()})
	if len(m.history) > m.historyMax {
		m.history = m.history[len(m.history)-m.historyMax:]
	}
}

// showToast displays kind-styled text in the footer and schedules its expiry.
// Uses tag-compare so concurrent Ticks from previous toasts cannot dismiss
// a newer notification.
func (m *Model) showToast(kind toast.Kind, text string) tea.Cmd {
	ttl := kind.TTL()
	m.toastSeq++
	seq := m.toastSeq
	m.toast = text
	m.toastKind = kind
	m.toastEnd = time.Now().Add(ttl)
	m.pushHistory(text, kind == toast.KindError)
	return tea.Tick(ttl, func(time.Time) tea.Msg { return toastExpiredMsg{Seq: seq} })
}

// Init triggers the first data load + polling tick + async history load
// + first pane-refresh tick.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadDataCmd(), m.tickCmd(), m.loadHistoryCmd(), m.paneRefresher.Tick())
}
