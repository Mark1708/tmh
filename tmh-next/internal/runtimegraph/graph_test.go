package runtimegraph_test

import (
	"testing"

	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

func validGraph() *runtimegraph.Graph {
	return &runtimegraph.Graph{
		Revision: 7,
		Machines: []*runtimegraph.Machine{{
			ID: "machine-local", Name: "local", Kind: runtimegraph.MachineLocal,
		}},
		BackendInstances: []*runtimegraph.BackendInstance{{
			ID: "backend-zellij", MachineID: "machine-local", Kind: runtimegraph.BackendZellij,
			Version: "0.45.1", Capabilities: []string{"workspace.list", "attach"},
		}},
		Workspaces: []*runtimegraph.Workspace{{
			ID: "workspace-alpha", BackendInstanceID: "backend-zellij", Name: "alpha",
			Management: runtimegraph.ManagementManaged,
		}},
		Views: []*runtimegraph.View{{
			ID: "view-editor", WorkspaceID: "workspace-alpha", Name: "editor", Position: 0,
		}},
		Surfaces: []*runtimegraph.Surface{{
			ID: "surface-shell", ViewID: "view-editor", Kind: runtimegraph.SurfaceTerminal,
			TerminalID: "terminal-shell", NativeID: "terminal_1",
		}, {
			ID: "surface-plugin", ViewID: "view-editor", Kind: runtimegraph.SurfacePlugin,
			NativeID: "plugin_1",
		}},
		Terminals: []*runtimegraph.Terminal{{
			ID: "terminal-shell", BackendInstanceID: "backend-zellij", NativeID: "terminal_1",
			Name: "shell", State: runtimegraph.TerminalLive,
		}},
		Agents: []*runtimegraph.Agent{{
			ID: "agent-one", TerminalID: "terminal-shell", Name: "worker",
		}},
		ActiveEntries: []*runtimegraph.ActiveEntry{{
			ID: "active-one", ViewID: "view-editor", Kind: runtimegraph.ActiveClient,
		}},
	}
}

func TestGraphValidatesNormalizedRelationships(t *testing.T) {
	g := validGraph()
	if err := g.Validate(); err != nil {
		t.Fatalf("valid graph rejected: %v", err)
	}

	workspace := g.WorkspaceForTerminal("terminal-shell")
	if workspace == nil || workspace.ID != "workspace-alpha" {
		t.Fatalf("workspace lookup = %#v", workspace)
	}
	terminals := g.TerminalsForWorkspace("workspace-alpha")
	if len(terminals) != 1 || terminals[0].ID != "terminal-shell" {
		t.Fatalf("workspace terminals = %#v", terminals)
	}
	if got := g.WorkspaceForAgent("agent-one"); got == nil || got.ID != "workspace-alpha" {
		t.Fatalf("agent workspace = %#v", got)
	}
}

func TestGraphRejectsTerminalOwnedByPluginSurface(t *testing.T) {
	g := validGraph()
	g.Surfaces[1].TerminalID = "terminal-shell"
	if err := g.Validate(); err == nil {
		t.Fatal("plugin surface with terminal binding accepted")
	}
}

func TestGraphRejectsDanglingActiveView(t *testing.T) {
	g := validGraph()
	g.ActiveEntries[0].ViewID = "view-missing"
	if err := g.Validate(); err == nil {
		t.Fatal("dangling active view accepted")
	}
}

func TestGraphRejectsNativeIdentityCollisionInsideBackend(t *testing.T) {
	g := validGraph()
	g.Terminals = append(g.Terminals, &runtimegraph.Terminal{
		ID: "terminal-copy", BackendInstanceID: "backend-zellij", NativeID: "terminal_1",
		Name: "copy", State: runtimegraph.TerminalLive,
	})
	if err := g.Validate(); err == nil {
		t.Fatal("duplicate backend native terminal id accepted")
	}
}
