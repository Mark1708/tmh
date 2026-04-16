// Package theme defines lipgloss styles for the tmh TUI. Catppuccin Mocha
// is the default; alternative flavours can be set via Apply.
package theme

import "github.com/charmbracelet/lipgloss"

// Palette holds the named colours used across the UI.
type Palette struct {
	Name string

	Bg, BgSubtle, BgOverlay   lipgloss.Color
	Text, TextDim, TextSubtle lipgloss.Color
	Border                    lipgloss.Color
	Accent, AccentDim         lipgloss.Color
	OK, Warn, Bad, Info       lipgloss.Color
	New, Drift, Gone          lipgloss.Color
}

// Mocha is the default Catppuccin Mocha palette.
var Mocha = Palette{
	Name:      "mocha",
	Bg:        "#1e1e2e",
	BgSubtle:  "#181825",
	BgOverlay: "#313244",
	Text:      "#cdd6f4",
	TextDim:   "#a6adc8",
	TextSubtle: "#7f849c",
	Border:    "#45475a",
	Accent:    "#89b4fa",
	AccentDim: "#74c7ec",
	OK:        "#a6e3a1",
	Warn:      "#f9e2af",
	Bad:       "#f38ba8",
	Info:      "#89dceb",
	New:       "#a6e3a1",
	Drift:     "#f9e2af",
	Gone:      "#f38ba8",
}

// Macchiato — slightly warmer.
var Macchiato = Palette{
	Name:       "macchiato",
	Bg:         "#24273a",
	BgSubtle:   "#1e2030",
	BgOverlay:  "#363a4f",
	Text:       "#cad3f5",
	TextDim:    "#a5adcb",
	TextSubtle: "#8087a2",
	Border:     "#494d64",
	Accent:     "#8aadf4",
	AccentDim:  "#7dc4e4",
	OK:         "#a6da95",
	Warn:       "#eed49f",
	Bad:        "#ed8796",
	Info:       "#91d7e3",
	New:        "#a6da95",
	Drift:      "#eed49f",
	Gone:       "#ed8796",
}

// Latte — light theme.
var Latte = Palette{
	Name:       "latte",
	Bg:         "#eff1f5",
	BgSubtle:   "#e6e9ef",
	BgOverlay:  "#ccd0da",
	Text:       "#4c4f69",
	TextDim:    "#5c5f77",
	TextSubtle: "#6c6f85",
	Border:     "#bcc0cc",
	Accent:     "#1e66f5",
	AccentDim:  "#04a5e5",
	OK:         "#40a02b",
	Warn:       "#df8e1d",
	Bad:        "#d20f39",
	Info:       "#179299",
	New:        "#40a02b",
	Drift:      "#df8e1d",
	Gone:       "#d20f39",
}

// Frappe — muted dark.
var Frappe = Palette{
	Name:       "frappe",
	Bg:         "#303446",
	BgSubtle:   "#292c3c",
	BgOverlay:  "#414559",
	Text:       "#c6d0f5",
	TextDim:    "#a5adce",
	TextSubtle: "#838ba7",
	Border:     "#51576d",
	Accent:     "#8caaee",
	AccentDim:  "#85c1dc",
	OK:         "#a6d189",
	Warn:       "#e5c890",
	Bad:        "#e78284",
	Info:       "#99d1db",
	New:        "#a6d189",
	Drift:      "#e5c890",
	Gone:       "#e78284",
}

// Available is the cycle order for Ctrl+Shift+T.
var Available = []Palette{Mocha, Macchiato, Frappe, Latte}

// Styles bundles every reusable lipgloss style derived from a palette.
type Styles struct {
	Palette Palette

	Background, Header, Footer, Tab            lipgloss.Style
	Panel, PanelFocus                          lipgloss.Style
	Title, Subtitle, Hint, Selected            lipgloss.Style
	StatusOK, StatusDrift, StatusNew, StatusGone lipgloss.Style
	// Toast is the default (info) style; ToastSuccess and ToastError are
	// kind-specific variants used when the message carries a known severity.
	Toast, ToastSuccess, ToastError lipgloss.Style
	Modal                           lipgloss.Style
	KeyBinding                                 lipgloss.Style
}

// New builds a Styles tree from a palette.
func New(p Palette) Styles {
	border := lipgloss.RoundedBorder()
	return Styles{
		Palette: p,
		Background: lipgloss.NewStyle().
			Background(p.Bg).Foreground(p.Text),
		Header: lipgloss.NewStyle().
			Background(p.BgSubtle).Foreground(p.Accent).Bold(true).
			Padding(0, 1),
		Footer: lipgloss.NewStyle().
			Background(p.BgSubtle).Foreground(p.TextSubtle).
			Padding(0, 1),
		Tab: lipgloss.NewStyle().Foreground(p.TextDim).Padding(0, 1),
		Panel: lipgloss.NewStyle().
			Border(border).BorderForeground(p.Border).
			Padding(0, 1),
		PanelFocus: lipgloss.NewStyle().
			Border(border).BorderForeground(p.Accent).
			Padding(0, 1),
		Title:    lipgloss.NewStyle().Foreground(p.Text).Bold(true),
		Subtitle: lipgloss.NewStyle().Foreground(p.TextDim),
		Hint:     lipgloss.NewStyle().Foreground(p.TextSubtle).Italic(true),
		Selected: lipgloss.NewStyle().
			Background(p.BgOverlay).Foreground(p.Text).Bold(true),
		StatusOK:    lipgloss.NewStyle().Foreground(p.OK),
		StatusDrift: lipgloss.NewStyle().Foreground(p.Drift).Bold(true),
		StatusNew:   lipgloss.NewStyle().Foreground(p.New).Bold(true),
		StatusGone:  lipgloss.NewStyle().Foreground(p.Gone).Bold(true),
		Toast: lipgloss.NewStyle().
			Background(p.BgOverlay).Foreground(p.Text).
			Padding(0, 1),
		ToastSuccess: lipgloss.NewStyle().
			Background(p.BgOverlay).Foreground(p.OK).Bold(true).
			Padding(0, 1),
		ToastError: lipgloss.NewStyle().
			Background(p.BgOverlay).Foreground(p.Bad).Bold(true).
			Padding(0, 1),
		Modal: lipgloss.NewStyle().
			Border(border).BorderForeground(p.Accent).
			Background(p.BgOverlay).Foreground(p.Text).
			Padding(1, 3),
		KeyBinding: lipgloss.NewStyle().Foreground(p.Accent).Bold(true),
	}
}

// Cycle returns the next palette in Available after the current one.
func Cycle(current Palette) Palette {
	for i, p := range Available {
		if p.Name == current.Name {
			return Available[(i+1)%len(Available)]
		}
	}
	return Available[0]
}
