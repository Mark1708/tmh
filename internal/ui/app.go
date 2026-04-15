// Package ui hosts the bubbletea application that powers `tmh` (no args).
//
// The model is a thin router: each screen is a sub-model with its own
// Update/View. Heavy work lives in internal/actions; the UI never calls
// tmux directly outside of polling tmux.Runner via the same actions.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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

	width, height int

	cfg     *config.Config
	listing *actions.Listing
	drift   []config.Drift

	current     Screen
	prev        Screen
	dashboard   *dashboardModel
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
	return &Model{
		deps:      deps,
		keys:      keys,
		st:        st,
		current:   ScreenDashboard,
		dashboard: newDashboard(keys, st),
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
		m.toast = "error: " + msg.Err.Error()
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
	case keyMatches(msg, m.keys.Attach):
		target := m.dashboard.SelectedTarget()
		if target == "" {
			return m, nil
		}
		return m, tea.Sequence(
			tea.ExitAltScreen,
			attachCmd(m.deps.Runner, target),
			tea.EnterAltScreen,
			m.loadDataCmd(),
		)
	case keyMatches(msg, m.keys.Enter):
		// Same as attach when in dashboard.
		target := m.dashboard.SelectedTarget()
		if target == "" {
			return m, nil
		}
		return m, tea.Sequence(
			tea.ExitAltScreen,
			attachCmd(m.deps.Runner, target),
			tea.EnterAltScreen,
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
		right = m.st.StatusDrift.Render(fmt.Sprintf("⚠ drift:%d", driftCount))
	}
	gap := strings.Repeat(" ", maxInt(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)))
	return left + gap + right
}

func (m *Model) renderFooter() string {
	hints := []string{
		m.st.KeyBinding.Render("a") + " attach",
		m.st.KeyBinding.Render("R") + " reload",
		m.st.KeyBinding.Render("s") + " sync",
		m.st.KeyBinding.Render("r") + " refresh",
		m.st.KeyBinding.Render("?") + " help",
		m.st.KeyBinding.Render("^T") + " theme",
		m.st.KeyBinding.Render("q") + " quit",
	}
	return m.st.Footer.Render(strings.Join(hints, " · "))
}

func (m *Model) renderErrorScreen() string {
	box := m.st.Modal.Render(
		m.st.StatusGone.Render("⚠ error\n\n") + m.errMsg + "\n\nesc / q to dismiss",
	)
	return placeMiddle(m.width, m.height, box)
}

func (m *Model) renderEmptyScreen() string {
	box := m.st.Modal.Render(
		"no sessions configured\n\npress n to create your first one",
	)
	return placeMiddle(m.width, m.height, box)
}

func (m *Model) overlayToast(body string) string {
	t := m.st.Toast.Render(m.toast)
	return overlayBottomRight(body, t, m.width, m.height)
}

func (m *Model) overlayHelp(body string) string {
	help := m.st.Modal.Render(m.helpText())
	return placeMiddle(m.width, m.height, help)
}

func (m *Model) helpText() string {
	keys := []string{
		m.st.KeyBinding.Render("j/k") + "  navigate",
		m.st.KeyBinding.Render("h/l") + "  collapse / expand",
		m.st.KeyBinding.Render("enter / a") + "  attach",
		m.st.KeyBinding.Render("r") + "  refresh",
		m.st.KeyBinding.Render("R") + "  reload dotfiles",
		m.st.KeyBinding.Render("s") + "  sync push",
		m.st.KeyBinding.Render("^T") + "  cycle theme",
		m.st.KeyBinding.Render("?") + "  toggle this help",
		m.st.KeyBinding.Render("q") + "  quit",
	}
	return m.st.Title.Render("keymap") + "\n\n" + strings.Join(keys, "\n")
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
		return actionDoneMsg{Text: "reload triggered"}
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
		text := fmt.Sprintf("sync: %d created, %d updated", len(rep.Created), len(rep.Updated))
		return actionDoneMsg{Text: text}
	}
}

func attachCmd(r tmux.Runner, target string) tea.Cmd {
	return func() tea.Msg {
		_ = actions.Attach(context.Background(), r, target)
		return nil
	}
}
