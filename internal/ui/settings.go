package ui

import (
	"time"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
)

// settingsFocus is the current input focus level in the master-detail layout.
type settingsFocus int

const (
	focusCategories settingsFocus = iota
	focusFields
)

// fieldKind describes how a settings row is rendered and interacted with.
type fieldKind int

const (
	fieldKindSelect   fieldKind = iota // ←→ cycles through options
	fieldKindToggle                     // ←→ / space flips bool
	fieldKindButton                     // enter executes action
	fieldKindReadOnly                   // display only
)

// settingsField is one row in the right panel.
type settingsField struct {
	label   string
	kind    fieldKind
	// select state
	choices []string
	chosen  int
	// toggle state
	on bool
	// button: activate returns a tea.Cmd (may be nil)
	activate func() tea.Cmd
	// read-only display value
	display string
}

const numSettingsCats = 7

const (
	catAppearance  = 0
	catDisplay     = 1
	catHistory     = 2
	catMarks       = 3
	catTmux        = 4
	catBehaviour   = 5
	catKeybindings = 6
)

// settingsModel is the master-detail settings overlay.
//
//   focusCategories — left panel; ↑↓ picks category, Enter/Tab enters fields
//   focusFields     — right panel; ↑↓ picks field, ←→ changes select/toggle
type settingsModel struct {
	keys          Keys
	st            theme.Styles
	str           UIStrings
	width, height int
	configPath    string

	focus    settingsFocus
	catIdx   int
	fieldIdx int

	dirty          bool // any non-live change pending Ctrl+S
	pendingDiscard bool // waiting for discard-confirm answer

	catNames [numSettingsCats]string
	fields   [numSettingsCats][]settingsField

	// live-apply callbacks
	onThemeApply            func(theme.Palette) tea.Cmd
	onLanguageApply         func(string) tea.Cmd
	onRefreshIntervalChange func(time.Duration)
}

// newSettings constructs the settings overlay from current runtime state.
func newSettings(
	keys Keys,
	st theme.Styles,
	str UIStrings,
	cfg *config.Config,
	configPath string,
	onThemeApply func(theme.Palette) tea.Cmd,
	onLanguageApply func(string) tea.Cmd,
	onRefreshIntervalChange func(time.Duration),
) *settingsModel {
	s := &settingsModel{
		keys:                    keys,
		st:                      st,
		str:                     str,
		configPath:              configPath,
		onThemeApply:            onThemeApply,
		onLanguageApply:         onLanguageApply,
		onRefreshIntervalChange: onRefreshIntervalChange,
	}

	var d config.Defaults
	if cfg != nil {
		d = cfg.Defaults
	}

	s.catNames = [numSettingsCats]string{
		i18n.T("tui.settings.cat.appearance"),
		i18n.T("tui.settings.cat.display"),
		i18n.T("tui.settings.cat.history"),
		i18n.T("tui.settings.cat.marks"),
		i18n.T("tui.settings.cat.tmux"),
		i18n.T("tui.settings.cat.behaviour"),
		i18n.T("tui.settings.cat.keybindings"),
	}

	s.fields[catAppearance] = s.buildAppearanceFields(d, st)
	s.fields[catDisplay] = buildDisplayFields(d)
	s.fields[catHistory] = s.buildHistoryFields(d)
	s.fields[catMarks] = s.buildMarksFields(d)
	s.fields[catTmux] = buildTmuxFields(d)
	s.fields[catBehaviour] = buildBehaviourFields(d)
	s.fields[catKeybindings] = buildKeybindingFields(keys, str)

	return s
}

func (s *settingsModel) Resize(w, h int)           { s.width, s.height = w, h }
func (s *settingsModel) SetStyles(st theme.Styles) { s.st = st }
func (s *settingsModel) SetStrings(str UIStrings)  { s.str = str }

func (s *settingsModel) Update(msg tea.Msg) (*settingsModel, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	// Discard-confirm overlay: only y/n/esc are active.
	if s.pendingDiscard {
		switch k.String() {
		case "y", "Y":
			s.dirty = false
			s.pendingDiscard = false
			return s, func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} }
		case "n", "N", "esc":
			s.pendingDiscard = false
		}
		return s, nil
	}

	// Ctrl+S saves regardless of focus level.
	if k.String() == "ctrl+s" {
		cmd := s.saveCmd()
		s.dirty = false
		return s, cmd
	}

	switch s.focus {
	case focusCategories:
		return s.updateCategories(k)
	case focusFields:
		return s.updateFields(k)
	}
	return s, nil
}

func (s *settingsModel) updateCategories(k tea.KeyMsg) (*settingsModel, tea.Cmd) {
	switch {
	case keyMatches(k, s.keys.Up):
		s.catIdx = (s.catIdx - 1 + numSettingsCats) % numSettingsCats
		s.fieldIdx = 0
	case keyMatches(k, s.keys.Down):
		s.catIdx = (s.catIdx + 1) % numSettingsCats
		s.fieldIdx = 0
	case keyMatches(k, s.keys.Tab), keyMatches(k, s.keys.Enter):
		if len(s.fields[s.catIdx]) > 0 {
			s.focus = focusFields
			s.fieldIdx = 0
		}
	case keyMatches(k, s.keys.Esc):
		if s.dirty {
			s.pendingDiscard = true
			return s, nil
		}
		return s, func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} }
	}
	return s, nil
}

func (s *settingsModel) updateFields(k tea.KeyMsg) (*settingsModel, tea.Cmd) {
	fields := s.fields[s.catIdx]
	if len(fields) == 0 {
		s.focus = focusCategories
		return s, nil
	}
	switch {
	case keyMatches(k, s.keys.Up):
		s.fieldIdx = (s.fieldIdx - 1 + len(fields)) % len(fields)
	case keyMatches(k, s.keys.Down):
		s.fieldIdx = (s.fieldIdx + 1) % len(fields)
	case k.String() == "shift+tab":
		s.focus = focusCategories
	case keyMatches(k, s.keys.Esc):
		s.focus = focusCategories
	case keyMatches(k, s.keys.Left):
		return s, s.changeField(-1)
	case keyMatches(k, s.keys.Right):
		return s, s.changeField(+1)
	case k.String() == " ":
		f := &s.fields[s.catIdx][s.fieldIdx]
		if f.kind == fieldKindToggle {
			return s, s.changeField(+1)
		}
	case keyMatches(k, s.keys.Enter):
		f := &s.fields[s.catIdx][s.fieldIdx]
		switch f.kind {
		case fieldKindButton:
			if f.activate != nil {
				return s, f.activate()
			}
		case fieldKindToggle:
			return s, s.changeField(+1)
		case fieldKindSelect:
			return s, s.changeField(+1)
		}
	}
	return s, nil
}

// changeField moves a select/toggle field by delta (-1 or +1) and handles
// live-apply side-effects.
func (s *settingsModel) changeField(delta int) tea.Cmd {
	f := &s.fields[s.catIdx][s.fieldIdx]
	switch f.kind {
	case fieldKindSelect:
		n := len(f.choices)
		if n == 0 {
			return nil
		}
		f.chosen = ((f.chosen + delta) % n + n) % n
	case fieldKindToggle:
		f.on = !f.on
	default:
		return nil
	}
	return s.applyLive(f)
}

// applyLive triggers live-apply for appearance and certain behaviour fields.
// All other changes just mark dirty for Ctrl+S.
func (s *settingsModel) applyLive(f *settingsField) tea.Cmd {
	themeKey := i18n.T("tui.settings.field.theme")
	langKey := i18n.T("tui.settings.field.language")
	refreshKey := i18n.T("tui.settings.field.auto_refresh")

	switch {
	case s.catIdx == catAppearance && f.label == themeKey:
		if s.onThemeApply != nil && f.chosen < len(theme.Available) {
			return s.onThemeApply(theme.Available[f.chosen])
		}
	case s.catIdx == catAppearance && f.label == langKey:
		langs := i18n.Available()
		if s.onLanguageApply != nil && f.chosen < len(langs) {
			return s.onLanguageApply(langs[f.chosen])
		}
	case s.catIdx == catBehaviour && f.label == refreshKey:
		if s.onRefreshIntervalChange != nil && f.chosen < len(f.choices) {
			d := parseRefreshInterval(f.choices[f.chosen])
			s.onRefreshIntervalChange(d)
		}
	default:
		s.dirty = true
	}
	return nil
}

// parseRefreshInterval converts a choice string like "2s" to a Duration.
// "off" or parse failures return 0.
func parseRefreshInterval(s string) time.Duration {
	if s == "off" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 2 * time.Second
	}
	return d
}
