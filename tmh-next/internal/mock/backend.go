package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
)

// Backend is the deterministic in-memory store behind the demo control client. All
// state transitions are pure functions of the current catalog plus the action;
// time is a logical clock and identifiers are monotonic counters. No real
// side effects are ever performed.
type Backend struct {
	mu         sync.Mutex
	cat        *domain.Catalog
	counters   map[string]int
	tickCount  int
	serialBase uint64
}

// Compile-time interface satisfaction lives in the wiring test; the mock only
// depends on domain.

// New returns an unloaded backend; Load seeds it with a scenario fixture.
func New() *Backend {
	return &Backend{counters: map[string]int{}}
}

// Load resets the backend to the pristine fixture of the scenario. A reload
// of the *currently loaded* scenario returns the live post-state instead of
// the fixture, so conflict recovery never loses accepted mutations.
func (b *Backend) Load(_ context.Context, sc domain.Scenario) (domain.Catalog, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cat != nil && b.cat.Scenario == sc {
		return *b.cat.Clone(), nil
	}
	fix, err := Fixture(sc)
	if err != nil {
		return domain.Catalog{}, err
	}
	b.cat = fix
	b.tickCount = 0
	b.serialBase = 0
	b.counters = primeCounters(fix)
	return *fix.Clone(), nil
}

// Search is a concurrent read: it never mutates state.
func (b *Backend) Search(_ context.Context, q domain.SearchQuery) (domain.SearchResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cat == nil {
		return domain.SearchResult{}, domain.Fail(domain.CodeNotFound, "backend not loaded")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 30
	}
	text := strings.ToLower(q.Text)
	var hits []domain.SearchHit
	add := func(scope string, ref domain.ResourceRef, title, snippet string) {
		if len(hits) >= limit {
			return
		}
		if text != "" &&
			!strings.Contains(strings.ToLower(title), text) &&
			!strings.Contains(strings.ToLower(snippet), text) {
			return
		}
		hits = append(hits, domain.SearchHit{Ref: ref, Title: title, Snippet: snippet, Scope: scope})
	}

	if q.Scope == "all" || q.Scope == "" || q.Scope == "agents" {
		for _, a := range b.cat.Agents {
			add("agents", domain.Ref(domain.KindAgent, a.ID), a.Name+" ("+string(a.State)+")",
				fmt.Sprintf("%s agent on %s, progress %d%%", a.Kind, a.WorkspaceID, a.Progress))
		}
	}
	if q.Scope == "all" || q.Scope == "" || q.Scope == "history" {
		for _, h := range b.cat.History {
			add("history", domain.Ref(domain.KindHistory, h.ID),
				fmt.Sprintf("stream %s:%s", h.Scope.Kind, h.Scope.ID),
				strings.Join(h.Lines, " / "))
		}
	}
	if q.Scope == "all" || q.Scope == "" || q.Scope == "events" {
		for _, e := range b.cat.Events {
			add("events", domain.Ref(domain.KindEvent, e.ID), e.Message, e.Detail)
		}
	}
	if q.Scope == "all" || q.Scope == "" {
		for _, w := range b.cat.Workspaces {
			add("workspaces", domain.Ref(domain.KindWorkspace, w.ID), w.Name,
				fmt.Sprintf("workspace on %s via %s (%s)", w.MachineID, w.BackendID, w.Status))
		}
		for _, t := range b.cat.Terminals {
			add("terminals", domain.Ref(domain.KindTerminal, t.ID), t.Name, t.Process+" "+t.CWD)
		}
	}
	return domain.SearchResult{Revision: b.cat.Revision, Hits: hits}, nil
}

// Execute applies one user action under the revision contract. Failures never
// mutate catalog, revision, clock, counters or audit data.
func (b *Backend) Execute(_ context.Context, a domain.Action) (domain.MutationResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cat == nil {
		return domain.MutationResult{}, domain.Fail(domain.CodeNotFound, "backend not loaded")
	}
	if a.ExpectedRevision != b.cat.Revision {
		return domain.MutationResult{}, domain.ErrRevisionConflict
	}
	work := b.cat.Clone()
	msg, err := b.apply(work, a)
	if err != nil {
		return domain.MutationResult{}, err
	}
	work.MutationSerial++
	if err := domain.ValidateCatalog(work); err != nil {
		return domain.MutationResult{}, domain.Fail(domain.CodeMalformedResult, "transition produced invalid catalog: %v", err)
	}
	base := b.cat.Revision
	b.cat = work
	return domain.MutationResult{
		BaseRevision: base,
		NewRevision:  work.Revision,
		Catalog:      work.Clone(),
		Action:       a,
		Message:      msg + " [MOCK]",
	}, nil
}

// Tick applies one scenario tick under the revision contract.
func (b *Backend) Tick(_ context.Context, expected uint64) (domain.MutationResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cat == nil {
		return domain.MutationResult{}, domain.Fail(domain.CodeNotFound, "backend not loaded")
	}
	if expected != b.cat.Revision {
		return domain.MutationResult{}, domain.ErrRevisionConflict
	}
	work := b.cat.Clone()
	b.tickCount++
	b.advance(work)

	sample := domain.PerfSample{
		At: work.Now, InputP50: 3 + b.tickCount%3, InputP95: 8 + b.tickCount%5,
		InputP99: 11 + b.tickCount%7, RedrawP99: 15 + b.tickCount%6,
		ActionP99: 30 + b.tickCount%9, QueueDepth: b.tickCount % 4,
	}
	work.Performance.Samples = append(work.Performance.Samples, sample)
	if len(work.Performance.Samples) > domain.MaxPerfSamples {
		work.Performance.Samples = work.Performance.Samples[len(work.Performance.Samples)-domain.MaxPerfSamples:]
	}
	for _, m := range work.Machines {
		if m.Status == domain.MachineOnline {
			m.Heartbeat = work.Now
		}
	}

	msg := "tick"
	if work.Scenario == domain.ScenarioDefault && b.tickCount%5 == 0 {
		msg = b.tickAgent(work)
	}
	if err := domain.ValidateCatalog(work); err != nil {
		return domain.MutationResult{}, domain.Fail(domain.CodeMalformedResult, "tick produced invalid catalog: %v", err)
	}
	base := b.cat.Revision
	b.cat = work
	return domain.MutationResult{
		BaseRevision: base,
		NewRevision:  work.Revision,
		Catalog:      work.Clone(),
		IsTick:       true,
		Message:      msg + " [MOCK]",
	}, nil
}

// advance moves the logical clock and revision exactly one step.
func (b *Backend) advance(c *domain.Catalog) {
	c.Revision++
	c.Now = c.Now.Add(time.Second)
}

// tickAgent bumps the lowest-ID running agent and records one agent event.
func (b *Backend) tickAgent(c *domain.Catalog) string {
	for _, a := range c.Agents {
		if a.State != domain.AgentRunning {
			continue
		}
		a.Progress = min(a.Progress+1, 99)
		b.addEvent(c, domain.EventAgent, domain.SeverityInfo,
			domain.Ref(domain.KindAgent, a.ID),
			fmt.Sprintf("progress %d%%", a.Progress))
		return fmt.Sprintf("tick: agent %s progress %d%%", a.ID, a.Progress)
	}
	return "tick"
}

func (b *Backend) nextID(prefix string) string {
	b.counters[prefix]++
	return fmt.Sprintf("%s-%04d", prefix, b.counters[prefix])
}

func (b *Backend) addEvent(c *domain.Catalog, cat domain.EventCategory, sev domain.EventSeverity, res domain.ResourceRef, msg string) {
	seq := 1
	if n := len(c.Events); n > 0 {
		seq = c.Events[n-1].Seq + 1
	}
	c.Events = append(c.Events, &domain.Event{
		ID: b.nextID("evt"), Seq: seq, At: c.Now, Category: cat, Severity: sev,
		Resource: res, Message: msg,
		Detail: fmt.Sprintf(`{"cat":%q,"msg":%q,"mock":true}`, cat, msg),
	})
}

func (b *Backend) addHistory(c *domain.Catalog, kind domain.ResourceKind, id string, lines []string) {
	seq := 1
	for _, h := range c.History {
		if h.Scope.Kind == kind && h.Scope.ID == id {
			seq = max(seq, h.Seq+1)
		}
	}
	c.History = append(c.History, &domain.HistoryChunk{
		ID: b.nextID("hist"), Scope: domain.HistoryScope{Kind: kind, ID: id},
		Seq: seq, At: c.Now, Lines: lines, Kind: "stream",
	})
}

// apply performs the action transition table on the working catalog. It must
// return before any revision/clock advance for failures (they never mutate).
func (b *Backend) apply(c *domain.Catalog, a domain.Action) (string, error) {
	// Failures evaluate BEFORE the clock moves; successes advance first so
	// timestamps and events record the post-state.
	pre := func() { b.advance(c) }
	switch a.Kind {
	case domain.ActionAttach:
		var ws *domain.Workspace
		var term *domain.Terminal
		switch a.Target.Kind {
		case domain.KindWorkspace:
			ws = c.WorkspaceByID(a.Target.ID)
			if ws == nil {
				return "", domain.Fail(domain.CodeNotFound, "workspace %s not found", a.Target.ID)
			}
			for _, id := range ws.TerminalIDs {
				if t := c.TerminalByID(id); t != nil && t.State == domain.TerminalLive {
					term = t
					break
				}
			}
		case domain.KindTerminal:
			term = c.TerminalByID(a.Target.ID)
			if term == nil {
				return "", domain.Fail(domain.CodeNotFound, "terminal %s not found", a.Target.ID)
			}
			ws = c.WorkspaceByID(term.WorkspaceID)
		default:
			return "", domain.Fail(domain.CodeBadTarget, "attach targets workspace or terminal")
		}
		if term == nil || term.State != domain.TerminalLive {
			return "", domain.Fail(domain.CodeNotLive, "no live terminal to attach")
		}
		pre()
		ws.LastAttached = c.Now
		ws.LastFocused = c.Now
		ws.SelectedSurfaceID = term.SurfaceID
		b.addEvent(c, domain.EventWorkspace, domain.SeverityInfo, domain.Ref(domain.KindWorkspace, ws.ID),
			fmt.Sprintf("attached via %s", term.ID))
		return fmt.Sprintf("attached to %s (surface %s)", term.ID, term.SurfaceID), nil

	case domain.ActionFreeze:
		ws := c.WorkspaceByID(a.Target.ID)
		if a.Target.Kind != domain.KindWorkspace || ws == nil {
			return "", domain.Fail(domain.CodeNotFound, "workspace %s not found", a.Target.ID)
		}
		pre()
		ws.Desired = ws.Observed.Clone()
		ws.Drift = nil
		ws.Status = domain.WorkspaceFrozen
		b.addEvent(c, domain.EventWorkspace, domain.SeverityInfo, domain.Ref(domain.KindWorkspace, ws.ID), "frozen at observed state")
		return "workspace frozen: desired := observed", nil

	case domain.ActionSplit:
		t := c.TerminalByID(a.Target.ID)
		if a.Target.Kind != domain.KindTerminal || t == nil {
			return "", domain.Fail(domain.CodeNotFound, "terminal %s not found", a.Target.ID)
		}
		if t.State != domain.TerminalLive {
			return "", domain.Fail(domain.CodeNotLive, "terminal %s is not live", t.ID)
		}
		ws := c.WorkspaceByID(t.WorkspaceID)
		if ws == nil {
			return "", domain.Fail(domain.CodeNotFound, "terminal %s has no workspace", t.ID)
		}
		pre()
		newID := b.nextID("term")
		sfID := b.nextID("sf")
		newSurface := surface(sfID, "terminal", t.CWD)
		inserted := false
		for index := range ws.Observed.Windows {
			if windowHasSurface(ws.Observed.Windows[index], t.SurfaceID) {
				ws.Observed.Windows[index].Layout = "even-horizontal"
				ws.Observed.Windows[index].Surfaces = append(ws.Observed.Windows[index].Surfaces, newSurface)
				inserted = true
				break
			}
		}
		if !inserted {
			return "", domain.Fail(domain.CodeInvalidState, "source terminal surface is not in the observed workspace")
		}
		// Creating a pane through tmh updates both the live topology and its
		// desired definition; it must not manufacture reconciliation drift.
		ws.Desired.Windows = cloneWindows(ws.Observed.Windows)
		nt := &domain.Terminal{
			ID: newID, Name: ws.Name + "-terminal", WorkspaceID: ws.ID, SurfaceID: sfID,
			BackendID: t.BackendID, State: domain.TerminalLive, Process: "zsh", CWD: t.CWD,
			Capabilities: append([]string(nil), t.Capabilities...), LastIO: c.Now, CreatedAt: c.Now,
		}
		c.Terminals = append(c.Terminals, nt)
		ws.TerminalIDs = append(ws.TerminalIDs, newID)
		b.addEvent(c, domain.EventTerminal, domain.SeverityInfo, domain.Ref(domain.KindTerminal, newID),
			fmt.Sprintf("created pane beside %s", t.ID))
		return fmt.Sprintf("New terminal pane %s created", newID), nil

	case domain.ActionLink:
		t := c.TerminalByID(a.Target.ID)
		if a.Target.Kind != domain.KindTerminal || t == nil {
			return "", domain.Fail(domain.CodeNotFound, "terminal %s not found", a.Target.ID)
		}
		ws := c.WorkspaceByID(a.Value)
		if ws == nil {
			return "", domain.Fail(domain.CodeBadTarget, "link target workspace %q not found", a.Value)
		}
		old := c.WorkspaceByID(t.WorkspaceID)
		pre()
		if old != nil {
			old.TerminalIDs = removeID(old.TerminalIDs, t.ID)
		}
		t.WorkspaceID = ws.ID
		if len(ws.Observed.Windows) > 0 && len(ws.Observed.Windows[0].Surfaces) > 0 {
			t.SurfaceID = ws.Observed.Windows[0].Surfaces[0].ID
		}
		ws.TerminalIDs = append(ws.TerminalIDs, t.ID)
		b.addEvent(c, domain.EventTerminal, domain.SeverityInfo, domain.Ref(domain.KindTerminal, t.ID),
			fmt.Sprintf("linked to workspace %s", ws.ID))
		return fmt.Sprintf("terminal %s linked to %s", t.ID, ws.ID), nil

	case domain.ActionPin, domain.ActionUnpin:
		v := c.ActiveViewByID(a.Target.ID)
		if a.Target.Kind != domain.KindActive || v == nil {
			return "", domain.Fail(domain.CodeNotFound, "active view %s not found", a.Target.ID)
		}
		if a.Kind == domain.ActionPin {
			if v.Pinned {
				return "", domain.Fail(domain.CodeInvalidState, "view %s already pinned", v.ID)
			}
			pre()
			v.Pinned = true
			v.Kind = domain.ActivePinned
			v.TTLSeconds = 0
		} else {
			if !v.Pinned {
				return "", domain.Fail(domain.CodeInvalidState, "view %s not pinned", v.ID)
			}
			pre()
			v.Pinned = false
			v.Kind = domain.ActiveRecent
			v.TTLSeconds = 900
		}
		b.addEvent(c, domain.EventWorkspace, domain.SeverityInfo, domain.Ref(domain.KindActive, v.ID), string(a.Kind))
		return fmt.Sprintf("%s %s", a.Kind, v.ID), nil

	case domain.ActionTouch:
		v := c.ActiveViewByID(a.Target.ID)
		if a.Target.Kind != domain.KindActive || v == nil {
			return "", domain.Fail(domain.CodeNotFound, "active view %s not found", a.Target.ID)
		}
		pre()
		v.LastUsed = c.Now
		b.addEvent(c, domain.EventWorkspace, domain.SeverityInfo, domain.Ref(domain.KindActive, v.ID), "touched")
		return "touched " + v.ID, nil

	case domain.ActionPrune:
		v := c.ActiveViewByID(a.Target.ID)
		if a.Target.Kind != domain.KindActive || v == nil {
			return "", domain.Fail(domain.CodeNotFound, "active view %s not found", a.Target.ID)
		}
		if v.Pinned {
			return "", domain.Fail(domain.CodeProtected, "view %s is pinned", v.ID)
		}
		if !v.ExpiredAt(c.Now).Before(c.Now) {
			return "", domain.Fail(domain.CodeProtected, "view %s has not expired", v.ID)
		}
		if !a.Confirmed {
			return "", domain.Fail(domain.CodeConfirmationNeeded, "prune requires confirmation")
		}
		pre()
		c.ActiveViews = removeView(c.ActiveViews, v.ID)
		b.addEvent(c, domain.EventWorkspace, domain.SeverityInfo, domain.Ref(domain.KindActive, v.ID), "pruned")
		return "pruned " + v.ID, nil

	case domain.ActionSend:
		t := c.TerminalByID(a.Target.ID)
		if a.Target.Kind != domain.KindTerminal || t == nil {
			return "", domain.Fail(domain.CodeNotFound, "terminal %s not found", a.Target.ID)
		}
		if t.State != domain.TerminalLive {
			return "", domain.Fail(domain.CodeNotLive, "terminal %s is not live", t.ID)
		}
		if strings.TrimSpace(a.Value) == "" {
			return "", domain.Fail(domain.CodeEmptyValue, "send requires non-empty input")
		}
		pre()
		t.LastIO = c.Now
		b.addHistory(c, domain.KindTerminal, t.ID, []string{
			"$ " + a.Value,
			"(" + t.Process + ") " + deterministicEcho(a.Value),
		})
		b.addEvent(c, domain.EventTerminal, domain.SeverityInfo, domain.Ref(domain.KindTerminal, t.ID), "input sent")
		return fmt.Sprintf("sent to %s: %q", t.ID, a.Value), nil

	case domain.ActionKill:
		t := c.TerminalByID(a.Target.ID)
		if a.Target.Kind != domain.KindTerminal || t == nil {
			return "", domain.Fail(domain.CodeNotFound, "terminal %s not found", a.Target.ID)
		}
		if t.State != domain.TerminalLive {
			return "", domain.Fail(domain.CodeNotLive, "terminal %s is not live", t.ID)
		}
		if !a.Confirmed {
			return "", domain.Fail(domain.CodeConfirmationNeeded, "kill requires confirmation")
		}
		pre()
		t.State = domain.TerminalExited
		detached := []string{}
		for _, ag := range c.Agents {
			if ag.TerminalID == t.ID {
				ag.TerminalID = ""
				detached = append(detached, ag.ID)
			}
		}
		b.addEvent(c, domain.EventTerminal, domain.SeverityWarn, domain.Ref(domain.KindTerminal, t.ID),
			fmt.Sprintf("killed; detached agents: %v", detached))
		return fmt.Sprintf("killed %s (detached %d agents)", t.ID, len(detached)), nil

	case domain.ActionAgentFocus:
		ag := c.AgentByID(a.Target.ID)
		if a.Target.Kind != domain.KindAgent || ag == nil {
			return "", domain.Fail(domain.CodeNotFound, "agent %s not found", a.Target.ID)
		}
		if ag.State == domain.AgentDone {
			return "", domain.Fail(domain.CodeInvalidState, "agent %s is done", ag.ID)
		}
		pre()
		ag.LastUsed = c.Now
		ag.LastFocused = c.Now
		b.addEvent(c, domain.EventAgent, domain.SeverityInfo, domain.Ref(domain.KindAgent, ag.ID), "focused")
		return "focused agent " + ag.ID, nil

	case domain.ActionAgentPrompt:
		ag := c.AgentByID(a.Target.ID)
		if a.Target.Kind != domain.KindAgent || ag == nil {
			return "", domain.Fail(domain.CodeNotFound, "agent %s not found", a.Target.ID)
		}
		if ag.State == domain.AgentDone {
			return "", domain.Fail(domain.CodeInvalidState, "agent %s is done and cannot accept prompts", ag.ID)
		}
		if strings.TrimSpace(a.Value) == "" {
			return "", domain.Fail(domain.CodeEmptyValue, "agent prompt requires non-empty input")
		}
		pre()
		ag.State = domain.AgentRunning
		ag.Prompt = a.Value
		ag.LastUsed = c.Now
		ag.Timeline = append(ag.Timeline,
			domain.AgentEntry{At: c.Now, Kind: "prompt", Text: a.Value},
			domain.AgentEntry{At: c.Now, Kind: "result", Text: deterministicEcho(a.Value)},
		)
		if ag.TerminalID != "" {
			b.addHistory(c, domain.KindAgent, ag.ID, []string{"prompt: " + a.Value, "result: " + deterministicEcho(a.Value)})
		}
		b.addEvent(c, domain.EventAgent, domain.SeverityInfo, domain.Ref(domain.KindAgent, ag.ID), "prompt accepted; running")
		return fmt.Sprintf("prompt accepted by %s", ag.ID), nil

	case domain.ActionAgentWait:
		ag := c.AgentByID(a.Target.ID)
		if a.Target.Kind != domain.KindAgent || ag == nil {
			return "", domain.Fail(domain.CodeNotFound, "agent %s not found", a.Target.ID)
		}
		if ag.State != domain.AgentRunning {
			return "", domain.Fail(domain.CodeInvalidState, "agent %s is %s, not running", ag.ID, ag.State)
		}
		pre()
		ag.State = domain.AgentWaiting
		b.addEvent(c, domain.EventAgent, domain.SeverityInfo, domain.Ref(domain.KindAgent, ag.ID), "waiting for input")
		return "agent " + ag.ID + " waiting", nil

	case domain.ActionAgentClaim:
		ag := c.AgentByID(a.Target.ID)
		if a.Target.Kind != domain.KindAgent || ag == nil {
			return "", domain.Fail(domain.CodeNotFound, "agent %s not found", a.Target.ID)
		}
		if ag.ClaimedBy != "" {
			return "", domain.Fail(domain.CodeAlreadyClaimed, "agent %s claimed by %s", ag.ID, ag.ClaimedBy)
		}
		if ag.State == domain.AgentDone {
			return "", domain.Fail(domain.CodeInvalidState, "agent %s is done", ag.ID)
		}
		pre()
		ag.ClaimedBy = "demo-operator"
		b.addEvent(c, domain.EventAgent, domain.SeverityInfo, domain.Ref(domain.KindAgent, ag.ID), "claimed by demo-operator")
		return "claimed " + ag.ID, nil

	case domain.ActionHistoryExport:
		if len(a.Selection) == 0 {
			return "", domain.Fail(domain.CodeEmptySelection, "export requires at least one history row")
		}
		rows := 0
		scopes := map[string]bool{}
		for _, id := range a.Selection {
			for _, h := range c.History {
				if h.ID == id {
					rows += len(h.Lines)
					scopes[string(h.Scope.Kind)+":"+h.Scope.ID] = true
				}
			}
		}
		if rows == 0 {
			return "", domain.Fail(domain.CodeEmptySelection, "selection matched no history rows")
		}
		pre()
		names := make([]string, 0, len(scopes))
		for s := range scopes {
			names = append(names, s)
		}
		sortStrings(names)
		c.LastExport = &domain.ExportDescriptor{
			At: c.Now, Rows: rows, Scopes: names,
			Filename: fmt.Sprintf("tmh-history-rev%d.json", c.Revision),
		}
		b.addEvent(c, domain.EventSystem, domain.SeverityInfo, domain.Ref(domain.KindHistory, "history"), "mock export recorded")
		return fmt.Sprintf("exported %d rows from %d scopes (mock file)", rows, len(names)), nil

	case domain.ActionSnapshotCreate:
		var targets []*domain.Workspace
		if a.Value == "all" {
			targets = c.Workspaces
		} else {
			ws := c.WorkspaceByID(a.Value)
			if a.Value == "" && a.Target.Kind == domain.KindWorkspace {
				ws = c.WorkspaceByID(a.Target.ID)
			}
			if ws == nil {
				return "", domain.Fail(domain.CodeNotFound, "workspace %q not found", a.Value)
			}
			targets = []*domain.Workspace{ws}
		}
		if len(targets) == 0 {
			return "", domain.Fail(domain.CodeNotFound, "no workspaces to snapshot")
		}
		pre()
		created := []string{}
		for _, ws := range targets {
			id := b.nextID("snap")
			c.Snapshots = append(c.Snapshots, &domain.Snapshot{
				ID: id, WorkspaceID: ws.ID, CreatedAt: c.Now, BackendID: ws.BackendID,
				Tags: []string{"manual"}, State: ws.Observed.Clone(),
				WindowCount: len(ws.Observed.Windows), SurfaceCount: ws.Observed.SurfaceCount(),
			})
			created = append(created, id)
			b.addEvent(c, domain.EventSnapshot, domain.SeverityInfo, domain.Ref(domain.KindSnapshot, id),
				fmt.Sprintf("snapshot of %s", ws.ID))
		}
		return fmt.Sprintf("created snapshots %v", created), nil

	case domain.ActionSnapshotTag:
		s := c.SnapshotByID(a.Target.ID)
		if a.Target.Kind != domain.KindSnapshot || s == nil {
			return "", domain.Fail(domain.CodeNotFound, "snapshot %s not found", a.Target.ID)
		}
		if strings.TrimSpace(a.Value) == "" {
			return "", domain.Fail(domain.CodeEmptyValue, "tag requires non-empty value")
		}
		pre()
		if !containsString(s.Tags, a.Value) {
			s.Tags = append(s.Tags, a.Value)
		}
		b.addEvent(c, domain.EventSnapshot, domain.SeverityInfo, domain.Ref(domain.KindSnapshot, s.ID), "tagged "+a.Value)
		return fmt.Sprintf("tagged %s with %q", s.ID, a.Value), nil

	case domain.ActionSnapshotDelete:
		s := c.SnapshotByID(a.Target.ID)
		if a.Target.Kind != domain.KindSnapshot || s == nil {
			return "", domain.Fail(domain.CodeNotFound, "snapshot %s not found", a.Target.ID)
		}
		if !a.Confirmed {
			return "", domain.Fail(domain.CodeConfirmationNeeded, "snapshot delete requires confirmation")
		}
		pre()
		c.Snapshots = removeSnapshot(c.Snapshots, s.ID)
		cascade := 0
		plans := c.RestorePlans[:0]
		for _, p := range c.RestorePlans {
			if p.SnapshotID == s.ID {
				cascade++
				continue
			}
			plans = append(plans, p)
		}
		c.RestorePlans = plans
		b.addEvent(c, domain.EventSnapshot, domain.SeverityWarn, domain.Ref(domain.KindSnapshot, s.ID),
			fmt.Sprintf("deleted (cascaded %d plans)", cascade))
		return fmt.Sprintf("deleted snapshot %s", s.ID), nil

	case domain.ActionSnapshotPlan:
		s := c.SnapshotByID(a.Target.ID)
		if a.Target.Kind != domain.KindSnapshot || s == nil {
			return "", domain.Fail(domain.CodeNotFound, "snapshot %s not found", a.Target.ID)
		}
		ws := c.WorkspaceByID(s.WorkspaceID)
		if ws == nil {
			return "", domain.Fail(domain.CodeNotFound, "snapshot workspace %s missing", s.WorkspaceID)
		}
		pre()
		plan := b.buildPlan(c, s, ws)
		c.RestorePlans = append(c.RestorePlans, plan)
		b.addEvent(c, domain.EventRestore, domain.SeverityInfo, domain.Ref(domain.KindPlan, plan.ID),
			fmt.Sprintf("plan built: %d ops", len(plan.Ops)))
		return fmt.Sprintf("restore plan %s created (%d ops)", plan.ID, len(plan.Ops)), nil

	case domain.ActionRestoreApply:
		p := c.RestorePlanByID(a.Target.ID)
		if a.Target.Kind != domain.KindPlan || p == nil {
			return "", domain.Fail(domain.CodeNotFound, "plan %s not found", a.Target.ID)
		}
		if p.Status != domain.PlanPending {
			return "", domain.Fail(domain.CodeStalePlan, "plan %s is %s, not pending", p.ID, p.Status)
		}
		if !a.Confirmed {
			return "", domain.Fail(domain.CodeConfirmationNeeded, "restore apply requires confirmation")
		}
		mode := domain.RestoreMode(a.Value)
		switch mode {
		case domain.RestorePush, domain.RestorePull, domain.RestoreFreeze:
		default:
			return "", domain.Fail(domain.CodeValidation, "apply mode must be push, pull or freeze")
		}
		// The included set is the UI selection when provided; otherwise the
		// ops stored as Included in the plan.
		var ops []*domain.RestoreOp
		if len(a.Selection) > 0 {
			sel := map[string]bool{}
			for _, id := range a.Selection {
				sel[id] = true
			}
			for i := range p.Ops {
				if p.Ops[i].Mode != mode {
					continue
				}
				if sel[p.Ops[i].ID] {
					p.Ops[i].Included = true
					ops = append(ops, &p.Ops[i])
				} else {
					p.Ops[i].Included = false
				}
			}
		} else {
			ops = p.IncludedOps(mode)
		}
		if len(ops) == 0 {
			return "", domain.Fail(domain.CodeEmptySelection, "no included %s ops", mode)
		}
		ws := c.WorkspaceByID(p.WorkspaceID)
		if ws == nil {
			return "", domain.Fail(domain.CodeNotFound, "plan workspace %s missing", p.WorkspaceID)
		}
		pre()
		var undo []domain.RestoreOp
		createdSnap := ""
		for _, op := range ops {
			if mode == domain.RestoreFreeze {
				id := b.nextID("snap")
				c.Snapshots = append(c.Snapshots, &domain.Snapshot{
					ID: id, WorkspaceID: ws.ID, CreatedAt: c.Now, BackendID: ws.BackendID,
					Tags: []string{"reconcile-freeze"}, State: ws.Observed.Clone(),
					WindowCount: len(ws.Observed.Windows), SurfaceCount: ws.Observed.SurfaceCount(),
				})
				createdSnap = id
				undo = append(undo, domain.RestoreOp{ID: op.ID, Mode: mode, Field: "snapshot:" + id, Before: id, Included: true})
				continue
			}
			state := &ws.Desired
			if mode == domain.RestorePush {
				state = &ws.Observed
			}
			if op.After == "" {
				if err := removeField(state, op.Field); err != nil {
					return "", domain.Fail(domain.CodeStalePlan, "op %s: %v", op.ID, err)
				}
			} else if err := setField(state, op.Field, op.After); err != nil {
				return "", domain.Fail(domain.CodeStalePlan, "op %s: %v", op.ID, err)
			}
			undo = append(undo, domain.RestoreOp{ID: op.ID, Mode: mode, Field: op.Field,
				Before: op.Before, After: op.After, BeforeAbsent: op.BeforeAbsent, Included: true})
			op.AppliedAt = c.Now
		}
		reconcileStatus(ws)
		p.Status = domain.PlanApplied
		p.AppliedAt = c.Now
		p.AppliedSerial = c.MutationSerial + 1
		p.UndoOps = undo
		b.addEvent(c, domain.EventRestore, domain.SeverityInfo, domain.Ref(domain.KindPlan, p.ID),
			fmt.Sprintf("applied %d %s ops (freeze snapshot %s)", len(ops), mode, createdSnap))
		return fmt.Sprintf("applied plan %s mode %s", p.ID, mode), nil

	case domain.ActionReconcileUndo:
		var latest *domain.RestorePlan
		for _, p := range c.RestorePlans {
			if p.Status == domain.PlanApplied && (latest == nil || p.ID > latest.ID) {
				latest = p
			}
		}
		if latest == nil {
			return "", domain.Fail(domain.CodeUndoUnavailable, "no applied plan to undo")
		}
		if a.Target.ID != "" && a.Target.ID != latest.ID {
			return "", domain.Fail(domain.CodeUndoStale, "plan %s is not the latest applied plan", a.Target.ID)
		}
		if c.MutationSerial != latest.AppliedSerial {
			return "", domain.Fail(domain.CodeUndoStale, "a later user mutation invalidated undo")
		}
		ws := c.WorkspaceByID(latest.WorkspaceID)
		if ws == nil {
			return "", domain.Fail(domain.CodeNotFound, "plan workspace %s missing", latest.WorkspaceID)
		}
		pre()
		for i := len(latest.UndoOps) - 1; i >= 0; i-- {
			op := latest.UndoOps[i]
			if op.Mode == domain.RestoreFreeze {
				c.Snapshots = removeSnapshot(c.Snapshots, op.Before)
				continue
			}
			state := &ws.Desired
			if op.Mode == domain.RestorePush {
				state = &ws.Observed
			}
			if op.BeforeAbsent || op.Before == "" {
				_ = removeField(state, op.Field)
			} else if err := setField(state, op.Field, op.Before); err != nil {
				return "", domain.Fail(domain.CodeUndoStale, "undo op %s: %v", op.ID, err)
			}
		}
		reconcileStatus(ws)
		latest.Status = domain.PlanUndone
		latest.UndoneAt = c.Now
		b.addEvent(c, domain.EventRestore, domain.SeverityInfo, domain.Ref(domain.KindPlan, latest.ID), "reconcile undone")
		return fmt.Sprintf("undid plan %s (field-level inverse)", latest.ID), nil

	case domain.ActionBackendProbe, domain.ActionBackendDoctor:
		be := c.BackendByID(a.Target.ID)
		if a.Target.Kind != domain.KindBackend || be == nil {
			return "", domain.Fail(domain.CodeNotFound, "backend %s not found", a.Target.ID)
		}
		if be.Status == domain.BackendUnavailable {
			if a.Kind == domain.ActionBackendProbe {
				return "", domain.Fail(domain.CodeProbeTimeout, "probe of %s timed out after 3000ms (mock)", be.ID)
			}
			pre()
			be.LastProbe = domain.ProbeResult{At: c.Now, OK: false,
				Findings: append([]string(nil), be.UnsupportedReasons...)}
			b.addEvent(c, domain.EventBackend, domain.SeverityWarn, domain.Ref(domain.KindBackend, be.ID), "doctor completed with findings")
			return fmt.Sprintf("doctor: %d findings (backend unavailable)", len(be.UnsupportedReasons)), nil
		}
		pre()
		latency := 3 + (len(be.Capabilities) * 2)
		findings := append([]string(nil), be.Capabilities...)
		if a.Kind == domain.ActionBackendDoctor {
			findings = append(findings, be.UnsupportedReasons...)
		}
		be.LastProbe = domain.ProbeResult{At: c.Now, OK: true, LatencyMS: latency, Findings: findings}
		b.addEvent(c, domain.EventBackend, domain.SeverityInfo, domain.Ref(domain.KindBackend, be.ID), string(a.Kind)+" ok")
		return fmt.Sprintf("%s %s: ok (%dms)", a.Kind, be.ID, latency), nil

	case domain.ActionBackendToggle:
		be := c.BackendByID(a.Target.ID)
		if a.Target.Kind != domain.KindBackend || be == nil {
			return "", domain.Fail(domain.CodeNotFound, "backend %s not found", a.Target.ID)
		}
		if be.Default {
			return "", domain.Fail(domain.CodeProtected, "default backend %s cannot be disabled", be.ID)
		}
		if be.Enabled && !a.Confirmed {
			return "", domain.Fail(domain.CodeConfirmationNeeded, "disabling a backend requires confirmation")
		}
		pre()
		be.Enabled = !be.Enabled
		b.addEvent(c, domain.EventBackend, domain.SeverityWarn, domain.Ref(domain.KindBackend, be.ID),
			fmt.Sprintf("enabled=%v", be.Enabled))
		return fmt.Sprintf("backend %s enabled=%v", be.ID, be.Enabled), nil

	case domain.ActionMachineConnect, domain.ActionMachinePing, domain.ActionMachineDoctor:
		m := c.MachineByID(a.Target.ID)
		if a.Target.Kind != domain.KindMachine || m == nil {
			return "", domain.Fail(domain.CodeNotFound, "machine %s not found", a.Target.ID)
		}
		if m.Status == domain.MachineOffline {
			return "", domain.Fail(domain.CodeUnreachable, "machine %s is offline", m.ID)
		}
		pre()
		m.Heartbeat = c.Now
		findings := []string{"ssh ok"}
		if a.Kind == domain.ActionMachineDoctor {
			findings = append(findings, "tmux 3.4", "disk 62%")
		}
		m.LastResult = domain.ProbeResult{At: c.Now, OK: true, LatencyMS: m.LatencyMS, Findings: findings}
		b.addEvent(c, domain.EventMachine, domain.SeverityInfo, domain.Ref(domain.KindMachine, m.ID), string(a.Kind)+" ok")
		return fmt.Sprintf("%s %s: ok (%dms)", a.Kind, m.ID, m.LatencyMS), nil

	case domain.ActionPerfRecord, domain.ActionPerfBenchmark:
		if !c.Performance.Available {
			return "", domain.Fail(domain.CodeBusy, "performance service unavailable")
		}
		pre()
		if a.Kind == domain.ActionPerfRecord {
			c.Performance.Traces = append(c.Performance.Traces, domain.PerfTrace{
				At: c.Now, Kind: "input", P99MS: 9 + len(c.Performance.Traces)%5, Detail: "recorded via mock recorder",
			})
			if len(c.Performance.Traces) > 20 {
				c.Performance.Traces = c.Performance.Traces[len(c.Performance.Traces)-20:]
			}
			b.addEvent(c, domain.EventSystem, domain.SeverityInfo, domain.Ref(domain.KindConfig, "performance"), "trace recorded")
			return fmt.Sprintf("recorded trace #%d", len(c.Performance.Traces)), nil
		}
		c.Performance.Benchmarks = append(c.Performance.Benchmarks, domain.PerfBenchmark{
			At: c.Now, Ops: 500, AvgMS: 4, P99MS: 11 + len(c.Performance.Benchmarks)%7,
		})
		if len(c.Performance.Benchmarks) > 20 {
			c.Performance.Benchmarks = c.Performance.Benchmarks[len(c.Performance.Benchmarks)-20:]
		}
		b.addEvent(c, domain.EventSystem, domain.SeverityInfo, domain.Ref(domain.KindConfig, "performance"), "benchmark completed")
		return fmt.Sprintf("benchmark #%d: 500 ops, p99 %dms", len(c.Performance.Benchmarks), c.Performance.Benchmarks[len(c.Performance.Benchmarks)-1].P99MS), nil

	case domain.ActionConfigValidate:
		cfg, err := decodeConfig(a.Value)
		if err != nil {
			return "", err
		}
		if err := cfg.Validate(); err != nil {
			return "", err
		}
		pre()
		b.addEvent(c, domain.EventConfig, domain.SeverityInfo, domain.Ref(domain.KindConfig, "config"), "settings validated: ok")
		return "Settings are valid", nil

	case domain.ActionConfigSave:
		cfg, err := decodeConfig(a.Value)
		if err != nil {
			return "", err
		}
		if err := cfg.Validate(); err != nil {
			return "", err
		}
		pre()
		c.Config = cfg
		b.addEvent(c, domain.EventConfig, domain.SeverityInfo, domain.Ref(domain.KindConfig, "config"),
			fmt.Sprintf("saved: history=%s retention=%dh", cfg.HistoryMode, cfg.RetentionHours))
		return "Settings saved in demo memory", nil

	case domain.ActionConfigReload:
		pre()
		c.Config = baseConfig()
		b.addEvent(c, domain.EventConfig, domain.SeverityInfo, domain.Ref(domain.KindConfig, "config"), "reset demo settings baseline")
		return "Settings reset to demo baseline", nil

	default:
		return "", domain.Fail(domain.CodeValidation, "unknown action kind %q", a.Kind)
	}
}

// buildPlan constructs the sorted restore plan for snapshot vs workspace.
func (b *Backend) buildPlan(c *domain.Catalog, s *domain.Snapshot, ws *domain.Workspace) *domain.RestorePlan {
	wsRef := domain.Ref(domain.KindWorkspace, ws.ID)
	var ops []domain.RestoreOp
	n := 0
	op := func(mode domain.RestoreMode, field, before, after string, beforeAbsent bool) {
		n++
		ops = append(ops, domain.RestoreOp{
			ID: fmt.Sprintf("op-%03d", n), Mode: mode, Target: wsRef,
			Field: field, Before: before, After: after, BeforeAbsent: beforeAbsent, Included: true,
		})
	}
	// push: snapshot desired → observed reality.
	for _, d := range diffStates(ws.Observed, s.State) {
		op(domain.RestorePush, d.field, d.from, d.to, d.fromAbsent)
	}
	// pull: current observed → desired.
	for _, d := range diffStates(ws.Desired, ws.Observed) {
		op(domain.RestorePull, d.field, d.from, d.to, d.fromAbsent)
	}
	// freeze: one op protecting the current observed state.
	n++
	ops = append(ops, domain.RestoreOp{
		ID: fmt.Sprintf("op-%03d", n), Mode: domain.RestoreFreeze,
		Target: domain.Ref(domain.KindSnapshot, ws.ID), Field: "snapshot",
		Before: "", After: fmt.Sprintf("%d windows/%d surfaces", len(ws.Observed.Windows), ws.Observed.SurfaceCount()),
		BeforeAbsent: true, Included: true,
	})
	domain.SortRestoreOps(ops)
	return &domain.RestorePlan{
		ID: b.nextID("plan"), SnapshotID: s.ID, WorkspaceID: ws.ID,
		Status: domain.PlanPending, CreatedAt: c.Now, Ops: ops,
	}
}

// reconcileStatus recomputes drift and status after restore operations.
func reconcileStatus(ws *domain.Workspace) {
	if ws.Desired.Equal(ws.Observed) {
		ws.Drift = nil
		if ws.Status != domain.WorkspaceFrozen {
			ws.Status = domain.WorkspaceOK
		}
		return
	}
	drift := []domain.DriftEntry{}
	for _, d := range diffStates(ws.Desired, ws.Observed) {
		drift = append(drift, domain.DriftEntry{Field: d.field, Desired: d.from, Observed: d.to})
	}
	ws.Drift = drift
	ws.Status = domain.WorkspaceDrift
}

type stateDiff struct {
	field      string
	from       string
	to         string
	fromAbsent bool
}

// diffStates diffs two workspace states field-by-field; from = a, to = b.
func diffStates(a, b domain.WorkspaceState) []stateDiff {
	fa := stateFields(a)
	fb := stateFields(b)
	fields := map[string]bool{}
	for f := range fa {
		fields[f] = true
	}
	for f := range fb {
		fields[f] = true
	}
	names := make([]string, 0, len(fields))
	for f := range fields {
		names = append(names, f)
	}
	sortStrings(names)
	var out []stateDiff
	for _, f := range names {
		va, aOK := fa[f]
		vb, bOK := fb[f]
		if aOK && bOK && va == vb {
			continue
		}
		out = append(out, stateDiff{field: f, from: va, to: vb, fromAbsent: !aOK})
	}
	return out
}

// stateFields flattens a workspace state into comparable string fields.
// stateFields flattens a workspace state into one composite field per window
// and surface so every diff op has an exact field-level inverse.
func stateFields(s domain.WorkspaceState) map[string]string {
	out := map[string]string{}
	for _, w := range s.Windows {
		out["window/"+w.ID] = w.Layout
		for _, sf := range w.Surfaces {
			out["surface/"+sf.ID] = sf.Title + "~" + sf.CWD
		}
	}
	return out
}

// setField writes a field path into a workspace state.
func setField(state *domain.WorkspaceState, field, value string) error {
	parts := strings.Split(field, "/")
	switch {
	case len(parts) == 2 && parts[0] == "window":
		for i := range state.Windows {
			if state.Windows[i].ID == parts[1] {
				state.Windows[i].Layout = value
				return nil
			}
		}
		state.Windows = append(state.Windows, domain.WindowSpec{
			ID: parts[1], Index: len(state.Windows), Layout: value,
		})
		return nil
	case len(parts) == 3 && parts[0] == "window" && parts[2] == "layout":
		return setField(state, "window/"+parts[1], value)
	case len(parts) == 2 && parts[0] == "surface":
		bits := strings.SplitN(value, "~", 2)
		title := bits[0]
		cwd := ""
		if len(bits) > 1 {
			cwd = bits[1]
		}
		for _, w := range state.Windows {
			for i := range w.Surfaces {
				if w.Surfaces[i].ID == parts[1] {
					w.Surfaces[i].Title = title
					w.Surfaces[i].CWD = cwd
					return nil
				}
			}
		}
		if len(state.Windows) == 0 {
			state.Windows = []domain.WindowSpec{{ID: "win-restored", Index: 0}}
		}
		last := &state.Windows[len(state.Windows)-1]
		last.Surfaces = append(last.Surfaces, domain.SurfaceSpec{ID: parts[1], Title: title, CWD: cwd})
		return nil
	case len(parts) == 3 && parts[0] == "surface":
		for _, w := range state.Windows {
			for i := range w.Surfaces {
				if w.Surfaces[i].ID == parts[1] {
					switch parts[2] {
					case "title":
						w.Surfaces[i].Title = value
					case "cwd":
						w.Surfaces[i].CWD = value
					case "split":
						w.Surfaces[i].Split = value
					default:
						return fmt.Errorf("unknown surface attribute %q", parts[2])
					}
					return nil
				}
			}
		}
		return fmt.Errorf("surface %s not present", parts[1])
	default:
		return fmt.Errorf("unknown field path %q", field)
	}
}

// removeField deletes a window or surface path from a state.
func removeField(state *domain.WorkspaceState, field string) error {
	parts := strings.Split(field, "/")
	if len(parts) != 2 {
		return fmt.Errorf("cannot remove composite field %q", field)
	}
	switch parts[0] {
	case "window":
		for i := range state.Windows {
			if state.Windows[i].ID == parts[1] {
				state.Windows = append(state.Windows[:i], state.Windows[i+1:]...)
				return nil
			}
		}
	case "surface":
		for wi := range state.Windows {
			for si := range state.Windows[wi].Surfaces {
				if state.Windows[wi].Surfaces[si].ID == parts[1] {
					w := &state.Windows[wi]
					w.Surfaces = append(w.Surfaces[:si], w.Surfaces[si+1:]...)
					return nil
				}
			}
		}
	}
	return fmt.Errorf("%s %s not present", parts[0], parts[1])
}

func decodeConfig(v string) (domain.Config, error) {
	var cfg domain.Config
	if strings.TrimSpace(v) == "" {
		return cfg, domain.Fail(domain.CodeEmptyValue, "config draft payload missing")
	}
	if err := json.Unmarshal([]byte(v), &cfg); err != nil {
		return cfg, domain.Fail(domain.CodeValidation, "config draft malformed: %v", err)
	}
	return cfg, nil
}

func deterministicEcho(in string) string {
	in = strings.TrimSpace(in)
	if len(in) > 48 {
		in = in[:48] + "…"
	}
	return "mock: processed «" + in + "»"
}

func removeID(ids []string, id string) []string {
	out := ids[:0]
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}

func removeView(views []*domain.ActiveView, id string) []*domain.ActiveView {
	out := views[:0]
	for _, v := range views {
		if v.ID != id {
			out = append(out, v)
		}
	}
	return out
}

func removeSnapshot(snaps []*domain.Snapshot, id string) []*domain.Snapshot {
	out := snaps[:0]
	for _, s := range snaps {
		if s.ID != id {
			out = append(out, s)
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// primeCounters derives the ID counters from a fixture so newly issued IDs
// never collide with existing entities.
func primeCounters(c *domain.Catalog) map[string]int {
	counters := map[string]int{}
	bump := func(id string) {
		i := strings.LastIndexByte(id, '-')
		if i < 0 {
			return
		}
		var n int
		if _, err := fmt.Sscanf(id[i+1:], "%d", &n); err != nil {
			return
		}
		prefix := id[:i]
		if n > counters[prefix] {
			counters[prefix] = n
		}
	}
	for _, t := range c.Terminals {
		bump(t.ID)
	}
	for _, h := range c.History {
		bump(h.ID)
	}
	for _, e := range c.Events {
		bump(e.ID)
	}
	for _, s := range c.Snapshots {
		bump(s.ID)
	}
	for _, p := range c.RestorePlans {
		bump(p.ID)
	}
	for _, v := range c.ActiveViews {
		bump(v.ID)
	}
	for _, a := range c.Agents {
		bump(a.ID)
	}
	for _, w := range c.Workspaces {
		for _, win := range w.Desired.Windows {
			bump(win.ID)
			for _, sf := range win.Surfaces {
				bump(sf.ID)
			}
		}
		for _, win := range w.Observed.Windows {
			bump(win.ID)
			for _, sf := range win.Surfaces {
				bump(sf.ID)
			}
		}
	}
	return counters
}

func windowHasSurface(window domain.WindowSpec, surfaceID string) bool {
	for _, surface := range window.Surfaces {
		if surface.ID == surfaceID {
			return true
		}
	}
	return false
}
