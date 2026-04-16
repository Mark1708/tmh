package ui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/ui/theme"
	"git.mark1708.ru/me/tmh/internal/ui/toast"
	"git.mark1708.ru/me/tmh/internal/xdg"
	"gopkg.in/yaml.v3"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func (s *settingsModel) Resize(w, h int)          { s.width, s.height = w, h }
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

// saveCmd returns a tea.Cmd that persists all pending settings to disk.
func (s *settingsModel) saveCmd() tea.Cmd {
	// Snapshot field state by value (fields are small structs).
	fields := s.fields
	cfgPath := s.configPath

	return func() tea.Msg {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			cfg = &config.Config{Version: 1}
			_ = os.MkdirAll(filepath.Dir(cfgPath), 0o755)
		}
		if cfg.Node == nil {
			cfg.Node = minimalConfigNode()
		}

		if ferrs := applyFieldsToConfig(cfg.Node, fields); len(ferrs) > 0 {
			return toastMsg{Kind: toast.KindError, Text: "save: " + ferrs[0].Error()}
		}
		if err := config.Write(cfg, cfgPath, config.WriteOptions{PreserveBlanks: true}); err != nil {
			return toastMsg{Kind: toast.KindError, Text: "save: " + err.Error()}
		}

		if err := writeTmuxConf(fields[catTmux]); err != nil {
			return toastMsg{Kind: toast.KindError, Text: "tmux.conf: " + err.Error()}
		}

		return toastMsg{Kind: toast.KindSuccess, Text: i18n.T("tui.settings.saved")}
	}
}

// applyFieldsToConfig writes all save-then-apply field values into the
// YAML node tree. Returns accumulated non-fatal errors.
func applyFieldsToConfig(root *yaml.Node, fields [numSettingsCats][]settingsField) []error {
	var errs []error
	pset := func(path, val string) {
		if err := config.PathSet(root, path, val); err != nil {
			errs = append(errs, err)
		}
	}
	pbool := func(path string, val bool) {
		if err := config.PathSetBool(root, path, val); err != nil {
			errs = append(errs, err)
		}
	}
	pint := func(path string, val int) {
		if err := config.PathSetInt(root, path, val); err != nil {
			errs = append(errs, err)
		}
	}

	// Display
	if df := fields[catDisplay]; len(df) >= 4 {
		pbool("defaults.display.show_processes_in_tree", df[0].on)
		pbool("defaults.display.show_footer_heatmap", df[1].on)
		pint("defaults.display.preview_default_pane", df[2].chosen)
		pset("defaults.display.tree_density", df[3].choices[df[3].chosen])
	}

	// History (skip read-only index 1 and button index 4)
	if hf := fields[catHistory]; len(hf) >= 5 {
		pset("defaults.history.retention", hf[0].choices[hf[0].chosen])
		pbool("defaults.history.auto_clear_on_startup", hf[2].on)
		pbool("defaults.history.archive_on_clear", hf[3].on)
	}

	// Marks
	if mf := fields[catMarks]; len(mf) >= 1 {
		pbool("defaults.marks.persist_across_sessions", mf[0].on)
	}

	// Tmux
	if tf := fields[catTmux]; len(tf) >= 6 {
		pset("defaults.tmux_integration.default_terminal", tf[0].choices[tf[0].chosen])
		pint("defaults.tmux_integration.escape_time_ms", atoi(tf[1].choices[tf[1].chosen]))
		pbool("defaults.tmux_integration.mouse_mode", tf[2].on)
		pbool("defaults.tmux_integration.status_right_integration", tf[3].on)
		pint("defaults.tmux_integration.base_index", atoi(tf[4].choices[tf[4].chosen]))
		pint("defaults.tmux_integration.pane_base_index", atoi(tf[5].choices[tf[5].chosen]))
	}

	// Behaviour
	if bf := fields[catBehaviour]; len(bf) >= 4 {
		pset("defaults.behaviour.auto_refresh_interval", bf[0].choices[bf[0].chosen])
		pbool("defaults.behaviour.dry_run_default", bf[1].on)
		pbool("defaults.behaviour.confirm_on_kill", bf[2].on)
		pbool("defaults.behaviour.optimistic_rendering", bf[3].on)
	}

	return errs
}

// writeTmuxConf atomically writes the tmh-managed tmux include-file.
func writeTmuxConf(tf []settingsField) error {
	if len(tf) < 6 {
		return nil
	}
	confPath := xdg.TmuxConfPath()
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(confPath), ".tmux.conf.tmp.*")
	if err != nil {
		return err
	}
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmp.Name())
		}
	}()

	w := bufio.NewWriter(tmp)
	_, _ = fmt.Fprintf(w, "# Generated by tmh settings — do not edit manually.\n")
	_, _ = fmt.Fprintf(w, "# Add to ~/.tmux.conf: source-file %s\n\n", confPath)
	_, _ = fmt.Fprintf(w, "set -g default-terminal %q\n", tf[0].choices[tf[0].chosen])
	_, _ = fmt.Fprintf(w, "set -sg escape-time %s\n", tf[1].choices[tf[1].chosen])
	if tf[2].on {
		_, _ = fmt.Fprintln(w, "set -g mouse on")
	} else {
		_, _ = fmt.Fprintln(w, "set -g mouse off")
	}
	_, _ = fmt.Fprintf(w, "set -g base-index %s\n", tf[4].choices[tf[4].chosen])
	_, _ = fmt.Fprintf(w, "setw -g pane-base-index %s\n", tf[5].choices[tf[5].chosen])
	if tf[3].on {
		_, _ = fmt.Fprintln(w, "set -ag status-right ' #(tmh status)'")
	}

	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	_ = tmp.Close()
	removeTmp = false
	return os.Rename(tmp.Name(), confPath)
}

// minimalConfigNode returns a bare YAML document node suitable for a new file.
func minimalConfigNode() *yaml.Node {
	doc := &yaml.Node{Kind: yaml.DocumentNode}
	doc.Content = []*yaml.Node{
		{Kind: yaml.MappingNode, Tag: "!!map"},
	}
	return doc
}

// atoi converts a decimal string to int; returns 0 on any error.
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ── View ────────────────────────────────────────────────────────────────────

func (s *settingsModel) View() string {
	w := s.contentWidth()
	const catW = 17
	sepChar := "│"
	rightW := w - catW - len(sepChar)
	if rightW < 12 {
		rightW = 12
	}

	p := s.st.Palette
	mb := modalBg(p)
	title := s.st.Title.Inherit(mb)
	hint := s.st.Hint.Inherit(mb)

	var b strings.Builder

	// Title row
	titleText := s.str.Settings.Title
	if s.dirty {
		titleText += " *"
	}
	b.WriteString(modalRow(p, w, title.Render(titleText)))
	b.WriteString("\n")

	// Top divider
	b.WriteString(s.renderHRule(w, catW, len(sepChar), "┬"))
	b.WriteString("\n")

	// Body rows
	nRight := len(s.fields[s.catIdx])
	nRows := numSettingsCats
	if nRight > nRows {
		nRows = nRight
	}
	for i := 0; i < nRows; i++ {
		leftCell := s.renderCatCell(i, catW)
		rightCell := s.renderFieldRow(i, rightW)
		row := leftCell + mb.Render(sepChar) + rightCell
		b.WriteString(modalRow(p, w, row))
		b.WriteString("\n")
	}

	// Bottom divider
	b.WriteString(s.renderHRule(w, catW, len(sepChar), "┴"))
	b.WriteString("\n")

	// Footer hint
	b.WriteString(modalRow(p, w, hint.Render(s.footerHint())))

	return placeMiddle(s.width, s.height, s.st.Modal.Render(b.String()), p)
}

// renderHRule renders a full-width horizontal rule with a junction at catW.
func (s *settingsModel) renderHRule(w, catW, sepW int, junction string) string {
	p := s.st.Palette
	mb := modalBg(p)
	left := strings.Repeat("─", catW)
	right := strings.Repeat("─", w-catW-sepW)
	return modalRow(p, w, mb.Render(left+junction+right))
}

// renderCatCell renders a left-panel cell for category index i.
func (s *settingsModel) renderCatCell(i, catW int) string {
	p := s.st.Palette
	mb := modalBg(p)
	if i >= numSettingsCats {
		return lipgloss.PlaceHorizontal(catW, lipgloss.Left, mb.Render(""),
			lipgloss.WithWhitespaceBackground(p.BgOverlay))
	}
	name := s.catNames[i]
	prefix := "  "
	if i == s.catIdx {
		prefix = "▸ "
	}
	label := prefix + truncate(name, catW-2)
	var line string
	switch {
	case i == s.catIdx && s.focus == focusCategories:
		line = s.st.Selected.Render(label)
	case i == s.catIdx:
		line = s.st.Subtitle.Inherit(mb).Render(label)
	default:
		line = mb.Render(label)
	}
	return lipgloss.PlaceHorizontal(catW, lipgloss.Left, line,
		lipgloss.WithWhitespaceBackground(p.BgOverlay))
}

// renderFieldRow renders a right-panel cell for field row i.
func (s *settingsModel) renderFieldRow(i, rightW int) string {
	p := s.st.Palette
	mb := modalBg(p)
	fields := s.fields[s.catIdx]
	if i >= len(fields) {
		return lipgloss.PlaceHorizontal(rightW, lipgloss.Left, mb.Render(""),
			lipgloss.WithWhitespaceBackground(p.BgOverlay))
	}
	f := fields[i]
	selected := s.focus == focusFields && i == s.fieldIdx

	labelW := rightW * 55 / 100
	valueW := rightW - labelW - 1
	if valueW < 6 {
		valueW = 6
	}

	labelText := " " + truncate(f.label, labelW-1)
	valueText := s.renderFieldValueRaw(f, selected, valueW)

	// Build the full row; apply Selected background when focused.
	if selected {
		// Pad between label and value.
		gap := labelW - lipgloss.Width(labelText)
		if gap < 0 {
			gap = 0
		}
		raw := labelText + strings.Repeat(" ", gap) + valueText
		line := s.st.Selected.Width(rightW).Render(raw)
		return lipgloss.PlaceHorizontal(rightW, lipgloss.Left, line,
			lipgloss.WithWhitespaceBackground(p.BgOverlay))
	}

	label := mb.Render(labelText)
	value := mb.Render(valueText)
	line := label + mb.Render(" ") + value
	return lipgloss.PlaceHorizontal(rightW, lipgloss.Left, line,
		lipgloss.WithWhitespaceBackground(p.BgOverlay))
}

// renderFieldValueRaw returns the plain (unstyled or minimally styled) value
// text for a field. The caller is responsible for applying row-level styling.
func (s *settingsModel) renderFieldValueRaw(f settingsField, selected bool, w int) string {
	switch f.kind {
	case fieldKindSelect:
		val := ""
		if f.chosen < len(f.choices) {
			val = f.choices[f.chosen]
		}
		text := truncate(val, w-4)
		if selected {
			return text + " ←→"
		}
		return text

	case fieldKindToggle:
		if f.on {
			return "✓ on"
		}
		return "  off"

	case fieldKindButton:
		return "[" + truncate(f.label, w-2) + "]"

	case fieldKindReadOnly:
		return truncate(f.display, w)
	}
	return ""
}

func (s *settingsModel) footerHint() string {
	if s.pendingDiscard {
		return s.str.Settings.HintDiscard
	}
	if s.dirty {
		return s.str.Settings.HintDirty
	}
	if s.focus == focusFields {
		return s.str.Settings.HintFields
	}
	return s.str.Settings.HintCategories
}

func (s *settingsModel) contentWidth() int {
	w := s.width * 65 / 100
	if w < 50 {
		w = 50
	}
	if w > 100 {
		w = 100
	}
	return w
}
