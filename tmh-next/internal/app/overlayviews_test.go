package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/testutil"
	"github.com/mark1708/tmh-next/internal/ui"
)

// TestOverlayViewsRender drives every root-owned overlay through the real
// view pipeline: help, palette, quick switch, prompt, confirm and selector
// panels all composite over the frame.
func TestOverlayViewsRender(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)

	// Help overlay
	h.fireKey(t, "?")
	if h.root.ov == nil || h.root.ov.kind != ovHelp {
		t.Fatal("help overlay did not open")
	}
	view := h.root.View().Content
	if !testutil.Contains(view, "global keys") {
		t.Fatalf("help overlay view missing:\n%s", testutil.Plain(view))
	}
	h.fireKey(t, "esc")
	if h.root.ov != nil {
		t.Fatal("help overlay did not close on esc")
	}

	// Palette renders navigation + page commands
	ran := 0
	h.active().commands = []ui.Command{{ID: "x.test", Title: "TestCommand", Run: func() tea.Cmd { ran++; return nil }}}
	h.fireKey(t, "ctrl+p")
	view = h.root.View().Content
	for _, want := range []string{"Command palette", "Dashboard", "Agents"} {
		if !testutil.Contains(view, want) {
			t.Fatalf("palette view missing %q:\n%s", want, testutil.Plain(view))
		}
	}
	// Filter to the page command and run it.
	h.fireKey(t, "/")
	for _, r := range "TestCommand" {
		h.fire(t, testutil.Rune(r))
	}
	if !testutil.Contains(h.root.overlayView(), "TestCommand") {
		t.Fatalf("filtered palette missing command:\n%s", testutil.Plain(h.root.overlayView()))
	}
	h.fireKey(t, "enter") // applies the filter
	h.fireKey(t, "enter") // chooses the focused row
	if ran != 1 {
		t.Fatalf("palette command ran %d times", ran)
	}
	if h.root.ov != nil {
		t.Fatal("palette did not close after run")
	}

	// Quick switch renders entity groups (long lists stay inside the canvas)
	h.fireKey(t, "ctrl+k")
	view = h.root.View().Content
	for _, want := range []string{"Quick switch", "workspace", "terminal"} {
		if !testutil.Contains(view, want) {
			t.Fatalf("quick switch view missing %q", want)
		}
	}
	if max := testutil.MaxLineWidth(view); max > 120 {
		t.Fatalf("quick switch overflow: %d", max)
	}
	h.fireKey(t, "esc")

	// Prompt renders title and input
	h.fire(t, ui.OpenPromptMsg{Title: "Send input", Placeholder: "type here"})
	view = h.root.View().Content
	if !testutil.Contains(view, "Send input") || !testutil.Contains(view, "esc to discard") {
		t.Fatalf("prompt view missing chrome:\n%s", testutil.Plain(view))
	}
	h.fireKey(t, "esc")

	// Confirm renders MOCK ONLY
	h.fire(t, ui.ConfirmActionMsg{Title: "Kill terminal", Detail: "terminal exits",
		Action: domain.Action{Kind: domain.ActionKill, Target: domain.Ref(domain.KindTerminal, "term-0001")}})
	view = h.root.View().Content
	if !testutil.Contains(view, "MOCK ONLY") {
		t.Fatalf("confirm view missing MOCK marker:\n%s", testutil.Plain(view))
	}
	// Reject via n
	h.fireKey(t, "n")
	if h.root.ov != nil {
		t.Fatal("confirm did not close on n")
	}

	// Context selector via page message path
	h.fire(t, ui.OpenSelectorMsg{Title: "Local diff", Options: []ui.SelectorOption{
		{ID: "d1", Title: "window/layout"}, {ID: "d2", Title: "surface/cwd"},
	}})
	view = h.root.View().Content
	if !testutil.Contains(view, "Local diff") || !testutil.Contains(view, "surface/cwd") {
		t.Fatalf("context selector view missing rows:\n%s", testutil.Plain(view))
	}
	h.fireKey(t, "esc")
}

// TestProductionSchedulerCmds verifies the production scheduler emits the
// typed timer/toast messages.
func TestProductionSchedulerCmds(t *testing.T) {
	s := ProductionScheduler{Delay: time.Millisecond}
	msg, ok := testutil.RunCmdFast(s.TimerCmd(7), time.Second)
	if !ok {
		t.Fatal("timer cmd did not finish")
	}
	if tm, is := msg.(TickTimerMsg); !is || tm.Token != 7 {
		t.Fatalf("timer cmd produced %T%v", msg, msg)
	}
	msg, ok = testutil.RunCmdFast(s.ToastCmd(3), 3*time.Second)
	if !ok {
		t.Fatal("toast cmd did not finish")
	}
	if tm, is := msg.(ToastExpiredMsg); !is || tm.Seq != 3 {
		t.Fatalf("toast cmd produced %T%v", msg, msg)
	}
}

// TestRunCLIRunsValidPath covers the wiring entry with valid flags.
func TestRunCLIRunsValidPath(t *testing.T) {
	start := RunCLI([]string{"--page", "agents", "--live=false"})
	if len(start.Stack) != 2 || start.Stack[1].Route != ui.RouteAgents {
		t.Fatalf("RunCLI stack = %v", start.Stack)
	}
	if start.Live {
		t.Fatal("--live=false not honored")
	}
}

// TestWorkspaceStartupResolvesFirst covers the workspace route default.
func TestWorkspaceStartupResolvesFirst(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false,
		Stack: []ui.Location{{Route: ui.RouteDashboard}, {Route: ui.RouteWorkspace}}})
	h.boot(t)
	loc := h.page(ui.RouteWorkspace).locs[0]
	if loc.Primary.ID != "active" {
		// sorted stable IDs of the fixture: active, base, ecp, …
		t.Fatalf("workspace default = %q, want active", loc.Primary.ID)
	}
}

// TestQuickSwitchSelectionNavigatesAndFocuses drives a quick switch choice
// end-to-end: navigation push plus the mock focus action.
func TestQuickSwitchSelectionNavigatesAndFocuses(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fireKey(t, "ctrl+k")
	// First option: workspace active (first in quick switch list)
	h.fireKey(t, "enter")
	// Navigation happened
	if h.root.activeLocation().Route != ui.RouteWorkspace {
		t.Fatalf("quick switch did not navigate: %v", h.root.activeLocation())
	}
	// The focus action reached the recording backend
	actions := h.rec.Actions()
	if len(actions) == 0 {
		t.Fatal("quick switch dispatched no focus action")
	}
	if actions[len(actions)-1].Kind != domain.ActionAttach {
		t.Fatalf("focus action = %s", actions[len(actions)-1].Kind)
	}
}

// TestQuickSwitchFilterAndDisabled proves the shared selector filter works
// in the palette context and disabled entries cannot run.
func TestQuickSwitchFilterAndDisabled(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.active().commands = []ui.Command{
		{ID: "ok", Title: "Runnable", Run: func() tea.Cmd { return nil }},
		{ID: "no", Title: "BlockedOne", Disabled: true, DisabledReason: "not available",
			Run: func() tea.Cmd { return nil }},
	}
	h.fireKey(t, "space")
	if h.root.ov == nil {
		t.Fatal("space did not open the selector")
	}
	// filter down to the disabled entry and try to run it
	h.fireKey(t, "/")
	for _, r := range "Blocked" {
		h.fire(t, testutil.Rune(r))
	}
	// Pump any async filter matches through the overlay's list.
	view := h.root.overlayView()
	if !testutil.Contains(view, "BlockedOne") {
		t.Fatalf("filtered view missing entry:\n%s", testutil.Plain(view))
	}
	h.fireKey(t, "enter") // applies the filter
	h.fireKey(t, "enter") // disabled → no-op
	// The overlay stays open because the entry cannot run.
	if h.root.ov == nil {
		t.Fatal("disabled entry executed and closed the selector")
	}
	h.fireKey(t, "esc")
	_ = strings.TrimSpace
}
