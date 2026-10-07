//go:build integration

package zellij_test

import (
	"os"
	"strings"
	"testing"

	"github.com/mark1708/tmh-next/internal/backend/zellij"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

func TestLiveDiscoveryOfManagedSession(t *testing.T) {
	if os.Getenv("TMH_ZELLIJ_INTEGRATION") != "1" {
		t.Skip("set TMH_ZELLIJ_INTEGRATION=1 to run against a disposable Zellij session")
	}
	session := os.Getenv("TMH_ZELLIJ_SESSION")
	if session == "" {
		t.Fatal("TMH_ZELLIJ_SESSION must name the disposable session")
	}
	binary := os.Getenv("TMH_ZELLIJ_BIN")
	if binary == "" {
		binary = "zellij"
	}

	graph, err := zellij.New(zellij.ExecRunner{Binary: binary}, zellij.Options{
		ManagedPrefix: "tmh-", MinimumVersion: "0.45.1",
	}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("live adapter returned invalid graph: %v", err)
	}

	workspace := liveWorkspace(graph, session)
	if workspace == nil {
		t.Fatalf("session %q not present in discovered workspaces", session)
	}
	if workspace.Management != runtimegraph.ManagementManaged {
		t.Fatalf("session %q management = %q, want managed", session, workspace.Management)
	}
	viewCount := 0
	for _, view := range graph.Views {
		if view.WorkspaceID == workspace.ID {
			viewCount++
		}
	}
	terminals := graph.TerminalsForWorkspace(workspace.ID)
	if viewCount == 0 || len(terminals) == 0 {
		t.Fatalf("session %q discovered views=%d terminals=%d", session, viewCount, len(terminals))
	}
	prefix := "session/" + session + "/terminal/"
	for _, terminal := range terminals {
		if !strings.HasPrefix(terminal.NativeID, prefix) {
			t.Fatalf("terminal native id %q lacks session scope %q", terminal.NativeID, prefix)
		}
	}
}

func liveWorkspace(graph *runtimegraph.Graph, name string) *runtimegraph.Workspace {
	for _, workspace := range graph.Workspaces {
		if workspace.Name == name {
			return workspace
		}
	}
	return nil
}
