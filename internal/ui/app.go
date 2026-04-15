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
	"os/exec"
	"strings"
	"time"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// Deps wires the side-effect surface the UI needs. Tests pass a MockRunner
// here; production passes CLIRunner + the real config path.
type Deps struct {
	Runner     tmux.Runner
	State      *state.DB
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

	toast    string
	toastEnd time.Time

	pollEvery time.Duration
}

// New constructs the root model.
func New(deps Deps) *Model {
	keys := DefaultKeys()
	st := theme.New(theme.Mocha)
	str := LoadStrings()
	return &Model{
		deps:      deps,
		keys:      keys,
		st:        st,
		str:       str,
		current:   ScreenDashboard,
		dashboard: newDashboard(keys, st, str),
		pollEvery: 2 * time.Second,
	}
}

// Init triggers the first data load + polling tick.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadDataCmd(), m.tickCmd())
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

	case dataLoadedMsg:
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
			m.current = ScreenError
			return m, nil
		}
		m.listing = msg.Listing
		m.drift = msg.Drift
		if m.dashboard != nil {
			m.dashboard.SetData(msg.Listing, msg.Drift)
		}
		return m, nil

	case toastMsg:
		m.toast = msg.Text
		ttl := msg.TTL
		if ttl == 0 {
			ttl = 2 * time.Second
		}
		m.toastEnd = time.Now().Add(ttl)
		return m, tea.Tick(ttl, func(time.Time) tea.Msg { return toastExpiredMsg{} })

	case toastExpiredMsg:
		if !time.Now().Before(m.toastEnd) {
			m.toast = ""
		}
		return m, nil

	case errorMsg:
		m.toast = i18n.Tf("tui.toast.error_prefix", map[string]any{"msg": msg.Err.Error()})
		m.toastEnd = time.Now().Add(3 * time.Second)
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return toastExpiredMsg{} })

	case actionDoneMsg:
		m.toast = msg.Text
		m.toastEnd = time.Now().Add(2 * time.Second)
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return toastExpiredMsg{} })

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
		m.settings = newSettings(m.keys, m.st, m.str, func(p theme.Palette) tea.Cmd {
			m.st = theme.New(p)
			if m.dashboard != nil {
				m.dashboard.SetStyles(m.st)
			}
			if m.settings != nil {
				m.settings.SetStyles(m.st)
			}
			return nil
		})
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
	return m, cmd
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
	default:
		body = m.dashboard.View()
	}

	if m.helpVisible {
		body = m.overlayHelp(body)
	}
	if m.toast != "" {
		body = m.overlayToast(body)
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
		m.st.KeyBinding.Render("R") + " " + m.str.Footer.Dotfiles,
		m.st.KeyBinding.Render("s") + " " + m.str.Footer.Sync,
		m.st.KeyBinding.Render("S") + " " + m.str.Footer.Settings,
		m.st.KeyBinding.Render(":") + " " + m.str.Footer.Palette,
		m.st.KeyBinding.Render("?") + " " + m.str.Footer.Help,
		m.st.KeyBinding.Render("q") + " " + m.str.Footer.Quit,
	}
	return m.st.Footer.Render(strings.Join(hints, " · "))
}

func (m *Model) renderErrorScreen() string {
	body := padBlock(m.st.StatusGone.Render(m.str.Modal.ErrorTitle) + "\n\n" + m.errMsg + "\n\n" + m.str.Modal.ErrorDismiss)
	return placeMiddle(m.width, m.height, m.st.Modal.Render(body), m.st.Palette)
}

func (m *Model) renderEmptyScreen() string {
	body := padBlock(m.str.Modal.EmptyTitle + "\n\n" + m.str.Modal.EmptyHint)
	return placeMiddle(m.width, m.height, m.st.Modal.Render(body), m.st.Palette)
}

func (m *Model) overlayToast(body string) string {
	t := m.st.Toast.Render(m.toast)
	return overlayBottomRight(body, t, m.width, m.height)
}

func (m *Model) overlayHelp(body string) string {
	// Pad every line to the widest so the modal's Background fills the
	// whole block — otherwise shorter rows leak through whatever is behind.
	help := padBlock(m.helpText())
	return placeMiddle(m.width, m.height, m.st.Modal.Render(help), m.st.Palette)
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
	var b strings.Builder
	b.WriteString(m.st.Title.Render(km.Title))
	b.WriteString("\n\n")
	for si, sec := range sections {
		b.WriteString(m.st.Subtitle.Render(sec.title))
		b.WriteString("\n")
		for _, row := range sec.rows {
			b.WriteString("  " + m.st.KeyBinding.Render(row[0]))
			b.WriteString("   " + row[1] + "\n")
		}
		if si < len(sections)-1 {
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- commands ---

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

// paletteActions builds the command list that the `:` palette filters.
func (m *Model) paletteActions() []PaletteAction {
	out := []PaletteAction{
		{Title: i18n.T("tui.palette.action.refresh.title"), Subtitle: i18n.T("tui.palette.action.refresh.subtitle"), Run: func() tea.Cmd { return m.loadDataCmd() }},
		{Title: i18n.T("tui.palette.action.reload.title"), Subtitle: i18n.T("tui.palette.action.reload.subtitle"), Run: func() tea.Cmd { return m.reloadAllCmd() }},
		{Title: i18n.T("tui.palette.action.sync.title"), Subtitle: i18n.T("tui.palette.action.sync.subtitle"), Run: func() tea.Cmd { return m.syncPushCmd() }},
		{Title: i18n.T("tui.palette.action.diff.title"), Subtitle: i18n.T("tui.palette.action.diff.subtitle"), Run: func() tea.Cmd {
			m.diff = newDiffScreen(m.keys, m.st, m.str, m.drift)
			m.diff.Resize(m.width, m.height-2)
			return func() tea.Msg { return switchScreenMsg{Screen: ScreenDiff} }
		}},
		{Title: i18n.T("tui.palette.action.undo.title"), Subtitle: i18n.T("tui.palette.action.undo.subtitle"), Run: func() tea.Cmd { return m.undoCmd() }},
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
