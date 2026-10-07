package production

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

const productionScenario domain.Scenario = "production"

func defaultConfig() domain.Config {
	return domain.Config{
		DefaultPage: "dashboard", LeaderDisplay: domain.LeaderPath,
		DefaultBackend: "backend-zellij", HistoryMode: domain.HistoryPersistent,
		RetentionHours: 168, Overflow: domain.OverflowArchiveOldest,
		RedactSecrets: true, RemoteTrust: domain.TrustLocalOnly,
	}
}

func defaultPerformance() domain.Performance {
	return domain.Performance{
		Available: true,
		Targets: domain.PerfTargets{
			InputP99MS: 15, RedrawP99MS: 20, ActionP99MS: 50,
		},
	}
}

// ProjectRuntime converts one normalized runtime observation into the domain
// catalog consumed by the control panel while retaining durable product data.
func ProjectRuntime(previous *domain.Catalog, graph *runtimegraph.Graph, desired map[string]domain.WorkspaceState, now time.Time) (*domain.Catalog, error) {
	if graph == nil {
		return nil, fmt.Errorf("project runtime: graph is nil")
	}
	if err := graph.Validate(); err != nil {
		return nil, fmt.Errorf("project runtime: %w", err)
	}
	catalog := &domain.Catalog{
		Scenario:    productionScenario,
		Now:         now.UTC(),
		Config:      defaultConfig(),
		Performance: defaultPerformance(),
	}
	if previous != nil {
		catalog.Revision = previous.Revision
		catalog.MutationSerial = previous.MutationSerial
		catalog.Config = previous.Config
		catalog.Events = cloneEvents(previous.Events)
		catalog.Snapshots = cloneSnapshots(previous.Snapshots)
		catalog.RestorePlans = clonePlans(previous.RestorePlans)
		catalog.Performance = clonePerformance(previous.Performance)
		catalog.LastExport = cloneExport(previous.LastExport)
	}
	// Live discovery itself is the production performance service. Older
	// persisted catalogs predate these defaults, so repair them on projection
	// without discarding recorded traces or benchmarks.
	catalog.Performance.Available = true
	if catalog.Performance.Targets == (domain.PerfTargets{}) {
		catalog.Performance.Targets = defaultPerformance().Targets
	}

	projectMachines(catalog, previous, graph, now)
	projectBackends(catalog, previous, graph, now)
	projectWorkspaces(catalog, previous, graph, desired, now)
	projectAgents(catalog, previous, graph, now)
	retainHistoricalResources(catalog, previous)
	projectActive(catalog, previous, graph, now)
	retainValidDurableData(catalog, previous)
	if err := domain.ValidateCatalog(catalog); err != nil {
		return nil, fmt.Errorf("projected catalog: %w", err)
	}
	return catalog, nil
}

func projectMachines(catalog, previous *domain.Catalog, graph *runtimegraph.Graph, now time.Time) {
	for _, machine := range graph.Machines {
		item := &domain.Machine{
			ID: machine.ID, Name: machine.Name, Host: machine.Host, Kind: string(machine.Kind),
			Status: domain.MachineOnline, Heartbeat: now.UTC(),
			LastResult: domain.ProbeResult{At: now.UTC(), OK: true},
		}
		for _, backend := range graph.BackendInstances {
			if backend.MachineID == machine.ID {
				item.Instances++
			}
		}
		if previous != nil {
			if old := previous.MachineByID(item.ID); old != nil {
				item.LastResult = old.LastResult
				item.Heartbeat = old.Heartbeat
				item.LatencyMS = old.LatencyMS
			}
		}
		catalog.Machines = append(catalog.Machines, item)
	}
}

func projectBackends(catalog, previous *domain.Catalog, graph *runtimegraph.Graph, now time.Time) {
	for i, backend := range graph.BackendInstances {
		name := strings.ToUpper(string(backend.Kind[:1])) + string(backend.Kind[1:])
		item := &domain.Backend{
			ID: backend.ID, Name: name, Status: domain.BackendHealthy,
			Enabled: true, Default: i == 0,
			Capabilities: append([]string(nil), backend.Capabilities...),
			LastProbe:    domain.ProbeResult{At: now.UTC(), OK: true}, Instances: 1,
		}
		if previous != nil {
			if old := previous.BackendByID(item.ID); old != nil {
				item.Enabled = old.Enabled
				item.Default = old.Default
				item.LastProbe = old.LastProbe
			}
		}
		catalog.Backends = append(catalog.Backends, item)
	}
	if len(catalog.Backends) == 0 {
		catalog.Backends = []*domain.Backend{{
			ID: "backend-zellij", Name: "Zellij", Status: domain.BackendUnavailable,
			Enabled: true, Default: true, UnsupportedReasons: []string{"runtime discovery unavailable"},
		}}
	}
	defaults := 0
	for _, backend := range catalog.Backends {
		if backend.Default {
			defaults++
		}
	}
	if defaults != 1 {
		for i, backend := range catalog.Backends {
			backend.Default = i == 0
			if backend.Default {
				backend.Enabled = true
			}
		}
	}
}

func projectWorkspaces(catalog, previous *domain.Catalog, graph *runtimegraph.Graph, desired map[string]domain.WorkspaceState, now time.Time) {
	viewsByWorkspace := make(map[string][]*runtimegraph.View)
	for _, view := range graph.Views {
		viewsByWorkspace[view.WorkspaceID] = append(viewsByWorkspace[view.WorkspaceID], view)
	}
	surfacesByView := make(map[string][]*runtimegraph.Surface)
	for _, surface := range graph.Surfaces {
		if surface.Kind == runtimegraph.SurfaceTerminal {
			surfacesByView[surface.ViewID] = append(surfacesByView[surface.ViewID], surface)
		}
	}
	terminalsByID := make(map[string]*runtimegraph.Terminal, len(graph.Terminals))
	for _, terminal := range graph.Terminals {
		terminalsByID[terminal.ID] = terminal
	}

	seenNames := make(map[string]bool, len(graph.Workspaces))
	for _, workspace := range graph.Workspaces {
		seenNames[workspace.Name] = true
		views := viewsByWorkspace[workspace.ID]
		sort.SliceStable(views, func(i, j int) bool { return views[i].Position < views[j].Position })
		observed := domain.WorkspaceState{Windows: make([]domain.WindowSpec, 0, len(views))}
		terminalIDs := make([]string, 0)
		selectedSurface := ""
		for _, view := range views {
			window := domain.WindowSpec{ID: "window:" + view.Name, Index: view.Position + 1, Layout: view.Layout}
			surfaces := surfacesByView[view.ID]
			sort.SliceStable(surfaces, func(i, j int) bool {
				if surfaces[i].Geometry.Y != surfaces[j].Geometry.Y {
					return surfaces[i].Geometry.Y < surfaces[j].Geometry.Y
				}
				return surfaces[i].Geometry.X < surfaces[j].Geometry.X
			})
			for i, surface := range surfaces {
				terminal := terminalsByID[surface.TerminalID]
				if terminal == nil {
					continue
				}
				surfaceID := "surface:" + view.Name + ":primary"
				if i > 0 {
					surfaceID = "surface:" + view.Name + ":" + terminal.ID
				}
				window.Surfaces = append(window.Surfaces, domain.SurfaceSpec{
					ID: surfaceID, Title: surface.Title, CWD: terminal.CWD, Split: splitFor(surface.Geometry, i),
				})
				state := domain.TerminalLive
				if terminal.State == runtimegraph.TerminalExited {
					state = domain.TerminalExited
				}
				createdAt, lastIO := now.UTC(), now.UTC()
				if previous != nil {
					if old := previous.TerminalByID(terminal.ID); old != nil {
						createdAt, lastIO = old.CreatedAt, old.LastIO
					}
				}
				catalog.Terminals = append(catalog.Terminals, &domain.Terminal{
					ID: terminal.ID, Name: terminal.Name, WorkspaceID: workspace.ID,
					SurfaceID: surfaceID, BackendID: terminal.BackendInstanceID,
					State: state, Process: terminal.Process, CWD: terminal.CWD,
					Capabilities: append([]string(nil), terminal.Capabilities...), LastIO: lastIO, CreatedAt: createdAt,
				})
				terminalIDs = append(terminalIDs, terminal.ID)
				if surface.Focused {
					selectedSurface = surfaceID
				}
			}
			observed.Windows = append(observed.Windows, window)
		}
		desiredState := cloneState(observed)
		if configured, ok := desired[workspace.Name]; ok {
			desiredState = alignDesired(configured, observed)
		}
		status, drift := workspaceDrift(desiredState, observed)
		item := &domain.Workspace{
			ID: workspace.ID, Name: workspace.Name, Desired: desiredState, Observed: observed,
			Drift: drift, Status: status, MachineID: machineForBackend(graph, workspace.BackendInstanceID),
			BackendID: workspace.BackendInstanceID, TerminalIDs: terminalIDs,
			SelectedSurfaceID: selectedSurface,
		}
		if previous != nil {
			if old := previous.WorkspaceByID(item.ID); old != nil {
				item.LastAttached, item.LastFocused = old.LastAttached, old.LastFocused
				item.Ephemeral = old.Ephemeral
				if old.Status == domain.WorkspaceFrozen {
					item.Desired = cloneState(old.Desired)
					item.Drift = nil
					item.Status = domain.WorkspaceFrozen
				}
			}
		}
		catalog.Workspaces = append(catalog.Workspaces, item)
	}

	backendID, machineID := catalog.Backends[0].ID, catalog.Machines[0].ID
	for name, state := range desired {
		if seenNames[name] {
			continue
		}
		workspaceID := stableID("workspace", backendID, name)
		status, drift := workspaceDrift(state, domain.WorkspaceState{})
		item := &domain.Workspace{
			ID: workspaceID, Name: name, Desired: state, Status: status, Drift: drift,
			MachineID: machineID, BackendID: backendID,
		}
		if previous != nil {
			if old := previous.WorkspaceByID(workspaceID); old != nil {
				item.LastAttached, item.LastFocused, item.Ephemeral = old.LastAttached, old.LastFocused, old.Ephemeral
				if old.Status == domain.WorkspaceFrozen {
					item.Desired = cloneState(old.Desired)
					item.Drift = nil
					item.Status = domain.WorkspaceFrozen
				}
			}
		}
		catalog.Workspaces = append(catalog.Workspaces, item)
	}
	if previous != nil {
		preserved := make(map[string]bool)
		for _, snapshot := range previous.Snapshots {
			preserved[snapshot.WorkspaceID] = true
		}
		for _, plan := range previous.RestorePlans {
			preserved[plan.WorkspaceID] = true
		}
		for _, old := range previous.Workspaces {
			if !preserved[old.ID] || workspaceByID(catalog.Workspaces, old.ID) != nil {
				continue
			}
			copy := *old
			copy.Desired = cloneState(old.Desired)
			copy.Observed = domain.WorkspaceState{}
			copy.TerminalIDs = nil
			copy.AgentIDs = nil
			copy.SelectedSurfaceID = ""
			copy.Status, copy.Drift = workspaceDrift(copy.Desired, copy.Observed)
			if old.Status == domain.WorkspaceFrozen {
				copy.Status = domain.WorkspaceFrozen
			}
			catalog.Workspaces = append(catalog.Workspaces, &copy)
		}
	}
	sort.SliceStable(catalog.Workspaces, func(i, j int) bool { return catalog.Workspaces[i].Name < catalog.Workspaces[j].Name })
	sort.SliceStable(catalog.Terminals, func(i, j int) bool { return catalog.Terminals[i].ID < catalog.Terminals[j].ID })
}

func projectAgents(catalog, previous *domain.Catalog, graph *runtimegraph.Graph, now time.Time) {
	previousByTerminal := make(map[string]*domain.Agent)
	if previous != nil {
		for _, agent := range previous.Agents {
			previousByTerminal[agent.TerminalID] = agent
		}
	}
	for _, terminal := range catalog.Terminals {
		kind, ok := agentKind(terminal.Process)
		if !ok || terminal.State != domain.TerminalLive {
			continue
		}
		workspace := catalog.WorkspaceByID(terminal.WorkspaceID)
		if workspace == nil {
			continue
		}
		agent := &domain.Agent{
			ID: stableID("agent", terminal.ID), Name: terminal.Process, Kind: kind,
			State: domain.AgentRunning, WorkspaceID: workspace.ID, TerminalID: terminal.ID,
			LastUsed: now.UTC(), LastFocused: now.UTC(),
		}
		if old := previousByTerminal[terminal.ID]; old != nil {
			agent.ID, agent.State, agent.Progress = old.ID, old.State, old.Progress
			agent.ClaimedBy, agent.Timeline = old.ClaimedBy, append([]domain.AgentEntry(nil), old.Timeline...)
			agent.LastUsed, agent.LastFocused, agent.Prompt = old.LastUsed, old.LastFocused, old.Prompt
		}
		catalog.Agents = append(catalog.Agents, agent)
		workspace.AgentIDs = append(workspace.AgentIDs, agent.ID)
	}
}

func projectActive(catalog, previous *domain.Catalog, graph *runtimegraph.Graph, now time.Time) {
	viewToWorkspace := make(map[string]string, len(graph.Views))
	viewTitles := make(map[string]string, len(graph.Views))
	for _, view := range graph.Views {
		viewToWorkspace[view.ID] = view.WorkspaceID
		viewTitles[view.ID] = view.Name
	}
	previousViews := make(map[string]*domain.ActiveView)
	if previous != nil {
		for _, view := range previous.ActiveViews {
			previousViews[view.ID] = view
		}
	}
	for _, entry := range graph.ActiveEntries {
		workspaceID := viewToWorkspace[entry.ViewID]
		if workspaceID == "" {
			continue
		}
		lastUsed := entry.LastUsed
		kind, pinned, ttl := domain.ActiveShared, false, 5*60*60
		if old := previousViews[entry.ID]; old != nil {
			if lastUsed.IsZero() {
				lastUsed = old.LastUsed
			}
			kind, pinned, ttl = old.Kind, old.Pinned, old.TTLSeconds
		}
		if lastUsed.IsZero() {
			lastUsed = now.UTC()
		}
		catalog.ActiveViews = append(catalog.ActiveViews, &domain.ActiveView{
			ID: entry.ID, Title: viewTitles[entry.ViewID], Kind: kind,
			WorkspaceID: workspaceID, Owner: entry.Owner, LastUsed: lastUsed, TTLSeconds: ttl, Pinned: pinned,
		})
	}
	if previous != nil {
		seen := make(map[string]bool, len(catalog.ActiveViews))
		for _, view := range catalog.ActiveViews {
			seen[view.ID] = true
		}
		for _, old := range previous.ActiveViews {
			if old.Pinned && !seen[old.ID] && catalog.WorkspaceByID(old.WorkspaceID) != nil {
				copy := *old
				catalog.ActiveViews = append(catalog.ActiveViews, &copy)
			}
		}
	}
}

func retainValidDurableData(catalog, previous *domain.Catalog) {
	if previous == nil {
		return
	}
	terminalExists := make(map[string]bool, len(catalog.Terminals))
	for _, terminal := range catalog.Terminals {
		terminalExists[terminal.ID] = true
	}
	agentExists := make(map[string]bool, len(catalog.Agents))
	for _, agent := range catalog.Agents {
		agentExists[agent.ID] = true
	}
	for _, chunk := range previous.History {
		if (chunk.Scope.Kind == domain.KindTerminal && terminalExists[chunk.Scope.ID]) ||
			(chunk.Scope.Kind == domain.KindAgent && agentExists[chunk.Scope.ID]) {
			copy := *chunk
			copy.Lines = append([]string(nil), chunk.Lines...)
			catalog.History = append(catalog.History, &copy)
		}
	}
	workspaceExists := make(map[string]bool, len(catalog.Workspaces))

	for _, workspace := range catalog.Workspaces {
		workspaceExists[workspace.ID] = true
	}
	backendExists := make(map[string]bool, len(catalog.Backends))
	for _, backend := range catalog.Backends {
		backendExists[backend.ID] = true
	}
	keptSnapshots := catalog.Snapshots[:0]
	for _, snapshot := range catalog.Snapshots {
		if workspaceExists[snapshot.WorkspaceID] && backendExists[snapshot.BackendID] {
			keptSnapshots = append(keptSnapshots, snapshot)
		}
	}
	catalog.Snapshots = keptSnapshots
	snapshotExists := make(map[string]bool, len(catalog.Snapshots))
	for _, snapshot := range catalog.Snapshots {
		snapshotExists[snapshot.ID] = true
	}
	keptPlans := catalog.RestorePlans[:0]
	for _, plan := range catalog.RestorePlans {
		if snapshotExists[plan.SnapshotID] && workspaceExists[plan.WorkspaceID] {
			keptPlans = append(keptPlans, plan)
		}
	}
	catalog.RestorePlans = keptPlans
}
func retainHistoricalResources(catalog, previous *domain.Catalog) {
	if previous == nil {
		return
	}
	neededTerminals := make(map[string]bool)
	neededAgents := make(map[string]bool)
	for _, chunk := range previous.History {
		switch chunk.Scope.Kind {
		case domain.KindTerminal:
			neededTerminals[chunk.Scope.ID] = true
		case domain.KindAgent:
			neededAgents[chunk.Scope.ID] = true
		}
	}
	for _, old := range previous.Terminals {
		if !neededTerminals[old.ID] || catalog.TerminalByID(old.ID) != nil {
			continue
		}
		workspace := catalog.WorkspaceByID(old.WorkspaceID)
		if workspace == nil {
			if priorWorkspace := previous.WorkspaceByID(old.WorkspaceID); priorWorkspace != nil &&
				catalog.BackendByID(priorWorkspace.BackendID) != nil && catalog.MachineByID(priorWorkspace.MachineID) != nil {
				copy := *priorWorkspace
				copy.Desired = cloneState(priorWorkspace.Desired)
				copy.Observed = cloneState(priorWorkspace.Observed)
				copy.TerminalIDs = nil
				copy.AgentIDs = nil
				catalog.Workspaces = append(catalog.Workspaces, &copy)
				workspace = &copy
			}
		}
		if workspace == nil {
			continue
		}
		copy := *old
		copy.State = domain.TerminalExited
		copy.SurfaceID = ""
		copy.Capabilities = append([]string(nil), old.Capabilities...)
		catalog.Terminals = append(catalog.Terminals, &copy)
		workspace.TerminalIDs = append(workspace.TerminalIDs, copy.ID)
	}
	for _, old := range previous.Agents {
		if !neededAgents[old.ID] || catalog.AgentByID(old.ID) != nil {
			continue
		}
		workspace := catalog.WorkspaceByID(old.WorkspaceID)
		if workspace == nil {
			continue
		}
		copy := *old
		copy.State = domain.AgentDone
		copy.TerminalID = ""
		copy.Timeline = append([]domain.AgentEntry(nil), old.Timeline...)
		catalog.Agents = append(catalog.Agents, &copy)
		workspace.AgentIDs = append(workspace.AgentIDs, copy.ID)
	}
}

func alignDesired(configured, observed domain.WorkspaceState) domain.WorkspaceState {
	aligned := cloneState(configured)
	observedByID := make(map[string]domain.WindowSpec, len(observed.Windows))
	for _, window := range observed.Windows {
		observedByID[window.ID] = window
	}
	for i := range aligned.Windows {
		window := &aligned.Windows[i]
		observedWindow, ok := observedByID[window.ID]
		if !ok && i < len(observed.Windows) {
			observedWindow = observed.Windows[i]
			ok = true
		}
		if !ok {
			continue
		}
		window.ID = observedWindow.ID
		window.Layout = observedWindow.Layout
		for j := range window.Surfaces {
			if j >= len(observedWindow.Surfaces) {
				continue
			}
			window.Surfaces[j].ID = observedWindow.Surfaces[j].ID
			window.Surfaces[j].Title = observedWindow.Surfaces[j].Title
			window.Surfaces[j].Split = observedWindow.Surfaces[j].Split
		}
	}
	return aligned
}

func workspaceDrift(desired, observed domain.WorkspaceState) (domain.WorkspaceStatus, []domain.DriftEntry) {
	if desired.Equal(observed) {
		return domain.WorkspaceOK, nil
	}
	return domain.WorkspaceDrift, []domain.DriftEntry{{
		Field:    "workspace.state",
		Desired:  fmt.Sprintf("%d windows/%d surfaces", len(desired.Windows), desired.SurfaceCount()),
		Observed: fmt.Sprintf("%d windows/%d surfaces", len(observed.Windows), observed.SurfaceCount()),
	}}
}

func cloneState(s domain.WorkspaceState) domain.WorkspaceState {
	out := domain.WorkspaceState{Windows: make([]domain.WindowSpec, len(s.Windows))}
	for i, window := range s.Windows {
		out.Windows[i] = window
		out.Windows[i].Surfaces = append([]domain.SurfaceSpec(nil), window.Surfaces...)
	}
	return out
}
func workspaceByID(workspaces []*domain.Workspace, id string) *domain.Workspace {
	for _, workspace := range workspaces {
		if workspace.ID == id {
			return workspace
		}
	}
	return nil
}

func machineForBackend(graph *runtimegraph.Graph, backendID string) string {
	for _, backend := range graph.BackendInstances {
		if backend.ID == backendID {
			return backend.MachineID
		}
	}
	return "machine-local"
}

func splitFor(geometry runtimegraph.Geometry, index int) string {
	if index == 0 {
		return "none"
	}
	if geometry.Columns >= geometry.Rows {
		return "vertical"
	}
	return "horizontal"
}

func agentKind(process string) (domain.AgentKind, bool) {
	name := strings.ToLower(filepathBase(process))
	switch {
	case strings.Contains(name, "claude"):
		return domain.AgentClaude, true
	case strings.Contains(name, "codex"):
		return domain.AgentCodex, true
	case strings.Contains(name, "opencode"):
		return domain.AgentOpenCode, true
	default:
		return "", false
	}
}

func filepathBase(path string) string {
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		return path[index+1:]
	}
	return path
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

func cloneEvents(in []*domain.Event) []*domain.Event {
	if in == nil {
		return nil
	}
	out := make([]*domain.Event, 0, len(in))
	for _, item := range in {
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func cloneSnapshots(in []*domain.Snapshot) []*domain.Snapshot {
	if in == nil {
		return nil
	}
	out := make([]*domain.Snapshot, 0, len(in))
	for _, item := range in {
		copy := *item
		copy.Tags = append([]string(nil), item.Tags...)
		copy.State = cloneState(item.State)
		out = append(out, &copy)
	}
	return out
}

func clonePlans(in []*domain.RestorePlan) []*domain.RestorePlan {
	if in == nil {
		return nil
	}
	out := make([]*domain.RestorePlan, 0, len(in))
	for _, item := range in {
		copy := *item
		copy.Ops = append([]domain.RestoreOp(nil), item.Ops...)
		copy.UndoOps = append([]domain.RestoreOp(nil), item.UndoOps...)
		out = append(out, &copy)
	}
	return out
}

func clonePerformance(in domain.Performance) domain.Performance {
	in.Samples = append([]domain.PerfSample(nil), in.Samples...)
	in.Traces = append([]domain.PerfTrace(nil), in.Traces...)
	in.Benchmarks = append([]domain.PerfBenchmark(nil), in.Benchmarks...)
	return in
}

func cloneExport(in *domain.ExportDescriptor) *domain.ExportDescriptor {
	if in == nil {
		return nil
	}
	copy := *in
	copy.Scopes = append([]string(nil), in.Scopes...)
	return &copy
}
