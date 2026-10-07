package theme

import (
	"testing"

	"github.com/mark1708/tmh-next/internal/testutil"
)

func TestPalettesAndStatus(t *testing.T) {
	dark := New(true)
	light := New(false)
	if !dark.IsDark || light.IsDark {
		t.Fatal("dark/light flags wrong")
	}
	if dark.Palette.Base == light.Palette.Base {
		t.Fatal("palettes identical")
	}
	// status words always render text + color
	for _, w := range []string{"OK", "DRIFT", "OFFLINE", "MOCK", "OTHER"} {
		if s := testutil.Plain(dark.Status(w)); s != w {
			t.Errorf("status %q rendered %q", w, s)
		}
	}
	if testutil.Plain(dark.Mock.Render("x")) != "x" {
		t.Fatal("mock style drops text")
	}
	if Mocha().Base == nil || Latte().Base == nil {
		t.Fatal("palette constructors empty")
	}
}
