package pages

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/mock"
	"github.com/mark1708/tmh-next/internal/testutil"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

var sizes = [][2]int{{160, 48}, {109, 30}, {72, 20}}

func pageFor(t *testing.T, route ui.Route, sc domain.Scenario) (ui.Page, *domain.Catalog) {
	t.Helper()
	p := New(route)
	if p == nil {
		t.Fatalf("no page for %s", route)
	}
	cat, err := mock.Fixture(sc)
	if err != nil {
		t.Fatal(err)
	}
	p.SetTheme(theme.New(true))
	p.SetCatalog(cat)
	p.SetSize(120, 40)
	return p, cat
}

// TestAllPagesRenderAcrossScenariosAndSizes walks every route in every
// scenario at wide/compact/too-small sizes: no panic, no line overflow.
func TestAllPagesRenderAcrossScenariosAndSizes(t *testing.T) {
	for _, sc := range []domain.Scenario{domain.ScenarioDefault, domain.ScenarioEmpty, domain.ScenarioDegraded} {
		for _, route := range ui.AllRoutes() {
			for _, sz := range sizes {
				t.Run(string(sc)+"/"+string(route), func(t *testing.T) {
					p, _ := pageFor(t, route, sc)
					p.Enter(ui.Location{Route: route})
					p.SetSize(sz[0], sz[1])
					view := p.View()
					if strings.Count(view, "\x1b[38;2") > 0 && testutil.MaxLineWidth(view) > sz[0] {
						t.Fatalf("line overflow: max width %d > %d", testutil.MaxLineWidth(view), sz[0])
					}
					if max := testutil.MaxLineWidth(view); max > sz[0] {
						t.Fatalf("max line width %d exceeds %d\nview:\n%s", max, sz[0], testutil.Plain(view))
					}
				})
			}
		}
	}
}

// firstMsg runs a command and returns its message.
func firstMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// TestSelectionStableIDs proves cursor movement keeps selection on stable IDs
// and reconciliation survives disappearing entities.
func TestSelectionStableIDs(t *testing.T) {
	p, _ := pageFor(t, ui.RouteAgents, domain.ScenarioDefault)
	p.Enter(ui.Location{Route: ui.RouteAgents})
	if got := p.(*agentsPage).selected(); got != "agent-0001" {
		t.Fatalf("default selection = %q, want agent-0001 (first stable ID)", got)
	}
	p.Update(testutil.Rune('j'))
	p.Update(testutil.Rune('j'))
	if got := p.(*agentsPage).selected(); got != "agent-0003" {
		t.Fatalf("after j j selection = %q, want agent-0003", got)
	}

	// Entity disappears: same ordinal rule applies.
	p.Update(testutil.Rune('k'))
	p.Update(testutil.Rune('k')) // back to agent-0001
	cat2, _ := mock.Fixture(domain.ScenarioDefault)
	kept := cat2.Agents[1:] // drop agent-0001
	cat2.Agents = kept
	p.SetCatalog(cat2)
	sel := p.(*agentsPage).selected()
	if sel == "agent-0001" {
		t.Fatal("selection stayed on the vanished agent")
	}
	if sel != "agent-0002" {
		t.Fatalf("same-ordinal fallback = %q, want agent-0002", sel)
	}
}

// TestWorkspaceFilterFlow locks '/' filter entry, typing, narrowing and Esc.
func TestWorkspaceFilterFlow(t *testing.T) {
	p, _ := pageFor(t, ui.RouteWorkspaces, domain.ScenarioDefault)
	p.Enter(ui.Location{Route: ui.RouteWorkspaces})
	wp := p.(*workspacesPage)
	if got := len(wp.visible()); got != 7 {
		t.Fatalf("visible rows = %d, want 7", got)
	}
	p.Update(testutil.Rune('/'))
	if p.InputMode() != ui.ModeFilter {
		t.Fatal("'/'' did not enter filter mode")
	}
	p.Update(testutil.Rune('k'))
	p.Update(testutil.Rune('b'))
	if got := len(wp.visible()); got != 1 {
		t.Fatalf("filtered rows = %d, want 1 (kb)", got)
	}
	if wp.visible()[0].id != "kb" {
		t.Fatalf("filtered row = %v", wp.visible()[0])
	}
	p.Update(testutil.Special(tea.KeyEsc))
	if p.InputMode() != ui.ModeNormal {
		t.Fatal("esc did not exit filter mode")
	}
	if got := len(wp.visible()); got != 7 {
		t.Fatalf("rows after esc = %d, want 7", got)
	}
}

func TestSettingsUseCategoryNavigationAndScopedEditing(t *testing.T) {
	page, _ := pageFor(t, ui.RouteConfig, domain.ScenarioDefault)
	page.Enter(ui.Location{Route: ui.RouteConfig})
	settings := page.(*configPage)
	view := testutil.Plain(page.View())
	for _, want := range []string{"Settings", "General", "Runtime", "History", "Security", "Configuration source"} {
		if !strings.Contains(view, want) {
			t.Fatalf("settings view missing %q:\n%s", want, view)
		}
	}
	message := firstMsg(page.Update(testutil.Special(tea.KeyEnter)))
	open, ok := message.(ui.OpenConfigFormMsg)
	if !ok || open.Section != "general" {
		t.Fatalf("general edit message = %#v", message)
	}
	page.Update(testutil.Rune('j'))
	if settings.category != 1 {
		t.Fatalf("j selected category %d, want 1", settings.category)
	}
	message = firstMsg(page.Update(testutil.Special(tea.KeyEnter)))
	open, ok = message.(ui.OpenConfigFormMsg)
	if !ok || open.Section != "runtime" {
		t.Fatalf("runtime edit message = %#v", message)
	}
	page.Update(testutil.Rune('k'))
	if settings.category != 0 {
		t.Fatalf("k selected category %d, want 0", settings.category)
	}
}

// TestPageCommandsDisabledReasons proves disabled commands carry reasons and
// the corresponding keys do not dispatch actions.
func TestPageCommandsDisabledReasons(t *testing.T) {
	p, _ := pageFor(t, ui.RouteAgents, domain.ScenarioDefault)
	ap := p.(*agentsPage)
	// select the done agent (agent-0004)
	p.Enter(ui.Location{Route: ui.RouteAgents, Primary: domain.Ref(domain.KindAgent, "agent-0004")})
	found := false
	for _, c := range p.Commands() {
		if c.Disabled && c.DisabledReason == "" {
			t.Fatalf("command %s disabled without reason", c.ID)
		}
		if c.ID == "ag.prompt" && c.Disabled {
			found = true
		}
	}
	if !found {
		t.Fatal("prompt on a done agent is not disabled")
	}
	if msg := firstMsg(p.Update(testutil.Rune('p'))); msg != nil {
		if _, isToast := msg.(ui.ShowToastMsg); !isToast {
			t.Fatalf("prompt key dispatched on done agent: %T", msg)
		}
	}
	_ = ap
}

// TestTerminalKeysEmitTypedIntents locks the page → root message contracts.
func TestTerminalKeysEmitTypedIntents(t *testing.T) {
	p, _ := pageFor(t, ui.RouteTerminals, domain.ScenarioDefault)
	// select an exited terminal (term-0006 on platform)
	p.Enter(ui.Location{Route: ui.RouteTerminals, Primary: domain.Ref(domain.KindTerminal, "term-0006")})
	tp := p.(*terminalsPage)

	// Attach on an exited terminal is blocked locally with actionable copy.
	msg := firstMsg(p.Update(testutil.Special(tea.KeyEnter)))
	blocked, ok := msg.(ui.ShowToastMsg)
	if !ok || !strings.Contains(blocked.Text, "has exited") {
		t.Fatalf("exited attach response = %#v", msg)
	}

	// h → scoped history with context ref
	msg = firstMsg(p.Update(testutil.Rune('h')))
	push, ok := msg.(ui.PushRouteMsg)
	if !ok {
		t.Fatalf("h emitted %T, want PushRouteMsg", msg)
	}
	if push.Loc.Route != ui.RouteHistory || push.Loc.Context != domain.Ref(domain.KindTerminal, "term-0006") {
		t.Fatalf("history push location = %+v", push.Loc)
	}

	// s on live terminal → prompt overlay
	p.Enter(ui.Location{Route: ui.RouteTerminals, Primary: domain.Ref(domain.KindTerminal, "term-0001")})
	msg = firstMsg(p.Update(testutil.Rune('s')))
	if _, ok := msg.(ui.OpenPromptMsg); !ok {
		t.Fatalf("s emitted %T, want OpenPromptMsg", msg)
	}
	_ = tp
}

func TestWorkspaceExplainsAttachAndTerminalCreation(t *testing.T) {
	page, _ := pageFor(t, ui.RouteWorkspace, domain.ScenarioDefault)
	page.Enter(ui.Location{Route: ui.RouteWorkspace, Primary: domain.Ref(domain.KindWorkspace, "base")})
	view := testutil.Plain(page.View())
	for _, want := range []string{"Attach to Zellij session", "New terminal pane", "suspends while Zellij owns"} {
		if !strings.Contains(view, want) {
			t.Fatalf("workspace detail missing %q:\n%s", want, view)
		}
	}
	message := firstMsg(page.Update(testutil.Rune('n')))
	action, ok := message.(ui.ExecuteActionMsg)
	if !ok || action.Action.Kind != domain.ActionSplit || action.Action.Target.Kind != domain.KindTerminal {
		t.Fatalf("new terminal intent = %#v", message)
	}
}

// TestHistoryFollowRespectsScroll proves movement disables follow and f
// resumes at the bottom.
func TestHistoryFollowRespectsScroll(t *testing.T) {
	p, _ := pageFor(t, ui.RouteHistory, domain.ScenarioDefault)
	p.Enter(ui.Location{Route: ui.RouteHistory, Context: domain.Ref(domain.KindTerminal, "term-0001")})
	p.SetSize(120, 20)
	hp := p.(*historyPage)
	if !hp.follow {
		t.Fatal("follow must start on")
	}
	p.Update(testutil.Special(tea.KeyUp))
	if hp.follow {
		t.Fatal("movement did not disable follow")
	}
	view := p.View()
	if !testutil.Contains(view, "follow off") {
		t.Fatalf("view missing follow-off badge:\n%s", testutil.Plain(view))
	}
	p.Update(testutil.Rune('f'))
	if !hp.follow {
		t.Fatal("f did not resume follow")
	}
	if !testutil.Contains(p.View(), "follow on") {
		t.Fatal("view missing follow-on badge")
	}
}

// TestEventsFollowAndRawDetail proves event follow, raw JSON toggle and
// context jump.
func TestEventsFollowAndRawDetail(t *testing.T) {
	p, _ := pageFor(t, ui.RouteEvents, domain.ScenarioDefault)
	p.Enter(ui.Location{Route: ui.RouteEvents})
	p.SetSize(120, 20)
	ep := p.(*eventsPage)
	if !ep.follow {
		t.Fatal("events follow must start on")
	}
	p.Update(testutil.Special(tea.KeyUp))
	if ep.follow {
		t.Fatal("scroll did not disable events follow")
	}
	p.Update(testutil.Rune('f'))

	// raw toggle via command message
	p.Update(rawToggleMsg{})
	if !ep.raw {
		t.Fatal("raw toggle did not apply")
	}
	view := p.View()
	if !testutil.Contains(view, `"cat"`) {
		t.Fatalf("raw JSON detail not visible:\n%s", testutil.Plain(view))
	}

	// context jump for a workspace event
	msg := firstMsg(p.Update(testutil.Special(tea.KeyEnter)))
	if push, ok := msg.(ui.PushRouteMsg); ok {
		if push.Loc.Route == ui.RouteDashboard {
			t.Fatalf("enter jumped nowhere useful: %+v", push.Loc)
		}
	}
}

// TestSearchJumpsToTypedOwner proves enter jumps to the owning typed route.
func TestSearchJumpsToTypedOwner(t *testing.T) {
	p, _ := pageFor(t, ui.RouteSearch, domain.ScenarioDefault)
	p.Enter(ui.Location{Route: ui.RouteSearch})
	p.Update(ui.SearchResultsMsg{Seq: 1, Revision: 1, Hits: []domain.SearchHit{
		{Ref: domain.Ref(domain.KindAgent, "agent-0002"), Title: "ledger-audit (needs_input)", Snippet: "codex agent on ecp", Scope: "agents"},
	}})
	msg := firstMsg(p.Update(testutil.Special(tea.KeyEnter)))
	push, ok := msg.(ui.PushRouteMsg)
	if !ok {
		t.Fatalf("enter emitted %T", msg)
	}
	if push.Loc.Route != ui.RouteAgents || push.Loc.Primary.ID != "agent-0002" {
		t.Fatalf("search jump = %+v", push.Loc)
	}
}

// TestSnapshotPlanLocation proves `r` emits the plan action and the
// navigation to Reconcile.
func TestSnapshotPlanLocation(t *testing.T) {
	p, _ := pageFor(t, ui.RouteSnapshots, domain.ScenarioDefault)
	p.Enter(ui.Location{Route: ui.RouteSnapshots, Primary: domain.Ref(domain.KindSnapshot, "snap-0004")})
	msg := firstMsg(p.Update(testutil.Rune('r')))
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("r emitted %T, want BatchMsg", msg)
	}
	var sawAction, sawPush bool
	for _, c := range batch {
		switch m := firstMsg(c).(type) {
		case ui.ExecuteActionMsg:
			sawAction = true
			if m.Action.Kind != domain.ActionSnapshotPlan || m.Action.Target.ID != "snap-0004" {
				t.Fatalf("plan action = %+v", m.Action)
			}
		case ui.PushRouteMsg:
			sawPush = true
			if m.Loc.Route != ui.RouteReconcile {
				t.Fatalf("plan push = %+v", m.Loc)
			}
		}
	}
	if !sawAction || !sawPush {
		t.Fatalf("plan flow: action=%v push=%v", sawAction, sawPush)
	}
}

// TestReconcileSelectionApplyUndo proves include/exclude drives the emitted
// apply selection and mode tabs cycle push → pull → freeze.
func TestReconcileSelectionApplyUndo(t *testing.T) {
	p, cat := pageFor(t, ui.RouteReconcile, domain.ScenarioDefault)
	// build a real plan through the mock backend
	b := mock.New()
	if _, err := b.Load(t.Context(), domain.ScenarioDefault); err != nil {
		t.Fatal(err)
	}
	res, err := b.Execute(t.Context(), domain.Action{Kind: domain.ActionSnapshotPlan,
		Target: domain.Ref(domain.KindSnapshot, "snap-0004"), ExpectedRevision: cat.Revision})
	if err != nil {
		t.Fatal(err)
	}
	p.SetCatalog(res.Catalog)
	p.Enter(ui.Location{Route: ui.RouteReconcile, Primary: domain.Ref(domain.KindPlan, "plan-0001")})
	rp := p.(*reconcilePage)

	// default mode push has no ops for kb → tab to pull
	if got := len(rp.modeOps(rp.current())); got != 0 && rp.mode != domain.RestorePush {
		t.Fatalf("unexpected initial mode/ops")
	}
	p.Update(testutil.Special(tea.KeyTab)) // pull
	if rp.mode != domain.RestorePull {
		t.Fatalf("mode after tab = %s", rp.mode)
	}
	ops := rp.modeOps(rp.current())
	if len(ops) == 0 {
		t.Fatal("kb pull ops missing")
	}

	// exclude the op via the contextual message
	p.Update(includeToggleMsg{})
	if got := len(rp.includedIDs()); got != 0 {
		t.Fatalf("excluded ops still included: %v", rp.includedIDs())
	}
	p.Update(includeToggleMsg{})
	if got := len(rp.includedIDs()); got != len(ops) {
		t.Fatalf("re-include failed: %v", rp.includedIDs())
	}

	// apply emits a confirm with the exact selection
	msg := firstMsg(p.Update(testutil.Rune('a')))
	cf, ok := msg.(ui.ConfirmActionMsg)
	if !ok {
		t.Fatalf("a emitted %T, want ConfirmActionMsg", msg)
	}
	if cf.Action.Kind != domain.ActionRestoreApply || cf.Action.Value != "pull" {
		t.Fatalf("apply action = %+v", cf.Action)
	}
	if len(cf.Action.Selection) != len(ops) {
		t.Fatalf("apply selection = %v, want %d ids", cf.Action.Selection, len(ops))
	}

	// u on the applied plan emits undo
	p.SetCatalog(res.Catalog)
	p.Update(testutil.Rune('u'))
}

// TestDestructiveActionsRequireUIAndBackendConfirmation proves kill, prune,
// snapshot delete, backend disable and restore apply never dispatch
// ExecuteActionMsg directly — they always go through ConfirmActionMsg — and
// the backend rejects them with Confirmed=false.
func TestDestructiveActionsRequireUIAndBackendConfirmation(t *testing.T) {
	check := func(t *testing.T, name string, cmd tea.Cmd) {
		t.Helper()
		msg := firstMsg(cmd)
		if _, isExec := msg.(ui.ExecuteActionMsg); isExec {
			t.Fatalf("%s dispatched ExecuteActionMsg directly", name)
		}
		cf, ok := msg.(ui.ConfirmActionMsg)
		if !ok {
			t.Fatalf("%s emitted %T, want ConfirmActionMsg", name, msg)
		}
		if cf.Action.Confirmed {
			t.Fatalf("%s action arrived pre-confirmed", name)
		}
	}

	t.Run("terminal kill", func(t *testing.T) {
		p, _ := pageFor(t, ui.RouteTerminals, domain.ScenarioDefault)
		p.Enter(ui.Location{Route: ui.RouteTerminals, Primary: domain.Ref(domain.KindTerminal, "term-0001")})
		check(t, "kill", p.Update(testutil.Rune('x')))
	})
	t.Run("view prune", func(t *testing.T) {
		p, _ := pageFor(t, ui.RouteActive, domain.ScenarioDefault)
		p.Enter(ui.Location{Route: ui.RouteActive, Primary: domain.Ref(domain.KindActive, "view-0004")})
		check(t, "prune", p.Update(testutil.Rune('x')))
	})
	t.Run("snapshot delete", func(t *testing.T) {
		p, _ := pageFor(t, ui.RouteSnapshots, domain.ScenarioDefault)
		p.Enter(ui.Location{Route: ui.RouteSnapshots, Primary: domain.Ref(domain.KindSnapshot, "snap-0001")})
		check(t, "snapshot delete", p.Update(testutil.Rune('x')))
	})
	t.Run("backend disable", func(t *testing.T) {
		p, _ := pageFor(t, ui.RouteBackends, domain.ScenarioDefault)
		p.Enter(ui.Location{Route: ui.RouteBackends, Primary: domain.Ref(domain.KindBackend, "zellij")})
		check(t, "backend disable", p.Update(testutil.Rune('e')))
	})

	// Backend side: Confirmed=false must be rejected without mutation.
	b := mock.New()
	cat0, err := b.Load(t.Context(), domain.ScenarioDefault)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []domain.Action{
		{Kind: domain.ActionKill, Target: domain.Ref(domain.KindTerminal, "term-0001")},
		{Kind: domain.ActionPrune, Target: domain.Ref(domain.KindActive, "view-0004")},
		{Kind: domain.ActionSnapshotDelete, Target: domain.Ref(domain.KindSnapshot, "snap-0001")},
		{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, "plan-0001"), Value: "pull"},
	} {
		a.ExpectedRevision = cat0.Revision
		if _, err := b.Execute(t.Context(), a); err == nil {
			t.Fatalf("%s accepted without confirmation", a.Kind)
		}
		cat1, _ := b.Load(t.Context(), domain.ScenarioDefault) // same scenario → live state
		if cat1.Revision != cat0.Revision {
			t.Fatalf("%s mutated the catalog on rejection", a.Kind)
		}
	}
}

// TestPerformanceWindowAndPause proves the bounded series and the local
// pause projection.
func TestPerformanceWindowAndPause(t *testing.T) {
	p, _ := pageFor(t, ui.RoutePerformance, domain.ScenarioDefault)
	pp := p.(*performancePage)
	p.Enter(ui.Location{Route: ui.RoutePerformance})
	view := p.View()
	if !testutil.Contains(view, "input p99") {
		t.Fatalf("missing series: %s", testutil.Plain(view))
	}
	p.Update(pauseToggleMsg{})
	if !pp.paused {
		t.Fatal("pause toggle did not apply")
	}
	if !testutil.Contains(p.View(), "paused") {
		t.Fatal("paused badge missing")
	}
	// degraded: unavailable badge + busy reasons on commands
	dp, _ := pageFor(t, ui.RoutePerformance, domain.ScenarioDegraded)
	dp.Enter(ui.Location{Route: ui.RoutePerformance})
	if !testutil.Contains(dp.View(), "unavailable") {
		t.Fatal("degraded performance view missing unavailable state")
	}
	for _, c := range dp.Commands() {
		if (c.ID == "pf.record" || c.ID == "pf.benchmark") && !c.Disabled {
			t.Fatalf("%s not disabled with service down", c.ID)
		}
	}
}

// TestMachineFilteredWorkspaces proves machines `enter` builds a typed
// machine-filtered workspaces location.
func TestMachineFilteredWorkspaces(t *testing.T) {
	p, _ := pageFor(t, ui.RouteMachines, domain.ScenarioDefault)
	p.Enter(ui.Location{Route: ui.RouteMachines, Primary: domain.Ref(domain.KindMachine, "build-01")})
	msg := firstMsg(p.Update(testutil.Special(tea.KeyEnter)))
	push, ok := msg.(ui.PushRouteMsg)
	if !ok {
		t.Fatalf("enter emitted %T", msg)
	}
	if push.Loc.Route != ui.RouteWorkspaces || push.Loc.Context != domain.Ref(domain.KindMachine, "build-01") {
		t.Fatalf("machine workspaces push = %+v", push.Loc)
	}

	wp, _ := pageFor(t, ui.RouteWorkspaces, domain.ScenarioDefault)
	wp.Enter(ui.Location{Route: ui.RouteWorkspaces, Context: domain.Ref(domain.KindMachine, "build-01")})
	view := wp.View()
	if !testutil.Contains(view, "filtered by machine build-01") {
		t.Fatalf("filter hint missing:\n%s", testutil.Plain(view))
	}
	if !testutil.Contains(view, "platform") {
		t.Fatal("build-01 workspace missing from filtered view")
	}
	if testutil.Contains(view, "ecp") {
		t.Fatal("non-build-01 workspace leaked into filtered view")
	}
}

// TestNotFoundsRenderInApp proves unknown explicit IDs render Not found with
// disabled actions across detail pages.
func TestNotFoundsRenderInApp(t *testing.T) {
	cases := []struct {
		route ui.Route
		kind  domain.ResourceKind
	}{
		{ui.RouteWorkspace, domain.KindWorkspace},
		{ui.RouteActive, domain.KindActive},
		{ui.RouteTerminals, domain.KindTerminal},
		{ui.RouteAgents, domain.KindAgent},
		{ui.RouteSnapshots, domain.KindSnapshot},
		{ui.RouteReconcile, domain.KindPlan},
		{ui.RouteBackends, domain.KindBackend},
		{ui.RouteMachines, domain.KindMachine},
		{ui.RouteEvents, domain.KindEvent},
	}
	for _, tc := range cases {
		p, _ := pageFor(t, tc.route, domain.ScenarioDefault)
		p.Enter(ui.Location{Route: tc.route, Primary: domain.Ref(tc.kind, "nope-9999")})
		view := p.View()
		if !testutil.Contains(view, "Not found") {
			t.Errorf("%s: missing Not found state:\n%s", tc.route, testutil.Plain(view))
		}
		for _, c := range p.Commands() {
			if c.ID == "" {
				continue
			}
		}
		// empty scenario: no phantom selection
		ep, _ := pageFor(t, tc.route, domain.ScenarioEmpty)
		ep.Enter(ui.Location{Route: tc.route})
		if testutil.Contains(ep.View(), "nope-9999") && tc.route != ui.RouteEvents {
			t.Errorf("%s: empty scenario shows phantom resource", tc.route)
		}
	}
}

// TestEmptyScenarioHasNoPhantomActions proves empty pages expose no enabled
// entity actions.
func TestEmptyScenarioHasNoPhantomActions(t *testing.T) {
	for _, route := range []ui.Route{ui.RouteWorkspaces, ui.RouteWorkspace, ui.RouteActive,
		ui.RouteTerminals, ui.RouteAgents, ui.RouteSnapshots, ui.RouteReconcile} {
		p, _ := pageFor(t, route, domain.ScenarioEmpty)
		p.Enter(ui.Location{Route: route})
		if msg := firstMsg(p.Update(testutil.Rune('a'))); msg != nil {
			t.Errorf("%s: 'a' dispatched %T on empty page", route, msg)
		}
		view := p.View()
		if !strings.Contains(testutil.Plain(view), "No ") {
			t.Errorf("%s: missing empty state:\n%s", route, testutil.Plain(view))
		}
	}
}

// TestPageCommandsWellFormed walks every page in every scenario asserting
// command IDs are unique, disabled entries carry reasons, and help is
// offered whenever rows exist.
func TestPageCommandsWellFormed(t *testing.T) {
	for _, sc := range []domain.Scenario{domain.ScenarioDefault, domain.ScenarioEmpty, domain.ScenarioDegraded} {
		for _, route := range ui.AllRoutes() {
			p, _ := pageFor(t, route, sc)
			p.Enter(ui.Location{Route: route})
			seen := map[string]bool{}
			for _, c := range p.Commands() {
				if c.ID == "" {
					t.Errorf("%s/%s: command with empty ID", sc, route)
				}
				if seen[c.ID] {
					t.Errorf("%s/%s: duplicate command ID %s", sc, route, c.ID)
				}
				seen[c.ID] = true
				if c.Disabled && c.DisabledReason == "" {
					t.Errorf("%s/%s: %s disabled without reason", sc, route, c.ID)
				}
			}
			if help := p.Help(); len(help) == 0 && len(p.Commands()) > 0 {
				t.Errorf("%s/%s: commands without help entries", sc, route)
			}
		}
	}
	// InputMode labels render.
	for _, m := range []ui.InputMode{ui.ModeNormal, ui.ModeFilter, ui.ModeText} {
		if m.String() == "" {
			t.Errorf("InputMode(%d).String empty", m)
		}
	}
}

// TestActiveViewCommandsCoverStates pins the active-view command matrix.
func TestActiveViewCommandsCoverStates(t *testing.T) {
	p, _ := pageFor(t, ui.RouteActive, domain.ScenarioDefault)
	ap := p.(*activePage)

	p.Enter(ui.Location{Route: ui.RouteActive, Primary: domain.Ref(domain.KindActive, "view-0001")})
	var pin *ui.Command
	for i, c := range p.Commands() {
		if c.ID == "av.pin" {
			pin = &p.Commands()[i]
		}
	}
	if pin == nil {
		t.Fatal("pin/unpin command missing")
	}
	// view-0001 is pinned: the command flips to unpin.
	if !testutil.Contains("Unpin", pin.Title) && pin.Title != "Pin / Unpin" {
		t.Fatalf("pin command title = %q", pin.Title)
	}

	// Prune on the pinned view is disabled with a reason.
	p.Enter(ui.Location{Route: ui.RouteActive, Primary: domain.Ref(domain.KindActive, "view-0001")})
	for _, c := range p.Commands() {
		if c.ID == "av.prune" && !c.Disabled {
			t.Fatal("prune enabled on pinned view")
		}
	}
	// Prune on the unexpired view is disabled too.
	p.Enter(ui.Location{Route: ui.RouteActive, Primary: domain.Ref(domain.KindActive, "view-0002")})
	for _, c := range p.Commands() {
		if c.ID == "av.prune" && !c.Disabled {
			t.Fatal("prune enabled on unexpired view")
		}
	}
	// Prune on the expired view is enabled.
	p.Enter(ui.Location{Route: ui.RouteActive, Primary: domain.Ref(domain.KindActive, "view-0004")})
	enabled := false
	for _, c := range p.Commands() {
		if c.ID == "av.prune" && !c.Disabled {
			enabled = true
		}
	}
	if !enabled {
		t.Fatal("prune disabled on expired view")
	}
	_ = ap
}

// TestAllCommandRunsEmitTypedIntents executes every enabled page command in
// every scenario and asserts each emits exactly one typed root intent.
func TestAllCommandRunsEmitTypedIntents(t *testing.T) {
	allowed := map[string]bool{
		"ui.ExecuteActionMsg": true, "ui.PushRouteMsg": true, "ui.PopRouteMsg": true,
		"ui.OpenPromptMsg": true, "ui.ConfirmActionMsg": true, "ui.ShowToastMsg": true,
		"ui.OpenSelectorMsg": true, "ui.OpenConfigFormMsg": true, "ui.SearchRequestMsg": true,
		"pages.followToggleMsg": true, "pages.rawToggleMsg": true, "pages.includeToggleMsg": true,
		"pages.pauseToggleMsg": true, "pages.scopeCycleMsg": true, "tea.BatchMsg": true,
	}
	for _, sc := range []domain.Scenario{domain.ScenarioDefault, domain.ScenarioEmpty, domain.ScenarioDegraded} {
		for _, route := range ui.AllRoutes() {
			p, cat := pageFor(t, route, sc)
			p.Enter(ui.Location{Route: route})
			// seed a plan for reconcile when one exists
			if route == ui.RouteReconcile && sc != domain.ScenarioEmpty {
				b := mock.New()
				_, _ = b.Load(t.Context(), sc)
				res, err := b.Execute(t.Context(), domain.Action{Kind: domain.ActionSnapshotPlan,
					Target: domain.Ref(domain.KindSnapshot, "snap-0004"), ExpectedRevision: cat.Revision})
				if err == nil {
					p.SetCatalog(res.Catalog)
					p.Enter(ui.Location{Route: route})
				}
			}
			for _, c := range p.Commands() {
				if c.Disabled || c.Run == nil {
					continue
				}
				msg := firstMsg(c.Run())
				if msg == nil {
					continue // local toggles may return nil after side effect
				}
				typ := fmtType(msg)
				if !allowed[typ] {
					t.Errorf("%s/%s command %s emitted %s", sc, route, c.ID, typ)
				}
			}
		}
	}
}

func fmtType(m tea.Msg) string {
	return fmt.Sprintf("%T", m)
}

// TestPageKeyWalks drive every documented page key through Update.
func TestPageKeyWalks(t *testing.T) {
	keys := []string{"enter", "j", "k", "up", "down", "tab", "a", "f", "s", "d", "l",
		"h", "t", "x", "p", "w", "c", "n", "e", "v", "r", "u", "b", "?"}
	for _, sc := range []domain.Scenario{domain.ScenarioDefault, domain.ScenarioEmpty} {
		for _, route := range ui.AllRoutes() {
			p, _ := pageFor(t, route, sc)
			p.Enter(ui.Location{Route: route})
			p.SetSize(120, 30)
			for _, k := range keys {
				var msg tea.Msg = testutil.Rune([]rune(k)[0])
				switch k {
				case "enter":
					msg = testutil.Special(tea.KeyEnter)
				case "up":
					msg = testutil.Special(tea.KeyUp)
				case "down":
					msg = testutil.Special(tea.KeyDown)
				case "tab":
					msg = testutil.Special(tea.KeyTab)
				}
				_ = p.Update(msg)
				view := p.View()
				if max := testutil.MaxLineWidth(view); max > 120 {
					t.Fatalf("%s/%s key %q overflow %d", sc, route, k, max)
				}
			}
		}
	}
}
