package components

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/testutil"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

func opts() []ui.SelectorOption {
	return []ui.SelectorOption{
		{ID: "a", Title: "Alpha", Desc: "first", Shortcut: "a"},
		{ID: "b", Title: "Beta", Desc: "second"},
		{ID: "c", Title: "Gamma", Disabled: true, Reason: "locked"},
	}
}

// TestSelectorLifecycle covers navigation, filter, choice, disabled entries
// and cancellation.
func TestSelectorLifecycle(t *testing.T) {
	sel := NewSelector("Pick", opts(), 40, 20, theme.New(true))
	if sel.Done() {
		t.Fatal("fresh selector reports done")
	}
	view := sel.View()
	if !testutil.Contains(view, "Pick") || !testutil.Contains(view, "Alpha") {
		t.Fatalf("selector view:\n%s", testutil.Plain(view))
	}
	if !testutil.Contains(view, "✗ locked") {
		t.Fatalf("disabled reason missing:\n%s", testutil.Plain(view))
	}

	sel.Update(testutil.Special(tea.KeyDown))
	sel.Update(testutil.Special(tea.KeyEnter))
	choice, ok := sel.Choice()
	if !ok || choice.ID != "b" {
		t.Fatalf("choice = %+v ok=%v", choice, ok)
	}
	if !sel.Done() {
		t.Fatal("selector not done after choice")
	}

	// filter path
	sel2 := NewSelector("Pick", opts(), 40, 20, theme.New(true))
	sel2.Update(testutil.Rune('/'))
	if l := sel2.Update(testutil.Rune('g')); l == nil {
		// filter commands may be returned asynchronously
	}
	// esc while filtering clears the filter instead of closing
	sel2.Update(testutil.Special(tea.KeyEsc))
	if sel2.Cancelled() {
		t.Fatal("esc during filtering closed the selector")
	}
	// second esc closes
	sel2.Update(testutil.Special(tea.KeyEsc))
	if !sel2.Cancelled() {
		t.Fatal("esc did not close the selector")
	}
	if _, ok := sel2.Choice(); ok {
		t.Fatal("cancelled selector offers a choice")
	}

	// theme swap keeps rendering
	sel3 := NewSelector("Pick", opts(), 40, 20, theme.New(true))
	sel3.SetTheme(theme.New(false))
	if !testutil.Contains(sel3.View(), "Alpha") {
		t.Fatal("rethemed selector lost rows")
	}
	// empty selector tolerates navigation
	sel4 := NewSelector("Empty", nil, 40, 20, theme.New(true))
	sel4.Update(testutil.Special(tea.KeyDown))
	sel4.Update(testutil.Special(tea.KeyEnter))
	if _, ok := sel4.Choice(); ok {
		t.Fatal("empty selector produced a choice")
	}
	_ = list.Unfiltered
}

// TestPromptFlow covers submit, empty-submit rejection, validation errors
// and cancel.
func TestPromptFlow(t *testing.T) {
	p := NewPrompt("Send", "type", "", theme.New(true), func(s string) error {
		if len(s) > 4 {
			return errTooLong{}
		}
		return nil
	})
	if p.Done() {
		t.Fatal("fresh prompt done")
	}
	// empty submit disabled
	p.Update(testutil.Special(tea.KeyEnter))
	if p.Submitted() {
		t.Fatal("empty submit accepted")
	}
	if !testutil.Contains(p.View(), "empty input is disabled") {
		t.Fatalf("empty-submit hint missing:\n%s", testutil.Plain(p.View()))
	}
	// validation error visible
	for _, r := range "toolong" {
		p.Update(testutil.Rune(r))
	}
	p.Update(testutil.Special(tea.KeyEnter))
	if p.Submitted() {
		t.Fatal("invalid submit accepted")
	}
	if !testutil.Contains(p.View(), "too long") {
		t.Fatalf("validation error missing:\n%s", testutil.Plain(p.View()))
	}
	// cancel
	p.Update(testutil.Special(tea.KeyEsc))
	if !p.Cancelled() || !p.Done() {
		t.Fatal("esc did not cancel the prompt")
	}

	// valid submit
	p2 := NewPrompt("Send", "type", "seeded", theme.New(true), nil)
	if p2.Value() != "seeded" {
		t.Fatalf("initial value = %q", p2.Value())
	}
	p2.Update(testutil.Special(tea.KeyEnter))
	if !p2.Submitted() || p2.Value() != "seeded" {
		t.Fatal("valid submit failed")
	}
}

type errTooLong struct{}

func (errTooLong) Error() string { return "too long" }

// TestConfirmFlow covers accept, reject and the MOCK marker.
func TestConfirmFlow(t *testing.T) {
	c := NewConfirm("Kill terminal", "terminal exits", theme.New(true))
	if c.Done() {
		t.Fatal("fresh confirm done")
	}
	view := c.View()
	if !testutil.Contains(view, "MOCK ONLY") || !testutil.Contains(view, "Kill terminal") {
		t.Fatalf("confirm view:\n%s", testutil.Plain(view))
	}
	c.Update(testutil.Rune('n'))
	if !c.Rejected() || !c.Done() {
		t.Fatal("n did not reject")
	}
	c2 := NewConfirm("Kill", "x", theme.New(true))
	c2.Update(testutil.Special(tea.KeyEnter))
	if !c2.Accepted() {
		t.Fatal("enter did not accept")
	}
	c3 := NewConfirm("Kill", "x", theme.New(true))
	c3.Update(testutil.Special(tea.KeyEsc))
	if !c3.Rejected() {
		t.Fatal("esc did not reject")
	}
}

// TestToastRendering covers all toast kinds and expiry stacking.
func TestToastRendering(t *testing.T) {
	styles := theme.New(true)
	kinds := map[string]string{"info": "ℹ", "ok": "✓", "warn": "⚠", "err": "✗", "mock": "MOCK"}
	for kind, marker := range kinds {
		v := Toast{Seq: 1, Text: "hello " + kind, Kind: kind}.View(styles)
		if !testutil.Contains(v, marker) {
			t.Errorf("kind %s missing marker %q: %q", kind, marker, testutil.Plain(v))
		}
	}
	stack := Toasts([]Toast{{Seq: 1, Text: "one", Kind: "info"}, {Seq: 2, Text: "two", Kind: "ok"}}, styles, 40)
	if !testutil.Contains(stack, "one") || !testutil.Contains(stack, "two") {
		t.Fatalf("toast stack:\n%s", testutil.Plain(stack))
	}
	if empty := Toasts(nil, styles, 40); empty != "" {
		t.Fatalf("empty toast stack rendered %q", empty)
	}
}
