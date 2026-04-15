package ui

import (
	"context"
	"strings"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// settingsSection identifies which horizontal section of the settings screen
// currently has focus. Tab / Shift+Tab rotates through them.
type settingsSection int

const (
	sectionLanguage settingsSection = iota
	sectionTheme
	sectionTmux
)

// settingsModel is the settings screen with three sections:
//  1. language — live-swap en/ru, persisted to config.yml
//  2. theme — cycle catppuccin palettes
//  3. tmux integration — readonly audit view (mirrors `tmh doctor` block)
type settingsModel struct {
	keys          Keys
	st            theme.Styles
	str           UIStrings
	width, height int

	section settingsSection

	languages []string
	langIdx   int

	themes   []theme.Palette
	themeIdx int

	tmuxFindings []actions.AuditFinding
	tmuxIdx      int

	onLanguageApply func(lang string) tea.Cmd
	onThemeApply    func(theme.Palette) tea.Cmd
}

func newSettings(
	keys Keys,
	st theme.Styles,
	str UIStrings,
	onThemeApply func(theme.Palette) tea.Cmd,
	onLanguageApply func(lang string) tea.Cmd,
	runner tmux.Runner,
) *settingsModel {
	s := &settingsModel{
		keys:            keys,
		st:              st,
		str:             str,
		languages:       i18n.Available(),
		themes:          theme.Available,
		onThemeApply:    onThemeApply,
		onLanguageApply: onLanguageApply,
	}
	// Seed selection indexes from the currently-active values.
	active := i18n.Active()
	for i, l := range s.languages {
		if l == active {
			s.langIdx = i
			break
		}
	}
	for i, p := range s.themes {
		if p.Name == st.Palette.Name {
			s.themeIdx = i
			break
		}
	}
	if runner != nil {
		s.tmuxFindings = actions.AuditTmuxConfig(context.Background(), runner)
	}
	return s
}

func (s *settingsModel) Resize(w, h int) { s.width, s.height = w, h }

// SetStyles updates the theme and (since a language change can also happen
// simultaneously) the string bundle.
func (s *settingsModel) SetStyles(st theme.Styles) { s.st = st }

// SetStrings replaces the translated bundle after a language switch.
func (s *settingsModel) SetStrings(str UIStrings) { s.str = str }

func (s *settingsModel) Update(msg tea.Msg) (*settingsModel, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch k.String() {
	case "tab":
		s.section = (s.section + 1) % 3
		return s, nil
	case "shift+tab":
		s.section = (s.section + 2) % 3
		return s, nil
	}
	switch s.section {
	case sectionLanguage:
		return s.updateLanguage(k)
	case sectionTheme:
		return s.updateTheme(k)
	case sectionTmux:
		return s.updateTmux(k)
	}
	return s, nil
}

func (s *settingsModel) updateLanguage(k tea.KeyMsg) (*settingsModel, tea.Cmd) {
	switch {
	case keyMatches(k, s.keys.Down), keyMatches(k, s.keys.Right):
		s.langIdx = (s.langIdx + 1) % len(s.languages)
		return s, s.applyLanguage()
	case keyMatches(k, s.keys.Up), keyMatches(k, s.keys.Left):
		s.langIdx = (s.langIdx - 1 + len(s.languages)) % len(s.languages)
		return s, s.applyLanguage()
	}
	return s, nil
}

func (s *settingsModel) updateTheme(k tea.KeyMsg) (*settingsModel, tea.Cmd) {
	switch {
	case keyMatches(k, s.keys.Down), keyMatches(k, s.keys.Right):
		s.themeIdx = (s.themeIdx + 1) % len(s.themes)
		return s, s.applyTheme()
	case keyMatches(k, s.keys.Up), keyMatches(k, s.keys.Left):
		s.themeIdx = (s.themeIdx - 1 + len(s.themes)) % len(s.themes)
		return s, s.applyTheme()
	}
	return s, nil
}

func (s *settingsModel) updateTmux(k tea.KeyMsg) (*settingsModel, tea.Cmd) {
	switch {
	case keyMatches(k, s.keys.Down):
		if s.tmuxIdx < len(s.tmuxFindings)-1 {
			s.tmuxIdx++
		}
	case keyMatches(k, s.keys.Up):
		if s.tmuxIdx > 0 {
			s.tmuxIdx--
		}
	}
	return s, nil
}

func (s *settingsModel) applyTheme() tea.Cmd {
	if s.onThemeApply == nil {
		return nil
	}
	return s.onThemeApply(s.themes[s.themeIdx])
}

func (s *settingsModel) applyLanguage() tea.Cmd {
	if s.onLanguageApply == nil {
		return nil
	}
	return s.onLanguageApply(s.languages[s.langIdx])
}

// View stacks the three sections vertically. Focused section gets an accent
// border; others are dimmed. Content width is capped so long audit messages
// don't balloon the modal past what the terminal can host.
func (s *settingsModel) View() string {
	bodyW := s.contentWidth()
	mb := modalBg(s.st.Palette)
	title := s.st.Title.Inherit(mb)
	hint := s.st.Hint.Inherit(mb)

	var b strings.Builder
	b.WriteString(paintLine(s.st.Palette, title.Render(s.str.Settings.Title)) + "\n\n")
	b.WriteString(s.renderLanguageSection(bodyW) + "\n")
	b.WriteString(s.renderThemeSection(bodyW) + "\n")
	b.WriteString(s.renderTmuxSection(bodyW) + "\n")
	footer := hint.Render(s.str.Settings.Hint + " · tab next section")
	b.WriteString("\n" + paintLine(s.st.Palette, footer))
	body := padBlock(b.String())
	return placeMiddle(s.width, s.height, s.st.Modal.Render(body), s.st.Palette)
}

// contentWidth caps the modal body to ≈60% of the terminal width so that
// long audit messages wrap/truncate instead of stretching the whole overlay.
func (s *settingsModel) contentWidth() int {
	w := s.width * 60 / 100
	if w < 40 {
		w = 40
	}
	if w > 90 {
		w = 90
	}
	return w
}

func (s *settingsModel) renderLanguageSection(w int) string {
	focused := s.section == sectionLanguage
	mb := modalBg(s.st.Palette)
	var b strings.Builder
	b.WriteString(sectionHeader(s.st, s.str.Settings.Language, focused) + "\n")
	for i, lang := range s.languages {
		marker := "  "
		if focused && i == s.langIdx {
			marker = "▸ "
		}
		label := strings.ToUpper(lang)
		line := mb.Render(marker + label)
		line = padRight(line, w)
		if focused && i == s.langIdx {
			line = s.st.Selected.Render(line)
		} else {
			line = paintLine(s.st.Palette, line)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (s *settingsModel) renderThemeSection(w int) string {
	focused := s.section == sectionTheme
	mb := modalBg(s.st.Palette)
	var b strings.Builder
	b.WriteString(sectionHeader(s.st, s.str.Settings.Theme, focused) + "\n")
	for i, p := range s.themes {
		marker := "  "
		if focused && i == s.themeIdx {
			marker = "▸ "
		}
		// Swatch has its own bg per block; we leave it as-is and just paint
		// the whitespace on either side.
		leftPad := mb.Render(marker + p.Name + "   ")
		line := leftPad + themeSwatch(p)
		if focused && i == s.themeIdx {
			line = s.st.Selected.Render(padRight(line, w))
		} else {
			line = paintLine(s.st.Palette, padRight(line, w))
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (s *settingsModel) renderTmuxSection(w int) string {
	focused := s.section == sectionTmux
	mb := modalBg(s.st.Palette)
	hint := s.st.Hint.Inherit(mb)
	var b strings.Builder
	b.WriteString(sectionHeader(s.st, s.str.Settings.Tmux, focused) + "\n")
	if len(s.tmuxFindings) == 0 {
		empty := paintLine(s.st.Palette, hint.Render("  "+i18n.T("tui.settings.tmux.empty")))
		b.WriteString(empty)
		return b.String()
	}
	// Columns: marker(2) + badge(2) + space + check(checkW) + space + message.
	const checkW = 28
	msgW := w - 2 - 2 - 1 - checkW - 1
	if msgW < 10 {
		msgW = 10
	}
	for i, f := range s.tmuxFindings {
		marker := "  "
		if focused && i == s.tmuxIdx {
			marker = "▸ "
		}
		badge := findingBadge(s.st, f.Level)
		check := truncate(f.Check, checkW)
		msgText := f.Message
		if f.MessageKey != "" {
			if t := i18n.T(f.MessageKey); t != f.MessageKey {
				msgText = t
			}
		}
		msg := truncate(msgText, msgW)
		line := mb.Render(marker) + badge + mb.Render(" "+padRight(check, checkW)+" "+padRight(msg, msgW))
		if focused && i == s.tmuxIdx {
			line = s.st.Selected.Render(padRight(line, w))
		} else {
			line = paintLine(s.st.Palette, padRight(line, w))
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// sectionHeader renders a section title; the focused one gets the accent
// subtitle style and an explicit marker so the user can tell which one Tab
// will affect. Painted with modal bg so it merges with the modal body.
func sectionHeader(st theme.Styles, title string, focused bool) string {
	mb := modalBg(st.Palette)
	var rendered string
	if focused {
		rendered = st.Subtitle.Inherit(mb).Render("▸ " + title)
	} else {
		rendered = st.Hint.Inherit(mb).Render("  " + title)
	}
	return paintLine(st.Palette, rendered)
}

func findingBadge(st theme.Styles, lvl actions.AuditLevel) string {
	switch lvl {
	case actions.AuditOK:
		return st.StatusOK.Render("✓")
	case actions.AuditWarn:
		return st.StatusDrift.Render("⚠")
	case actions.AuditError:
		return st.StatusGone.Render("✗")
	}
	return " "
}

// themeSwatch renders 5 coloured blocks demonstrating bg/fg/accent/status.
func themeSwatch(p theme.Palette) string {
	block := func(bg lipgloss.Color, fg lipgloss.Color, label string) string {
		return lipgloss.NewStyle().Background(bg).Foreground(fg).Padding(0, 1).Render(label)
	}
	return block(p.Bg, p.Text, "Aa") +
		block(p.BgOverlay, p.Accent, "★") +
		block(p.OK, p.Bg, "ok") +
		block(p.Drift, p.Bg, "~") +
		block(p.Gone, p.Bg, "!")
}
