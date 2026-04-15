package ui

import (
	"strings"

	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// settingsModel is the settings screen. Currently shows the theme picker;
// future sections (keybindings, trust management, default profile) slot in
// as additional items.
type settingsModel struct {
	keys          Keys
	st            theme.Styles
	str           UIStrings
	width, height int

	themes     []theme.Palette
	themeIndex int

	// onThemeApply is called when the user accepts a theme change, letting
	// the root model update its style bundle and rebuild dependents.
	onThemeApply func(theme.Palette) tea.Cmd
}

func newSettings(keys Keys, st theme.Styles, str UIStrings, onThemeApply func(theme.Palette) tea.Cmd) *settingsModel {
	s := &settingsModel{
		keys:         keys,
		st:           st,
		str:          str,
		themes:       theme.Available,
		onThemeApply: onThemeApply,
	}
	for i, p := range s.themes {
		if p.Name == st.Palette.Name {
			s.themeIndex = i
			break
		}
	}
	return s
}

func (s *settingsModel) Resize(w, h int) { s.width, s.height = w, h }

func (s *settingsModel) SetStyles(st theme.Styles) { s.st = st }

func (s *settingsModel) Update(msg tea.Msg) (*settingsModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case keyMatches(k, s.keys.Down), keyMatches(k, s.keys.Right):
			s.themeIndex = (s.themeIndex + 1) % len(s.themes)
			return s, s.apply()
		case keyMatches(k, s.keys.Up), keyMatches(k, s.keys.Left):
			s.themeIndex = (s.themeIndex - 1 + len(s.themes)) % len(s.themes)
			return s, s.apply()
		}
	}
	return s, nil
}

func (s *settingsModel) apply() tea.Cmd {
	if s.onThemeApply == nil {
		return nil
	}
	return s.onThemeApply(s.themes[s.themeIndex])
}

// View renders a single-pane settings screen. The theme picker lists every
// palette with a live colour preview (bg + fg + accent badge).
func (s *settingsModel) View() string {
	var b strings.Builder
	b.WriteString(s.st.Title.Render(s.str.Settings.Title) + "\n\n")

	b.WriteString(s.st.Subtitle.Render(s.str.Settings.Theme) + "\n")
	for i, p := range s.themes {
		marker := "  "
		if i == s.themeIndex {
			marker = "▸ "
		}
		swatch := themeSwatch(p)
		name := p.Name
		line := marker + name + "   " + swatch
		if i == s.themeIndex {
			line = s.st.Selected.Render(padRight(line, 60))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + s.st.Hint.Render(s.str.Settings.Hint))

	body := padBlock(b.String())
	return placeMiddle(s.width, s.height, s.st.Modal.Render(body), s.st.Palette)
}

// themeSwatch renders 4 coloured blocks demonstrating bg/fg/accent/status.
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
