package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/ui/toast"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) activePruneCmd() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		cfg, err := loadActiveConfig(deps)
		if err != nil {
			return errorMsg{Err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		report, err := actions.PruneActiveWindows(ctx, deps.Runner, deps.State, cfg, time.Now())
		if err != nil {
			return errorMsg{Err: err}
		}
		text := i18n.Tf("tui.toast.active_pruned", map[string]any{
			"unlinked": report.Unlinked, "deleted": report.Deleted, "orphaned": report.Orphaned,
		})
		return activeActionResult(text, m.loadDataCmd())
	}
}

func (m *Model) activeRemoveCmd(windowID string) tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		cfg, err := loadActiveConfig(deps)
		if err != nil {
			return errorMsg{Err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := actions.RemoveActiveWindow(ctx, deps.Runner, deps.State, cfg, windowID); err != nil {
			return errorMsg{Err: err}
		}
		text := i18n.Tf("tui.toast.active_removed", map[string]any{"id": windowID})
		return activeActionResult(text, m.loadDataCmd())
	}
}

func (m *Model) activeRecoverCmd(windowID, destination string) tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		cfg, err := loadActiveConfig(deps)
		if err != nil {
			return errorMsg{Err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := actions.RecoverActiveWindow(ctx, deps.Runner, deps.State, cfg, windowID, destination); err != nil {
			return errorMsg{Err: err}
		}
		text := i18n.Tf("tui.toast.active_recovered", map[string]any{"id": windowID, "session": destination})
		return activeActionResult(text, m.loadDataCmd())
	}
}

func loadActiveConfig(deps Deps) (config.ActiveSessionConfig, error) {
	if deps.LoadConfig == nil {
		return config.ActiveSessionConfig{}, fmt.Errorf("active settings unavailable")
	}
	cfg, err := deps.LoadConfig()
	if err != nil {
		return config.ActiveSessionConfig{}, err
	}
	return cfg.Defaults.TmuxIntegration.Active, nil
}

func activeActionResult(text string, refresh tea.Cmd) tea.Msg {
	return tea.Batch(
		func() tea.Msg { return actionDoneMsg{Text: text} },
		refresh,
		func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} },
	)()
}

func (m *Model) invalidActiveDestinationCmd(destination string) tea.Cmd {
	return func() tea.Msg {
		return toastMsg{Kind: toast.KindError, Text: i18n.Tf(
			"tui.toast.active_destination_invalid", map[string]any{"session": destination},
		)}
	}
}
