package app

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/mock"
	"github.com/mark1708/tmh-next/internal/testutil"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// fakePage is a recording Page double.
type fakePage struct {
	route    ui.Route
	mode     ui.InputMode
	keys     []string
	locs     []ui.Location
	cats     int
	sizes    [][2]int
	themes   int
	viewText string
	commands []ui.Command
	help     []key.Binding
	searches []SearchResultsMsg
}

func (f *fakePage) Route() ui.Route              { return f.route }
func (f *fakePage) Enter(l ui.Location) tea.Cmd  { f.locs = append(f.locs, l); return nil }
func (f *fakePage) SetCatalog(_ *domain.Catalog) { f.cats++ }
func (f *fakePage) SetSize(w, h int)             { f.sizes = append(f.sizes, [2]int{w, h}) }
func (f *fakePage) SetTheme(_ theme.Styles)      { f.themes++ }
func (f *fakePage) InputMode() ui.InputMode      { return f.mode }
func (f *fakePage) View() string                 { return f.viewText }
func (f *fakePage) Commands() []ui.Command       { return f.commands }
func (f *fakePage) Help() []key.Binding          { return f.help }

func (f *fakePage) Update(m tea.Msg) tea.Cmd {
	switch msg := m.(type) {
	case tea.KeyPressMsg:
		f.keys = append(f.keys, msg.String())
		switch {
		case msg.String() == "f" && f.mode == ui.ModeNormal:
			f.mode = ui.ModeFilter
		case msg.String() == "esc" && f.mode != ui.ModeNormal:
			f.mode = ui.ModeNormal
		}
	case SearchResultsMsg:
		f.searches = append(f.searches, msg)
	}
	return nil
}

func (f *fakePage) lastKeys(n int) string {
	if len(f.keys) < n {
		n = len(f.keys)
	}
	return strings.Join(f.keys[len(f.keys)-n:], ",")
}

// harness wires a Root over deterministic doubles.
type harness struct {
	root  *Root
	pages map[ui.Route]*fakePage
	sched *ManualScheduler
	rec   *testutil.RecordingBackend
}

func newHarness(t *testing.T, start Startup) *harness {
	t.Helper()
	cat, err := mock.Fixture(start.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	rec := testutil.NewRecordingBackend(cat)
	sched := &ManualScheduler{}
	pages := map[ui.Route]*fakePage{}
	for _, route := range ui.AllRoutes() {
		pages[route] = &fakePage{route: route}
	}
	root := NewRoot(Options{
		Client:      rec,
		DemoTicker:  rec,
		Scheduler:   sched,
		Live:        start.Live,
		Stack:       start.Stack,
		QuickSwitch: start.QuickSwitch,
		Factory:     func(route ui.Route) ui.Page { return pages[route] },
	})
	return &harness{root: root, pages: pages, sched: sched, rec: rec}
}

// fire feeds one message (expanding BatchMsg) and pumps every returned
// command chain.
func (h *harness) fire(t *testing.T, msg tea.Msg) {
	t.Helper()
	var queue []tea.Cmd
	feed := func(m tea.Msg) {
		if m == nil {
			return
		}
		if batch, ok := m.(tea.BatchMsg); ok {
			for _, bc := range batch {
				if bc != nil {
					queue = append(queue, bc)
				}
			}
			return
		}
		_, next := h.root.Update(m)
		if next != nil {
			queue = append(queue, next)
		}
	}
	feed(msg)
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		res := c()
		if res == nil {
			continue
		}
		feed(res)
	}
}

// boot runs Init, completes the initial load and installs a wide size.
func (h *harness) boot(t *testing.T) {
	t.Helper()
	cmd := h.root.Init()
	if cmd == nil {
		t.Fatal("Init returned no load command")
	}
	h.fire(t, cmd())
	h.fire(t, testutil.Size(120, 40))
}

func (h *harness) page(r ui.Route) *fakePage { return h.pages[r] }

func (h *harness) active() *fakePage { return h.pages[h.root.activeLocation().Route] }

// fireKey presses a key through the full precedence chain.
func (h *harness) fireKey(t *testing.T, s string) {
	t.Helper()
	h.fire(t, keyMsg(s))
}

func keyMsg(s string) tea.KeyPressMsg {
	if strings.HasPrefix(s, "ctrl+") {
		return testutil.Ctrl(rune(s[5]))
	}
	switch s {
	case "enter":
		return testutil.Special(tea.KeyEnter)
	case "backspace":
		return testutil.Special(tea.KeyBackspace)
	case "esc":
		return testutil.Special(tea.KeyEsc)
	case "tab":
		return testutil.Special(tea.KeyTab)
	case "space":
		return testutil.Space()
	case "up":
		return testutil.Special(tea.KeyUp)
	case "down":
		return testutil.Special(tea.KeyDown)
	}
	return testutil.Rune([]rune(s)[0])
}

// lastTimerToken returns the most recently armed token.
func (h *harness) lastTimerToken() uint64 {
	if len(h.sched.TimerTokens) == 0 {
		return 0
	}
	return h.sched.TimerTokens[len(h.sched.TimerTokens)-1]
}

// TestInitialStacksAndResourcePolicies locks CLI validation, startup stacks
// and runtime default resolution.
func TestInitialStacksAndResourcePolicies(t *testing.T) {
	ok := func(name string, args ...string) ([]ui.Location, bool) {
		t.Helper()
		st, err := ParseStartup(args)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return st.Stack, st.QuickSwitch
	}

	var stack []ui.Location
	var qs bool

	stack, qs = ok("no-flags")
	if len(stack) != 1 || stack[0].Route != ui.RouteDashboard || !qs {
		t.Fatalf("no-flags stack=%v quickSwitch=%v", stack, qs)
	}
	stack, qs = ok("dashboard", "--dashboard")
	if len(stack) != 1 || stack[0].Route != ui.RouteDashboard || qs {
		t.Fatalf("--dashboard stack=%v quickSwitch=%v", stack, qs)
	}
	stack, _ = ok("page", "--page", "agents")
	if len(stack) != 2 || stack[0].Route != ui.RouteDashboard || stack[1].Route != ui.RouteAgents {
		t.Fatalf("--page agents stack=%v", stack)
	}
	stack, _ = ok("page-dashboard", "--page", "dashboard")
	if len(stack) != 1 || stack[0].Route != ui.RouteDashboard {
		t.Fatalf("--page dashboard stack=%v", stack)
	}

	stack, _ = ok("resource-primary", "--page", "agents", "--resource", "agent:agent-0002")
	if stack[1].Primary != domain.Ref(domain.KindAgent, "agent-0002") {
		t.Fatalf("primary = %v", stack[1].Primary)
	}
	stack, _ = ok("resource-context-machine", "--page", "workspaces", "--resource", "machine:homelab")
	if stack[1].Context != domain.Ref(domain.KindMachine, "homelab") {
		t.Fatalf("context = %v", stack[1].Context)
	}
	stack, _ = ok("resource-context-terminal", "--page", "history", "--resource", "terminal:term-0001")
	if stack[1].Context != domain.Ref(domain.KindTerminal, "term-0001") {
		t.Fatalf("context = %v", stack[1].Context)
	}

	bad := []struct {
		name string
		args []string
	}{
		{"dashboard+page", []string{"--dashboard", "--page", "agents"}},
		{"resource-without-page", []string{"--resource", "agent:agent-0001"}},
		{"unknown-page", []string{"--page", "nope"}},
		{"unknown-scenario", []string{"--scenario", "chaos"}},
		{"dashboard-rejects-resource", []string{"--page", "dashboard", "--resource", "workspace:base"}},
		{"search-rejects-resource", []string{"--page", "search", "--resource", "agent:agent-0001"}},
		{"performance-rejects-resource", []string{"--page", "performance", "--resource", "machine:local"}},
		{"wrong-kind", []string{"--page", "agents", "--resource", "workspace:base"}},
		{"unknown-kind", []string{"--page", "agents", "--resource", "team:t1"}},
		{"no-colon", []string{"--page", "agents", "--resource", "agent"}},
		{"empty-id", []string{"--page", "agents", "--resource", "agent:"}},
		{"empty-kind", []string{"--page", "agents", "--resource", ":agent-0001"}},
		{"positional", []string{"extra"}},
	}
	for _, tc := range bad {
		if _, err := ParseStartup(tc.args); err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}

	// Runtime: omitted primary resolves to the first stable ID after load.
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false,
		Stack: []ui.Location{{Route: ui.RouteDashboard}, {Route: ui.RouteAgents}}})
	h.boot(t)
	if loc := h.page(ui.RouteAgents).locs[0]; loc.Primary.ID != "agent-0001" {
		t.Fatalf("omitted agent primary = %q, want agent-0001", loc.Primary.ID)
	}

	// Runtime: empty scenario leaves collections empty — no phantom selection.
	h2 := newHarness(t, Startup{Scenario: domain.ScenarioEmpty, Live: false,
		Stack: []ui.Location{{Route: ui.RouteDashboard}, {Route: ui.RouteAgents}}})
	h2.boot(t)
	if loc := h2.page(ui.RouteAgents).locs[0]; loc.Primary.ID != "" {
		t.Fatalf("empty scenario resolved a primary: %v", loc.Primary)
	}
}

// TestRoutePushReplaceBack locks navigation semantics: equal push is a no-op,
// replace swaps the top, pop honors the Dashboard lower bound.
func TestRoutePushReplaceBack(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)

	h.fire(t, PushRouteMsg{Loc: ui.Location{Route: ui.RouteAgents}})
	h.fire(t, PushRouteMsg{Loc: ui.Location{Route: ui.RouteTerminals}})
	if got := len(h.root.stack); got != 3 {
		t.Fatalf("stack depth = %d, want 3", got)
	}
	h.fire(t, PushRouteMsg{Loc: ui.Location{Route: ui.RouteTerminals}})
	if got := len(h.root.stack); got != 3 {
		t.Fatalf("equal push changed stack depth to %d", got)
	}
	h.fire(t, PushRouteMsg{Loc: ui.Location{Route: ui.RouteEvents}, Replace: true})
	if got := len(h.root.stack); got != 3 {
		t.Fatalf("replace changed depth to %d", got)
	}
	if h.root.activeLocation().Route != ui.RouteEvents {
		t.Fatal("replace did not swap the top")
	}

	h.fireKey(t, "esc")
	h.fireKey(t, "esc")
	if h.root.activeLocation().Route != ui.RouteDashboard {
		t.Fatalf("esc did not return to dashboard: %v", h.root.activeLocation())
	}
	h.fireKey(t, "esc")
	if got := len(h.root.stack); got != 1 {
		t.Fatalf("dashboard lower bound violated: depth %d", got)
	}
}

// TestExplicitResourceDoesNotFallback locks that an explicit unknown ID is
// preserved verbatim and never silently replaced.
func TestExplicitResourceDoesNotFallback(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false,
		Stack: []ui.Location{{Route: ui.RouteDashboard}, {Route: ui.RouteAgents, Primary: domain.Ref(domain.KindAgent, "agent-9999")}}})
	h.boot(t)
	loc := h.page(ui.RouteAgents).locs[0]
	if loc.Primary.ID != "agent-9999" {
		t.Fatalf("explicit primary replaced by %q", loc.Primary.ID)
	}
}

// TestOverlayOwnsEscape proves an open overlay swallows every key and Esc
// closes it without reaching the page.
func TestOverlayOwnsEscape(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fireKey(t, "ctrl+p")
	if h.root.ov == nil || h.root.ov.kind != ovPalette {
		t.Fatal("ctrl+p did not open the palette")
	}
	before := len(h.active().keys)
	h.fireKey(t, "j")
	h.fireKey(t, "down")
	if got := len(h.active().keys); got != before {
		t.Fatalf("page received keys during overlay: %q", h.active().lastKeys(4))
	}
	h.fireKey(t, "esc")
	if h.root.ov != nil {
		t.Fatal("esc did not close the palette")
	}
	if got := len(h.active().keys); got != before {
		t.Fatalf("page received keys after esc: %q", h.active().lastKeys(4))
	}
}

// TestFilterOwnsEscapeAndQ proves page filter/text mode owns every key
// including Esc and q.
func TestFilterOwnsEscapeAndQ(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fireKey(t, "f") // fake page enters filter mode
	if h.active().mode != ui.ModeFilter {
		t.Fatal("page did not enter filter mode")
	}
	h.fireKey(t, "q")
	h.fireKey(t, "ctrl+p")
	if h.root.ov != nil {
		t.Fatal("global palette fired during page filter mode")
	}
	if !strings.Contains(h.active().lastKeys(2), "q") {
		t.Fatalf("page did not receive q in filter mode: %q", h.active().lastKeys(4))
	}
	if got := len(h.root.stack); got != 1 {
		t.Fatal("q quit the app from filter mode")
	}
	h.fireKey(t, "esc")
	if h.active().mode != ui.ModeNormal {
		t.Fatal("esc did not exit filter mode")
	}
}

// TestTooSmallSuspendsOverlay proves only q/ctrl+c work under 72×20 and the
// overlay survives the resize.
func TestTooSmallSuspendsOverlay(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, testutil.Size(120, 40))
	h.fireKey(t, "ctrl+p")
	if h.root.ov == nil {
		t.Fatal("palette did not open")
	}

	h.fire(t, testutil.Size(70, 18))
	if !h.root.tooSmall {
		t.Fatal("70x18 not classified too small")
	}
	h.fireKey(t, "j")
	h.fireKey(t, "ctrl+k")
	if h.root.ov.kind != ovPalette {
		t.Fatalf("too-small mode replaced the overlay: %v", h.root.ov.kind)
	}
	if testutil.Contains(h.root.View().Content, "72") == false {
		t.Fatal("too-small view missing required size hint")
	}

	var quitSeen bool
	_, cmd := h.root.Update(keyMsg("q"))
	for cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			quitSeen = true
			break
		}
	}

	h.fire(t, testutil.Size(120, 40))
	if h.root.tooSmall || h.root.ov == nil || h.root.ov.kind != ovPalette {
		t.Fatal("overlay state lost after resize")
	}
	if h.root.mut.kind != mutNone || h.root.currentRevision() != 1 {
		t.Fatal("resize changed mutation state")
	}
	_ = quitSeen
}

// TestSpaceAlwaysOpensContextActions proves Space opens the selector instead
// of mutating page state, and choosing runs the command exactly once.
func TestSpaceAlwaysOpensContextActions(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	ran := 0
	h.active().commands = []ui.Command{{
		ID: "cmd.count", Title: "Count", Description: "increments", Shortcut: "a",
		Run: func() tea.Cmd { ran++; return nil },
	}}
	h.fireKey(t, "space")
	if h.root.ov == nil || h.root.ov.kind != ovSelector {
		t.Fatal("space did not open the context selector")
	}
	if ran != 0 {
		t.Fatal("space executed the command directly")
	}
	if testutil.Contains(h.root.overlayView(), "Count") == false {
		t.Fatal("selector does not list the page command")
	}
	h.fireKey(t, "enter") // choose the focused row
	if ran != 1 {
		t.Fatalf("command ran %d times, want 1", ran)
	}
	if h.root.ov != nil {
		t.Fatal("selector still open after choose")
	}
}

// TestCtrlCAlwaysQuits proves ctrl+c quits from every ownership context.
func TestCtrlCAlwaysQuits(t *testing.T) {
	quits := func(h *harness) bool {
		_, cmd := h.root.Update(keyMsg("ctrl+c"))
		return producesQuit(cmd, h)
	}
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, testutil.Size(120, 40))
	if !quits(h) {
		t.Fatal("ctrl+c did not quit in normal mode")
	}

	h = newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, testutil.Size(120, 40))
	h.fireKey(t, "ctrl+p")
	if !quits(h) {
		t.Fatal("ctrl+c did not quit during overlay")
	}

	h = newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, testutil.Size(120, 40))
	h.fireKey(t, "f")
	if !quits(h) {
		t.Fatal("ctrl+c did not quit during filter mode")
	}

	h = newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, testutil.Size(60, 10))
	if !quits(h) {
		t.Fatal("ctrl+c did not quit in too-small mode")
	}
}

func producesQuit(cmd tea.Cmd, h *harness) bool {
	for cmd != nil {
		res := cmd()
		if _, ok := res.(tea.QuitMsg); ok {
			return true
		}
		if res == nil {
			return false
		}
		if b, ok := res.(tea.BatchMsg); ok {
			for _, c := range b {
				if producesQuit(c, h) {
					return true
				}
			}
			return false
		}
		_, next := h.root.Update(res)
		cmd = next
	}
	return false
}

// mutHarness wires the root over a gated backend for lane proofs.
func mutHarness(t *testing.T, live bool) (*harness, *testutil.GatedBackend) {
	t.Helper()
	cat, err := mock.Fixture(domain.ScenarioDefault)
	if err != nil {
		t.Fatal(err)
	}
	rec := testutil.NewRecordingBackend(cat)
	gated := testutil.NewGatedBackend(rec)
	sched := &ManualScheduler{}
	pages := map[ui.Route]*fakePage{}
	for _, route := range ui.AllRoutes() {
		pages[route] = &fakePage{route: route}
	}
	root := NewRoot(Options{
		Client: gated, DemoTicker: gated, Scheduler: sched,
		Live: live, Stack: []ui.Location{{Route: ui.RouteDashboard}},
		Factory: func(route ui.Route) ui.Page { return pages[route] },
	})
	h := &harness{root: root, pages: pages, sched: sched, rec: rec}
	h.boot(t)
	return h, gated
}

// dispatchAsync runs cmd on a goroutine so gated backends can block without
// deadlocking the harness; produced messages arrive on the channel.
func (h *harness) dispatchAsync(cmd tea.Cmd) <-chan tea.Msg {
	ch := make(chan tea.Msg, 16)
	go func() {
		var expand func(c tea.Cmd)
		expand = func(c tea.Cmd) {
			if c == nil {
				return
			}
			res := c()
			if res == nil {
				return
			}
			if b, ok := res.(tea.BatchMsg); ok {
				for _, cc := range b {
					expand(cc)
				}
				return
			}
			ch <- res
		}
		expand(cmd)
		close(ch)
	}()
	return ch
}

// step feeds one root message and collects the returned command.
func (h *harness) step(msg tea.Msg) tea.Cmd {
	_, cmd := h.root.Update(msg)
	return cmd
}

// drainAsync processes a dispatched command channel, feeding results back and
// recursively dispatching any follow-up commands (still async).
func (h *harness) drainAsync(t *testing.T, ch <-chan tea.Msg) {
	t.Helper()
	for msg := range ch {
		cmd := h.step(msg)
		if cmd != nil {
			h.drainAsync(t, h.dispatchAsync(cmd))
		}
	}
}

// TestMutatorsNeverOverlap proves at most one Execute/Tick is in flight and
// user actions arriving mid-flight are rejected without a backend call.
func TestMutatorsNeverOverlap(t *testing.T) {
	h, gated := mutHarness(t, true)
	gated.CloseExecGate()

	cmd := h.step(ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch,
		Target: domain.Ref(domain.KindActive, "view-0001")}})
	if h.root.mut.kind != mutAction {
		t.Fatal("action did not enter the lane")
	}
	ch := h.dispatchAsync(cmd) // blocks on the exec gate

	// A second user action while the lane is busy: rejected, never dispatched.
	h.step(ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch,
		Target: domain.Ref(domain.KindActive, "view-0002")}})
	if got := len(h.rec.Actions()); got != 0 {
		t.Fatalf("busy lane dispatched a second action (%d calls)", got)
	}

	// Timer fires while the action is in flight: pending bit only.
	h.step(TickTimerMsg{Token: h.lastTimerToken()})
	if !h.root.tickPending {
		t.Fatal("due timer during busy lane did not set the pending bit")
	}

	gated.ReleaseExecGate()
	h.drainAsync(t, ch)
	if h.root.mut.kind != mutNone {
		t.Fatalf("lane still busy after action: %v", h.root.mut.kind)
	}
	if got := len(h.rec.Actions()); got != 1 {
		t.Fatalf("action calls = %d, want 1", got)
	}
}

// TestSingleTimerToken proves exactly one armed timer exists at any moment.
func TestSingleTimerToken(t *testing.T) {
	h, gated := mutHarness(t, true)
	if got := len(h.sched.TimerTokens); got != 1 {
		t.Fatalf("initial timers armed = %d, want 1", got)
	}
	gated.CloseTickGate()
	cmd := h.step(TickTimerMsg{Token: h.lastTimerToken()})
	if h.root.mut.kind != mutTick {
		t.Fatal("due token did not launch a tick")
	}
	ch := h.dispatchAsync(cmd) // blocks on the tick gate
	gated.ReleaseTickGate()
	h.drainAsync(t, ch)
	if h.root.mut.kind != mutNone {
		t.Fatal("tick did not complete")
	}
	if got := len(h.sched.TimerTokens); got != 2 {
		t.Fatalf("timers armed after tick = %d, want exactly 2", got)
	}
}

// TestActionDuringTickIsRejected proves user actions during a tick get an
// info toast and no backend dispatch.
func TestActionDuringTickIsRejected(t *testing.T) {
	h, gated := mutHarness(t, true)
	gated.CloseTickGate()
	cmd := h.step(TickTimerMsg{Token: h.lastTimerToken()})
	if h.root.mut.kind != mutTick {
		t.Fatal("tick not in flight")
	}
	ch := h.dispatchAsync(cmd)

	h.step(ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch,
		Target: domain.Ref(domain.KindActive, "view-0001")}})
	if got := len(h.rec.Actions()); got != 0 {
		t.Fatalf("action dispatched during tick (%d)", got)
	}
	if len(h.root.toasts) == 0 {
		t.Fatal("no busy toast surfaced")
	}
	gated.ReleaseTickGate()
	h.drainAsync(t, ch)
	if got := len(h.rec.Actions()); got != 0 {
		t.Fatalf("tick completion dispatched the rejected action (%d)", got)
	}
}

// TestTickPendingCoalesces proves repeated due timers while busy collapse
// into a single pending tick.
func TestTickPendingCoalesces(t *testing.T) {
	h, gated := mutHarness(t, true)
	gated.CloseExecGate()

	cmd := h.step(ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch,
		Target: domain.Ref(domain.KindActive, "view-0001")}})
	ch := h.dispatchAsync(cmd)

	tok := h.lastTimerToken()
	h.step(TickTimerMsg{Token: tok})
	h.step(TickTimerMsg{Token: tok}) // duplicate due: already disarmed
	h.step(TickTimerMsg{Token: tok})
	if !h.root.tickPending {
		t.Fatal("pending bit not set")
	}

	gated.ReleaseExecGate()
	h.drainAsync(t, ch)
	if h.rec.Ticks() > 1 {
		t.Fatalf("pending ticks ran %d times, want <= 1", h.rec.Ticks())
	}
	if overlaps := gated.Overlaps(); len(overlaps) != 0 {
		t.Fatalf("overlap violation: %v", overlaps)
	}
}

// TestStaleTimerAndMutationTokensAreIgnored proves stale tokens and results
// with mismatched identity/revision never change state.
func TestStaleTimerAndMutationTokensAreIgnored(t *testing.T) {
	h, _ := mutHarness(t, true)
	rev := h.root.currentRevision()

	h.fire(t, TickTimerMsg{Token: 9999})
	if h.root.mut.kind != mutNone || h.rec.Ticks() != 0 {
		t.Fatal("stale timer token launched a tick")
	}

	h.fire(t, MutationResultMsg{Result: domain.MutationResult{
		BaseRevision: rev, NewRevision: rev + 5, Catalog: nil,
	}})
	if h.root.currentRevision() != rev {
		t.Fatalf("malformed result changed revision to %d", h.root.currentRevision())
	}

	h.fire(t, MutationResultMsg{Result: domain.MutationResult{
		BaseRevision: rev, NewRevision: rev + 1,
		Catalog: &domain.Catalog{Revision: rev + 1},
	}})
	if h.root.currentRevision() != rev {
		t.Fatalf("foreign result changed revision to %d", h.root.currentRevision())
	}

	h.fire(t, MutationResultMsg{Result: domain.MutationResult{
		BaseRevision: rev, NewRevision: rev + 1, IsTick: true,
		Catalog: &domain.Catalog{Revision: rev + 1},
	}})
	if h.root.currentRevision() != rev {
		t.Fatal("stray tick result applied")
	}
}

// TestActionCompletionDoesNotDuplicateArmedTimer proves an action finishing
// while a timer is armed never arms a second one.
func TestActionCompletionDoesNotDuplicateArmedTimer(t *testing.T) {
	h, _ := mutHarness(t, true)
	h.fire(t, ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch,
		Target: domain.Ref(domain.KindActive, "view-0001")}})
	if got := len(h.sched.TimerTokens); got != 1 {
		t.Fatalf("timers armed after action = %d, want 1 (original still armed)", got)
	}
	if h.rec.Ticks() != 0 {
		t.Fatal("action completion launched an extra tick")
	}
	if h.root.mut.kind != mutNone {
		t.Fatal("action did not complete")
	}
}

// TestConflictReloadsWithoutRetry proves a revision conflict triggers exactly
// one reload, surfaces a recoverable toast and never retries the action.
func TestConflictReloadsWithoutRetry(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.rec.SetExecErr(domain.ErrRevisionConflict)

	h.fire(t, ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch,
		Target: domain.Ref(domain.KindActive, "view-0001")}})

	if got := h.rec.Loads(); got != 2 {
		t.Fatalf("loads = %d, want 2 (initial + conflict reload)", got)
	}
	if got := len(h.rec.Actions()); got != 1 {
		t.Fatalf("conflicting action retried: %d action calls", got)
	}
	if h.root.mut.kind != mutNone {
		t.Fatal("lane stuck busy after reload")
	}
	found := false
	for _, toast := range h.root.toasts {
		if strings.Contains(toast.Text, "conflict") {
			found = true
		}
	}
	if !found {
		t.Fatal("no recoverable conflict toast")
	}
	h.rec.ClearExecErr()
	h.fire(t, ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch,
		Target: domain.Ref(domain.KindActive, "view-0002")}})
	if got := len(h.rec.Actions()); got != 2 {
		t.Fatalf("post-recovery action not accepted: %d", got)
	}
}

// TestSearchSeqAndRevision locks the search lane: only the latest sequence
// lands; revision mismatches re-execute the query exactly once.
func TestSearchSeqAndRevision(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)

	// Issue query 1 but hold its result back until query 2 supersedes it.
	cmd1 := h.step(SearchRequestMsg{Text: "led", Scope: "agents"})
	cmd2 := h.step(SearchRequestMsg{Text: "ledger", Scope: "agents"})
	h.fire(t, cmd2()) // seq 2 result arrives first
	if got := len(h.page(ui.RouteSearch).searches); got != 1 {
		t.Fatalf("latest result not delivered: %d", got)
	}
	h.fire(t, cmd1()) // stale seq 1 result arrives late: ignored
	if got := len(h.page(ui.RouteSearch).searches); got != 1 {
		t.Fatalf("stale-lane result delivered: %d", got)
	}

	// Revision mismatch triggers exactly one re-execution.
	callsBefore := len(h.rec.Searches())
	h.rec.SetSearchRevs(0)
	h.fire(t, SearchRequestMsg{Text: "drift", Scope: "events"})
	if calls := len(h.rec.Searches()) - callsBefore; calls != 2 {
		t.Fatalf("search calls = %d, want 2 (initial + one re-execution)", calls)
	}
	// The re-execution carries the current revision and is delivered.
	if got := len(h.page(ui.RouteSearch).searches); got != 2 {
		t.Fatalf("re-executed result not delivered: %d deliveries", got)
	}
	if got := h.page(ui.RouteSearch).searches[len(h.page(ui.RouteSearch).searches)-1]; len(got.Hits) == 0 {
		t.Fatal("delivered re-execution carried no hits")
	}
}

// TestToastSequence proves an old expiry never clears a newer toast.
func TestToastSequence(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, ShowToastMsg{Text: "first", Kind: "info"})
	h.fire(t, ShowToastMsg{Text: "second", Kind: "ok"})
	if len(h.root.toasts) != 2 {
		t.Fatalf("toasts = %d, want 2", len(h.root.toasts))
	}
	h.fire(t, ToastExpiredMsg{Seq: 1})
	if len(h.root.toasts) != 1 || h.root.toasts[0].Text != "second" {
		t.Fatalf("old expiry cleared the newer toast: %+v", h.root.toasts)
	}
	h.fire(t, ToastExpiredMsg{Seq: 2})
	if len(h.root.toasts) != 0 {
		t.Fatalf("toasts = %+v, want empty", h.root.toasts)
	}
}

func TestDuplicateNotificationsCoalesce(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.fire(t, ShowToastMsg{Text: "Live updates unavailable", Kind: "warn"})
	firstSeq := h.root.toasts[0].Seq
	h.fire(t, ShowToastMsg{Text: "Live updates unavailable", Kind: "warn"})
	if len(h.root.toasts) != 1 || h.root.toasts[0].Seq == firstSeq {
		t.Fatalf("duplicate notifications = %+v", h.root.toasts)
	}
}

// TestQuickSwitchEntityBranches covers every quick-switch entity kind.
func TestQuickSwitchEntityBranches(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)

	pick := func(down int) {
		h.fireKey(t, "ctrl+k")
		for range down {
			h.fireKey(t, "j")
		}
		h.fireKey(t, "enter")
	}

	// workspace (0), active view (7), terminal (11), agent (18)
	pick(0)
	if h.root.activeLocation().Route != ui.RouteWorkspace {
		t.Fatalf("ws pick → %v", h.root.activeLocation().Route)
	}
	pick(7)
	if h.root.activeLocation().Route != ui.RouteActive {
		t.Fatalf("view pick → %v", h.root.activeLocation().Route)
	}
	pick(11)
	if h.root.activeLocation().Route != ui.RouteTerminals {
		t.Fatalf("terminal pick → %v", h.root.activeLocation().Route)
	}
	pick(18)
	if h.root.activeLocation().Route != ui.RouteAgents {
		t.Fatalf("agent pick → %v", h.root.activeLocation().Route)
	}
	if kinds := len(h.rec.Actions()); kinds < 4 {
		t.Fatalf("focus actions dispatched = %d, want ≥4", kinds)
	}
}

// TestActionFailureSurfacesErrorToast proves non-conflict failures show an
// actionable notification and re-arm the lane.
func TestActionFailureSurfacesErrorToast(t *testing.T) {
	h, _ := mutHarness(t, true)
	h.rec.SetExecErr(domain.Fail(domain.CodeNotLive, "terminal term-0006 is not live"))
	h.fire(t, ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionAttach,
		Target: domain.Ref(domain.KindTerminal, "term-0006")}})

	found := false
	for _, toast := range h.root.toasts {
		if strings.Contains(toast.Text, "Attach unavailable") &&
			strings.Contains(toast.Text, "terminal term-0006 is not live") &&
			!strings.Contains(toast.Text, "not_live:") &&
			toast.Kind == "err" {
			found = true
		}
	}
	if !found {
		t.Fatalf("actionable error toast missing: %+v", h.root.toasts)
	}
	if h.root.mut.kind != mutNone {
		t.Fatal("lane stuck after recoverable failure")
	}
	// Lane accepts new work after the recoverable failure.
	h.fire(t, ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch,
		Target: domain.Ref(domain.KindActive, "view-0001")}})
	if got := len(h.rec.Actions()); got != 2 {
		t.Fatalf("lane accepted %d actions after failure, want 2", got)
	}
	if h.root.mut.kind != mutNone {
		t.Fatal("lane stuck after the follow-up action completed")
	}
}

func TestReloadFailureClearsMutationLane(t *testing.T) {
	h := newHarness(t, Startup{Scenario: domain.ScenarioDefault, Live: false})
	h.boot(t)
	h.root.mut = mutator{kind: mutReload}
	h.fire(t, ReloadFailedMsg{Err: domain.Fail(domain.CodeUnreachable, "tmhd socket closed")})
	if h.root.mut.kind != mutNone {
		t.Fatal("reload failure left mutation lane busy")
	}
	if len(h.root.toasts) == 0 ||
		!strings.Contains(h.root.toasts[len(h.root.toasts)-1].Text, "Reload unavailable") {
		t.Fatalf("reload notification = %+v", h.root.toasts)
	}
}

// TestTickConflictReloadsAndSearchLane covers the tick-failure conflict
// recovery and the search lane over the gated backend.
func TestTickConflictReloadsAndSearchLane(t *testing.T) {
	h, gated := mutHarness(t, true)
	// Force a tick revision conflict: stale expected revision.
	gated.Inner.SetExecRevsForTick(99)
	h.fire(t, TickTimerMsg{Token: h.lastTimerToken()})
	// The tick fails with a conflict → reload → recoverable toast.
	recovered := false
	for _, toast := range h.root.toasts {
		if strings.Contains(toast.Text, "conflict") {
			recovered = true
		}
	}
	if !recovered {
		t.Fatalf("tick conflict did not surface: %+v", h.root.toasts)
	}
	if h.root.mut.kind != mutNone {
		t.Fatal("lane busy after tick conflict reload")
	}

	// Search lane works through the gated backend.
	h.fire(t, SearchRequestMsg{Text: "ledger", Scope: "agents"})
	if got := len(h.rec.Searches()); got != 1 {
		t.Fatalf("gated search calls = %d", got)
	}
}
