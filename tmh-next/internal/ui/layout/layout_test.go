package layout

import (
	"strings"
	"testing"

	"github.com/mark1708/tmh-next/internal/testutil"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

func TestClassifyThresholds(t *testing.T) {
	cases := []struct {
		w, h int
		want Mode
	}{
		{160, 48, ModeWide}, {110, 30, ModeWide},
		{109, 30, ModeCompact}, {72, 20, ModeCompact},
		{71, 30, ModeTooSmall}, {80, 19, ModeTooSmall}, {0, 0, ModeTooSmall},
	}
	for _, c := range cases {
		if got := Classify(c.w, c.h); got != c.want {
			t.Errorf("Classify(%d,%d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestOverlayGeometry(t *testing.T) {
	if x, y := OverlayGeometry(100, 40, 40, 10); x != 30 || y != 15 {
		t.Fatalf("center = (%d,%d), want (30,15)", x, y)
	}
	if x, y := OverlayGeometry(20, 10, 40, 10); x != 0 || y != 0 {
		t.Fatalf("oversized panel not clamped: (%d,%d)", x, y)
	}
}

func TestComposeOverlayAndTooSmall(t *testing.T) {
	base := strings.Repeat("x", 200)
	panel := "PANEL"
	out := ComposeOverlay(base, panel, theme.New(true))
	if !testutil.Contains(out, "PANEL") {
		t.Fatal("overlay not composited")
	}
	ts := TooSmall(60, 12, theme.New(true))
	if !strings.Contains(testutil.Plain(ts), "72") || !strings.Contains(testutil.Plain(ts), "60") {
		t.Fatalf("too-small view: %q", testutil.Plain(ts))
	}
}
