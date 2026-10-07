package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/mock"
	"github.com/mark1708/tmh-next/internal/testutil"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/pages"
)

// Wiring proof: the demo client satisfies the app data boundary.
var _ control.Client = (*mock.Client)(nil)

// realHarness boots the root over REAL pages and the REAL mock backend.
type realHarness struct {
	root   *Root
	client *mock.Client
	sched  *ManualScheduler
}

func newRealHarness(t *testing.T, start Startup) *realHarness {
	t.Helper()
	client := mock.NewClient(start.Scenario)
	sched := &ManualScheduler{}
	root := NewRoot(Options{
		Client: client, DemoTicker: client, Scheduler: sched, Live: start.Live,
		Stack: start.Stack, QuickSwitch: start.QuickSwitch, Factory: pages.Factory,
	})
	h := &realHarness{root: root, client: client, sched: sched}
	h.bootReal(t)
	return h
}

func (h *realHarness) bootReal(t *testing.T) {
	t.Helper()
	cmd := h.root.Init()
	h.pumpReal(t, cmd)
	h.step(testutil.Size(120, 40))
}

// step feeds one message and returns the follow-up command.
func (h *realHarness) step(msg tea.Msg) tea.Cmd {
	_, cmd := h.root.Update(msg)
	return cmd
}

// pumpReal drains a command chain, expanding batches and feeding results
// back (bounded), ignoring timer-driven commands.
func (h *realHarness) pumpReal(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		res, ok := testutil.RunCmdFast(c, fastGate)
		if !ok {
			continue // timer-driven (blink/tick) — skip deterministically
		}
		if res == nil {
			continue
		}
		if batch, ok := res.(tea.BatchMsg); ok {
			for _, bc := range batch {
				queue = append(queue, bc)
			}
			continue
		}
		next := h.step(res)
		if next != nil {
			queue = append(queue, next)
		}
	}
}

// key presses a key through the full precedence chain.
func (h *realHarness) key(t *testing.T, s string) {
	t.Helper()
	h.pumpReal(t, h.step(keyMsg(s)))
}

const fastGate = 150000000 // 150ms in ns, kept as int for clarity

// openForm opens the focused General editor through the real page.
func (h *realHarness) openForm(t *testing.T) {
	h.openSectionForm(t, 0)
}

func (h *realHarness) openSectionForm(t *testing.T, section int) {
	t.Helper()
	h.key(t, "ctrl+p")
	// palette: navigate to Settings (last navigation option = index 14)
	for range 14 {
		h.key(t, "j")
	}
	h.key(t, "enter")
	for range section {
		h.key(t, "j")
	}
	h.key(t, "enter")
	if h.root.ov == nil || h.root.ov.kind != ovConfig {
		t.Fatalf("settings form did not open (overlay=%+v)", h.root.ov)
	}
}

func formOf(t *testing.T, h *realHarness) *huh.Form {
	t.Helper()
	f, ok := h.root.ov.form.(*huh.Form)
	if !ok {
		t.Fatal("no huh form open")
	}
	return f
}

// TestConfigSaveDiscardValidation drives the real root-owned Huh form: an
// invalid draft stays editable with zero saves, a valid draft saves exactly
// once (Config + Event, no terminal History), Esc discards without touching
// the catalog.
func TestConfigSaveDiscardValidation(t *testing.T) {
	t.Run("esc discards without mutation", func(t *testing.T) {
		h := newRealHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
		h.openForm(t)
		before := h.root.cat
		h.key(t, "esc")
		if h.root.ov != nil {
			t.Fatal("esc did not close the form")
		}
		if h.root.cat != before || h.root.cat.Revision != before.Revision {
			t.Fatal("esc mutated the catalog")
		}
		if h.root.activeLocation().Route != ui.RouteConfig {
			t.Fatalf("esc popped the route: %v", h.root.activeLocation())
		}
	})

	t.Run("invalid draft stays visible without saving", func(t *testing.T) {
		h := newRealHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
		_, _ = h.client.Snapshot(t.Context()) // warm
		h.openSectionForm(t, 2)

		// Corrupt the History draft: persistent → memory while retention stays
		// 720 (violates memory ≤ 24h).
		h.key(t, "k")
		h.key(t, "enter") // → retention input (720 stays)
		h.key(t, "enter") // → overflow policy
		h.key(t, "enter") // → save confirm
		h.key(t, "y")     // save=yes → invalid (memory + 720h)

		f := formOf(t, h)
		if f.State != huh.StateNormal {
			t.Fatalf("invalid draft completed the form: %v", f.State)
		}
		view := h.root.ov.form.View()
		if !testutil.Contains(view, "memory history retention") {
			t.Fatalf("cross-field error not visible:\n%s", testutil.Plain(view))
		}
		if h.root.cat.Config.HistoryMode != domain.HistoryPersistent {
			t.Fatal("invalid draft changed the catalog config")
		}

		// Discard the corrupted draft with the root-owned Esc: no save, no
		// catalog change, route unchanged.
		route := h.root.activeLocation().Route
		h.key(t, "esc")
		if h.root.ov != nil {
			t.Fatal("esc did not discard the invalid draft")
		}
		if h.root.cat.Config.HistoryMode != domain.HistoryPersistent || h.root.cat.Revision != 1 {
			t.Fatal("esc mutated the catalog")
		}
		if h.root.activeLocation().Route != route {
			t.Fatal("esc popped the route from inside the form")
		}
	})

	t.Run("valid draft saves exactly once", func(t *testing.T) {
		h := newRealHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
		h.openForm(t)

		// Keep the valid General baseline; walk to Save and accept.
		h.key(t, "enter")
		h.key(t, "enter")
		h.key(t, "y")

		if h.root.ov != nil {
			t.Fatal("completed form did not close")
		}
		cfg := h.root.cat.Config
		if cfg.HistoryMode != domain.HistoryPersistent || cfg.RetentionHours != 720 {
			t.Fatalf("config save did not apply: %+v", cfg)
		}
		if h.root.cat.Revision != 2 {
			t.Fatalf("revision after save = %d, want 2 (event recorded)", h.root.cat.Revision)
		}
		if got := len(h.root.cat.History); got != 5 {
			t.Fatalf("config save polluted terminal history: %d chunks", got)
		}
		last := h.root.cat.Events[len(h.root.cat.Events)-1]
		if !strings.Contains(last.Message, "saved") {
			t.Fatalf("config save event missing: %q", last.Message)
		}
	})
}

// TestHuhInvalidSubmitStaysVisibleAndValidSubmitsOnce locks the root-owned
// contract against the real pages: invalid submit keeps the form visible and
// dispatches zero config-save actions; valid submit dispatches exactly one.
func TestHuhInvalidSubmitStaysVisibleAndValidSubmitsOnce(t *testing.T) {
	h := newRealHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.openSectionForm(t, 2)

	saves := func() int {
		n := 0
		for _, e := range h.root.cat.Events {
			if e.Category == domain.EventConfig && strings.Contains(e.Message, "saved") {
				n++
			}
		}
		return n
	}

	// Invalid: memory + 720 retention.
	h.key(t, "k")
	h.key(t, "enter")
	h.key(t, "enter")
	h.key(t, "enter")
	h.key(t, "y")

	if saves() != 0 {
		t.Fatalf("invalid submit dispatched %d config saves", saves())
	}
	f := formOf(t, h)
	if f.State != huh.StateNormal {
		t.Fatalf("invalid submit state = %v", f.State)
	}
	if h.root.ov == nil {
		t.Fatal("invalid submit closed the form")
	}

	// A fresh, valid form submits exactly once.
	h2 := newRealHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h2.openForm(t)
	h2.key(t, "enter")
	h2.key(t, "enter")
	h2.key(t, "y")
	if h2.root.ov != nil {
		t.Fatal("form still open after completion")
	}
	saves2 := 0
	for _, e := range h2.root.cat.Events {
		if e.Category == domain.EventConfig && strings.Contains(e.Message, "saved") {
			saves2++
		}
	}
	if saves2 != 1 {
		t.Fatalf("valid submit dispatched %d config saves, want 1", saves2)
	}
	if got := saves(); got != 0 {
		t.Fatalf("invalid leg ended with %d saves", got)
	}
}

// TestHuhEscapeNeverPopsRoute proves root-owned Esc discards the draft and
// never reaches navigation while the form is open.
func TestHuhEscapeNeverPopsRoute(t *testing.T) {
	h := newRealHarness(t, Startup{
		Scenario: domain.ScenarioDefault, Live: false,
		Stack: []ui.Location{{Route: ui.RouteDashboard}, {Route: ui.RouteConfig}},
	})
	h.openForm(t)
	depth := len(h.root.stack)
	h.key(t, "esc")
	if len(h.root.stack) != depth {
		t.Fatalf("esc popped the route stack: %d → %d", depth, len(h.root.stack))
	}
	if h.root.activeLocation().Route != ui.RouteConfig {
		t.Fatalf("route after esc = %v", h.root.activeLocation().Route)
	}
	if h.root.ov != nil {
		t.Fatal("form not discarded")
	}
	// Esc after discard behaves normally again (pops to dashboard).
	h.key(t, "esc")
	if h.root.activeLocation().Route != ui.RouteDashboard {
		t.Fatalf("post-discard esc did not pop: %v", h.root.activeLocation())
	}
}

// TestCockpitTour walks the full mock product surface through the real root:
// picker startup, palette navigation, agent prompt, snapshot plan →
// reconcile apply → undo.
func TestCockpitTour(t *testing.T) {
	h := newRealHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false, QuickSwitch: true})
	if h.root.ov == nil || h.root.ov.kind != ovQuickSwitch {
		t.Fatalf("picker-first startup did not open quick switch: %+v", h.root.ov)
	}
	h.key(t, "esc") // → Dashboard
	if h.root.activeLocation().Route != ui.RouteDashboard {
		t.Fatalf("quick switch esc did not land on dashboard: %v", h.root.activeLocation())
	}
	if !testutil.Contains(h.root.View().Content, "Dashboard") {
		t.Fatal("dashboard view missing title")
	}

	// Palette → Agents (index 5 in AllRoutes order).
	h.key(t, "ctrl+p")
	for range 5 {
		h.key(t, "j")
	}
	h.key(t, "enter")
	if h.root.activeLocation().Route != ui.RouteAgents {
		t.Fatalf("palette did not navigate to agents: %v", h.root.activeLocation())
	}
	// Select the needs_input agent (agent-0002 is second by stable ID).
	h.key(t, "j")
	// Prompt via root-owned prompt overlay.
	h.key(t, "p")
	if h.root.ov == nil || h.root.ov.kind != ovPrompt {
		t.Fatalf("agent prompt overlay missing: %+v", h.root.ov)
	}
	for _, r := range "use fiscal 2025" {
		h.pumpReal(t, h.step(testutil.Rune(r)))
	}
	h.key(t, "enter")

	agent := h.root.cat.AgentByID("agent-0002")
	if agent == nil || agent.State != domain.AgentRunning {
		t.Fatalf("prompt did not accept: %+v", agent)
	}
	if got := len(agent.Timeline); got != 4 {
		t.Fatalf("timeline entries = %d, want 4", got)
	}
	if h.root.cat.Revision != 2 {
		t.Fatalf("revision after prompt = %d, want 2", h.root.cat.Revision)
	}
	histChunks := len(h.root.cat.History)

	// Snapshots → plan → Reconcile → apply → undo.
	h.key(t, "ctrl+p")
	for range 8 { // snapshots is index 8 in AllRoutes order
		h.key(t, "j")
	}
	h.key(t, "enter")
	if h.root.activeLocation().Route != ui.RouteSnapshots {
		t.Fatalf("palette did not navigate to snapshots: %v", h.root.activeLocation())
	}
	// snap-0004 (kb) carries drift: its plan has pull + freeze ops.
	h.key(t, "j")
	h.key(t, "j")
	h.key(t, "j")
	h.key(t, "r") // plan + navigate
	if h.root.activeLocation().Route != ui.RouteReconcile {
		t.Fatalf("plan did not navigate to reconcile: %v", h.root.activeLocation())
	}
	planID := h.root.cat.RestorePlans[len(h.root.cat.RestorePlans)-1].ID
	h.key(t, "tab") // push → pull (kb has one pull op)
	if h.root.ov != nil {
		t.Fatal("unexpected overlay before apply")
	}
	h.key(t, "a") // apply (confirm)
	if h.root.ov == nil || h.root.ov.kind != ovConfirm {
		t.Fatalf("apply did not open confirmation: %+v", h.root.ov)
	}
	if !testutil.Contains(h.root.ov.confirm.View(), "MOCK") {
		t.Fatal("confirmation missing MOCK marker")
	}
	h.key(t, "y")
	kb := h.root.cat.WorkspaceByID("kb")
	if kb.Status != domain.WorkspaceOK || len(kb.Drift) != 0 {
		for _, toast := range h.root.toasts {
			t.Logf("toast: %+v", toast)
		}
		t.Fatalf("apply did not reconcile kb: %s drift=%d", kb.Status, len(kb.Drift))
	}
	h.key(t, "u") // undo
	kb = h.root.cat.WorkspaceByID("kb")
	if kb.Status != domain.WorkspaceDrift || len(kb.Drift) != 1 {
		t.Fatalf("undo did not restore drift: %s drift=%d", kb.Status, len(kb.Drift))
	}
	if h.root.cat.RestorePlanByID(planID).Status != domain.PlanUndone {
		t.Fatal("undo did not mark the plan undone")
	}
	if got := len(h.root.cat.History); got != histChunks {
		t.Fatalf("history chunks = %d, want %d (plan/apply/undo add no terminal history)", got, histChunks)
	}
}

// TestDegradedInlineRecoverableErrors proves degraded failures surface inline
// toasts without breaking input, per PTY Launch B.
func TestDegradedInlineRecoverableErrors(t *testing.T) {
	h := newRealHarness(t, Startup{
		Scenario: domain.ScenarioDegraded, Live: false,
		Stack: []ui.Location{{Route: ui.RouteDashboard}, {Route: ui.RouteBackends, Primary: domain.Ref(domain.KindBackend, "native")}},
	})
	if h.root.cat.BackendByID("native") == nil {
		t.Fatal("native backend missing in degraded startup")
	}
	rev := h.root.cat.Revision

	// Probe native → timeout, inline error notification, revision unchanged.
	h.key(t, "p")
	if h.root.cat.Revision != rev {
		t.Fatalf("failed probe bumped revision: %d → %d", rev, h.root.cat.Revision)
	}
	found := false
	for _, toast := range h.root.toasts {
		if strings.Contains(toast.Text, "Backend probe timed out") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no actionable timeout notification: %+v", h.root.toasts)
	}

	// Input still works: navigate to machines and ping the offline node.
	h.key(t, "esc") // back to dashboard
	h.key(t, "ctrl+p")
	for range 11 { // machines is index 11
		h.key(t, "j")
	}
	h.key(t, "enter")
	h.key(t, "j") // select homelab? machines sorted: build-01, homelab, local → j = homelab
	h.key(t, "j") // local
	h.key(t, "p") // ping local → ok
	ok := false
	for _, toast := range h.root.toasts {
		if strings.Contains(toast.Text, "machine-ping") || strings.Contains(toast.Text, "ok") {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("healthy ping toast missing: %+v", h.root.toasts)
	}

	// Offline machine: a clear connect error appears without mutation.
	h.key(t, "k")
	h.key(t, "k") // back to build-01
	rev = h.root.cat.Revision
	h.key(t, "c")
	if h.root.cat.Revision != rev {
		t.Fatal("offline connect mutated the catalog")
	}
	seen := false
	for _, toast := range h.root.toasts {
		if strings.Contains(toast.Text, "Machine connect unavailable") &&
			strings.Contains(toast.Text, "build-01 is offline") {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("offline connect notification missing: %+v", h.root.toasts)
	}
	// q quits cleanly from anywhere.
	if cmd := h.step(keyMsg("q")); cmd == nil {
		t.Fatal("q did not quit")
	}
}
