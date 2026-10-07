// Package layout implements the responsive frame contract: wide, compact and
// too-small modes, master/detail splitting and ANSI-aware overlay geometry.
package layout

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// Mode is the responsive layout mode.
type Mode int

const (
	ModeTooSmall Mode = iota
	ModeCompact
	ModeWide
)

// Thresholds per the fixed layout contract.
const (
	WideMinWidth    = 110
	CompactMinWidth = 72
	MinHeight       = 20
)

// Classify returns the layout mode for a terminal size.
func Classify(width, height int) Mode {
	if width < CompactMinWidth || height < MinHeight {
		return ModeTooSmall
	}
	if width >= WideMinWidth {
		return ModeWide
	}
	return ModeCompact
}

// OverlayGeometry centers a panel of panelW×panelH inside w×h and returns
// the top-left corner. Panels never exceed the canvas.
func OverlayGeometry(w, h, panelW, panelH int) (x, y int) {
	x = (w - panelW) / 2
	y = (h - panelH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

// ComposeOverlay renders base with panel centered on top using the Lip Gloss
// v2 Canvas/Compositor (ANSI-aware, no byte-offset math).
func ComposeOverlay(base, panel string, styles theme.Styles) string {
	w := lipgloss.Width(base)
	h := lipgloss.Height(base)
	pw := lipgloss.Width(panel)
	ph := lipgloss.Height(panel)
	x, y := OverlayGeometry(w, h, pw, ph)
	canvas := lipgloss.NewCanvas(w, h)
	comp := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(panel).X(x).Y(y).Z(1).ID("overlay"),
	)
	return canvas.Compose(comp).Render()
}

// TooSmall renders the dedicated too-small view: required size, actual size
// and the only accepted keys.
func TooSmall(width, height int, styles theme.Styles) string {
	_ = styles
	msg := fmt.Sprintf(
		"tmh-next needs at least %d×%d\nterminal is %d×%d\n\nenlarge the window — press q to quit",
		CompactMinWidth, MinHeight, width, height,
	)
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Render(msg)
}
