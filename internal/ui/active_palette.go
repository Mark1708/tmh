package ui

import (
	"slices"
	"strings"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"

	tea "github.com/charmbracelet/bubbletea"
)

type activeActionKind string

const (
	activeActionPrune   activeActionKind = "prune"
	activeActionRemove  activeActionKind = "remove"
	activeActionRecover activeActionKind = "recover"
)

type activePaletteAction struct {
	PaletteAction
	Kind     activeActionKind
	WindowID string
}

func (m *Model) activePaletteActions() []activePaletteAction {
	report := m.activeStatus
	if !report.Enabled || !report.Owned || report.Collision {
		return nil
	}
	out := []activePaletteAction{{
		Kind: activeActionPrune,
		PaletteAction: PaletteAction{
			Title:    i18n.T("tui.palette.action.active_prune.title"),
			Subtitle: i18n.T("tui.palette.action.active_prune.subtitle"),
			Run:      func() tea.Cmd { return m.activePruneCmd() },
		},
	}}
	for _, entry := range report.Entries {
		if action, ok := m.activeEntryAction(entry); ok {
			out = append(out, action)
		}
	}
	return out
}

func (m *Model) activeEntryAction(entry actions.ActiveWindowStatus) (activePaletteAction, bool) {
	if entry.State == state.ActiveWindowOrphaned {
		return m.activeRecoverPaletteAction(entry), true
	}
	healthy := entry.State == state.ActiveWindowTracked && !entry.Expired &&
		entry.ActiveIndex != nil && len(entry.SourceLinks) > 0
	if !healthy {
		return activePaletteAction{}, false
	}
	windowID := entry.WindowID
	return activePaletteAction{
		Kind: activeActionRemove, WindowID: windowID,
		PaletteAction: PaletteAction{
			Title:    i18n.Tf("tui.palette.action.active_remove.title", map[string]any{"id": windowID}),
			Subtitle: i18n.T("tui.palette.action.active_remove.subtitle"),
			Run:      func() tea.Cmd { return m.activeRemoveCmd(windowID) },
		},
	}, true
}

func (m *Model) activeRecoverPaletteAction(entry actions.ActiveWindowStatus) activePaletteAction {
	windowID := entry.WindowID
	destinations := strings.Join(m.activeRecoveryDestinations(), ", ")
	return activePaletteAction{
		Kind: activeActionRecover, WindowID: windowID,
		PaletteAction: PaletteAction{
			Title:       i18n.Tf("tui.palette.action.active_recover.title", map[string]any{"id": windowID}),
			Subtitle:    i18n.Tf("tui.palette.action.active_recover.subtitle", map[string]any{"sessions": destinations}),
			NeedsParam:  true,
			ParamPrompt: i18n.T("tui.palette.action.active_recover.prompt"),
			ParamRun: func(destination string) tea.Cmd {
				if !m.isActiveRecoveryDestination(destination) {
					return m.invalidActiveDestinationCmd(destination)
				}
				return m.activeRecoverCmd(windowID, destination)
			},
		},
	}
}

func (m *Model) activeRecoveryDestinations() []string {
	if m.listing == nil {
		return nil
	}
	out := make([]string, 0, len(m.listing.Sessions))
	for _, session := range m.listing.Sessions {
		if session.Live && session.Name != actions.ActiveSessionName {
			out = append(out, session.Name)
		}
	}
	return out
}

func (m *Model) isActiveRecoveryDestination(destination string) bool {
	return slices.Contains(m.activeRecoveryDestinations(), destination)
}
