package pages

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mark1708/tmh-next/internal/ui/theme"
)

const pageGap = 2

// panel gives every page section a stable visual boundary. Width is the
// final rendered width, including the border and horizontal padding.
func (b *base) panel(title, body string, width int) string {
	if width < 8 {
		width = 8
	}
	inner := width - 4 // rounded border (2) + horizontal padding (2)
	if inner < 1 {
		inner = 1
	}
	content := b.styles.Title.Render(title)
	if strings.TrimSpace(body) != "" {
		content += "\n" + body
	}
	return b.styles.Panel.Width(inner).Render(theme.PaintBackground(content, b.styles.Palette.Mantle))
}

// splitView lays two multiline blocks out side by side. The old implementation
// concatenated the complete left string before the right string, which put the
// detail column below the master column instead of beside it.
func (b *base) splitView(left, right string, leftPercent int) string {
	if leftPercent < 25 || leftPercent > 75 {
		leftPercent = 55
	}
	available := b.width - pageGap
	if available < 16 {
		return left + "\n" + right
	}
	leftWidth := available * leftPercent / 100
	rightWidth := available - leftWidth
	leftColumn := lipgloss.NewStyle().Width(leftWidth).MaxWidth(leftWidth).Render(left)
	rightColumn := lipgloss.NewStyle().Width(rightWidth).MaxWidth(rightWidth).Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, strings.Repeat(" ", pageGap), rightColumn)
}

// panelSplit is the common wide master/detail treatment.
func (b *base) panelSplit(leftTitle, leftBody, rightTitle, rightBody string, leftPercent int) string {
	available := b.width - pageGap
	if available < 32 {
		return b.panel(leftTitle, leftBody, b.width) + "\n" + b.panel(rightTitle, rightBody, b.width)
	}
	leftWidth := available * leftPercent / 100
	rightWidth := available - leftWidth
	return b.splitView(
		b.panel(leftTitle, leftBody, leftWidth),
		b.panel(rightTitle, rightBody, rightWidth),
		leftPercent,
	)
}

// panelSplitHeight stretches the final wide dashboard row to the available
// viewport height, preserving the full-screen cockpit silhouette.
func (b *base) panelSplitHeight(leftTitle, leftBody, rightTitle, rightBody string, leftPercent, height int) string {
	available := b.width - pageGap
	if available < 32 {
		return b.panelHeight(leftTitle, leftBody, b.width, height)
	}
	leftWidth := available * leftPercent / 100
	rightWidth := available - leftWidth
	return b.splitView(
		b.panelHeight(leftTitle, leftBody, leftWidth, height),
		b.panelHeight(rightTitle, rightBody, rightWidth, height),
		leftPercent,
	)
}

func (b *base) panelHeight(title, body string, width, height int) string {
	content := b.styles.Header.Render(title)
	if body != "" {
		content += "\n" + body
	}
	style := b.styles.Panel.Width(max(1, width-2))
	if height > 2 {
		style = style.Height(height - 2)
	}
	return style.Render(theme.PaintBackground(content, b.styles.Palette.Mantle))
}

func (b *base) remainingHeight(rendered string, minimum int) int {
	used := strings.Count(rendered, "\n") + 1
	return max(minimum, b.height-used)
}

func (b *base) metricCard(label, value, meta string, width int) string {
	var content strings.Builder
	content.WriteString(b.styles.Dim.Render(strings.ToUpper(label)))
	content.WriteString("\n")
	content.WriteString(b.styles.Header.Render(value))
	if meta != "" {
		content.WriteString("\n")
		content.WriteString(b.styles.Dim.Render(meta))
	}
	return b.styles.Panel.Width(max(1, width-4)).Render(
		theme.PaintBackground(content.String(), b.styles.Palette.Mantle),
	)
}

func horizontalCards(cards []string) string {
	if len(cards) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cards)*2-1)
	for i, card := range cards {
		if i > 0 {
			parts = append(parts, strings.Repeat(" ", pageGap))
		}
		parts = append(parts, card)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
