package ui

import (
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/ui/theme"
	"github.com/mark1708/tmh/internal/ui/toast"

	tea "github.com/charmbracelet/bubbletea"
)

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
			return m.doctorCmd()
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
	// Parametrized actions (4.5).
	out = append(out, PaletteAction{
		Title:       i18n.T("tui.palette.action.mark.title"),
		Subtitle:    i18n.T("tui.palette.action.mark.subtitle"),
		NeedsParam:  true,
		ParamPrompt: "letter (e.g. a)",
		ParamRun: func(letter string) tea.Cmd {
			r := []rune(letter)
			if len(r) == 0 {
				return nil
			}
			target := m.dashboard.SelectedTarget()
			cursor := m.dashboard.effectiveCursor()
			if target == "" || m.marksStore == nil {
				return nil
			}
			m.marksStore.SetMark(r[0], target, cursor)
			return func() tea.Msg {
				return tea.Batch(
					func() tea.Msg {
						return toastMsg{Kind: toast.KindSuccess, Text: i18n.Tf("tui.toast.mark_set", map[string]any{"letter": string(r[0])})}
					},
					func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} },
				)()
			}
		},
	})
	out = append(out, PaletteAction{
		Title:       i18n.T("tui.palette.action.goto.title"),
		Subtitle:    i18n.T("tui.palette.action.goto.subtitle"),
		NeedsParam:  true,
		ParamPrompt: "process name (e.g. vim)",
		ParamRun: func(name string) tea.Cmd {
			return m.gotoProcCmd(name)
		},
	})

	if m.listing != nil {
		for _, s := range m.listing.Sessions {
			s := s
			count := len(s.Windows)
			var subtitle string
			if i18n.Active() == "ru" {
				subtitle = i18n.PluralRu(count,
					i18n.T("tui.palette.action.attach.windows_one"),
					i18n.Tf("tui.palette.action.attach.windows_few", map[string]any{"count": count}),
					i18n.Tf("tui.palette.action.attach.windows_many", map[string]any{"count": count}),
				)
			} else {
				subtitle = i18n.Tf("tui.palette.action.attach.subtitle", map[string]any{"count": count})
			}
			out = append(out, PaletteAction{
				Title:    i18n.Tf("tui.palette.action.attach.title", map[string]any{"name": s.Name}),
				Subtitle: subtitle,
				Run:      func() tea.Cmd { return tea.Sequence(attachCmd(m.deps.Runner, m.deps.Runner.InTmux(), s.Name), m.loadDataCmd()) },
			})
		}
	}
	return out
}
