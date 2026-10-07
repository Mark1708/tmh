// Package zellij implements the read-only Zellij runtime adapter. It talks to
// Zellij exclusively through argv-safe CLI invocations and projects the
// observed sessions, tabs and panes into the normalized runtime graph.
package zellij

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

type Runner interface {
	Run(context.Context, ...string) ([]byte, error)
}

type Options struct {
	MachineID         string
	BackendInstanceID string
	ManagedPrefix     string
	MinimumVersion    string
}

type Adapter struct {
	runner Runner
	opts   Options
}

func New(runner Runner, opts Options) *Adapter {
	if opts.MachineID == "" {
		opts.MachineID = "machine-local"
	}
	if opts.BackendInstanceID == "" {
		opts.BackendInstanceID = "backend-zellij"
	}
	if opts.ManagedPrefix == "" {
		opts.ManagedPrefix = "tmh-"
	}
	if opts.MinimumVersion == "" {
		opts.MinimumVersion = "0.45.1"
	}
	return &Adapter{runner: runner, opts: opts}
}

func (a *Adapter) Discover(ctx context.Context) (*runtimegraph.Graph, error) {
	if a == nil || a.runner == nil {
		return nil, fmt.Errorf("zellij discover: runner is nil")
	}
	version, err := a.probe(ctx)
	if err != nil {
		return nil, err
	}
	rawSessions, err := a.runner.Run(ctx, "list-sessions", "--short", "--no-formatting")
	if err != nil {
		if !strings.Contains(strings.ToLower(string(rawSessions)+" "+err.Error()), "no active zellij sessions found") {
			return nil, fmt.Errorf("list sessions: %w", err)
		}
		rawSessions = nil
	}
	sessions := sessionNames(rawSessions)

	graph := &runtimegraph.Graph{
		Machines: []*runtimegraph.Machine{{
			ID: a.opts.MachineID, Name: "local", Kind: runtimegraph.MachineLocal,
		}},
		BackendInstances: []*runtimegraph.BackendInstance{{
			ID: a.opts.BackendInstanceID, MachineID: a.opts.MachineID,
			Kind: runtimegraph.BackendZellij, Version: version,
			Capabilities: []string{
				"workspace.list", "view.list", "surface.list", "terminal.inspect", "client.list", "attach",
			},
		}},
	}
	for _, session := range sessions {
		if err := a.discoverSession(ctx, graph, session); err != nil {
			return nil, fmt.Errorf("zellij session %q: %w", session, err)
		}
	}
	if err := graph.Validate(); err != nil {
		return nil, fmt.Errorf("zellij graph: %w", err)
	}
	return graph, nil
}

func (a *Adapter) discoverSession(ctx context.Context, graph *runtimegraph.Graph, session string) error {
	tabsRaw, err := a.run(ctx, "list tabs", "--session", session, "action", "list-tabs", "--all", "--json")
	if err != nil {
		return err
	}
	panesRaw, err := a.run(ctx, "list panes", "--session", session, "action", "list-panes", "--all", "--json")
	if err != nil {
		return err
	}
	clientsRaw, err := a.run(ctx, "list clients", "--session", session, "action", "list-clients")
	if err != nil {
		return err
	}

	var tabs []tabDTO
	if err := json.Unmarshal(tabsRaw, &tabs); err != nil {
		return fmt.Errorf("decode tabs: %w", err)
	}
	var panes []paneDTO
	if err := json.Unmarshal(panesRaw, &panes); err != nil {
		return fmt.Errorf("decode panes: %w", err)
	}

	workspaceID := stableID("workspace", a.opts.BackendInstanceID, session)
	management := runtimegraph.ManagementObserved
	if strings.HasPrefix(session, a.opts.ManagedPrefix) {
		management = runtimegraph.ManagementManaged
	}
	graph.Workspaces = append(graph.Workspaces, &runtimegraph.Workspace{
		ID: workspaceID, BackendInstanceID: a.opts.BackendInstanceID,
		Name: session, NativeID: session, Management: management,
	})

	viewByTab := make(map[int]*runtimegraph.View, len(tabs))
	for i, tab := range tabs {
		if tab.TabID == nil || tab.Position == nil || tab.Name == nil {
			return fmt.Errorf("tab %d lacks required id, position or name", i)
		}
		if _, exists := viewByTab[*tab.TabID]; exists {
			return fmt.Errorf("duplicate tab id %d", *tab.TabID)
		}
		view := &runtimegraph.View{
			ID:          stableID("view", a.opts.BackendInstanceID, session, strconv.Itoa(*tab.TabID)),
			WorkspaceID: workspaceID, Name: *tab.Name, Position: *tab.Position,
			NativeID: strconv.Itoa(*tab.TabID), Active: tab.Active,
		}
		if tab.ActiveSwapLayoutName != nil {
			view.Layout = *tab.ActiveSwapLayoutName
		}
		viewByTab[*tab.TabID] = view
		graph.Views = append(graph.Views, view)
	}

	viewByPane := make(map[string]*runtimegraph.View, len(panes))
	for i, pane := range panes {
		if pane.ID == nil || pane.IsPlugin == nil || pane.TabID == nil || pane.Title == nil || pane.Exited == nil {
			return fmt.Errorf("pane %d lacks required id, type, tab, title or state", i)
		}
		view := viewByTab[*pane.TabID]
		if view == nil {
			return fmt.Errorf("pane %d references unknown tab %d", *pane.ID, *pane.TabID)
		}
		prefix := "terminal"
		kind := runtimegraph.SurfaceTerminal
		if *pane.IsPlugin {
			prefix = "plugin"
			kind = runtimegraph.SurfacePlugin
		}
		paneID := prefix + "_" + strconv.Itoa(*pane.ID)
		nativeID := "session/" + session + "/" + prefix + "/" + paneID
		surface := &runtimegraph.Surface{
			ID:     stableID("surface", a.opts.BackendInstanceID, session, paneID),
			ViewID: view.ID, Kind: kind, NativeID: nativeID, Title: *pane.Title,
			Geometry: runtimegraph.Geometry{
				X: valueOrZero(pane.X), Y: valueOrZero(pane.Y), Rows: valueOrZero(pane.Rows), Columns: valueOrZero(pane.Columns),
			},
			Focused: valueOrFalse(pane.Focused), Floating: valueOrFalse(pane.Floating),
		}
		if kind == runtimegraph.SurfaceTerminal {
			terminalID := stableID("terminal", a.opts.BackendInstanceID, session, paneID)
			surface.TerminalID = terminalID
			state := runtimegraph.TerminalLive
			if *pane.Exited {
				state = runtimegraph.TerminalExited
			}
			terminal := &runtimegraph.Terminal{
				ID: terminalID, BackendInstanceID: a.opts.BackendInstanceID,
				NativeID: nativeID, Name: *pane.Title, State: state,
				Process: pane.Command, CWD: pane.CWD, ExitStatus: pane.ExitStatus,
				Capabilities: []string{"attach", "screen.dump_best_effort"},
			}
			graph.Terminals = append(graph.Terminals, terminal)
		}
		view.SurfaceIDs = append(view.SurfaceIDs, surface.ID)
		graph.Surfaces = append(graph.Surfaces, surface)
		viewByPane[paneID] = view
	}

	for _, client := range parseClients(string(clientsRaw)) {
		view := viewByPane[client.paneID]
		if view == nil {
			continue
		}
		graph.ActiveEntries = append(graph.ActiveEntries, &runtimegraph.ActiveEntry{
			ID:     stableID("active", a.opts.BackendInstanceID, session, client.id),
			ViewID: view.ID, Kind: runtimegraph.ActiveClient, ClientID: client.id,
		})
	}
	return nil
}

func (a *Adapter) probe(ctx context.Context) (string, error) {
	raw, err := a.run(ctx, "version probe", "--version")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 || fields[0] != "zellij" {
		return "", fmt.Errorf("zellij version probe: unexpected output %q", strings.TrimSpace(string(raw)))
	}
	if compareVersion(fields[1], a.opts.MinimumVersion) < 0 {
		return "", fmt.Errorf("requires zellij >= %s, found %s", a.opts.MinimumVersion, fields[1])
	}
	return fields[1], nil
}

func (a *Adapter) run(ctx context.Context, operation string, args ...string) ([]byte, error) {
	out, err := a.runner.Run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return out, nil
}

type tabDTO struct {
	TabID                *int    `json:"tab_id"`
	Position             *int    `json:"position"`
	Name                 *string `json:"name"`
	Active               bool    `json:"active"`
	ActiveSwapLayoutName *string `json:"active_swap_layout_name"`
}

type paneDTO struct {
	ID         *int    `json:"id"`
	IsPlugin   *bool   `json:"is_plugin"`
	Focused    *bool   `json:"is_focused"`
	Floating   *bool   `json:"is_floating"`
	Title      *string `json:"title"`
	Exited     *bool   `json:"exited"`
	ExitStatus *int    `json:"exit_status"`
	X          *int    `json:"pane_x"`
	Y          *int    `json:"pane_y"`
	Rows       *int    `json:"pane_rows"`
	Columns    *int    `json:"pane_columns"`
	TabID      *int    `json:"tab_id"`
	Command    string  `json:"pane_command"`
	CWD        string  `json:"pane_cwd"`
}

type clientDTO struct {
	id     string
	paneID string
}

func parseClients(raw string) []clientDTO {
	lines := strings.Split(raw, "\n")
	clients := make([]clientDTO, 0, max(0, len(lines)-1))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "CLIENT_ID" {
			continue
		}
		clients = append(clients, clientDTO{id: fields[0], paneID: fields[1]})
	}
	return clients
}

func sessionNames(raw []byte) []string {
	lines := strings.Split(string(raw), "\n")
	names := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func stableID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	sum := hash.Sum(nil)
	return prefix + "-" + hex.EncodeToString(sum[:8])
}

func compareVersion(left, right string) int {
	l, lok := parseVersion(left)
	r, rok := parseVersion(right)
	if !lok || !rok {
		return strings.Compare(left, right)
	}
	for i := range l {
		if l[i] < r[i] {
			return -1
		}
		if l[i] > r[i] {
			return 1
		}
	}
	return 0
}

func parseVersion(version string) ([3]int, bool) {
	var parsed [3]int
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), "-", 2)
	segments := strings.Split(parts[0], ".")
	if len(segments) != len(parsed) {
		return parsed, false
	}
	for i, segment := range segments {
		value, err := strconv.Atoi(segment)
		if err != nil || value < 0 {
			return parsed, false
		}
		parsed[i] = value
	}
	return parsed, true
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func valueOrFalse(value *bool) bool {
	return value != nil && *value
}
