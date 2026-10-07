package app

import (
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/testutil"
	"github.com/mark1708/tmh-next/internal/ui"
)

func TestRootViewFillsWideTerminalCanvas(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, testutil.Size(192, 54))
	view := testutil.Plain(h.root.View().Content)
	if width := testutil.MaxLineWidth(view); width != 192 {
		t.Fatalf("root canvas width = %d, want 192", width)
	}
	if height := len(strings.Split(view, "\n")); height < 52 {
		t.Fatalf("root canvas height = %d, want at least 52", height)
	}
}

func TestPaletteIsBoundedOnWideTerminal(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, testutil.Size(192, 54))
	h.fireKey(t, "ctrl+p")
	panel := testutil.Plain(h.root.overlayView())
	if width := testutil.MaxLineWidth(panel); width > 72 {
		t.Fatalf("palette width = %d, want <= 72", width)
	}
	if height := len(strings.Split(panel, "\n")); height > 36 {
		t.Fatalf("palette height = %d, want <= 36", height)
	}
}

func TestRootViewPaintsBackgroundUnderEveryVisibleGlyph(t *testing.T) {
	t.Run("config page", func(t *testing.T) {
		h := newRealHarness(t, Startup{
			Scenario: domain.ScenarioDefault,
			Live:     false,
			Stack: []ui.Location{
				{Route: ui.RouteDashboard},
				{Route: ui.RouteConfig},
			},
		})
		h.step(testutil.Size(192, 54))
		assertEveryVisibleGlyphHasBackground(t, h.root.View().Content)
	})

	t.Run("dashboard panels", func(t *testing.T) {
		h := newRealHarness(t, Startup{
			Scenario: domain.ScenarioDefault,
			Live:     false,
			Stack:    []ui.Location{{Route: ui.RouteDashboard}},
		})
		h.step(testutil.Size(192, 54))
		assertEveryVisibleGlyphHasBackground(t, h.root.View().Content)
	})

	t.Run("command palette", func(t *testing.T) {
		h := newRealHarness(t, Startup{
			Scenario: domain.ScenarioDefault,
			Live:     false,
			Stack:    []ui.Location{{Route: ui.RouteDashboard}},
		})
		h.step(testutil.Size(192, 54))
		h.key(t, "ctrl+p")
		assertEveryVisibleGlyphHasBackground(t, h.root.View().Content)
	})
}

func assertEveryVisibleGlyphHasBackground(t *testing.T, rendered string) {
	t.Helper()
	backgroundSet := false
	for i := 0; i < len(rendered); {
		if rendered[i] == '\x1b' && i+1 < len(rendered) && rendered[i+1] == '[' {
			end := i + 2
			for end < len(rendered) && (rendered[end] < 0x40 || rendered[end] > 0x7e) {
				end++
			}
			if end >= len(rendered) {
				t.Fatalf("unterminated ANSI sequence at byte %d", i)
			}
			if rendered[end] == 'm' {
				applyBackgroundSGR(rendered[i+2:end], &backgroundSet)
			}
			i = end + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(rendered[i:])
		if r != '\n' && !unicode.IsSpace(r) && !backgroundSet {
			t.Fatalf("visible rune %q at byte %d is rendered without an explicit background", r, i)
		}
		i += size
	}
}

func applyBackgroundSGR(parameters string, backgroundSet *bool) {
	if parameters == "" {
		*backgroundSet = false
		return
	}
	for _, parameter := range strings.Split(parameters, ";") {
		code, err := strconv.Atoi(parameter)
		if err != nil {
			continue
		}
		switch {
		case code == 0 || code == 49:
			*backgroundSet = false
		case code == 48, code >= 40 && code <= 47, code >= 100 && code <= 107:
			*backgroundSet = true
		}
	}
}
