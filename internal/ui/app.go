// Package ui hosts the bubbletea application that powers `tmh` (no args).
//
// The model is a thin router: each screen is a sub-model with its own
// Update/View. Heavy work lives in internal/actions; the UI never calls
// tmux directly outside of polling tmux.Runner via the same actions.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	appstate "git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/ui/errrender"
	"git.mark1708.ru/me/tmh/internal/ui/pane"
	"git.mark1708.ru/me/tmh/internal/ui/refresh"
	"git.mark1708.ru/me/tmh/internal/ui/theme"
	"git.mark1708.ru/me/tmh/internal/ui/toast"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// Update routes messages to active screens.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.dashboard != nil {
			m.dashboard.Resize(msg.Width, msg.Height-2) // header+footer
		}
		if m.palette != nil {
			m.palette.Resize(msg.Width, msg.Height)
		}
		if m.confirm != nil {
			m.confirm.Resize(msg.Width, msg.Height)
		}
		if m.diff != nil {
			m.diff.Resize(msg.Width, msg.Height-2)
		}
		if m.settings != nil {
			m.settings.Resize(msg.Width, msg.Height)
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.loadDataCmd(), m.tickCmd())

	case refresh.TickMsg:
		// Always reschedule the tick to keep the loop alive.
		cmd := m.paneRefresher.Tick()
		// Skip the fetch while a text-input widget has focus (input-pause rule).
		inputFocused := (m.palette != nil && m.current == ScreenPalette) ||
			(m.dashboard != nil && m.dashboard.FilterActive())
		if inputFocused {
			return m, cmd
		}
		seq := m.paneRefresher.BumpSeq()
		return m, tea.Batch(cmd, m.paneRefresher.Fetch(m.deps.Runner, seq))

	case refresh.PaneDataMsg:
		// Drop stale results from a previous fetch cadence.
		if msg.Seq != m.paneRefresher.Seq() {
			return m, nil
		}
		m.paneProvider.SetAll(msg.Data)
		if m.dashboard != nil {
			m.dashboard.UpdateCommands()
		}
		return m, nil

	case dataLoadedMsg:
		if msg.Err != nil {
			m.errMsg = errrender.Render(msg.Err)
			m.current = ScreenError
			return m, nil
		}
		m.listing = msg.Listing
		m.drift = msg.Drift
		if m.dashboard != nil {
			m.dashboard.SetData(msg.Listing, msg.Drift)
		}
		return m, m.maybeLoadPreview()

	case previewLoadedMsg:
		if m.dashboard != nil && msg.Err == nil {
			m.dashboard.SetPreview(msg.Target, msg.Data)
		}
		return m, nil

	case toastMsg:
		kind := msg.Kind
		text := msg.Text
		// Allow caller to override the default kind TTL.
		if msg.TTL != 0 {
			m.toastSeq++
			seq := m.toastSeq
			m.toast = text
			m.toastKind = kind
			m.toastEnd = time.Now().Add(msg.TTL)
			m.pushHistory(text, kind == toast.KindError)
			return m, tea.Tick(msg.TTL, func(time.Time) tea.Msg { return toastExpiredMsg{Seq: seq} })
		}
		return m, m.showToast(kind, text)

	case toastExpiredMsg:
		// Tag-compare: only dismiss if the Seq matches the current counter.
		if msg.Seq == m.toastSeq {
			m.toast = ""
		}
		return m, nil

	case errorMsg:
		rendered := errrender.Render(msg.Err)
		return m, m.showToast(toast.KindError,
			i18n.Tf("tui.toast.error_prefix", map[string]any{"msg": rendered}))

	case actionDoneMsg:
		return m, m.showToast(toast.KindSuccess, msg.Text)

	case historyLoadedMsg:
		if msg.Err != nil {
			// Non-fatal: show an error toast and continue with empty history.
			return m, m.showToast(toast.KindError,
				i18n.Tf("tui.toast.error_prefix", map[string]any{"msg": msg.Err.Error()}))
		}
		// Merge disk history into RAM ring-buffer (disk entries first, oldest first).
		for _, e := range msg.Entries {
			isErr := e.Result == "err"
			t, _ := time.Parse(time.RFC3339, e.Ts)
			if t.IsZero() {
				t = time.Now()
			}
			m.history = append(m.history, toastEntry{Text: e.Details, Err: isErr, Stamp: t})
		}
		// Truncate to historyMax.
		if len(m.history) > m.historyMax {
			m.history = m.history[len(m.history)-m.historyMax:]
		}
		return m, nil

	case historyClearedMsg:
		if msg.Err != nil {
			return m, m.showToast(toast.KindError,
				i18n.Tf("tui.toast.error_prefix", map[string]any{"msg": msg.Err.Error()}))
		}
		m.history = nil
		return m, m.showToast(toast.KindSuccess, i18n.T("tui.toast.history_cleared"))

	case switchScreenMsg:
		m.prev = m.current
		m.current = msg.Screen
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	// route messages to current screen
	if m.current == ScreenDashboard && m.dashboard != nil {
		_, cmd := m.dashboard.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys handled regardless of screen.
	switch {
	case keyMatches(msg, m.keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, m.keys.Help):
		m.helpVisible = !m.helpVisible
		return m, nil
	case keyMatches(msg, m.keys.History):
		if m.current == ScreenHistory {
			m.current = ScreenDashboard
		} else {
			m.prev = m.current
			m.current = ScreenHistory
		}
		return m, nil
	case keyMatches(msg, m.keys.Theme):
		m.st = theme.New(theme.Cycle(m.st.Palette))
		if m.dashboard != nil {
			m.dashboard.SetStyles(m.st)
		}
		return m, nil
	case keyMatches(msg, m.keys.Esc):
		if m.helpVisible {
			m.helpVisible = false
			return m, nil
		}
		// Settings handles its own Esc (dirty-state confirm). Let it through.
		if m.current == ScreenSettings {
			break
		}
		if m.current != ScreenDashboard {
			m.current = ScreenDashboard
			return m, nil
		}
	}

	// Per-screen routing.
	switch m.current {
	case ScreenDashboard:
		return m.handleDashboardKey(msg)
	case ScreenPalette:
		var cmd tea.Cmd
		m.palette, cmd = m.palette.Update(msg)
		return m, cmd
	case ScreenConfirm:
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		return m, cmd
	case ScreenDiff:
		var cmd tea.Cmd
		m.diff, cmd = m.diff.Update(msg)
		return m, cmd
	case ScreenSettings:
		var cmd tea.Cmd
		m.settings, cmd = m.settings.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Refresh):
		return m, m.loadDataCmd()
	case keyMatches(msg, m.keys.Reload):
		return m, m.reloadAllCmd()
	case keyMatches(msg, m.keys.Sync):
		return m, m.syncPushCmd()
	case keyMatches(msg, m.keys.Palette):
		m.palette = newPalette(m.keys, m.st, m.str, m.paletteActions())
		m.palette.Resize(m.width, m.height)
		m.current = ScreenPalette
		return m, nil
	case keyMatches(msg, m.keys.Diff):
		m.diff = newDiffScreen(m.keys, m.st, m.str, m.drift)
		m.diff.Resize(m.width, m.height-2)
		m.current = ScreenDiff
		return m, nil
	case keyMatches(msg, m.keys.NewSession):
		return m, m.newSessionCmd()
	case keyMatches(msg, m.keys.Kill):
		target := m.dashboard.SelectedTarget()
		if target == "" {
			return m, nil
		}
		m.confirm = newConfirm(m.keys, m.st, m.str,
			i18n.Tf("tui.modal.kill.title_fmt", map[string]any{"target": target}),
			m.str.Modal.KillBody,
			func() tea.Cmd { return m.killTargetCmd(target) },
		)
		m.confirm.Resize(m.width, m.height)
		m.current = ScreenConfirm
		return m, nil
	case keyMatches(msg, m.keys.Undo):
		return m, m.undoCmd()
	case keyMatches(msg, m.keys.Settings):
		m.settings = newSettings(m.keys, m.st, m.str,
			m.cfg,
			m.deps.ConfigPath,
			func(p theme.Palette) tea.Cmd {
				m.st = theme.New(p)
				if m.dashboard != nil {
					m.dashboard.SetStyles(m.st)
				}
				if m.settings != nil {
					m.settings.SetStyles(m.st)
				}
				return nil
			},
			m.applyLanguage,
			func(d time.Duration) {
				if d <= 0 {
					// "off" — stop the refresher by setting a very large interval.
					m.paneRefresher.SetInterval(24 * time.Hour)
				} else {
					m.paneRefresher.SetInterval(d)
				}
			},
		)
		m.settings.Resize(m.width, m.height)
		m.current = ScreenSettings
		return m, nil
	case keyMatches(msg, m.keys.Attach), keyMatches(msg, m.keys.Enter):
		target := m.dashboard.SelectedTarget()
		if target == "" {
			return m, nil
		}
		return m, tea.Sequence(
			attachCmd(m.deps.Runner, m.deps.Runner.InTmux(), target),
			m.loadDataCmd(),
		)
	}
	_, cmd := m.dashboard.Update(msg)
	return m, tea.Batch(cmd, m.maybeLoadPreview())
}

// View renders the active screen + persistent overlays (toast, help).
func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	var body string
	switch m.current {
	case ScreenDashboard:
		body = m.renderDashboardScreen()
	case ScreenError:
		body = m.renderErrorScreen()
	case ScreenEmpty:
		body = m.renderEmptyScreen()
	case ScreenPalette:
		if m.palette != nil {
			body = m.palette.View()
		}
	case ScreenConfirm:
		if m.confirm != nil {
			body = m.confirm.View()
		}
	case ScreenDiff:
		if m.diff != nil {
			body = lipgloss.JoinVertical(lipgloss.Left,
				m.renderHeader(), m.diff.View(), m.renderFooter())
		}
	case ScreenSettings:
		if m.settings != nil {
			body = m.settings.View()
		}
	case ScreenHistory:
		body = m.renderHistory()
	default:
		body = m.dashboard.View()
	}

	if m.helpVisible {
		body = m.overlayHelp(body)
	}
	return body
}

func (m *Model) renderDashboardScreen() string {
	header := m.renderHeader()
	footer := m.renderFooter()
	body := m.dashboard.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *Model) renderHeader() string {
	left := m.st.Header.Render(fmt.Sprintf("tmh · %s", m.deps.ConfigPath))
	driftCount := 0
	for _, d := range m.drift {
		if d.Status != config.StatusOK {
			driftCount++
		}
	}
	right := ""
	if driftCount > 0 {
		right = m.st.StatusDrift.Render(i18n.Tf("tui.header.drift_badge", map[string]any{"count": driftCount}))
	}
	gap := strings.Repeat(" ", maxInt(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)))
	return left + gap + right
}

func (m *Model) renderFooter() string {
	hints := []string{
		m.st.KeyBinding.Render("a") + " " + m.str.Footer.Attach,
		m.st.KeyBinding.Render("n") + " " + m.str.Footer.NewSession,
		m.st.KeyBinding.Render("d") + " " + m.str.Footer.Kill,
		m.st.KeyBinding.Render("R") + " " + m.str.Footer.Dotfiles,
		m.st.KeyBinding.Render("s") + " " + m.str.Footer.Sync,
		m.st.KeyBinding.Render("S") + " " + m.str.Footer.Settings,
		m.st.KeyBinding.Render(":") + " " + m.str.Footer.Palette,
		m.st.KeyBinding.Render("^L") + " " + m.str.Footer.History,
		m.st.KeyBinding.Render("?") + " " + m.str.Footer.Help,
		m.st.KeyBinding.Render("q") + " " + m.str.Footer.Quit,
	}
	hintsStr := strings.Join(hints, " · ")

	// Footer heatmap (Variant 14): "live N · idle N" shown on the right when
	// enabled in Display settings.
	heatmap := ""
	if m.cfg != nil && m.cfg.Defaults.Display.ShowFooterHeatmap && m.paneProvider != nil {
		live, idle := m.paneProvider.Stats()
		heatmap = m.st.Hint.Render(fmt.Sprintf("live %d · idle %d", live, idle))
	}

	if m.toast == "" && heatmap == "" || m.width == 0 {
		return m.st.Footer.Render(hintsStr)
	}
	if m.toast == "" {
		// Only heatmap — right-align it.
		contentW := m.width - 2
		heatW := lipgloss.Width(heatmap)
		hintsW := contentW - heatW - 1
		if hintsW < 0 {
			hintsW = 0
		}
		line := truncate(hintsStr, hintsW) + strings.Repeat(" ", maxInt(1, contentW-lipgloss.Width(truncate(hintsStr, hintsW))-heatW)) + heatmap
		return m.st.Footer.Render(line)
	}
	// Inline toast right-aligned on the same footer row. We truncate hints
	// from the right so the toast always fits; the full hint set stays
	// visible in `?` help and in the palette.
	toastStyle := m.st.Toast
	switch m.toastKind {
	case toast.KindSuccess:
		toastStyle = m.st.ToastSuccess
	case toast.KindError:
		toastStyle = m.st.ToastError
	}
	toast := toastStyle.Render(m.toast)
	contentW := m.width - 2 // Footer has Padding(0, 1)
	toastW := lipgloss.Width(toast)
	hintsW := contentW - toastW - 1
	if hintsW < 0 {
		hintsW = 0
	}
	line := truncate(hintsStr, hintsW) + strings.Repeat(" ", maxInt(1, contentW-lipgloss.Width(truncate(hintsStr, hintsW))-toastW)) + toast
	return m.st.Footer.Render(line)
}

func (m *Model) renderErrorScreen() string {
	mb := modalBg(m.st.Palette)
	rowW := maxInt(40, m.width-12)
	title := m.st.StatusGone.Inherit(mb).Render(m.str.Modal.ErrorTitle)
	var b strings.Builder
	b.WriteString(modalRow(m.st.Palette, rowW, title))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	for _, line := range strings.Split(m.errMsg, "\n") {
		b.WriteString(modalRow(m.st.Palette, rowW, mb.Render(line)))
		b.WriteString("\n")
	}
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, mb.Render(m.str.Modal.ErrorDismiss)))
	return placeMiddle(m.width, m.height, m.st.Modal.Render(b.String()), m.st.Palette)
}

func (m *Model) renderEmptyScreen() string {
	mb := modalBg(m.st.Palette)
	rowW := maxInt(40, m.width-12)
	var b strings.Builder
	b.WriteString(modalRow(m.st.Palette, rowW, mb.Render(m.str.Modal.EmptyTitle)))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, mb.Render(m.str.Modal.EmptyHint)))
	return placeMiddle(m.width, m.height, m.st.Modal.Render(b.String()), m.st.Palette)
}

func (m *Model) overlayHelp(body string) string {
	// helpText already produces modalRow-padded lines; just wrap with the
	// Modal border + bg and centre it on the screen.
	_ = body
	return placeMiddle(m.width, m.height, m.st.Modal.Render(m.helpText()), m.st.Palette)
}

func (m *Model) helpText() string {
	km := m.str.Keymap
	sections := []struct {
		title string
		rows  [][2]string
	}{
		{km.SectionNav, [][2]string{
			{"j / k / ↑↓", km.NavUpdown},
			{"h / l", km.NavCollapse},
			{"g / G", km.NavTopBottom},
			{"PgUp / PgDn", km.NavPage},
		}},
		{km.SectionActions, [][2]string{
			{"enter / a", km.ActionAttach},
			{"n", km.ActionNew},
			{"d", km.ActionKill},
			{"u", km.ActionUndo},
		}},
		{km.SectionSync, [][2]string{
			{"r", km.SyncRefresh},
			{"R", km.SyncReload},
			{"s", km.SyncPush},
			{"D", km.SyncDiff},
		}},
		{km.SectionOther, [][2]string{
			{": / Ctrl+P", km.OtherPalette},
			{"S", km.OtherSettings},
			{"? / esc", km.OtherHelp},
			{"q / Ctrl+C", km.OtherQuit},
		}},
	}
	mb := modalBg(m.st.Palette)
	title := m.st.Title.Inherit(mb)
	subtitle := m.st.Subtitle.Inherit(mb)
	keyBind := m.st.KeyBinding.Inherit(mb)
	plain := mb

	// Width budget: modal content area (screen width minus border + padding).
	rowW := maxInt(40, m.width-12)

	var b strings.Builder
	b.WriteString(modalRow(m.st.Palette, rowW, title.Render(km.Title)))
	b.WriteString("\n")
	b.WriteString(modalRow(m.st.Palette, rowW, ""))
	b.WriteString("\n")
	for si, sec := range sections {
		b.WriteString(modalRow(m.st.Palette, rowW, subtitle.Render(sec.title)))
		b.WriteString("\n")
		for _, row := range sec.rows {
			line := plain.Render("  ") + keyBind.Render(row[0]) + plain.Render("   "+row[1])
			b.WriteString(modalRow(m.st.Palette, rowW, line))
			b.WriteString("\n")
		}
		if si < len(sections)-1 {
			b.WriteString(modalRow(m.st.Palette, rowW, ""))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- commands ---

// loadHistoryCmd asynchronously reads persistent history from disk.
func (m *Model) loadHistoryCmd() tea.Cmd {
	if m.historyStore == nil {
		return nil
	}
	hs := m.historyStore
	return func() tea.Msg {
		entries, err := hs.Load()
		if err != nil {
			return historyLoadedMsg{Err: err}
		}
		disk := make([]historyDiskEntry, 0, len(entries))
		for _, e := range entries {
			disk = append(disk, historyDiskEntry{
				Ts: e.Ts, Action: e.Action, Target: e.Target,
				Result: e.Result, Details: e.Details,
			})
		}
		return historyLoadedMsg{Entries: disk}
	}
}

// appendHistoryCmd persists a single action to disk asynchronously.
func (m *Model) appendHistoryCmd(action, target, result, details string) tea.Cmd {
	if m.historyStore == nil {
		return nil
	}
	hs := m.historyStore
	e := appstate.HistoryEntry{
		Ts: time.Now().UTC().Format(time.RFC3339),
		Action:  action,
		Target:  target,
		Result:  result,
		Details: details,
	}
	return func() tea.Msg {
		_ = hs.Append(e) // best-effort; errors are not surfaced for writes
		return nil
	}
}

// clearHistoryCmd wipes all persistent history.
func (m *Model) clearHistoryCmd() tea.Cmd {
	if m.historyStore == nil {
		return func() tea.Msg { return historyClearedMsg{} }
	}
	hs := m.historyStore
	return func() tea.Msg {
		archivePath, err := hs.Clear()
		return historyClearedMsg{ArchivePath: archivePath, Err: err}
	}
}

func (m *Model) loadDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cfg, err := m.deps.LoadConfig()
		if err != nil {
			cfg = &config.Config{Version: 1}
		}
		listing, err := actions.BuildListing(ctx, m.deps.Runner, cfg, m.deps.Profile)
		if err != nil {
			return dataLoadedMsg{Err: err}
		}
		resolved, err := config.Resolve(cfg, m.deps.Profile)
		if err != nil {
			resolved = &config.Resolved{}
		}
		snap, err := collectLive(ctx, m.deps.Runner)
		if err != nil {
			return dataLoadedMsg{Err: err}
		}
		drift := config.Diff(resolved, snap)
		m.cfg = cfg
		return dataLoadedMsg{Listing: listing, Drift: drift}
	}
}

func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.pollEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) reloadAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := actions.Reload(ctx, m.deps.Runner, m.deps.State, "~/.zshrc",
			actions.ReloadOptions{Shell: true, Tmux: true, Busy: true})
		if err != nil {
			return errorMsg{Err: err}
		}
		return actionDoneMsg{Text: m.str.Toast.ReloadTriggered}
	}
}

func (m *Model) syncPushCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg, err := m.deps.LoadConfig()
		if err != nil {
			return errorMsg{Err: err}
		}
		rep, err := actions.Push(ctx, m.deps.Runner, cfg, actions.SyncOptions{Profile: m.deps.Profile})
		if err != nil {
			return errorMsg{Err: err}
		}
		text := i18n.Tf("tui.toast.sync_report", map[string]any{
			"created": len(rep.Created),
			"updated": len(rep.Updated),
		})
		return actionDoneMsg{Text: text}
	}
}

// attachCmd hands the controlling terminal over to tmux for an
// attach/switch-client. tea.ExecProcess properly suspends the bubbletea
// event loop, restores the alt-screen on return, and gives the child
// process direct access to stdin/stdout/stderr — without this, tmux
// receives a useless pipe and the user can't type into the attached
// session.
func attachCmd(r tmux.Runner, inTmux bool, target string) tea.Cmd {
	args := []string{"attach-session", "-t", target}
	if inTmux {
		// switch-client doesn't take over the terminal; it sends a tmux
		// command to the running client, then returns immediately. Run via
		// runner so the parent process keeps its TTY.
		return func() tea.Msg {
			_ = r.SwitchClient(context.Background(), target)
			return nil
		}
	}
	cmd := exec.Command("tmux", args...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return errorMsg{Err: fmt.Errorf("attach: %w", err)}
		}
		return nil
	})
}

func (m *Model) killTargetCmd(target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		// Snapshot before kill so undo can restore.
		if m.deps.State != nil {
			if live, err := actions.CaptureLive(ctx, m.deps.Runner); err == nil {
				for _, s := range live {
					if s.Name == target {
						payload, _ := jsonMarshal(s)
						_, _ = m.deps.State.InsertEvent(ctx, "kill_session", target, string(payload))
						break
					}
				}
			}
		}
		if err := m.deps.Runner.KillSession(ctx, target); err != nil {
			return errorMsg{Err: err}
		}
		killed := i18n.Tf("tui.toast.session_killed", map[string]any{"name": target})
		return tea.Batch(
			func() tea.Msg { return actionDoneMsg{Text: killed} },
			m.loadDataCmd(),
			func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} },
		)()
	}
}

func (m *Model) undoCmd() tea.Cmd {
	return func() tea.Msg {
		if m.deps.State == nil {
			return errorMsg{Err: fmt.Errorf("%s", m.str.Toast.UndoUnavailable)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		target, err := actions.UndoLast(ctx, m.deps.Runner, m.deps.State)
		if err != nil {
			return errorMsg{Err: err}
		}
		restored := i18n.Tf("tui.toast.session_restored", map[string]any{"name": target})
		return tea.Batch(
			func() tea.Msg { return actionDoneMsg{Text: restored} },
			m.loadDataCmd(),
		)()
	}
}

// newSessionCmd launches the `tmh new` wizard as a subprocess. Bubbletea
// owns the controlling TTY in alt-screen mode, so huh can't render its form
// inside this process; tea.ExecProcess suspends the event loop, hands the
// terminal over to the child, then restores the alt-screen when the child
// exits. The next polling tick picks up any newly-created session.
func (m *Model) newSessionCmd() tea.Cmd {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = os.Args[0]
	}
	cmd := exec.Command(exe, "new")
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return errorMsg{Err: fmt.Errorf("new: %w", err)}
		}
		return actionDoneMsg{Text: i18n.T("tui.toast.session_created")}
	})
}

// initCmd runs actions.Init so the palette can create every configured
// session that isn't already live. Toasts success count or error.
func (m *Model) initCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cfg, err := m.deps.LoadConfig()
		if err != nil {
			return errorMsg{Err: err}
		}
		if err := actions.Init(ctx, m.deps.Runner, cfg, actions.InitOptions{Profile: m.deps.Profile}); err != nil {
			return errorMsg{Err: err}
		}
		return actionDoneMsg{Text: "init: " + i18n.T("tui.toast.reload_triggered")}
	}
}

// snapshotSaveCmd captures the current live state under an auto-timestamped
// name (tmh-YYYYMMDD-HHMMSS). The user can inspect / restore via CLI.
func (m *Model) snapshotSaveCmd() tea.Cmd {
	return func() tea.Msg {
		if m.deps.State == nil {
			return errorMsg{Err: fmt.Errorf("%s", m.str.Toast.UndoUnavailable)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		name := "tmh-" + time.Now().Format("20060102-150405")
		if err := actions.SaveSnapshot(ctx, m.deps.Runner, m.deps.State, name); err != nil {
			return errorMsg{Err: err}
		}
		return actionDoneMsg{Text: "snapshot: " + name}
	}
}

// doctorCmd runs the tmux audit in-process and pushes a one-line summary
// (✓n ⚠n ✗n) to the history so the palette user can inspect results.
func (m *Model) doctorCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		findings := actions.AuditTmuxConfig(ctx, m.deps.Runner)
		var ok, warn, errs int
		for _, f := range findings {
			switch f.Level {
			case actions.AuditOK:
				ok++
			case actions.AuditWarn:
				warn++
			case actions.AuditError:
				errs++
			}
		}
		text := fmt.Sprintf("doctor: ✓%d ⚠%d ✗%d", ok, warn, errs)
		return actionDoneMsg{Text: text}
	}
}

// maybeLoadPreview triggers an async CapturePane for the current selection
// when the dashboard's cached preview doesn't match. Returns nil when no
// fetch is needed (no selection, or cache is fresh).
func (m *Model) maybeLoadPreview() tea.Cmd {
	if m.dashboard == nil {
		return nil
	}
	target, stale := m.dashboard.PreviewStale()
	if !stale || target == "" {
		return nil
	}
	return m.loadPreviewCmd(target)
}

func (m *Model) loadPreviewCmd(target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		// tmux accepts `session` or `session:window`; for session-level rows
		// we capture the active window's first pane.
		data, err := m.deps.Runner.CapturePane(ctx, target, 200)
		if err != nil {
			return previewLoadedMsg{Target: target, Err: err}
		}
		return previewLoadedMsg{Target: target, Data: string(data)}
	}
}

// applyLanguage switches the localizer to lang, rebuilds the in-memory
// UIStrings bundle, propagates it to long-lived sub-models, and persists the
// choice to config.yml so subsequent launches inherit it. Called from the
// settings screen language selector.
func (m *Model) applyLanguage(lang string) tea.Cmd {
	if err := i18n.Init(lang); err != nil {
		return func() tea.Msg { return errorMsg{Err: err} }
	}
	m.str = LoadStrings()
	if m.dashboard != nil {
		m.dashboard.SetStrings(m.str)
	}
	if m.settings != nil {
		m.settings.SetStrings(m.str)
	}
	// Persist defaults.lang; non-fatal if the config file is missing.
	cfg, err := config.Load(m.deps.ConfigPath)
	if err != nil {
		return nil
	}
	if err := config.PathSet(cfg.Node, "defaults.lang", lang); err != nil {
		return func() tea.Msg { return errorMsg{Err: err} }
	}
	if err := config.Write(cfg, m.deps.ConfigPath, config.WriteOptions{PreserveBlanks: true}); err != nil {
		return func() tea.Msg { return errorMsg{Err: err} }
	}
	return nil
}

// paletteActions builds the command list that the `:` palette filters.
func (m *Model) paletteActions() []PaletteAction {
	out := []PaletteAction{
		{Title: i18n.T("tui.palette.action.refresh.title"), Subtitle: i18n.T("tui.palette.action.refresh.subtitle"), Run: func() tea.Cmd { return m.loadDataCmd() }},
		{Title: i18n.T("tui.palette.action.reload.title"), Subtitle: i18n.T("tui.palette.action.reload.subtitle"), Run: func() tea.Cmd { return m.reloadAllCmd() }},
		{Title: i18n.T("tui.palette.action.sync.title"), Subtitle: i18n.T("tui.palette.action.sync.subtitle"), Run: func() tea.Cmd { return m.syncPushCmd() }},
		{Title: i18n.T("tui.palette.action.init.title"), Subtitle: i18n.T("tui.palette.action.init.subtitle"), Run: func() tea.Cmd { return m.initCmd() }},
		{Title: i18n.T("tui.palette.action.diff.title"), Subtitle: i18n.T("tui.palette.action.diff.subtitle"), Run: func() tea.Cmd {
			m.diff = newDiffScreen(m.keys, m.st, m.str, m.drift)
			m.diff.Resize(m.width, m.height-2)
			return func() tea.Msg { return switchScreenMsg{Screen: ScreenDiff} }
		}},
		{Title: i18n.T("tui.palette.action.snapshot_save.title"), Subtitle: i18n.T("tui.palette.action.snapshot_save.subtitle"), Run: func() tea.Cmd { return m.snapshotSaveCmd() }},
		{Title: i18n.T("tui.palette.action.undo.title"), Subtitle: i18n.T("tui.palette.action.undo.subtitle"), Run: func() tea.Cmd { return m.undoCmd() }},
		{Title: i18n.T("tui.palette.action.settings.title"), Subtitle: i18n.T("tui.palette.action.settings.subtitle"), Run: func() tea.Cmd {
			return func() tea.Msg { return switchScreenMsg{Screen: ScreenSettings} }
		}},
		{Title: i18n.T("tui.palette.action.tmux_audit.title"), Subtitle: i18n.T("tui.palette.action.tmux_audit.subtitle"), Run: func() tea.Cmd {
			return func() tea.Msg { return switchScreenMsg{Screen: ScreenSettings} }
		}},
		{Title: i18n.T("tui.palette.action.doctor.title"), Subtitle: i18n.T("tui.palette.action.doctor.subtitle"), Run: func() tea.Cmd { return m.doctorCmd() }},
		{Title: i18n.T("tui.palette.action.history.title"), Subtitle: i18n.T("tui.palette.action.history.subtitle"), Run: func() tea.Cmd {
			return func() tea.Msg { return switchScreenMsg{Screen: ScreenHistory} }
		}},
		{Title: i18n.T("tui.palette.action.theme.title"), Subtitle: i18n.T("tui.palette.action.theme.subtitle"), Run: func() tea.Cmd {
			m.st = theme.New(theme.Cycle(m.st.Palette))
			if m.dashboard != nil {
				m.dashboard.SetStyles(m.st)
			}
			return nil
		}},
		{Title: i18n.T("tui.palette.action.quit.title"), Subtitle: i18n.T("tui.palette.action.quit.subtitle"), Run: func() tea.Cmd { return tea.Quit }},
	}
	if m.listing != nil {
		for _, s := range m.listing.Sessions {
			s := s
			out = append(out, PaletteAction{
				Title:    i18n.Tf("tui.palette.action.attach.title", map[string]any{"name": s.Name}),
				Subtitle: i18n.Tf("tui.palette.action.attach.subtitle", map[string]any{"count": len(s.Windows)}),
				Run:      func() tea.Cmd { return tea.Sequence(attachCmd(m.deps.Runner, m.deps.Runner.InTmux(), s.Name), m.loadDataCmd()) },
			})
		}
	}
	return out
}
