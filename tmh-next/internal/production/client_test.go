package production

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
	"github.com/mark1708/tmh-next/internal/store"
)

type sequenceDiscoverer struct {
	graphs []*runtimegraph.Graph
	index  int
}

func (d *sequenceDiscoverer) Discover(context.Context) (*runtimegraph.Graph, error) {
	index := d.index
	if index >= len(d.graphs) {
		index = len(d.graphs) - 1
	}
	d.index++
	return d.graphs[index], nil
}

type blockingDiscoverer struct{}

func (blockingDiscoverer) Discover(ctx context.Context) (*runtimegraph.Graph, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestClientSnapshotPersistsAndOnlyRevisesOnObservedChange(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	base := testGraph("base")
	changed := testGraph("base", "infra")
	discoverer := &sequenceDiscoverer{graphs: []*runtimegraph.Graph{base, base, changed}}
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client, err := NewClient(Options{Discoverer: discoverer, Store: db, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}

	first, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || len(first.Catalog.Workspaces) != 1 {
		t.Fatalf("first snapshot = rev %d workspaces %d", first.Revision, len(first.Catalog.Workspaces))
	}
	second, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision != 1 {
		t.Fatalf("unchanged runtime revised catalog to %d", second.Revision)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	events, errs := client.Watch(watchCtx, first.EventSeq)
	third, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if third.Revision != 2 || len(third.Catalog.Workspaces) != 2 {
		t.Fatalf("changed snapshot = rev %d workspaces %d", third.Revision, len(third.Catalog.Workspaces))
	}
	select {
	case event := <-events:
		if event.Revision != 2 || event.BaseRevision != 1 {
			t.Fatalf("watch event = %+v", event)
		}
	case err := <-errs:
		t.Fatalf("watch failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("watch event not delivered")
	}
}

func TestConfiguredWorkspaceIsManaged(t *testing.T) {
	graph := testGraph("base", "personal")
	graph.Workspaces[0].Management = runtimegraph.ManagementObserved
	graph.Workspaces[1].Management = runtimegraph.ManagementObserved
	markConfiguredManaged(graph, map[string]domain.WorkspaceState{"base": {}})
	if graph.Workspaces[0].Management != runtimegraph.ManagementManaged {
		t.Fatalf("configured workspace management = %q", graph.Workspaces[0].Management)
	}
	if graph.Workspaces[1].Management != runtimegraph.ManagementObserved {
		t.Fatalf("unconfigured workspace management = %q", graph.Workspaces[1].Management)
	}
}

func TestSnapshotBoundsRuntimeDiscovery(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client, err := NewClient(Options{
		Discoverer: blockingDiscoverer{}, Store: db, OperationTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Snapshot(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("snapshot error = %v", err)
	}
}

func testGraph(names ...string) *runtimegraph.Graph {
	graph := &runtimegraph.Graph{
		Revision: 1,
		Machines: []*runtimegraph.Machine{{ID: "machine-local", Name: "local", Kind: runtimegraph.MachineLocal}},
		BackendInstances: []*runtimegraph.BackendInstance{{
			ID: "backend-zellij", MachineID: "machine-local", Kind: runtimegraph.BackendZellij,
			Version: "0.45.1", Capabilities: []string{"sessions", "panes"},
		}},
	}
	for index, name := range names {
		workspaceID := stableID("workspace", "backend-zellij", name)
		viewID := stableID("view", "backend-zellij", name, "tab-1")
		terminalID := stableID("terminal", "backend-zellij", name, "terminal_1")
		surfaceID := stableID("surface", "backend-zellij", name, "terminal_1")
		graph.Workspaces = append(graph.Workspaces, &runtimegraph.Workspace{
			ID: workspaceID, BackendInstanceID: "backend-zellij",
			NativeID: name, Name: name, Management: runtimegraph.ManagementManaged,
		})
		graph.Views = append(graph.Views, &runtimegraph.View{
			ID: viewID, WorkspaceID: workspaceID, NativeID: "tab-1", Name: "main", Position: index,
			SurfaceIDs: []string{surfaceID}, Active: true,
		})
		graph.Surfaces = append(graph.Surfaces, &runtimegraph.Surface{
			ID: surfaceID, ViewID: viewID, Kind: runtimegraph.SurfaceTerminal,
			NativeID: "session/" + name + "/terminal/terminal_1", TerminalID: terminalID, Title: "shell", Focused: true,
		})
		graph.Terminals = append(graph.Terminals, &runtimegraph.Terminal{
			ID: terminalID, BackendInstanceID: "backend-zellij", NativeID: "session/" + name + "/terminal/terminal_1",
			Name: "shell", State: runtimegraph.TerminalLive, Process: "zsh", CWD: "/tmp",
		})
	}
	return graph
}
