package zellij_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mark1708/tmh-next/internal/backend/zellij"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

type fakeResult struct {
	out string
	err error
}

type fakeRunner struct {
	results map[string]fakeResult
	calls   [][]string
}

func commandKey(args ...string) string { return strings.Join(args, "\x00") }

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	result, ok := f.results[commandKey(args...)]
	if !ok {
		return nil, errors.New("unexpected command: " + strings.Join(args, " "))
	}
	return []byte(result.out), result.err
}

func discoveryRunner() *fakeRunner {
	f := &fakeRunner{results: make(map[string]fakeResult)}
	f.results[commandKey("--version")] = fakeResult{out: "zellij 0.45.1\n"}
	f.results[commandKey("list-sessions", "--short", "--no-formatting")] = fakeResult{out: "tmh-alpha\nexternal\n"}

	addSession := func(name, tabs, panes, clients string) {
		f.results[commandKey("--session", name, "action", "list-tabs", "--all", "--json")] = fakeResult{out: tabs}
		f.results[commandKey("--session", name, "action", "list-panes", "--all", "--json")] = fakeResult{out: panes}
		f.results[commandKey("--session", name, "action", "list-clients")] = fakeResult{out: clients}
	}
	addSession("tmh-alpha",
		`[{"tab_id":0,"position":0,"name":"editor","active":true,"active_swap_layout_name":"default"}]`,
		`[
			{"id":1,"is_plugin":false,"is_focused":true,"is_floating":false,"title":"shell","exited":false,"exit_status":null,"pane_x":0,"pane_y":1,"pane_rows":23,"pane_columns":100,"tab_id":0,"pane_command":"zsh","pane_cwd":"/work/alpha"},
			{"id":1,"is_plugin":true,"is_focused":false,"is_floating":false,"title":"status","exited":false,"pane_x":0,"pane_y":0,"pane_rows":1,"pane_columns":100,"tab_id":0,"plugin_url":"zellij:status-bar"}
		]`,
		"CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND\n7 terminal_1 zsh\n",
	)
	addSession("external",
		`[{"tab_id":4,"position":0,"name":"ops","active":true}]`,
		`[{"id":8,"is_plugin":false,"is_focused":true,"is_floating":true,"title":"ops","exited":true,"exit_status":2,"pane_x":2,"pane_y":3,"pane_rows":20,"pane_columns":80,"tab_id":4,"pane_command":"bash","pane_cwd":"/srv"}]`,
		"CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND\n",
	)
	return f
}

func newAdapter(r zellij.Runner) *zellij.Adapter {
	return zellij.New(r, zellij.Options{
		MachineID: "machine-local", BackendInstanceID: "backend-zellij",
		ManagedPrefix: "tmh-", MinimumVersion: "0.45.1",
	})
}

func TestDiscoverBuildsNormalizedGraph(t *testing.T) {
	runner := discoveryRunner()
	graph, err := newAdapter(runner).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("adapter produced invalid graph: %v", err)
	}
	if len(graph.Workspaces) != 2 || len(graph.Views) != 2 || len(graph.Surfaces) != 3 || len(graph.Terminals) != 2 {
		t.Fatalf("unexpected graph sizes: workspaces=%d views=%d surfaces=%d terminals=%d",
			len(graph.Workspaces), len(graph.Views), len(graph.Surfaces), len(graph.Terminals))
	}

	managed := workspaceByName(graph, "tmh-alpha")
	if managed == nil || managed.Management != runtimegraph.ManagementManaged {
		t.Fatalf("managed workspace = %#v", managed)
	}
	observed := workspaceByName(graph, "external")
	if observed == nil || observed.Management != runtimegraph.ManagementObserved {
		t.Fatalf("observed workspace = %#v", observed)
	}

	if got := graph.TerminalsForWorkspace(managed.ID); len(got) != 1 || got[0].Process != "zsh" {
		t.Fatalf("managed terminals = %#v", got)
	}
	pluginCount := 0
	for _, surface := range graph.Surfaces {
		if surface.Kind == runtimegraph.SurfacePlugin {
			pluginCount++
			if surface.TerminalID != "" || surface.NativeID != "plugin_1" {
				t.Fatalf("plugin surface = %#v", surface)
			}
		}
	}
	if pluginCount != 1 {
		t.Fatalf("plugin surfaces = %d", pluginCount)
	}
	if len(graph.ActiveEntries) != 1 || graph.ActiveEntries[0].ClientID != "7" {
		t.Fatalf("active entries = %#v", graph.ActiveEntries)
	}
}

func TestDiscoverUsesStableIDsAcrossScans(t *testing.T) {
	first, err := newAdapter(discoveryRunner()).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := newAdapter(discoveryRunner()).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resourceIDs(first), resourceIDs(second)) {
		t.Fatalf("unstable ids:\nfirst:  %#v\nsecond: %#v", resourceIDs(first), resourceIDs(second))
	}
}

func TestDiscoverRejectsMalformedSessionPayload(t *testing.T) {
	runner := discoveryRunner()
	runner.results[commandKey("--session", "external", "action", "list-panes", "--all", "--json")] = fakeResult{out: `[{"id":8,"tab_id":99}]`}
	_, err := newAdapter(runner).Discover(context.Background())
	if err == nil || !strings.Contains(err.Error(), `session "external"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestProbeRejectsUnsupportedVersion(t *testing.T) {
	runner := discoveryRunner()
	runner.results[commandKey("--version")] = fakeResult{out: "zellij 0.44.0\n"}
	_, err := newAdapter(runner).Discover(context.Background())
	if err == nil || !strings.Contains(err.Error(), "requires zellij >= 0.45.1") {
		t.Fatalf("error = %v", err)
	}
}

func workspaceByName(graph *runtimegraph.Graph, name string) *runtimegraph.Workspace {
	for _, workspace := range graph.Workspaces {
		if workspace.Name == name {
			return workspace
		}
	}
	return nil
}

func resourceIDs(graph *runtimegraph.Graph) []string {
	ids := make([]string, 0, len(graph.Workspaces)+len(graph.Views)+len(graph.Surfaces)+len(graph.Terminals))
	for _, workspace := range graph.Workspaces {
		ids = append(ids, workspace.ID)
	}
	for _, view := range graph.Views {
		ids = append(ids, view.ID)
	}
	for _, surface := range graph.Surfaces {
		ids = append(ids, surface.ID)
	}
	for _, terminal := range graph.Terminals {
		ids = append(ids, terminal.ID)
	}
	return ids
}
