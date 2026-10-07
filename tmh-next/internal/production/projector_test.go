package production

import (
	"testing"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

func TestProjectRuntimeBuildsValidatedLiveCatalog(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	graph := &runtimegraph.Graph{
		Revision: 7,
		Machines: []*runtimegraph.Machine{{ID: "machine-local", Name: "local", Kind: runtimegraph.MachineLocal}},
		BackendInstances: []*runtimegraph.BackendInstance{{
			ID: "backend-zellij", MachineID: "machine-local", Kind: runtimegraph.BackendZellij,
			Version: "0.45.1", Capabilities: []string{"attach", "terminal.send", "terminal.close"},
		}},
		Workspaces: []*runtimegraph.Workspace{{
			ID: "workspace-base", BackendInstanceID: "backend-zellij", Name: "base",
			NativeID: "base", Management: runtimegraph.ManagementManaged,
		}},
		Views: []*runtimegraph.View{{
			ID: "view-root", WorkspaceID: "workspace-base", Name: "root", Position: 0,
			NativeID: "0", Active: true, SurfaceIDs: []string{"surface-shell"},
		}},
		Surfaces: []*runtimegraph.Surface{{
			ID: "surface-shell", ViewID: "view-root", Kind: runtimegraph.SurfaceTerminal,
			TerminalID: "terminal-shell", NativeID: "session/base/terminal/terminal_1",
			Title: "shell", Focused: true,
		}},
		Terminals: []*runtimegraph.Terminal{{
			ID: "terminal-shell", BackendInstanceID: "backend-zellij",
			NativeID: "session/base/terminal/terminal_1", Name: "shell",
			State: runtimegraph.TerminalLive, Process: "zsh", CWD: "/Users/mark",
			Capabilities: []string{"attach", "terminal.send", "terminal.close"},
		}},
		ActiveEntries: []*runtimegraph.ActiveEntry{{
			ID: "active-client", ViewID: "view-root", Kind: runtimegraph.ActiveClient,
			ClientID: "1", LastUsed: now,
		}},
	}
	desired := map[string]domain.WorkspaceState{
		"base": {Windows: []domain.WindowSpec{{
			ID: "window:root", Index: 1,
			Surfaces: []domain.SurfaceSpec{{ID: "surface:root:1", CWD: "/Users/mark", Split: "none"}},
		}}},
	}

	catalog, err := ProjectRuntime(nil, graph, desired, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateCatalog(catalog); err != nil {
		t.Fatalf("projected catalog invalid: %v", err)
	}
	if catalog.Scenario != domain.Scenario("production") {
		t.Fatalf("scenario = %q", catalog.Scenario)
	}
	workspace := catalog.WorkspaceByID("workspace-base")
	if workspace == nil || workspace.Status != domain.WorkspaceOK || len(workspace.TerminalIDs) != 1 {
		t.Fatalf("workspace = %+v", workspace)
	}
	terminal := catalog.TerminalByID("terminal-shell")
	if terminal == nil || terminal.CWD != "/Users/mark" || terminal.State != domain.TerminalLive {
		t.Fatalf("terminal = %+v", terminal)
	}
	if len(catalog.ActiveViews) != 1 || catalog.ActiveViews[0].WorkspaceID != workspace.ID {
		t.Fatalf("active views = %+v", catalog.ActiveViews)
	}
	if len(catalog.Backends) != 1 || catalog.Backends[0].Name != "Zellij" {
		t.Fatalf("backends = %+v", catalog.Backends)
	}
	if !catalog.Performance.Available ||
		catalog.Performance.Targets != (domain.PerfTargets{InputP99MS: 15, RedrawP99MS: 20, ActionP99MS: 50}) {
		t.Fatalf("performance service = %+v", catalog.Performance)
	}

	catalog.History = []*domain.HistoryChunk{{
		ID: "history-1", Scope: domain.HistoryScope{Kind: domain.KindTerminal, ID: terminal.ID},
		Seq: 1, At: now, Lines: []string{"prompt"}, Kind: "stream",
	}}
	graph.Surfaces = nil
	graph.Terminals = nil
	graph.Views[0].SurfaceIDs = nil
	closed, err := ProjectRuntime(catalog, graph, desired, now.Add(time.Second))
	if err != nil {
		t.Fatalf("project after pane close: %v", err)
	}
	terminal = closed.TerminalByID("terminal-shell")
	if terminal == nil || terminal.State != domain.TerminalExited || terminal.SurfaceID != "" {
		t.Fatalf("closed terminal = %+v", terminal)
	}
}

func TestProjectRuntimePreservesDurableProductState(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	previous := &domain.Catalog{
		Revision: 12, Scenario: domain.Scenario("production"), Now: now.Add(-time.Minute),
		Workspaces: []*domain.Workspace{{
			ID: "gone", Name: "gone", Status: domain.WorkspaceOK,
			MachineID: "machine-local", BackendID: "backend-zellij",
		}},
		Snapshots: []*domain.Snapshot{{
			ID: "snapshot-1", WorkspaceID: "gone", BackendID: "backend-zellij",
		}},
		Events: []*domain.Event{{
			ID: "event-1", Seq: 1, At: now.Add(-time.Minute), Category: domain.EventSystem,
			Severity: domain.SeverityInfo, Resource: domain.Ref(domain.KindConfig, "config"), Message: "saved",
		}},
		Backends: []*domain.Backend{{
			ID: "backend-zellij", Name: "Zellij", Status: domain.BackendHealthy,
			Enabled: true, Default: true,
		}},
		Machines: []*domain.Machine{{
			ID: "machine-local", Name: "local", Kind: "local", Status: domain.MachineOnline,
		}},
		Config: defaultConfig(),
	}
	graph := &runtimegraph.Graph{
		Machines:         []*runtimegraph.Machine{{ID: "machine-local", Name: "local", Kind: runtimegraph.MachineLocal}},
		BackendInstances: []*runtimegraph.BackendInstance{{ID: "backend-zellij", MachineID: "machine-local", Kind: runtimegraph.BackendZellij, Version: "0.45.1"}},
	}
	catalog, err := ProjectRuntime(previous, graph, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Performance.Available ||
		catalog.Performance.Targets != defaultPerformance().Targets {
		t.Fatalf("persisted catalog did not acquire performance defaults: %+v", catalog.Performance)
	}
	if catalog.Revision != previous.Revision || len(catalog.Snapshots) != 1 || len(catalog.Events) != 1 {
		t.Fatalf("durable state was not preserved: %+v", catalog)
	}
}
