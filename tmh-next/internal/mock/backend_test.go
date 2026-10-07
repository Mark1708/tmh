package mock

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
)

func load(t *testing.T, b *Backend, sc domain.Scenario) *domain.Catalog {
	t.Helper()
	cat, err := b.Load(context.Background(), sc)
	if err != nil {
		t.Fatalf("load %s: %v", sc, err)
	}
	return &cat
}

func execOK(t *testing.T, b *Backend, a domain.Action) domain.MutationResult {
	t.Helper()
	a.ExpectedRevision = b.snapshotForTest().Revision
	res, err := b.Execute(context.Background(), a)
	if err != nil {
		t.Fatalf("execute %s: unexpected error %v", a.Kind, err)
	}
	return res
}

func execFail(t *testing.T, b *Backend, a domain.Action, wantCode domain.ErrCode) error {
	t.Helper()
	a.ExpectedRevision = b.snapshotForTest().Revision
	_, err := b.Execute(context.Background(), a)
	if err == nil {
		t.Fatalf("execute %s: expected %s failure, got success", a.Kind, wantCode)
	}
	if got := domain.ErrCodeOf(err); got != wantCode {
		t.Fatalf("execute %s: code %s, want %s (%v)", a.Kind, got, wantCode, err)
	}
	return err
}

func tickOK(t *testing.T, b *Backend) domain.MutationResult {
	t.Helper()
	res, err := b.Tick(context.Background(), b.snapshotForTest().Revision)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	return res
}

func tickFail(t *testing.T, b *Backend, expected uint64) {
	t.Helper()
	if _, err := b.Tick(context.Background(), expected); err == nil {
		t.Fatalf("tick(expected=%d): expected revision conflict", expected)
	} else if !domain.IsRevisionConflict(err) {
		t.Fatalf("tick(expected=%d): error %v, want revision conflict", expected, err)
	}
}

// snapshotForTest exposes a clone of the current catalog for assertions.
func (b *Backend) snapshotForTest() *domain.Catalog {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cat.Clone()
}

func fresh(t *testing.T) *Backend {
	t.Helper()
	b := New()
	load(t, b, domain.ScenarioDefault)
	return b
}

// TestFixturesHaveReferentialIntegrity proves every scenario fixture passes
// full catalog validation and clones into independent snapshots.
func TestFixturesHaveReferentialIntegrity(t *testing.T) {
	for _, sc := range []domain.Scenario{domain.ScenarioDefault, domain.ScenarioEmpty, domain.ScenarioDegraded} {
		t.Run(string(sc), func(t *testing.T) {
			cat, err := Fixture(sc)
			if err != nil {
				t.Fatalf("fixture: %v", err)
			}
			if err := domain.ValidateCatalog(cat); err != nil {
				t.Fatalf("validate: %v", err)
			}
			clone := cat.Clone()
			clone.Revision = 999
			if cat.Revision == 999 {
				t.Fatal("clone shares revision with original")
			}
			if len(clone.Workspaces) > 0 {
				clone.Workspaces[0].Name = "mutated"
				if cat.Workspaces[0].Name == "mutated" {
					t.Fatal("clone shares workspace structs with original")
				}
			}
			if len(clone.History) > 0 {
				clone.History[0].Lines[0] = "mutated"
				if cat.History[0].Lines[0] == "mutated" {
					t.Fatal("clone shares history lines with original")
				}
			}
			if err := clone.Config.Validate(); err != nil {
				t.Fatalf("baseline config invalid: %v", err)
			}
		})
	}
}

// TestScenarioIsolation proves Load resets the backend to the pristine
// fixture of the requested scenario with fresh counters and tick state.
func TestScenarioIsolation(t *testing.T) {
	b := New()
	def := load(t, b, domain.ScenarioDefault)
	execOK(t, b, domain.Action{Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, "term-0001"), Value: "ls"})
	tickOK(t, b)

	empty := load(t, b, domain.ScenarioEmpty)
	if len(empty.Workspaces) != 0 || len(empty.Agents) != 0 || len(empty.History) != 0 || len(empty.Events) != 0 || len(empty.Terminals) != 0 {
		t.Fatalf("empty scenario has rows: ws=%d terms=%d agents=%d hist=%d evt=%d",
			len(empty.Workspaces), len(empty.Terminals), len(empty.Agents), len(empty.History), len(empty.Events))
	}
	if empty.Revision != 1 || !empty.Now.Equal(epoch) {
		t.Fatalf("empty reload rev=%d now=%s, want 1 at epoch", empty.Revision, empty.Now)
	}
	if len(empty.Backends) != 3 || len(empty.Machines) != 3 || !empty.Performance.Available {
		t.Fatal("empty scenario must keep backends/machines/performance")
	}
	tickOK(t, b)
	emptyTick := tickOK(t, b)
	if got := len(emptyTick.Catalog.Events); got != 0 {
		t.Fatalf("empty tick created %d events", got)
	}

	again := load(t, b, domain.ScenarioDefault)
	if !again.Now.Equal(def.Now) || again.Revision != def.Revision {
		t.Fatal("default reload is not pristine: clock/revision leaked")
	}
	if got := len(again.History); got != len(def.History) {
		t.Fatalf("default history after reload = %d, want pristine %d", got, len(def.History))
	}
}

// TestLogicalClockAndIDs proves the exact one-second clock advance per
// accepted mutation and monotonic ID issuance without collisions.
func TestLogicalClockAndIDs(t *testing.T) {
	b := fresh(t)

	r1 := execOK(t, b, domain.Action{Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, "term-0001"), Value: "echo one"})
	if r1.BaseRevision != 1 || r1.NewRevision != 2 {
		t.Fatalf("send revisions %d→%d, want 1→2", r1.BaseRevision, r1.NewRevision)
	}
	if want := epoch.Add(1 * time.Second); !r1.Catalog.Now.Equal(want) {
		t.Fatalf("clock after 1 mutation = %s, want %s", r1.Catalog.Now, want)
	}
	if r1.Catalog.Revision != r1.NewRevision {
		t.Fatalf("catalog revision %d != result revision %d", r1.Catalog.Revision, r1.NewRevision)
	}
	if got := r1.Catalog.History[len(r1.Catalog.History)-1].ID; got != "hist-0006" {
		t.Fatalf("first new history id = %s, want hist-0006", got)
	}

	r2 := tickOK(t, b)
	if r2.NewRevision != 3 || !r2.Catalog.Now.Equal(epoch.Add(2*time.Second)) {
		t.Fatalf("tick result rev=%d now=%s", r2.NewRevision, r2.Catalog.Now)
	}
	tickFail(t, b, 99) // stale expected revision rejected

	r3 := execOK(t, b, domain.Action{Kind: domain.ActionSnapshotCreate, Target: domain.Ref(domain.KindWorkspace, "base")})
	snap := r3.Catalog.Snapshots[len(r3.Catalog.Snapshots)-1]
	if snap.ID != "snap-0005" {
		t.Fatalf("new snapshot id = %s, want snap-0005", snap.ID)
	}
	r4 := execOK(t, b, domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, snap.ID)})
	plan := r4.Catalog.RestorePlans[len(r4.Catalog.RestorePlans)-1]
	if plan.ID != "plan-0001" {
		t.Fatalf("new plan id = %s, want plan-0001", plan.ID)
	}
	lastEvt := r4.Catalog.Events[len(r4.Catalog.Events)-1]
	if lastEvt.ID != "evt-0011" {
		t.Fatalf("last event id = %s, want evt-0011 (8 fixture + 3 action events; ticks add none)", lastEvt.ID)
	}
}

// TestEveryActionTransition walks the action table: success path plus every
// failure code listed in the plan.
func TestEveryActionTransition(t *testing.T) {
	t.Run("attach", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionAttach, Target: domain.Ref(domain.KindWorkspace, "nope")}, domain.CodeNotFound)
		execFail(t, b, domain.Action{Kind: domain.ActionAttach, Target: domain.Ref(domain.KindWorkspace, "platform")}, domain.CodeNotLive)
		res := execOK(t, b, domain.Action{Kind: domain.ActionAttach, Target: domain.Ref(domain.KindWorkspace, "base")})
		ws := res.Catalog.WorkspaceByID("base")
		if ws.SelectedSurfaceID == "" || ws.LastAttached.IsZero() {
			t.Fatal("attach did not update focus/selected surface")
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionAttach, Target: domain.Ref(domain.KindTerminal, "term-0003")})
		if res.Catalog.WorkspaceByID("ecp").LastFocused.IsZero() {
			t.Fatal("terminal attach did not focus workspace")
		}
	})

	t.Run("freeze", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionFreeze, Target: domain.Ref(domain.KindWorkspace, "nope")}, domain.CodeNotFound)
		res := execOK(t, b, domain.Action{Kind: domain.ActionFreeze, Target: domain.Ref(domain.KindWorkspace, "kb")})
		kb := res.Catalog.WorkspaceByID("kb")
		if kb.Status != domain.WorkspaceFrozen || len(kb.Drift) != 0 || !kb.Desired.Equal(kb.Observed) {
			t.Fatalf("freeze did not reconcile: status=%s drift=%d", kb.Status, len(kb.Drift))
		}
	})

	t.Run("split link", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionSplit, Target: domain.Ref(domain.KindTerminal, "term-0006")}, domain.CodeNotLive)
		res := execOK(t, b, domain.Action{Kind: domain.ActionSplit, Target: domain.Ref(domain.KindTerminal, "term-0001")})
		nt := res.Catalog.Terminals[len(res.Catalog.Terminals)-1]
		if nt.ID != "term-0008" || nt.WorkspaceID != "base" || nt.State != domain.TerminalLive {
			t.Fatalf("split produced %+v", nt)
		}
		if res.Catalog.WorkspaceByID("base").Observed.HasSurface(nt.SurfaceID) != true {
			t.Fatal("split surface missing from observed state")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionLink, Target: domain.Ref(domain.KindTerminal, "term-0001"), Value: "missing"}, domain.CodeBadTarget)
		res = execOK(t, b, domain.Action{Kind: domain.ActionLink, Target: domain.Ref(domain.KindTerminal, "term-0002"), Value: "ecp"})
		t2 := res.Catalog.TerminalByID("term-0002")
		if t2.WorkspaceID != "ecp" {
			t.Fatal("link did not move terminal")
		}
		if !containsString(res.Catalog.WorkspaceByID("ecp").TerminalIDs, "term-0002") {
			t.Fatal("link did not add terminal to target workspace")
		}
		if containsString(res.Catalog.WorkspaceByID("base").TerminalIDs, "term-0002") {
			t.Fatal("link left terminal in old workspace")
		}
	})

	t.Run("active views", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionPin, Target: domain.Ref(domain.KindActive, "view-0001")}, domain.CodeInvalidState)
		execOK(t, b, domain.Action{Kind: domain.ActionUnpin, Target: domain.Ref(domain.KindActive, "view-0001")})
		execFail(t, b, domain.Action{Kind: domain.ActionUnpin, Target: domain.Ref(domain.KindActive, "view-0002")}, domain.CodeInvalidState)
		res := execOK(t, b, domain.Action{Kind: domain.ActionPin, Target: domain.Ref(domain.KindActive, "view-0002")})
		if !res.Catalog.ActiveViewByID("view-0002").Pinned {
			t.Fatal("pin did not stick")
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionTouch, Target: domain.Ref(domain.KindActive, "view-0003")})
		if res.Catalog.ActiveViewByID("view-0003").LastUsed.IsZero() {
			t.Fatal("touch did not update last used")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionTouch, Target: domain.Ref(domain.KindActive, "view-9999")}, domain.CodeNotFound)
	})

	t.Run("prune", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionPrune, Target: domain.Ref(domain.KindActive, "view-0001")}, domain.CodeProtected)
		execFail(t, b, domain.Action{Kind: domain.ActionPrune, Target: domain.Ref(domain.KindActive, "view-0003")}, domain.CodeProtected)
		execFail(t, b, domain.Action{Kind: domain.ActionPrune, Target: domain.Ref(domain.KindActive, "view-0004")}, domain.CodeConfirmationNeeded)
		res := execOK(t, b, domain.Action{Kind: domain.ActionPrune, Target: domain.Ref(domain.KindActive, "view-0004"), Confirmed: true})
		if res.Catalog.ActiveViewByID("view-0004") != nil {
			t.Fatal("prune did not remove expired view")
		}
	})

	t.Run("send kill", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, "term-0001"), Value: "  "}, domain.CodeEmptyValue)
		execFail(t, b, domain.Action{Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, "term-0006"), Value: "x"}, domain.CodeNotLive)
		res := execOK(t, b, domain.Action{Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, "term-0001"), Value: "make test"})
		last := res.Catalog.History[len(res.Catalog.History)-1]
		if last.Scope.ID != "term-0001" || len(last.Lines) != 2 {
			t.Fatalf("send history = %+v", last)
		}
		execFail(t, b, domain.Action{Kind: domain.ActionKill, Target: domain.Ref(domain.KindTerminal, "term-0001")}, domain.CodeConfirmationNeeded)
		res = execOK(t, b, domain.Action{Kind: domain.ActionKill, Target: domain.Ref(domain.KindTerminal, "term-0001"), Confirmed: true})
		if res.Catalog.TerminalByID("term-0001").State != domain.TerminalExited {
			t.Fatal("kill did not exit terminal")
		}
		if res.Catalog.AgentByID("agent-0001").TerminalID != "" {
			t.Fatal("kill did not detach dependent agent")
		}
	})

	t.Run("agent lifecycle", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionAgentFocus, Target: domain.Ref(domain.KindAgent, "agent-0004")}, domain.CodeInvalidState)
		res := execOK(t, b, domain.Action{Kind: domain.ActionAgentFocus, Target: domain.Ref(domain.KindAgent, "agent-0002")})
		if res.Catalog.AgentByID("agent-0002").LastFocused.IsZero() {
			t.Fatal("agent focus did not update timestamps")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionAgentPrompt, Target: domain.Ref(domain.KindAgent, "agent-0004"), Value: "hi"}, domain.CodeInvalidState)
		execFail(t, b, domain.Action{Kind: domain.ActionAgentPrompt, Target: domain.Ref(domain.KindAgent, "agent-0002"), Value: ""}, domain.CodeEmptyValue)
		histBefore := len(b.snapshotForTest().History)
		res = execOK(t, b, domain.Action{Kind: domain.ActionAgentPrompt, Target: domain.Ref(domain.KindAgent, "agent-0002"), Value: "use fiscal 2025"})
		ag := res.Catalog.AgentByID("agent-0002")
		if ag.State != domain.AgentRunning || len(ag.Timeline) != 4 {
			t.Fatalf("prompt state=%s timeline=%d", ag.State, len(ag.Timeline))
		}
		if len(res.Catalog.History) != histBefore+1 {
			t.Fatalf("attached agent prompt history delta = %d, want 1", len(res.Catalog.History)-histBefore)
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionAgentWait, Target: domain.Ref(domain.KindAgent, "agent-0002")})
		if res.Catalog.AgentByID("agent-0002").State != domain.AgentWaiting {
			t.Fatal("wait did not pause agent")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionAgentWait, Target: domain.Ref(domain.KindAgent, "agent-0003")}, domain.CodeInvalidState)
		execOK(t, b, domain.Action{Kind: domain.ActionAgentClaim, Target: domain.Ref(domain.KindAgent, "agent-0003")})
		execFail(t, b, domain.Action{Kind: domain.ActionAgentClaim, Target: domain.Ref(domain.KindAgent, "agent-0003")}, domain.CodeAlreadyClaimed)
		execFail(t, b, domain.Action{Kind: domain.ActionAgentClaim, Target: domain.Ref(domain.KindAgent, "agent-0004")}, domain.CodeInvalidState)
	})

	t.Run("history export", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionHistoryExport, Selection: nil}, domain.CodeEmptySelection)
		res := execOK(t, b, domain.Action{Kind: domain.ActionHistoryExport, Selection: []string{"hist-0001", "hist-0002"}})
		if res.Catalog.LastExport == nil || res.Catalog.LastExport.Rows == 0 {
			t.Fatal("export descriptor not recorded")
		}
		if len(res.Catalog.History) != len(b.snapshotForTest().History) {
			t.Fatal("export must not change history rows")
		}
	})

	t.Run("snapshots", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionSnapshotCreate, Target: domain.Ref(domain.KindWorkspace, "base"), Value: "missing"}, domain.CodeNotFound)
		res := execOK(t, b, domain.Action{Kind: domain.ActionSnapshotCreate, Target: domain.Ref(domain.KindWorkspace, "base")})
		snap := res.Catalog.Snapshots[len(res.Catalog.Snapshots)-1]
		if snap.WindowCount == 0 {
			t.Fatal("snapshot captured no windows")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionSnapshotTag, Target: domain.Ref(domain.KindSnapshot, snap.ID), Value: ""}, domain.CodeEmptyValue)
		res = execOK(t, b, domain.Action{Kind: domain.ActionSnapshotTag, Target: domain.Ref(domain.KindSnapshot, snap.ID), Value: "pre-demo"})
		if !containsString(res.Catalog.SnapshotByID(snap.ID).Tags, "pre-demo") {
			t.Fatal("tag not attached")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionSnapshotDelete, Target: domain.Ref(domain.KindSnapshot, snap.ID)}, domain.CodeConfirmationNeeded)
		res = execOK(t, b, domain.Action{Kind: domain.ActionSnapshotDelete, Target: domain.Ref(domain.KindSnapshot, snap.ID), Confirmed: true})
		if res.Catalog.SnapshotByID(snap.ID) != nil {
			t.Fatal("snapshot not deleted")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, "snap-9999")}, domain.CodeNotFound)
	})

	t.Run("restore apply undo", func(t *testing.T) {
		b := fresh(t)
		res := execOK(t, b, domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, "snap-0004")})
		plan := res.Catalog.RestorePlans[len(res.Catalog.RestorePlans)-1]
		modes := map[domain.RestoreMode]int{}
		for _, op := range plan.Ops {
			modes[op.Mode]++
		}
		if modes[domain.RestorePull] != 1 || modes[domain.RestoreFreeze] != 1 {
			t.Fatalf("kb plan ops = %v, want 1 pull + 1 freeze", modes)
		}
		execFail(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, plan.ID), Value: "pull"}, domain.CodeConfirmationNeeded)
		execFail(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, plan.ID), Value: "push", Confirmed: true}, domain.CodeEmptySelection)
		res = execOK(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, plan.ID), Value: "pull", Confirmed: true})
		kb := res.Catalog.WorkspaceByID("kb")
		if kb.Status != domain.WorkspaceOK || len(kb.Drift) != 0 {
			t.Fatalf("apply did not reconcile kb: status=%s drift=%d", kb.Status, len(kb.Drift))
		}
		execFail(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, plan.ID), Value: "pull", Confirmed: true}, domain.CodeStalePlan)
		res = execOK(t, b, domain.Action{Kind: domain.ActionReconcileUndo, Target: domain.Ref(domain.KindPlan, plan.ID)})
		kb = res.Catalog.WorkspaceByID("kb")
		if kb.Status != domain.WorkspaceDrift || len(kb.Drift) != 1 {
			t.Fatalf("undo did not restore drift: status=%s drift=%d", kb.Status, len(kb.Drift))
		}
		if res.Catalog.RestorePlanByID(plan.ID).Status != domain.PlanUndone {
			t.Fatal("undo did not mark plan undone")
		}
	})

	t.Run("freeze apply creates safety snapshot", func(t *testing.T) {
		b := fresh(t)
		res := execOK(t, b, domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, "snap-0004")})
		plan := res.Catalog.RestorePlans[len(res.Catalog.RestorePlans)-1]
		snapsBefore := len(b.snapshotForTest().Snapshots)
		res = execOK(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, plan.ID), Value: "freeze", Confirmed: true})
		if got := len(res.Catalog.Snapshots); got != snapsBefore+1 {
			t.Fatalf("freeze apply created %d snapshots, want +1", got-snapsBefore)
		}
		created := res.Catalog.Snapshots[len(res.Catalog.Snapshots)-1]
		if !containsString(created.Tags, "reconcile-freeze") {
			t.Fatalf("freeze snapshot tags = %v", created.Tags)
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionReconcileUndo, Target: domain.Ref(domain.KindPlan, plan.ID)})
		if res.Catalog.SnapshotByID(created.ID) != nil {
			t.Fatal("undo of freeze did not delete created snapshot")
		}
	})

	t.Run("undo preconditions", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionReconcileUndo}, domain.CodeUndoUnavailable)
		res := execOK(t, b, domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, "snap-0004")})
		plan := res.Catalog.RestorePlans[len(res.Catalog.RestorePlans)-1]
		execOK(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, plan.ID), Value: "pull", Confirmed: true})
		execOK(t, b, domain.Action{Kind: domain.ActionTouch, Target: domain.Ref(domain.KindActive, "view-0002")})
		execFail(t, b, domain.Action{Kind: domain.ActionReconcileUndo, Target: domain.Ref(domain.KindPlan, plan.ID)}, domain.CodeUndoStale)
	})

	t.Run("backends", func(t *testing.T) {
		b := fresh(t)
		execFail(t, b, domain.Action{Kind: domain.ActionBackendProbe, Target: domain.Ref(domain.KindBackend, "native")}, domain.CodeProbeTimeout)
		res := execOK(t, b, domain.Action{Kind: domain.ActionBackendProbe, Target: domain.Ref(domain.KindBackend, "tmux")})
		if !res.Catalog.BackendByID("tmux").LastProbe.OK {
			t.Fatal("tmux probe not recorded")
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionBackendDoctor, Target: domain.Ref(domain.KindBackend, "native")})
		if res.Catalog.BackendByID("native").LastProbe.OK {
			t.Fatal("native doctor should keep failing status")
		}
		if len(res.Catalog.BackendByID("native").LastProbe.Findings) == 0 {
			t.Fatal("native doctor produced no findings")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionBackendToggle, Target: domain.Ref(domain.KindBackend, "tmux")}, domain.CodeProtected)
		execFail(t, b, domain.Action{Kind: domain.ActionBackendToggle, Target: domain.Ref(domain.KindBackend, "zellij")}, domain.CodeConfirmationNeeded)
		res = execOK(t, b, domain.Action{Kind: domain.ActionBackendToggle, Target: domain.Ref(domain.KindBackend, "zellij"), Confirmed: true})
		if res.Catalog.BackendByID("zellij").Enabled {
			t.Fatal("toggle did not disable zellij")
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionBackendToggle, Target: domain.Ref(domain.KindBackend, "zellij")})
		if !res.Catalog.BackendByID("zellij").Enabled {
			t.Fatal("re-enable without confirmation failed")
		}
	})

	t.Run("machines", func(t *testing.T) {
		b := New()
		load(t, b, domain.ScenarioDegraded)
		execFail(t, b, domain.Action{Kind: domain.ActionMachineConnect, Target: domain.Ref(domain.KindMachine, "build-01")}, domain.CodeUnreachable)
		execFail(t, b, domain.Action{Kind: domain.ActionMachinePing, Target: domain.Ref(domain.KindMachine, "build-01")}, domain.CodeUnreachable)
		execFail(t, b, domain.Action{Kind: domain.ActionMachineDoctor, Target: domain.Ref(domain.KindMachine, "nope")}, domain.CodeNotFound)
		res := execOK(t, b, domain.Action{Kind: domain.ActionMachinePing, Target: domain.Ref(domain.KindMachine, "homelab")})
		if !res.Catalog.MachineByID("homelab").LastResult.OK {
			t.Fatal("homelab ping not recorded")
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionMachineDoctor, Target: domain.Ref(domain.KindMachine, "local")})
		if len(res.Catalog.MachineByID("local").LastResult.Findings) < 2 {
			t.Fatal("doctor findings missing")
		}
	})

	t.Run("performance", func(t *testing.T) {
		b := fresh(t)
		res := execOK(t, b, domain.Action{Kind: domain.ActionPerfRecord})
		if len(res.Catalog.Performance.Traces) != 1 {
			t.Fatalf("traces = %d, want 1", len(res.Catalog.Performance.Traces))
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionPerfBenchmark})
		if len(res.Catalog.Performance.Benchmarks) != 1 {
			t.Fatalf("benchmarks = %d, want 1", len(res.Catalog.Performance.Benchmarks))
		}
		d := New()
		load(t, d, domain.ScenarioDegraded)
		execFail(t, d, domain.Action{Kind: domain.ActionPerfRecord}, domain.CodeBusy)
		execFail(t, d, domain.Action{Kind: domain.ActionPerfBenchmark}, domain.CodeBusy)
	})

	t.Run("config", func(t *testing.T) {
		b := fresh(t)
		good, _ := json.Marshal(domain.Config{
			DefaultPage: "dashboard", LeaderDisplay: domain.LeaderIcon, DefaultBackend: "tmux",
			HistoryMode: domain.HistoryMemory, RetentionHours: 24, Overflow: domain.OverflowReject,
			RedactSecrets: true, RemoteTrust: domain.TrustLocalOnly,
		})
		bad, _ := json.Marshal(domain.Config{
			DefaultPage: "dashboard", LeaderDisplay: domain.LeaderIcon, DefaultBackend: "tmux",
			HistoryMode: domain.HistoryMemory, RetentionHours: 48, Overflow: domain.OverflowReject,
			RemoteTrust: domain.TrustLocalOnly,
		})
		rawInput, _ := json.Marshal(domain.Config{
			DefaultPage: "dashboard", LeaderDisplay: domain.LeaderPath, DefaultBackend: "zellij",
			HistoryMode: domain.HistoryOff, Overflow: domain.OverflowReject,
			AllowRawInput: true, RemoteTrust: domain.TrustPrompt,
		})
		execFail(t, b, domain.Action{Kind: domain.ActionConfigValidate, Value: "not-json"}, domain.CodeValidation)
		execFail(t, b, domain.Action{Kind: domain.ActionConfigValidate, Value: string(bad)}, domain.CodeValidation)
		execOK(t, b, domain.Action{Kind: domain.ActionConfigValidate, Value: string(good)})
		execFail(t, b, domain.Action{Kind: domain.ActionConfigSave, Value: string(rawInput)}, domain.CodeValidation)
		histBefore := len(b.snapshotForTest().History)
		res := execOK(t, b, domain.Action{Kind: domain.ActionConfigSave, Value: string(good)})
		if res.Catalog.Config.HistoryMode != domain.HistoryMemory || res.Catalog.Config.RetentionHours != 24 {
			t.Fatalf("config save did not apply: %+v", res.Catalog.Config)
		}
		if len(res.Catalog.History) != histBefore {
			t.Fatal("config save polluted terminal history")
		}
		if res.Catalog.TerminalByID("term-0001") == nil {
			t.Fatal("config save must not touch terminals")
		}
		res = execOK(t, b, domain.Action{Kind: domain.ActionConfigReload})
		if res.Catalog.Config.HistoryMode != domain.HistoryPersistent || res.Catalog.Config.RetentionHours != 720 {
			t.Fatalf("config reload did not restore baseline: %+v", res.Catalog.Config)
		}
	})
}

// TestFailuresDoNotMutate proves a rejected action leaves revision, clock,
// serial, counters and audit data exactly unchanged.
func TestFailuresDoNotMutate(t *testing.T) {
	b := fresh(t)
	before := b.snapshotForTest()

	failing := []domain.Action{
		{Kind: domain.ActionAttach, Target: domain.Ref(domain.KindWorkspace, "platform")},
		{Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, "term-0006"), Value: "x"},
		{Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, "term-0001"), Value: ""},
		{Kind: domain.ActionKill, Target: domain.Ref(domain.KindTerminal, "term-0001")},
		{Kind: domain.ActionPrune, Target: domain.Ref(domain.KindActive, "view-0004")},
		{Kind: domain.ActionSnapshotDelete, Target: domain.Ref(domain.KindSnapshot, "snap-0001")},
		{Kind: domain.ActionBackendToggle, Target: domain.Ref(domain.KindBackend, "tmux")},
		{Kind: domain.ActionBackendProbe, Target: domain.Ref(domain.KindBackend, "native")},
		{Kind: domain.ActionAgentPrompt, Target: domain.Ref(domain.KindAgent, "agent-0004"), Value: "x"},
		{Kind: domain.ActionReconcileUndo},
		{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, "plan-9999"), Value: "pull", Confirmed: true},
		{Kind: domain.ActionConfigValidate, Value: "{{"},
	}
	for _, a := range failing {
		a.ExpectedRevision = before.Revision
		if _, err := b.Execute(context.Background(), a); err == nil {
			t.Fatalf("%s unexpectedly succeeded", a.Kind)
		}
	}
	if _, err := b.Tick(context.Background(), 123); err == nil {
		t.Fatal("stale tick unexpectedly succeeded")
	}

	after := b.snapshotForTest()
	if after.Revision != before.Revision || !after.Now.Equal(before.Now) || after.MutationSerial != before.MutationSerial {
		t.Fatalf("failure changed revision/clock/serial: %d/%s/%d → %d/%s/%d",
			before.Revision, before.Now, before.MutationSerial, after.Revision, after.Now, after.MutationSerial)
	}
	if len(after.Events) != len(before.Events) || len(after.History) != len(before.History) ||
		len(after.Snapshots) != len(before.Snapshots) || len(after.Terminals) != len(before.Terminals) {
		t.Fatal("failure changed audit or entity rows")
	}
	for i := range after.Events {
		if after.Events[i].ID != before.Events[i].ID {
			t.Fatal("failure changed event ids")
		}
	}
}

// TestReconcileUndoInvalidation locks the undo lifecycle: ticks never
// invalidate undo, any later user action does.
func TestReconcileUndoInvalidation(t *testing.T) {
	build := func(t *testing.T) (*Backend, string) {
		b := fresh(t)
		res := execOK(t, b, domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, "snap-0004")})
		planID := res.Catalog.RestorePlans[len(res.Catalog.RestorePlans)-1].ID
		execOK(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, planID), Value: "pull", Confirmed: true})
		return b, planID
	}

	t.Run("ticks do not invalidate undo", func(t *testing.T) {
		b, planID := build(t)
		for range 4 {
			tickOK(t, b)
		}
		res := execOK(t, b, domain.Action{Kind: domain.ActionReconcileUndo, Target: domain.Ref(domain.KindPlan, planID)})
		if res.Catalog.RestorePlanByID(planID).Status != domain.PlanUndone {
			t.Fatal("undo after ticks failed")
		}
		kb := res.Catalog.WorkspaceByID("kb")
		if kb.Status != domain.WorkspaceDrift {
			t.Fatalf("undo after ticks did not restore drift: %s", kb.Status)
		}
	})

	t.Run("user action invalidates undo", func(t *testing.T) {
		b, planID := build(t)
		execOK(t, b, domain.Action{Kind: domain.ActionTouch, Target: domain.Ref(domain.KindActive, "view-0002")})
		execFail(t, b, domain.Action{Kind: domain.ActionReconcileUndo, Target: domain.Ref(domain.KindPlan, planID)}, domain.CodeUndoStale)
	})

	t.Run("only latest applied plan undoes", func(t *testing.T) {
		b := fresh(t)
		res := execOK(t, b, domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, "snap-0004")})
		first := res.Catalog.RestorePlans[len(res.Catalog.RestorePlans)-1].ID
		res = execOK(t, b, domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, "snap-0001")})
		second := res.Catalog.RestorePlans[len(res.Catalog.RestorePlans)-1].ID
		execOK(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, first), Value: "pull", Confirmed: true})
		// After undoing first, second can be applied and undone by itself.
		res = execOK(t, b, domain.Action{Kind: domain.ActionReconcileUndo, Target: domain.Ref(domain.KindPlan, first)})
		if res.Catalog.RestorePlanByID(first).Status != domain.PlanUndone {
			t.Fatal("first plan undo failed")
		}
		snapsBefore := len(b.snapshotForTest().Snapshots)
		res = execOK(t, b, domain.Action{Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, second), Value: "freeze", Confirmed: true})
		if len(res.Catalog.Snapshots) != snapsBefore+1 {
			t.Fatal("freeze apply did not create safety snapshot")
		}
		execFail(t, b, domain.Action{Kind: domain.ActionReconcileUndo, Target: domain.Ref(domain.KindPlan, first)}, domain.CodeUndoStale)
	})
}

// TestSearchRevision proves search reads are revision-tagged and scoped.
func TestSearchRevision(t *testing.T) {
	b := fresh(t)
	ctx := context.Background()

	r0, err := b.Search(ctx, domain.SearchQuery{Text: "", Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if r0.Revision != 1 {
		t.Fatalf("search revision = %d, want 1", r0.Revision)
	}

	r1, err := b.Search(ctx, domain.SearchQuery{Text: "ledger", Scope: "agents"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Hits) == 0 || r1.Hits[0].Scope != "agents" {
		t.Fatalf("agent search hits = %+v", r1.Hits)
	}

	r2, err := b.Search(ctx, domain.SearchQuery{Text: "make test", Scope: "history"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Hits) == 0 {
		t.Fatal("history search found nothing")
	}

	execOK(t, b, domain.Action{Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, "term-0007"), Value: "grep unique-token"})

	r3, err := b.Search(ctx, domain.SearchQuery{Text: "unique-token", Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if r3.Revision != 2 {
		t.Fatalf("search revision after mutation = %d, want 2", r3.Revision)
	}
	found := false
	for _, h := range r3.Hits {
		if h.Scope == "history" {
			found = true
		}
	}
	if !found {
		t.Fatal("sent input not searchable in history")
	}

	r4, err := b.Search(ctx, domain.SearchQuery{Text: "", Scope: "events"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range r4.Hits {
		if h.Scope != "events" {
			t.Fatalf("scope leak: %+v", h)
		}
	}
	limited, err := b.Search(ctx, domain.SearchQuery{Text: "", Scope: "all", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.Hits) > 3 {
		t.Fatalf("limit ignored: %d hits", len(limited.Hits))
	}
}

// TestScenarioTickContracts proves the per-scenario tick semantics.
func TestScenarioTickContracts(t *testing.T) {
	t.Run("default agent bump every fifth accepted tick", func(t *testing.T) {
		b := fresh(t)
		progress0 := b.snapshotForTest().AgentByID("agent-0001").Progress
		for i := 1; i <= 5; i++ {
			res := tickOK(t, b)
			if got := len(res.Catalog.Performance.Samples); got != 24+i {
				t.Fatalf("tick %d perf samples = %d, want %d", i, got, 24+i)
			}
			agentEvents := 0
			for _, e := range res.Catalog.Events {
				if e.Category == domain.EventAgent && e.Message == "progress 43%" {
					agentEvents++
				}
			}
			if i == 5 && agentEvents != 1 {
				t.Fatalf("fifth tick agent events = %d, want 1", agentEvents)
			}
			if i < 5 && agentEvents != 0 {
				t.Fatalf("tick %d produced agent event early", i)
			}
		}
		if got := b.snapshotForTest().AgentByID("agent-0001").Progress; got != progress0+1 {
			t.Fatalf("agent progress = %d, want %d", got, progress0+1)
		}
	})

	t.Run("empty tick creates no rows", func(t *testing.T) {
		b := New()
		load(t, b, domain.ScenarioEmpty)
		for range 6 {
			res := tickOK(t, b)
			c := res.Catalog
			if len(c.Workspaces)+len(c.Agents)+len(c.History)+len(c.Events)+len(c.Snapshots)+len(c.Terminals) != 0 {
				t.Fatal("empty tick created rows")
			}
			if len(c.Performance.Samples) == 0 {
				t.Fatal("empty tick dropped performance updates")
			}
		}
	})

	t.Run("degraded tick never heals", func(t *testing.T) {
		b := New()
		load(t, b, domain.ScenarioDegraded)
		for range 7 {
			res := tickOK(t, b)
			m := res.Catalog.MachineByID("build-01")
			if m.Status != domain.MachineOffline {
				t.Fatal("degraded tick healed offline machine")
			}
			if res.Catalog.BackendByID("native").Status != domain.BackendUnavailable {
				t.Fatal("degraded tick healed native backend")
			}
			if res.Catalog.Performance.Available {
				t.Fatal("degraded tick enabled performance service")
			}
		}
	})

	t.Run("default tick without running agents adds no event", func(t *testing.T) {
		b := fresh(t)
		for _, a := range b.snapshotForTest().Agents {
			if a.State == domain.AgentRunning {
				execOK(t, b, domain.Action{Kind: domain.ActionAgentWait, Target: domain.Ref(domain.KindAgent, a.ID)})
			}
		}
		for range 5 {
			tickOK(t, b)
		}
		for _, e := range b.snapshotForTest().Events {
			if e.Category == domain.EventAgent && e.Message == "progress 43%" {
				t.Fatal("agent event added with no running agent")
			}
		}
	})
}

// TestEveryListedKindIsExercised cross-checks the action table test coverage
// against the canonical kind list.
func TestEveryListedKindIsExercised(t *testing.T) {
	exercised := map[domain.ActionKind]bool{
		domain.ActionAttach: true, domain.ActionFreeze: true, domain.ActionSplit: true,
		domain.ActionLink: true, domain.ActionPin: true, domain.ActionUnpin: true,
		domain.ActionTouch: true, domain.ActionPrune: true, domain.ActionSend: true,
		domain.ActionKill: true, domain.ActionAgentFocus: true, domain.ActionAgentPrompt: true,
		domain.ActionAgentWait: true, domain.ActionAgentClaim: true, domain.ActionHistoryExport: true,
		domain.ActionSnapshotCreate: true, domain.ActionSnapshotTag: true, domain.ActionSnapshotDelete: true,
		domain.ActionSnapshotPlan: true, domain.ActionRestoreApply: true, domain.ActionReconcileUndo: true,
		domain.ActionBackendProbe: true, domain.ActionBackendDoctor: true, domain.ActionBackendToggle: true,
		domain.ActionMachineConnect: true, domain.ActionMachinePing: true, domain.ActionMachineDoctor: true,
		domain.ActionPerfRecord: true, domain.ActionPerfBenchmark: true, domain.ActionConfigValidate: true,
		domain.ActionConfigSave: true, domain.ActionConfigReload: true,
	}
	for _, kind := range domain.AllActionKinds() {
		if !exercised[kind] {
			t.Errorf("action kind %s not exercised by TestEveryActionTransition", kind)
		}
	}
	if got := len(domain.AllActionKinds()); got != len(exercised) {
		t.Fatalf("AllActionKinds returns %d, canonical set has %d", got, len(exercised))
	}
}

// TestFieldWriteBack exercises the composite field write/remove helpers used
// by restore apply and undo.
func TestFieldWriteBack(t *testing.T) {
	st := domain.WorkspaceState{Windows: []domain.WindowSpec{{
		ID: "win-0001", Index: 0, Layout: "tiled",
		Surfaces: []domain.SurfaceSpec{surface("sf-0001", "editor", "~/a")},
	}}}

	// composite surface write-back
	if err := setField(&st, "surface/sf-0001", "revised~/b"); err != nil {
		t.Fatal(err)
	}
	if got := st.Windows[0].Surfaces[0].Title; got != "revised" {
		t.Fatalf("surface title = %q", got)
	}
	// surface add on empty window list
	st2 := domain.WorkspaceState{}
	if err := setField(&st2, "surface/sf-0009", "new~/c"); err != nil {
		t.Fatal(err)
	}
	if len(st2.Windows) != 1 || len(st2.Windows[0].Surfaces) != 1 {
		t.Fatalf("surface add produced %+v", st2)
	}
	// window layout write + window add
	if err := setField(&st, "window/win-0001", "main-vertical"); err != nil {
		t.Fatal(err)
	}
	if got := st.Windows[0].Layout; got != "main-vertical" {
		t.Fatalf("layout = %q", got)
	}
	if err := setField(&st, "window/win-0002", "even-horizontal"); err != nil {
		t.Fatal(err)
	}
	if len(st.Windows) != 2 || st.Windows[1].ID != "win-0002" {
		t.Fatalf("window add produced %+v", st.Windows)
	}
	// removals
	if err := removeField(&st, "surface/sf-0001"); err != nil {
		t.Fatal(err)
	}
	if err := removeField(&st, "window/win-0002"); err != nil {
		t.Fatal(err)
	}
	if len(st.Windows) != 1 || len(st.Windows[0].Surfaces) != 0 {
		t.Fatalf("removals produced %+v", st)
	}
	// unknown paths error
	if err := setField(&st, "bogus/path", "x"); err == nil {
		t.Fatal("bogus setField accepted")
	}
	if err := setField(&st, "surface/sf-nope/title", "x"); err == nil {
		t.Fatal("unknown surface attribute path accepted")
	}
	if err := removeField(&st, "window/nope"); err == nil {
		t.Fatal("unknown removal accepted")
	}
	if err := removeField(&st, "surface/only/window"); err == nil {
		t.Fatal("composite removal accepted")
	}
	// granular attribute set on an existing surface
	if err := setField(&st, "surface/sf-0001/cwd", "~/z"); err == nil {
		t.Fatal("attribute write on missing surface accepted")
	}
}

// TestPlanOrderingStable proves the canonical plan sort and its validation.
func TestPlanOrderingStable(t *testing.T) {
	ops := []domain.RestoreOp{
		{ID: "op-003", Mode: domain.RestoreFreeze, Field: "snapshot", Target: domain.Ref(domain.KindSnapshot, "kb")},
		{ID: "op-001", Mode: domain.RestorePull, Field: "surface/sf-0008", Target: domain.Ref(domain.KindWorkspace, "kb")},
		{ID: "op-002", Mode: domain.RestorePush, Field: "surface/sf-0007", Target: domain.Ref(domain.KindWorkspace, "kb")},
	}
	domain.SortRestoreOps(ops)
	want := []string{"op-003", "op-001", "op-002"} // mode order: freeze < pull < push
	for i, op := range ops {
		if op.ID != want[i] {
			t.Fatalf("order[%d] = %s, want %s", i, op.ID, want[i])
		}
	}
}

// TestValidateNegativePaths drives validation failures for enums, refs and
// plan ordering.
func TestValidateNegativePaths(t *testing.T) {
	base, err := Fixture(domain.ScenarioDefault)
	if err != nil {
		t.Fatal(err)
	}

	invalid := func(name string, mutate func(*domain.Catalog)) {
		t.Helper()
		c := base.Clone()
		mutate(c)
		if err := domain.ValidateCatalog(c); err == nil {
			t.Errorf("%s: validation passed", name)
		}
	}

	invalid("agent kind", func(c *domain.Catalog) { c.Agents[0].Kind = "skynet" })
	invalid("agent state", func(c *domain.Catalog) { c.Agents[0].State = "dreaming" })
	invalid("agent progress", func(c *domain.Catalog) { c.Agents[0].Progress = 150 })
	invalid("terminal state", func(c *domain.Catalog) { c.Terminals[0].State = "limbo" })
	invalid("workspace status", func(c *domain.Catalog) { c.Workspaces[0].Status = "chaos" })
	invalid("machine status", func(c *domain.Catalog) { c.Machines[0].Status = "sleeping" })
	invalid("active kind", func(c *domain.Catalog) { c.ActiveViews[0].Kind = "ghost" })
	invalid("pinned kind mismatch", func(c *domain.Catalog) {
		c.ActiveViews[1].Pinned = true // view-0002 is shared
	})
	invalid("event severity", func(c *domain.Catalog) { c.Events[0].Severity = "meh" })
	invalid("event category", func(c *domain.Catalog) { c.Events[0].Category = "party" })
	invalid("plan mode", func(c *domain.Catalog) {
		c.RestorePlans = append(c.RestorePlans, &domain.RestorePlan{
			ID: "plan-9999", SnapshotID: "snap-0001", WorkspaceID: "base",
			Ops: []domain.RestoreOp{{ID: "op-001", Mode: "sideways", Field: "x",
				Target: domain.Ref(domain.KindWorkspace, "base")}},
		})
	})
	invalid("plan unsorted", func(c *domain.Catalog) {
		c.RestorePlans = append(c.RestorePlans, &domain.RestorePlan{
			ID: "plan-9998", SnapshotID: "snap-0001", WorkspaceID: "base",
			Ops: []domain.RestoreOp{
				{ID: "op-002", Mode: domain.RestorePull, Field: "b", Target: domain.Ref(domain.KindWorkspace, "base")},
				{ID: "op-001", Mode: domain.RestorePush, Field: "a", Target: domain.Ref(domain.KindWorkspace, "base")},
			},
		})
	})
	invalid("perf bound", func(c *domain.Catalog) {
		c.Performance.Samples = make([]domain.PerfSample, domain.MaxPerfSamples+1)
	})
	invalid("duplicate id", func(c *domain.Catalog) {
		cp := *c.Agents[0]
		cp.Timeline = append([]domain.AgentEntry(nil), c.Agents[0].Timeline...)
		c.Agents = append(c.Agents, &cp)
	})
}
