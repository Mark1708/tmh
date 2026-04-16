package ui

import (
	"time"

	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/ui/errrender"
	"git.mark1708.ru/me/tmh/internal/ui/refresh"
	"git.mark1708.ru/me/tmh/internal/ui/theme"
	"git.mark1708.ru/me/tmh/internal/ui/toast"

	tea "github.com/charmbracelet/bubbletea"
)

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
		if msg.Cfg != nil {
			m.cfg = msg.Cfg
			if m.dashboard != nil {
				paneBase := 0
				if m.cfg.Defaults.TmuxIntegration.PaneBaseIndex != nil {
					paneBase = *m.cfg.Defaults.TmuxIntegration.PaneBaseIndex
				}
				m.dashboard.SetPaneBaseIndex(paneBase)
			}
		}
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

	case undoHintMsg:
		m.undoHint = msg.Text
		return m, nil

	case pendingOpExpiredMsg:
		// Cancel the pending mark operation only if it matches the current one.
		if m.pendingOp == msg.Op {
			m.pendingOp = 0
		}
		return m, nil

	case gotoProcMsg:
		// Jump to the pane found by gotoProcCmd — runs on the main goroutine so
		// dashboard and marksStore accesses are safe.
		if m.marksStore != nil && m.dashboard != nil {
			if cur := m.dashboard.SelectedTarget(); cur != "" {
				m.marksStore.PushLocation(cur, m.dashboard.effectiveCursor())
			}
		}
		if m.dashboard != nil {
			m.dashboard.restoreCursorByID(msg.Target)
		}
		m.current = ScreenDashboard
		return m, m.maybeLoadPreview()

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
	// ── Two-step mark operations (4.1 + 4.2) ──────────────────────────────
	// If a pending operation is live, consume the next key as the mark letter.
	if m.pendingOp != 0 && time.Now().Before(m.pendingOpExpiry) {
		op := m.pendingOp
		m.pendingOp = 0
		key := msg.String()
		if len([]rune(key)) == 1 {
			letter := []rune(key)[0]
			switch op {
			case 'm':
				// Set mark at current position.
				target := m.dashboard.SelectedTarget()
				cursor := m.dashboard.effectiveCursor()
				if target != "" && m.marksStore != nil {
					m.marksStore.SetMark(letter, target, cursor)
					return m, m.showToast(toast.KindSuccess,
						i18n.Tf("tui.toast.mark_set", map[string]any{"letter": string(letter)}))
				}
				return m, nil
			case '\'':
				// Jump to mark.
				if m.marksStore != nil {
					mark, ok := m.marksStore.GetMark(letter)
					if !ok {
						return m, m.showToast(toast.KindError,
							i18n.Tf("tui.toast.mark_not_found", map[string]any{"letter": string(letter)}))
					}
					// Push current location before jumping.
					if cur := m.dashboard.SelectedTarget(); cur != "" {
						m.marksStore.PushLocation(cur, m.dashboard.effectiveCursor())
					}
					m.dashboard.restoreCursorByID(mark.Target)
					return m, m.maybeLoadPreview()
				}
				return m, nil
			}
		}
		// Unrecognised second key — cancel silently and fall through.
	}
	m.pendingOp = 0

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
		level := m.dashboard.SelectedLevel()
		var killCmd func() tea.Cmd
		switch level {
		case levelPane:
			killCmd = func() tea.Cmd { return m.killPaneCmd(target) }
		case levelWindow:
			killCmd = func() tea.Cmd { return m.killWindowCmd(target) }
		default:
			killCmd = func() tea.Cmd { return m.killTargetCmd(target) }
		}
		m.confirm = newConfirm(m.keys, m.st, m.str,
			i18n.Tf("tui.modal.kill.title_fmt", map[string]any{"target": target}),
			m.str.Modal.KillBody,
			killCmd,
		)
		// Provide a dry-run description (4.6).
		m.confirm.DryRunDesc = "would kill " + target
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
		// Push current location before attaching so '' can bring the user back.
		if m.marksStore != nil {
			m.marksStore.PushLocation(target, m.dashboard.effectiveCursor())
		}
		return m, tea.Sequence(
			attachCmd(m.deps.Runner, m.deps.Runner.InTmux(), target),
			m.loadDataCmd(),
		)

	case msg.String() == "''" || keyMatches(msg, m.keys.PrevLoc):
		// Pop last location and jump to it (vi-style: swap current ↔ prev).
		if m.marksStore == nil || !m.marksStore.HasLastLocation() {
			return m, m.showToast(toast.KindInfo, i18n.T("tui.toast.no_prev_location"))
		}
		cur := m.dashboard.SelectedTarget()
		loc, ok := m.marksStore.PopLocation()
		if !ok {
			return m, nil
		}
		// Push the current position only when it differs from the destination so
		// repeated '' presses cycle back and forth without filling the ring with
		// duplicates.
		if cur != "" && cur != loc.Target {
			m.marksStore.PushLocation(cur, m.dashboard.effectiveCursor())
		}
		m.dashboard.restoreCursorByID(loc.Target)
		return m, m.maybeLoadPreview()

	case msg.String() == "m" && m.marksStore != nil:
		// Begin two-step set-mark.
		m.pendingOp = 'm'
		m.pendingOpExpiry = time.Now().Add(2 * time.Second)
		op := m.pendingOp
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return pendingOpExpiredMsg{Op: op} })

	case msg.String() == "'" && m.marksStore != nil:
		// Begin two-step jump-to-mark (single apostrophe, not double).
		m.pendingOp = '\''
		m.pendingOpExpiry = time.Now().Add(2 * time.Second)
		op := m.pendingOp
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return pendingOpExpiredMsg{Op: op} })
	}
	_, cmd := m.dashboard.Update(msg)
	return m, tea.Batch(cmd, m.maybeLoadPreview())
}
