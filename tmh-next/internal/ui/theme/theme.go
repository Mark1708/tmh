// Package theme defines Catppuccin Mocha (dark) / Latte (light) semantic
// styles for the whole cockpit. Dark is the default until the terminal
// answers tea.BackgroundColorMsg; a response rebuilds every style set.
package theme

import (
	"charm.land/lipgloss/v2"
	"fmt"
	"image/color"
	"strings"
)

// Palette is a Catppuccin flavour subset used by the cockpit.
type Palette struct {
	Base, Mantle, Crust    color.Color
	Text, Subtext, Overlay color.Color
	Surface0, Surface1     color.Color
	Blue, Sapphire         color.Color
	Green, Yellow, Red     color.Color
	Mauve, Peach, Teal     color.Color
}

// Mocha is the dark flavour (default).
func Mocha() Palette {
	return Palette{
		Base: lipgloss.Color("#1e1e2e"), Mantle: lipgloss.Color("#181825"), Crust: lipgloss.Color("#11111b"),
		Text: lipgloss.Color("#cdd6f4"), Subtext: lipgloss.Color("#a6adc8"), Overlay: lipgloss.Color("#6c7086"),
		Surface0: lipgloss.Color("#313244"), Surface1: lipgloss.Color("#45475a"),
		Blue: lipgloss.Color("#89b4fa"), Sapphire: lipgloss.Color("#74c7ec"),
		Green: lipgloss.Color("#a6e3a1"), Yellow: lipgloss.Color("#f9e2af"), Red: lipgloss.Color("#f38ba8"),
		Mauve: lipgloss.Color("#cba6f7"), Peach: lipgloss.Color("#fab387"), Teal: lipgloss.Color("#94e2d5"),
	}
}

// Latte is the light flavour.
func Latte() Palette {
	return Palette{
		Base: lipgloss.Color("#eff1f5"), Mantle: lipgloss.Color("#e6e9ef"), Crust: lipgloss.Color("#dce0e8"),
		Text: lipgloss.Color("#4c4f69"), Subtext: lipgloss.Color("#6c6f85"), Overlay: lipgloss.Color("#9ca0b0"),
		Surface0: lipgloss.Color("#ccd0da"), Surface1: lipgloss.Color("#bcc0cc"),
		Blue: lipgloss.Color("#1e66f5"), Sapphire: lipgloss.Color("#209fb5"),
		Green: lipgloss.Color("#40a02b"), Yellow: lipgloss.Color("#df8e1d"), Red: lipgloss.Color("#d20f39"),
		Mauve: lipgloss.Color("#8839ef"), Peach: lipgloss.Color("#fe640b"), Teal: lipgloss.Color("#179299"),
	}
}

// Styles is the semantic style set shared by root, components and pages.
type Styles struct {
	Palette Palette
	IsDark  bool

	Base       lipgloss.Style // body text
	Dim        lipgloss.Style // secondary text
	Title      lipgloss.Style // page/panel titles
	Header     lipgloss.Style // header bar text
	Footer     lipgloss.Style // footer bar text
	Breadcrumb lipgloss.Style

	OK     lipgloss.Style // OK / healthy
	Warn   lipgloss.Style // DRIFT / needs input
	Err    lipgloss.Style // OFFLINE / errors
	Info   lipgloss.Style // informational accents
	Accent lipgloss.Style

	Mock     lipgloss.Style // MOCK badge — every mock surface
	Chip     lipgloss.Style // small status chips
	Panel    lipgloss.Style // bordered panels
	Overlay  lipgloss.Style // modal overlay frame
	Selected lipgloss.Style // selected rows

	KeyValue lipgloss.Style // key column of key/value rows

	Bar       lipgloss.Style // sparkline bar
	BarHot    lipgloss.Style // sparkline bar over target
	Workspace lipgloss.Style
	Terminal  lipgloss.Style
	Agent     lipgloss.Style
	Snapshot  lipgloss.Style
	Plan      lipgloss.Style
}

// New builds the semantic style set for a flavour.
func New(isDark bool) Styles {
	p := Mocha()
	if !isDark {
		p = Latte()
	}
	pad := lipgloss.NewStyle().Padding(0, 1)
	s := Styles{
		Palette:    p,
		IsDark:     isDark,
		Base:       lipgloss.NewStyle().Foreground(p.Text),
		Dim:        lipgloss.NewStyle().Foreground(p.Subtext),
		Title:      lipgloss.NewStyle().Foreground(p.Blue).Bold(true),
		Header:     lipgloss.NewStyle().Foreground(p.Text).Bold(true),
		Footer:     lipgloss.NewStyle().Foreground(p.Subtext),
		Breadcrumb: lipgloss.NewStyle().Foreground(p.Subtext),
		OK:         lipgloss.NewStyle().Foreground(p.Green),
		Warn:       lipgloss.NewStyle().Foreground(p.Yellow),
		Err:        lipgloss.NewStyle().Foreground(p.Red),
		Info:       lipgloss.NewStyle().Foreground(p.Blue),
		Accent:     lipgloss.NewStyle().Foreground(p.Mauve),
		Mock:       lipgloss.NewStyle().Foreground(p.Peach).Bold(true),
		Chip:       pad.Foreground(p.Text).Background(p.Crust),
		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Surface1).
			Background(p.Mantle).
			Padding(0, 1),
		Overlay: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Blue).
			Background(p.Mantle).
			Padding(0, 2),
		Selected:  lipgloss.NewStyle().Foreground(p.Crust).Background(p.Blue).Bold(true),
		KeyValue:  lipgloss.NewStyle().Foreground(p.Subtext),
		Bar:       lipgloss.NewStyle().Foreground(p.Blue),
		BarHot:    lipgloss.NewStyle().Foreground(p.Red),
		Workspace: lipgloss.NewStyle().Foreground(p.Teal),
		Terminal:  lipgloss.NewStyle().Foreground(p.Sapphire),
		Agent:     lipgloss.NewStyle().Foreground(p.Mauve),
		Snapshot:  lipgloss.NewStyle().Foreground(p.Green),
		Plan:      lipgloss.NewStyle().Foreground(p.Yellow),
	}
	return s
}

// PaintBackground makes a container background survive nested ANSI styles.
// Lip Gloss emits a full SGR reset after each styled span; without an
// immediate reapply, subsequent text falls back to the terminal's default
// background and becomes transparent in terminals with opacity enabled.
func PaintBackground(content string, background color.Color) string {
	if content == "" {
		return ""
	}
	r, g, b, _ := background.RGBA()
	backgroundSGR := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)

	var painted strings.Builder
	painted.Grow(len(content) + len(backgroundSGR)*4)
	painted.WriteString(backgroundSGR)
	for cursor := 0; cursor < len(content); {
		relative := strings.Index(content[cursor:], "\x1b[")
		if relative < 0 {
			painted.WriteString(content[cursor:])
			break
		}
		start := cursor + relative
		painted.WriteString(content[cursor:start])
		end := start + 2
		for end < len(content) && (content[end] < 0x40 || content[end] > 0x7e) {
			end++
		}
		if end >= len(content) {
			painted.WriteString(content[start:])
			break
		}
		painted.WriteString(content[start : end+1])
		if content[end] == 'm' && sgrResetsBackground(content[start+2:end]) {
			painted.WriteString(backgroundSGR)
		}
		cursor = end + 1
	}
	painted.WriteString("\x1b[m")
	return painted.String()
}

func sgrResetsBackground(parameters string) bool {
	if parameters == "" {
		return true
	}
	for _, parameter := range strings.Split(parameters, ";") {
		if parameter == "" || parameter == "0" || parameter == "49" {
			return true
		}
	}
	return false
}

// Select renders nested styled content on a continuous selection background.
func (s Styles) Select(content string) string {
	return s.Selected.Render(PaintBackground(content, s.Palette.Blue))
}

// Status renders a status word with its color.
func (s Styles) Status(word string) string {
	switch word {
	case "OK", "LIVE", "ONLINE", "HEALTHY", "RUNNING", "APPLIED", "PENDING", "VALID":
		return s.OK.Render(word)
	case "DRIFT", "NEEDS INPUT", "WAITING", "PREVIEW", "FROZEN", "WARN":
		return s.Warn.Render(word)
	case "OFFLINE", "ERROR", "EXITED", "UNAVAILABLE", "FAILED":
		return s.Err.Render(word)
	case "MOCK":
		return s.Mock.Render(word)
	default:
		return s.Dim.Render(word)
	}
}
