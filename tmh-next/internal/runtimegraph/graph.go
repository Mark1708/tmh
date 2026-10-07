// Package runtimegraph defines the normalized production resource graph shared
// by runtime adapters and the control plane. Native runtime identifiers are
// bindings only; tmh resource IDs remain stable across adapter restarts.
package runtimegraph

import (
	"fmt"
	"time"
)

type MachineKind string

const (
	MachineLocal  MachineKind = "local"
	MachineRemote MachineKind = "remote"
)

type BackendKind string

const (
	BackendZellij BackendKind = "zellij"
	BackendTmux   BackendKind = "tmux"
	BackendNative BackendKind = "native"
)

type ManagementMode string

const (
	ManagementObserved ManagementMode = "observed"
	ManagementAdopted  ManagementMode = "adopted"
	ManagementManaged  ManagementMode = "managed"
)

type SurfaceKind string

const (
	SurfaceTerminal SurfaceKind = "terminal"
	SurfacePlugin   SurfaceKind = "plugin"
)

type TerminalState string

const (
	TerminalLive   TerminalState = "live"
	TerminalExited TerminalState = "exited"
)

type ActiveKind string

const (
	ActiveClient ActiveKind = "client"
	ActiveRecent ActiveKind = "recent"
	ActivePinned ActiveKind = "pinned"
	ActiveShared ActiveKind = "shared"
)

type Machine struct {
	ID   string      `json:"id"`
	Name string      `json:"name"`
	Kind MachineKind `json:"kind"`
	Host string      `json:"host,omitempty"`
}

type BackendInstance struct {
	ID           string      `json:"id"`
	MachineID    string      `json:"machine_id"`
	Kind         BackendKind `json:"kind"`
	Version      string      `json:"version"`
	Capabilities []string    `json:"capabilities,omitempty"`
	ObservedAt   time.Time   `json:"observed_at,omitempty"`
}

type Workspace struct {
	ID                string         `json:"id"`
	BackendInstanceID string         `json:"backend_instance_id"`
	Name              string         `json:"name"`
	Management        ManagementMode `json:"management"`
	NativeID          string         `json:"native_id,omitempty"`
	LastSeenAt        time.Time      `json:"last_seen_at,omitempty"`
}

type View struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspace_id"`
	Name        string   `json:"name"`
	Position    int      `json:"position"`
	Layout      string   `json:"layout,omitempty"`
	NativeID    string   `json:"native_id,omitempty"`
	Active      bool     `json:"active"`
	SurfaceIDs  []string `json:"surface_ids,omitempty"`
}

type Geometry struct {
	X       int `json:"x"`
	Y       int `json:"y"`
	Rows    int `json:"rows"`
	Columns int `json:"columns"`
}

type Surface struct {
	ID         string      `json:"id"`
	ViewID     string      `json:"view_id"`
	Kind       SurfaceKind `json:"kind"`
	TerminalID string      `json:"terminal_id,omitempty"`
	NativeID   string      `json:"native_id"`
	Title      string      `json:"title,omitempty"`
	Geometry   Geometry    `json:"geometry"`
	Focused    bool        `json:"focused"`
	Floating   bool        `json:"floating"`
	Pinned     bool        `json:"pinned"`
}

type Terminal struct {
	ID                string        `json:"id"`
	BackendInstanceID string        `json:"backend_instance_id"`
	NativeID          string        `json:"native_id"`
	Name              string        `json:"name"`
	State             TerminalState `json:"state"`
	Process           string        `json:"process,omitempty"`
	CWD               string        `json:"cwd,omitempty"`
	ExitStatus        *int          `json:"exit_status,omitempty"`
	Capabilities      []string      `json:"capabilities,omitempty"`
}

type Agent struct {
	ID         string `json:"id"`
	TerminalID string `json:"terminal_id,omitempty"`
	ViewID     string `json:"view_id,omitempty"`
	Name       string `json:"name"`
	Kind       string `json:"kind,omitempty"`
	State      string `json:"state,omitempty"`
}

type ActiveEntry struct {
	ID       string     `json:"id"`
	ViewID   string     `json:"view_id"`
	Kind     ActiveKind `json:"kind"`
	ClientID string     `json:"client_id,omitempty"`
	Owner    string     `json:"owner,omitempty"`
	LastUsed time.Time  `json:"last_used,omitempty"`
}

type Graph struct {
	Revision         uint64             `json:"revision"`
	Machines         []*Machine         `json:"machines"`
	BackendInstances []*BackendInstance `json:"backend_instances"`
	Workspaces       []*Workspace       `json:"workspaces"`
	Views            []*View            `json:"views"`
	Surfaces         []*Surface         `json:"surfaces"`
	Terminals        []*Terminal        `json:"terminals"`
	Agents           []*Agent           `json:"agents"`
	ActiveEntries    []*ActiveEntry     `json:"active_entries"`
}

type ValidationError struct {
	Resource string
	ID       string
	Message  string
}

func (e *ValidationError) Error() string {
	if e.ID == "" {
		return fmt.Sprintf("%s: %s", e.Resource, e.Message)
	}
	return fmt.Sprintf("%s %q: %s", e.Resource, e.ID, e.Message)
}

func invalid(resource, id, format string, args ...any) error {
	return &ValidationError{Resource: resource, ID: id, Message: fmt.Sprintf(format, args...)}
}

func (g *Graph) Validate() error {
	if g == nil {
		return invalid("graph", "", "is nil")
	}

	machines := make(map[string]*Machine, len(g.Machines))
	for _, machine := range g.Machines {
		if machine == nil {
			return invalid("machine", "", "is nil")
		}
		if err := addUnique(machines, machine.ID, machine, "machine"); err != nil {
			return err
		}
		if machine.Kind != MachineLocal && machine.Kind != MachineRemote {
			return invalid("machine", machine.ID, "unknown kind %q", machine.Kind)
		}
	}

	backends := make(map[string]*BackendInstance, len(g.BackendInstances))
	for _, backend := range g.BackendInstances {
		if backend == nil {
			return invalid("backend instance", "", "is nil")
		}
		if err := addUnique(backends, backend.ID, backend, "backend instance"); err != nil {
			return err
		}
		if machines[backend.MachineID] == nil {
			return invalid("backend instance", backend.ID, "references unknown machine %q", backend.MachineID)
		}
		switch backend.Kind {
		case BackendZellij, BackendTmux, BackendNative:
		default:
			return invalid("backend instance", backend.ID, "unknown kind %q", backend.Kind)
		}
	}

	workspaces := make(map[string]*Workspace, len(g.Workspaces))
	for _, workspace := range g.Workspaces {
		if workspace == nil {
			return invalid("workspace", "", "is nil")
		}
		if err := addUnique(workspaces, workspace.ID, workspace, "workspace"); err != nil {
			return err
		}
		if backends[workspace.BackendInstanceID] == nil {
			return invalid("workspace", workspace.ID, "references unknown backend instance %q", workspace.BackendInstanceID)
		}
		switch workspace.Management {
		case ManagementObserved, ManagementAdopted, ManagementManaged:
		default:
			return invalid("workspace", workspace.ID, "unknown management mode %q", workspace.Management)
		}
	}

	views := make(map[string]*View, len(g.Views))
	for _, view := range g.Views {
		if view == nil {
			return invalid("view", "", "is nil")
		}
		if err := addUnique(views, view.ID, view, "view"); err != nil {
			return err
		}
		if workspaces[view.WorkspaceID] == nil {
			return invalid("view", view.ID, "references unknown workspace %q", view.WorkspaceID)
		}
	}

	terminals := make(map[string]*Terminal, len(g.Terminals))
	nativeTerminals := make(map[string]string, len(g.Terminals))
	for _, terminal := range g.Terminals {
		if terminal == nil {
			return invalid("terminal", "", "is nil")
		}
		if err := addUnique(terminals, terminal.ID, terminal, "terminal"); err != nil {
			return err
		}
		if backends[terminal.BackendInstanceID] == nil {
			return invalid("terminal", terminal.ID, "references unknown backend instance %q", terminal.BackendInstanceID)
		}
		if terminal.NativeID == "" {
			return invalid("terminal", terminal.ID, "has empty native id")
		}
		nativeKey := terminal.BackendInstanceID + "\x00" + terminal.NativeID
		if previous := nativeTerminals[nativeKey]; previous != "" {
			return invalid("terminal", terminal.ID, "native id %q already belongs to %q", terminal.NativeID, previous)
		}
		nativeTerminals[nativeKey] = terminal.ID
		if terminal.State != TerminalLive && terminal.State != TerminalExited {
			return invalid("terminal", terminal.ID, "unknown state %q", terminal.State)
		}
	}

	surfaces := make(map[string]*Surface, len(g.Surfaces))
	surfaceByTerminal := make(map[string]*Surface, len(g.Surfaces))
	for _, surface := range g.Surfaces {
		if surface == nil {
			return invalid("surface", "", "is nil")
		}
		if err := addUnique(surfaces, surface.ID, surface, "surface"); err != nil {
			return err
		}
		view := views[surface.ViewID]
		if view == nil {
			return invalid("surface", surface.ID, "references unknown view %q", surface.ViewID)
		}
		switch surface.Kind {
		case SurfaceTerminal:
			terminal := terminals[surface.TerminalID]
			if terminal == nil {
				return invalid("surface", surface.ID, "references unknown terminal %q", surface.TerminalID)
			}
			workspace := workspaces[view.WorkspaceID]
			if terminal.BackendInstanceID != workspace.BackendInstanceID {
				return invalid("surface", surface.ID, "terminal and workspace belong to different backend instances")
			}
			if previous := surfaceByTerminal[surface.TerminalID]; previous != nil {
				return invalid("surface", surface.ID, "terminal %q is already placed by surface %q", surface.TerminalID, previous.ID)
			}
			surfaceByTerminal[surface.TerminalID] = surface
		case SurfacePlugin:
			if surface.TerminalID != "" {
				return invalid("surface", surface.ID, "plugin surface cannot reference terminal %q", surface.TerminalID)
			}
		default:
			return invalid("surface", surface.ID, "unknown kind %q", surface.Kind)
		}
	}
	for id := range terminals {
		if surfaceByTerminal[id] == nil {
			return invalid("terminal", id, "has no surface placement")
		}
	}

	for _, view := range g.Views {
		seen := make(map[string]struct{}, len(view.SurfaceIDs))
		for _, surfaceID := range view.SurfaceIDs {
			if _, duplicate := seen[surfaceID]; duplicate {
				return invalid("view", view.ID, "lists surface %q more than once", surfaceID)
			}
			seen[surfaceID] = struct{}{}
			surface := surfaces[surfaceID]
			if surface == nil {
				return invalid("view", view.ID, "lists unknown surface %q", surfaceID)
			}
			if surface.ViewID != view.ID {
				return invalid("view", view.ID, "surface %q belongs to view %q", surfaceID, surface.ViewID)
			}
		}
	}

	agents := make(map[string]*Agent, len(g.Agents))
	for _, agent := range g.Agents {
		if agent == nil {
			return invalid("agent", "", "is nil")
		}
		if err := addUnique(agents, agent.ID, agent, "agent"); err != nil {
			return err
		}
		if agent.TerminalID == "" && agent.ViewID == "" {
			return invalid("agent", agent.ID, "requires terminal or view context")
		}
		if agent.TerminalID != "" && terminals[agent.TerminalID] == nil {
			return invalid("agent", agent.ID, "references unknown terminal %q", agent.TerminalID)
		}
		if agent.ViewID != "" && views[agent.ViewID] == nil {
			return invalid("agent", agent.ID, "references unknown view %q", agent.ViewID)
		}
	}

	activeEntries := make(map[string]*ActiveEntry, len(g.ActiveEntries))
	for _, entry := range g.ActiveEntries {
		if entry == nil {
			return invalid("active entry", "", "is nil")
		}
		if err := addUnique(activeEntries, entry.ID, entry, "active entry"); err != nil {
			return err
		}
		if views[entry.ViewID] == nil {
			return invalid("active entry", entry.ID, "references unknown view %q", entry.ViewID)
		}
		switch entry.Kind {
		case ActiveClient, ActiveRecent, ActivePinned, ActiveShared:
		default:
			return invalid("active entry", entry.ID, "unknown kind %q", entry.Kind)
		}
	}

	return nil
}

func addUnique[T any](items map[string]T, id string, item T, resource string) error {
	if id == "" {
		return invalid(resource, "", "has empty id")
	}
	if _, exists := items[id]; exists {
		return invalid(resource, id, "duplicate id")
	}
	items[id] = item
	return nil
}

func (g *Graph) WorkspaceForTerminal(terminalID string) *Workspace {
	if g == nil {
		return nil
	}
	for _, surface := range g.Surfaces {
		if surface == nil || surface.TerminalID != terminalID {
			continue
		}
		view := g.ViewByID(surface.ViewID)
		if view != nil {
			return g.WorkspaceByID(view.WorkspaceID)
		}
	}
	return nil
}

func (g *Graph) WorkspaceForAgent(agentID string) *Workspace {
	if g == nil {
		return nil
	}
	for _, agent := range g.Agents {
		if agent == nil || agent.ID != agentID {
			continue
		}
		if agent.TerminalID != "" {
			return g.WorkspaceForTerminal(agent.TerminalID)
		}
		if view := g.ViewByID(agent.ViewID); view != nil {
			return g.WorkspaceByID(view.WorkspaceID)
		}
		return nil
	}
	return nil
}

func (g *Graph) TerminalsForWorkspace(workspaceID string) []*Terminal {
	if g == nil {
		return nil
	}
	terminalIDs := make(map[string]struct{})
	for _, surface := range g.Surfaces {
		if surface == nil || surface.TerminalID == "" {
			continue
		}
		view := g.ViewByID(surface.ViewID)
		if view != nil && view.WorkspaceID == workspaceID {
			terminalIDs[surface.TerminalID] = struct{}{}
		}
	}
	terminals := make([]*Terminal, 0, len(terminalIDs))
	for _, terminal := range g.Terminals {
		if terminal != nil {
			if _, ok := terminalIDs[terminal.ID]; ok {
				terminals = append(terminals, terminal)
			}
		}
	}
	return terminals
}

func (g *Graph) WorkspaceByID(id string) *Workspace {
	if g == nil {
		return nil
	}
	for _, workspace := range g.Workspaces {
		if workspace != nil && workspace.ID == id {
			return workspace
		}
	}
	return nil
}

func (g *Graph) ViewByID(id string) *View {
	if g == nil {
		return nil
	}
	for _, view := range g.Views {
		if view != nil && view.ID == id {
			return view
		}
	}
	return nil
}
