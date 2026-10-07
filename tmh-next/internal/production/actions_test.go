package production

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
	"github.com/mark1708/tmh-next/internal/store"
)

type recordingRunner struct {
	calls [][]string
	err   error
}

func (r *recordingRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	for _, arg := range args {
		if arg == "new-tab" {
			return []byte("2\n"), r.err
		}
	}
	return []byte("zellij 0.45.1\n"), r.err
}

func TestExecuteSendsExactArgvAndDeduplicatesIdempotencyKey(t *testing.T) {
	client, runner, db := actionClient(t)
	defer db.Close()
	ctx := context.Background()
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	terminal := snapshot.Catalog.Terminals[0]
	action := domain.Action{
		Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, terminal.ID),
		Value: "echo safe; touch /tmp/not-executed", ExpectedRevision: snapshot.Revision,
	}
	first, err := client.Execute(ctx, action)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--session", "base", "action", "write-chars", "--pane-id", "terminal_1", action.Value}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0], want) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
	}
	second, err := client.Execute(ctx, action)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || second.NewRevision != first.NewRevision {
		t.Fatalf("duplicate command reran: calls=%d result=%+v", len(runner.calls), second)
	}
}

func TestExecuteRejectsDestructiveCommandBeforeSubprocess(t *testing.T) {
	client, runner, db := actionClient(t)
	defer db.Close()
	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Execute(context.Background(), domain.Action{
		Kind: domain.ActionKill, Target: domain.Ref(domain.KindTerminal, snapshot.Catalog.Terminals[0].ID),
		ExpectedRevision: snapshot.Revision,
	})
	if domain.ErrCodeOf(err) != domain.CodeConfirmationNeeded {
		t.Fatalf("kill error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("subprocess ran before confirmation: %#v", runner.calls)
	}
}

func TestExecuteProtectsObservedUnmanagedWorkspace(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	graph := testGraph("personal")
	graph.Workspaces[0].Management = runtimegraph.ManagementObserved
	runner := &recordingRunner{}
	client, err := NewClient(Options{
		Discoverer: &sequenceDiscoverer{graphs: []*runtimegraph.Graph{graph}},
		Runner:     runner, Store: db,
		Now: func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Execute(context.Background(), domain.Action{
		Kind: domain.ActionSend, Target: domain.Ref(domain.KindTerminal, snapshot.Catalog.Terminals[0].ID),
		Value: "blocked", ExpectedRevision: snapshot.Revision,
	})
	if domain.ErrCodeOf(err) != domain.CodeProtected {
		t.Fatalf("send error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unmanaged command reached subprocess: %#v", runner.calls)
	}
}

func TestSplitRejectsACommandWithNoPersistentRuntimeEffect(t *testing.T) {
	client, runner, db := actionClient(t)
	defer db.Close()
	before, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Execute(context.Background(), domain.Action{
		Kind: domain.ActionSplit, Target: domain.Ref(domain.KindTerminal, before.Catalog.Terminals[0].ID),
		ExpectedRevision: before.Revision,
	})
	if domain.ErrCodeOf(err) != domain.CodeInvalidState {
		t.Fatalf("split error = %v", err)
	}
	after, snapshotErr := client.Snapshot(context.Background())
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if after.Revision != before.Revision || len(runner.calls) != 1 {
		t.Fatalf("failed split changed state: revision %d -> %d, calls=%#v", before.Revision, after.Revision, runner.calls)
	}
}

func TestAttachReturnsClientSideExecInstruction(t *testing.T) {
	client, runner, db := actionClient(t)
	defer db.Close()
	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Execute(context.Background(), domain.Action{
		Kind: domain.ActionAttach, Target: domain.Ref(domain.KindWorkspace, snapshot.Catalog.Workspaces[0].ID),
		ExpectedRevision: snapshot.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.InteractiveCommand, []string{"zellij", "attach", "base"}) {
		t.Fatalf("interactive command = %#v", result.InteractiveCommand)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("daemon attempted interactive subprocess: %#v", runner.calls)
	}
}

func TestPushSnapshotCreatesInteractiveShellInNewTab(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	client, runner, db := actionClient(t)
	defer db.Close()
	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restore := &domain.Snapshot{State: domain.WorkspaceState{Windows: []domain.WindowSpec{{
		ID:       "window:restored",
		Surfaces: []domain.SurfaceSpec{{ID: "surface-restored", CWD: "/tmp"}},
	}}}}
	if err := client.pushSnapshot(
		context.Background(), testGraph("base"), snapshot.Catalog.Workspaces[0], restore,
	); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"--session", "base", "action", "new-tab", "--name", "restored", "--cwd", "/tmp"},
		{"--session", "base", "action", "new-pane", "--tab-id", "2", "--no-focus", "--cwd", "/tmp", "--", "/bin/zsh", "-il"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
	}
}

func TestProductionCatalogAndRuntimeActionFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	graph := testGraph("base")
	graph.Terminals[0].Process = "codex"
	graph.ActiveEntries = []*runtimegraph.ActiveEntry{{
		ID: "active-1", ViewID: graph.Views[0].ID, Kind: runtimegraph.ActiveClient,
		ClientID: "client-1", LastUsed: time.Date(2026, 10, 7, 11, 0, 0, 0, time.UTC),
	}}
	runner := &recordingRunner{}
	client, err := NewClient(Options{
		Discoverer: &sequenceDiscoverer{graphs: []*runtimegraph.Graph{graph}},
		Runner:     runner, Store: db,
		Now: func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	execute := func(action domain.Action) domain.MutationResult {
		t.Helper()
		snapshot, snapshotErr := client.Snapshot(ctx)
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		action.ExpectedRevision = snapshot.Revision
		result, executeErr := client.Execute(ctx, action)
		if executeErr != nil {
			t.Fatalf("%s: %v", action.Kind, executeErr)
		}
		return result
	}

	initial, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	workspace := initial.Catalog.Workspaces[0]
	agent := initial.Catalog.Agents[0]
	active := initial.Catalog.ActiveViews[0]

	execute(domain.Action{Kind: domain.ActionAgentPrompt, Target: domain.Ref(domain.KindAgent, agent.ID), Value: "inspect drift"})
	execute(domain.Action{Kind: domain.ActionAgentWait, Target: domain.Ref(domain.KindAgent, agent.ID)})
	execute(domain.Action{Kind: domain.ActionAgentClaim, Target: domain.Ref(domain.KindAgent, agent.ID)})
	execute(domain.Action{Kind: domain.ActionPin, Target: domain.Ref(domain.KindActive, active.ID)})
	execute(domain.Action{Kind: domain.ActionUnpin, Target: domain.Ref(domain.KindActive, active.ID)})
	execute(domain.Action{Kind: domain.ActionTouch, Target: domain.Ref(domain.KindActive, active.ID)})
	execute(domain.Action{Kind: domain.ActionFreeze, Target: domain.Ref(domain.KindWorkspace, workspace.ID)})

	created := execute(domain.Action{Kind: domain.ActionSnapshotCreate, Target: domain.Ref(domain.KindWorkspace, workspace.ID)})
	snapshot := created.Catalog.Snapshots[len(created.Catalog.Snapshots)-1]
	execute(domain.Action{Kind: domain.ActionSnapshotTag, Target: domain.Ref(domain.KindSnapshot, snapshot.ID), Value: "release"})
	planned := execute(domain.Action{Kind: domain.ActionSnapshotPlan, Target: domain.Ref(domain.KindSnapshot, snapshot.ID)})
	plan := planned.Catalog.RestorePlans[len(planned.Catalog.RestorePlans)-1]
	execute(domain.Action{
		Kind: domain.ActionRestoreApply, Target: domain.Ref(domain.KindPlan, plan.ID),
		Value: string(domain.RestorePull), Confirmed: true,
	})
	execute(domain.Action{Kind: domain.ActionSnapshotDelete, Target: domain.Ref(domain.KindSnapshot, snapshot.ID), Confirmed: true})

	execute(domain.Action{Kind: domain.ActionBackendProbe, Target: domain.Ref(domain.KindBackend, initial.Catalog.Backends[0].ID)})
	execute(domain.Action{Kind: domain.ActionMachinePing, Target: domain.Ref(domain.KindMachine, initial.Catalog.Machines[0].ID)})
	execute(domain.Action{Kind: domain.ActionPerfRecord})
	execute(domain.Action{Kind: domain.ActionPerfBenchmark})

	current, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	configJSON, err := json.Marshal(current.Catalog.Config)
	if err != nil {
		t.Fatal(err)
	}
	execute(domain.Action{Kind: domain.ActionConfigValidate, Value: string(configJSON)})
	config := current.Catalog.Config
	config.RetentionHours = 24
	configJSON, _ = json.Marshal(config)
	execute(domain.Action{Kind: domain.ActionConfigSave, Value: string(configJSON)})
	execute(domain.Action{Kind: domain.ActionConfigReload})

	current, err = client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Catalog.History) == 0 {
		t.Fatal("agent prompt did not produce history")
	}
	exported := execute(domain.Action{
		Kind:      domain.ActionHistoryExport,
		Selection: []string{current.Catalog.History[0].ID},
	})
	if exported.Catalog.LastExport == nil || exported.Catalog.LastExport.Rows == 0 {
		t.Fatalf("history export = %+v", exported.Catalog.LastExport)
	}
	if len(runner.calls) < 2 {
		t.Fatalf("native commands = %#v", runner.calls)
	}
}

func actionClient(t *testing.T) (*Client, *recordingRunner, *store.SQLite) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	graph := testGraph("base")
	client, err := NewClient(Options{
		Discoverer: &sequenceDiscoverer{graphs: []*runtimegraph.Graph{graph}}, Runner: runner, Store: db,
		Now: func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, runner, db
}
