package ui

import (
	"fmt"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/ui/theme"
	"github.com/mark1708/tmh/internal/ui/toast"

	tea "github.com/charmbracelet/bubbletea"
)

func (s *settingsModel) buildAppearanceFields(_ config.Defaults, st theme.Styles) []settingsField {
	themeNames := make([]string, len(theme.Available))
	for i, p := range theme.Available {
		themeNames[i] = p.Name
	}
	themeIdx := 0
	for i, p := range theme.Available {
		if p.Name == st.Palette.Name {
			themeIdx = i
			break
		}
	}
	langs := i18n.Available()
	langIdx := 0
	active := i18n.Active()
	for i, l := range langs {
		if l == active {
			langIdx = i
			break
		}
	}
	return []settingsField{
		{label: i18n.T("tui.settings.field.theme"), kind: fieldKindSelect, choices: themeNames, chosen: themeIdx},
		{label: i18n.T("tui.settings.field.language"), kind: fieldKindSelect, choices: langs, chosen: langIdx},
	}
}

func buildDisplayFields(d config.Defaults) []settingsField {
	densityChoices := []string{"normal", "compact"}
	densityIdx := 0
	for i, c := range densityChoices {
		if c == d.Display.TreeDensity {
			densityIdx = i
			break
		}
	}
	previewChoices := []string{"first", "active", "last"}
	previewIdx := d.Display.PreviewDefaultPane
	if previewIdx < 0 || previewIdx >= len(previewChoices) {
		previewIdx = 0
	}
	return []settingsField{
		{label: i18n.T("tui.settings.field.show_processes"), kind: fieldKindToggle, on: d.Display.ShowProcessesInTree},
		{label: i18n.T("tui.settings.field.show_heatmap"), kind: fieldKindToggle, on: d.Display.ShowFooterHeatmap},
		{label: i18n.T("tui.settings.field.preview_pane"), kind: fieldKindSelect, choices: previewChoices, chosen: previewIdx},
		{label: i18n.T("tui.settings.field.tree_density"), kind: fieldKindSelect, choices: densityChoices, chosen: densityIdx},
	}
}

func (s *settingsModel) buildHistoryFields(d config.Defaults) []settingsField {
	retChoices := []string{"7d", "30d", "90d", "forever"}
	retIdx := 1 // default 30d
	for i, c := range retChoices {
		if c == d.History.Retention {
			retIdx = i
			break
		}
	}
	autoClear := true
	if d.History.AutoClearOnStartup != nil {
		autoClear = *d.History.AutoClearOnStartup
	}
	archiveOnClear := true
	if d.History.ArchiveOnClear != nil {
		archiveOnClear = *d.History.ArchiveOnClear
	}
	maxE := d.History.MaxEntries
	if maxE == 0 {
		maxE = 1000
	}
	clearBtn := settingsField{
		label: i18n.T("tui.settings.field.clear_history"),
		kind:  fieldKindButton,
	}
	clearBtn.activate = func() tea.Cmd {
		return func() tea.Msg { return clearHistoryMsg{} }
	}
	return []settingsField{
		{label: i18n.T("tui.settings.field.retention"), kind: fieldKindSelect, choices: retChoices, chosen: retIdx},
		{label: i18n.T("tui.settings.field.max_entries"), kind: fieldKindReadOnly, display: fmt.Sprintf("%d", maxE)},
		{label: i18n.T("tui.settings.field.auto_clear"), kind: fieldKindToggle, on: autoClear},
		{label: i18n.T("tui.settings.field.archive_on_clear"), kind: fieldKindToggle, on: archiveOnClear},
		clearBtn,
	}
}

func (s *settingsModel) buildMarksFields(d config.Defaults) []settingsField {
	persist := true
	if d.Marks.PersistAcrossSessions != nil {
		persist = *d.Marks.PersistAcrossSessions
	}
	resetBtn := settingsField{
		label: i18n.T("tui.settings.field.reset_marks"),
		kind:  fieldKindButton,
	}
	resetBtn.activate = func() tea.Cmd {
		return func() tea.Msg {
			return toastMsg{Kind: toast.KindInfo, Text: "marks: not yet implemented"}
		}
	}
	return []settingsField{
		{label: i18n.T("tui.settings.field.persist_marks"), kind: fieldKindToggle, on: persist},
		resetBtn,
	}
}

func buildTmuxFields(d config.Defaults) []settingsField {
	termChoices := []string{"tmux-256color", "xterm-256color", "screen-256color"}
	termIdx := choiceIdx(termChoices, d.TmuxIntegration.DefaultTerminal)
	escChoices := []string{"0", "50", "500"}
	escIdx := 0
	if d.TmuxIntegration.EscapeTimeMs != nil {
		escIdx = choiceIdx(escChoices, fmt.Sprintf("%d", *d.TmuxIntegration.EscapeTimeMs))
	}
	mouse := true
	if d.TmuxIntegration.MouseMode != nil {
		mouse = *d.TmuxIntegration.MouseMode
	}
	statusRight := false
	if d.TmuxIntegration.StatusRightIntegration != nil {
		statusRight = *d.TmuxIntegration.StatusRightIntegration
	}
	idxChoices := []string{"0", "1"}
	baseIdx := 1
	if d.TmuxIntegration.BaseIndex != nil {
		baseIdx = *d.TmuxIntegration.BaseIndex
	}
	paneBaseIdx := 1
	if d.TmuxIntegration.PaneBaseIndex != nil {
		paneBaseIdx = *d.TmuxIntegration.PaneBaseIndex
	}
	return []settingsField{
		{label: i18n.T("tui.settings.field.default_terminal"), kind: fieldKindSelect, choices: termChoices, chosen: termIdx},
		{label: i18n.T("tui.settings.field.escape_time"), kind: fieldKindSelect, choices: escChoices, chosen: escIdx},
		{label: i18n.T("tui.settings.field.mouse_mode"), kind: fieldKindToggle, on: mouse},
		{label: i18n.T("tui.settings.field.status_right"), kind: fieldKindToggle, on: statusRight},
		{label: i18n.T("tui.settings.field.base_index"), kind: fieldKindSelect, choices: idxChoices, chosen: choiceIdx(idxChoices, fmt.Sprintf("%d", baseIdx))},
		{label: i18n.T("tui.settings.field.pane_base_index"), kind: fieldKindSelect, choices: idxChoices, chosen: choiceIdx(idxChoices, fmt.Sprintf("%d", paneBaseIdx))},
	}
}

func buildBehaviourFields(d config.Defaults) []settingsField {
	refreshChoices := []string{"1s", "2s", "5s", "10s", "off"}
	refreshIdx := 1 // default 2s
	if d.Behaviour.AutoRefreshInterval != "" {
		refreshIdx = choiceIdx(refreshChoices, d.Behaviour.AutoRefreshInterval)
	}
	confirmKill := true
	if d.Behaviour.ConfirmOnKill != nil {
		confirmKill = *d.Behaviour.ConfirmOnKill
	}
	return []settingsField{
		{label: i18n.T("tui.settings.field.auto_refresh"), kind: fieldKindSelect, choices: refreshChoices, chosen: refreshIdx},
		{label: i18n.T("tui.settings.field.dry_run"), kind: fieldKindToggle, on: d.Behaviour.DryRunDefault},
		{label: i18n.T("tui.settings.field.confirm_kill"), kind: fieldKindToggle, on: confirmKill},
		{label: i18n.T("tui.settings.field.optimistic"), kind: fieldKindToggle, on: d.Behaviour.OptimisticRendering},
	}
}

func buildKeybindingFields(keys Keys, str UIStrings) []settingsField {
	pairs := [][2]string{
		{keys.Up.Help().Key, str.Keymap.NavUpdown},
		{keys.Top.Help().Key + "/" + keys.Bottom.Help().Key, str.Keymap.NavTopBottom},
		{keys.PgUp.Help().Key + "/" + keys.PgDown.Help().Key, str.Keymap.NavPage},
		{keys.Attach.Help().Key, str.Keymap.ActionAttach},
		{keys.NewSession.Help().Key, str.Keymap.ActionNew},
		{keys.Kill.Help().Key, str.Keymap.ActionKill},
		{keys.Undo.Help().Key, str.Keymap.ActionUndo},
		{keys.Refresh.Help().Key, str.Keymap.SyncRefresh},
		{keys.Reload.Help().Key, str.Keymap.SyncReload},
		{keys.Diff.Help().Key, str.Keymap.SyncDiff},
		{keys.Palette.Help().Key, str.Keymap.OtherPalette},
		{keys.Settings.Help().Key, str.Keymap.OtherSettings},
		{keys.Help.Help().Key, str.Keymap.OtherHelp},
		{keys.Quit.Help().Key, str.Keymap.OtherQuit},
	}
	out := make([]settingsField, len(pairs))
	for i, p := range pairs {
		out[i] = settingsField{label: p[0], kind: fieldKindReadOnly, display: p[1]}
	}
	return out
}

// choiceIdx returns the index of value in choices, or 0 if not found.
func choiceIdx(choices []string, value string) int {
	for i, c := range choices {
		if c == value {
			return i
		}
	}
	return 0
}
