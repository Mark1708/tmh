package domain

import (
	"sort"
	"strings"
)

// ValidateCatalog checks the invariants every accepted catalog must satisfy:
// unique IDs, resolvable cross-references, allowed enum values, resource
// ownership, restore-op targets and configuration invariants. It returns the
// first violation as a typed validation error.
func ValidateCatalog(c *Catalog) error {
	if c == nil {
		return Fail(CodeValidation, "catalog is nil")
	}

	// --- ID uniqueness -------------------------------------------------
	ids := map[string]string{}
	unique := func(kind, id string) error {
		if id == "" {
			return Fail(CodeValidation, "%s with empty id", kind)
		}
		key := kind + "/" + id
		if prev, dup := ids[key]; dup {
			return Fail(CodeValidation, "duplicate %s id %q (also %s)", kind, id, prev)
		}
		ids[key] = id
		return nil
	}
	for _, w := range c.Workspaces {
		if err := unique("workspace", w.ID); err != nil {
			return err
		}
	}
	for _, v := range c.ActiveViews {
		if err := unique("active", v.ID); err != nil {
			return err
		}
	}
	for _, t := range c.Terminals {
		if err := unique("terminal", t.ID); err != nil {
			return err
		}
	}
	for _, a := range c.Agents {
		if err := unique("agent", a.ID); err != nil {
			return err
		}
	}
	for _, h := range c.History {
		if err := unique("history", h.ID); err != nil {
			return err
		}
	}
	for _, e := range c.Events {
		if err := unique("event", e.ID); err != nil {
			return err
		}
	}
	for _, s := range c.Snapshots {
		if err := unique("snapshot", s.ID); err != nil {
			return err
		}
	}
	for _, p := range c.RestorePlans {
		if err := unique("plan", p.ID); err != nil {
			return err
		}
	}
	for _, b := range c.Backends {
		if err := unique("backend", b.ID); err != nil {
			return err
		}
	}
	for _, m := range c.Machines {
		if err := unique("machine", m.ID); err != nil {
			return err
		}
	}

	// --- index ---------------------------------------------------------
	machines := map[string]bool{}
	for _, m := range c.Machines {
		machines[m.ID] = true
	}
	backends := map[string]bool{}
	defaultBackends := 0
	for _, b := range c.Backends {
		backends[b.ID] = true
		if b.Default {
			defaultBackends++
		}
		if err := validBackendStatus(b); err != nil {
			return err
		}
	}
	if defaultBackends != 1 {
		return Fail(CodeValidation, "exactly one default backend required, found %d", defaultBackends)
	}
	terminals := map[string]*Terminal{}
	for _, t := range c.Terminals {
		terminals[t.ID] = t
		if t.State != TerminalLive && t.State != TerminalExited {
			return Fail(CodeValidation, "terminal %s has unknown state %q", t.ID, t.State)
		}
	}
	agents := map[string]*Agent{}
	for _, a := range c.Agents {
		agents[a.ID] = a
		if err := validAgent(a); err != nil {
			return err
		}
	}
	workspaces := map[string]*Workspace{}
	for _, w := range c.Workspaces {
		workspaces[w.ID] = w
		if err := validWorkspace(w, machines, backends, terminals, agents); err != nil {
			return err
		}
	}
	snapshots := map[string]*Snapshot{}
	for _, s := range c.Snapshots {
		snapshots[s.ID] = s
		if _, ok := workspaces[s.WorkspaceID]; !ok {
			return Fail(CodeValidation, "snapshot %s references unknown workspace %s", s.ID, s.WorkspaceID)
		}
		if !backends[s.BackendID] {
			return Fail(CodeValidation, "snapshot %s references unknown backend %s", s.ID, s.BackendID)
		}
	}

	// --- terminals & agents back-links ---------------------------------
	for _, t := range c.Terminals {
		w, ok := workspaces[t.WorkspaceID]
		if !ok {
			return Fail(CodeValidation, "terminal %s references unknown workspace %s", t.ID, t.WorkspaceID)
		}
		if !containsID(w.TerminalIDs, t.ID) {
			return Fail(CodeValidation, "workspace %s does not list terminal %s", w.ID, t.ID)
		}
		if !backends[t.BackendID] {
			return Fail(CodeValidation, "terminal %s references unknown backend %s", t.ID, t.BackendID)
		}
		if t.SurfaceID != "" && !w.Observed.hasSurface(t.SurfaceID) && !w.Desired.hasSurface(t.SurfaceID) {
			return Fail(CodeValidation, "terminal %s references unknown surface %s", t.ID, t.SurfaceID)
		}
	}
	for _, a := range c.Agents {
		w, ok := workspaces[a.WorkspaceID]
		if !ok {
			return Fail(CodeValidation, "agent %s references unknown workspace %s", a.ID, a.WorkspaceID)
		}
		if !containsID(w.AgentIDs, a.ID) {
			return Fail(CodeValidation, "workspace %s does not list agent %s", w.ID, a.ID)
		}
		if a.TerminalID != "" {
			t, ok := terminals[a.TerminalID]
			if !ok {
				return Fail(CodeValidation, "agent %s references unknown terminal %s", a.ID, a.TerminalID)
			}
			if t.State != TerminalLive {
				return Fail(CodeValidation, "agent %s attached to non-live terminal %s", a.ID, a.TerminalID)
			}
		}
	}

	// --- active views ----------------------------------------------------
	for _, v := range c.ActiveViews {
		if _, ok := workspaces[v.WorkspaceID]; !ok {
			return Fail(CodeValidation, "active view %s references unknown workspace %s", v.ID, v.WorkspaceID)
		}
		switch v.Kind {
		case ActiveShared, ActiveRecent, ActivePinned:
		default:
			return Fail(CodeValidation, "active view %s has unknown kind %q", v.ID, v.Kind)
		}
		if v.Pinned && v.Kind != ActivePinned {
			return Fail(CodeValidation, "active view %s pinned but kind %q", v.ID, v.Kind)
		}
	}

	// --- history & events ------------------------------------------------
	for _, h := range c.History {
		if h.Scope.Kind != KindTerminal && h.Scope.Kind != KindAgent {
			return Fail(CodeValidation, "history %s has scope kind %q", h.ID, h.Scope.Kind)
		}
		if h.Scope.Kind == KindTerminal && terminals[h.Scope.ID] == nil {
			return Fail(CodeValidation, "history %s references unknown terminal %s", h.ID, h.Scope.ID)
		}
		if h.Scope.Kind == KindAgent && agents[h.Scope.ID] == nil {
			return Fail(CodeValidation, "history %s references unknown agent %s", h.ID, h.Scope.ID)
		}
	}
	lastSeq := 0
	for _, e := range c.Events {
		if e.Seq <= lastSeq {
			return Fail(CodeValidation, "event %s seq %d not increasing", e.ID, e.Seq)
		}
		lastSeq = e.Seq
		if !ValidResourceKind(e.Resource.Kind) {
			return Fail(CodeValidation, "event %s has unknown resource kind %q", e.ID, e.Resource.Kind)
		}
		switch e.Severity {
		case SeverityInfo, SeverityWarn, SeverityError:
		default:
			return Fail(CodeValidation, "event %s has unknown severity %q", e.ID, e.Severity)
		}
		switch e.Category {
		case EventWorkspace, EventTerminal, EventAgent, EventSnapshot, EventRestore,
			EventBackend, EventMachine, EventConfig, EventSystem:
		default:
			return Fail(CodeValidation, "event %s has unknown category %q", e.ID, e.Category)
		}
	}

	// --- restore plans -----------------------------------------------------
	for _, p := range c.RestorePlans {
		s, ok := snapshots[p.SnapshotID]
		if !ok {
			return Fail(CodeValidation, "plan %s references unknown snapshot %s", p.ID, p.SnapshotID)
		}
		if s.WorkspaceID != p.WorkspaceID {
			return Fail(CodeValidation, "plan %s workspace %s differs from snapshot workspace %s", p.ID, p.WorkspaceID, s.WorkspaceID)
		}
		if _, ok := workspaces[p.WorkspaceID]; !ok {
			return Fail(CodeValidation, "plan %s references unknown workspace %s", p.ID, p.WorkspaceID)
		}
		switch p.Status {
		case PlanPending, PlanApplied, PlanUndone:
		default:
			return Fail(CodeValidation, "plan %s has unknown status %q", p.ID, p.Status)
		}
		if err := validRestoreOps(p); err != nil {
			return err
		}
	}

	// --- machines -----------------------------------------------------------
	for _, m := range c.Machines {
		if m.Status != MachineOnline && m.Status != MachineOffline {
			return Fail(CodeValidation, "machine %s has unknown status %q", m.ID, m.Status)
		}
		if m.Kind != "local" && m.Kind != "remote" {
			return Fail(CodeValidation, "machine %s has unknown kind %q", m.ID, m.Kind)
		}
	}

	// --- performance & config ------------------------------------------------
	if len(c.Performance.Samples) > MaxPerfSamples {
		return Fail(CodeValidation, "performance series exceeds bound of %d", MaxPerfSamples)
	}
	if err := c.Config.Validate(); err != nil {
		return err
	}

	return nil
}

// MaxPerfSamples bounds the retained performance series.
const MaxPerfSamples = 120

func containsID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (s WorkspaceState) hasSurface(id string) bool {
	for _, w := range s.Windows {
		for _, sf := range w.Surfaces {
			if sf.ID == id {
				return true
			}
		}
	}
	return false
}

func validWorkspace(w *Workspace, machines, backends map[string]bool, terminals map[string]*Terminal, agents map[string]*Agent) error {
	if !machines[w.MachineID] {
		return Fail(CodeValidation, "workspace %s references unknown machine %s", w.ID, w.MachineID)
	}
	if !backends[w.BackendID] {
		return Fail(CodeValidation, "workspace %s references unknown backend %s", w.ID, w.BackendID)
	}
	switch w.Status {
	case WorkspaceOK, WorkspaceDrift, WorkspaceFrozen:
	default:
		return Fail(CodeValidation, "workspace %s has unknown status %q", w.ID, w.Status)
	}
	for _, id := range w.TerminalIDs {
		t, ok := terminals[id]
		if !ok {
			return Fail(CodeValidation, "workspace %s lists unknown terminal %s", w.ID, id)
		}
		if t.WorkspaceID != w.ID {
			return Fail(CodeValidation, "terminal %s back-links to %s, not %s", id, t.WorkspaceID, w.ID)
		}
	}
	for _, id := range w.AgentIDs {
		a, ok := agents[id]
		if !ok {
			return Fail(CodeValidation, "workspace %s lists unknown agent %s", w.ID, id)
		}
		if a.WorkspaceID != w.ID {
			return Fail(CodeValidation, "agent %s back-links to %s, not %s", id, a.WorkspaceID, w.ID)
		}
	}
	driftExpected := !w.Desired.Equal(w.Observed)
	if driftExpected && len(w.Drift) == 0 && w.Status != WorkspaceFrozen {
		return Fail(CodeValidation, "workspace %s states differ but drift is empty", w.ID)
	}
	if !driftExpected && len(w.Drift) != 0 {
		return Fail(CodeValidation, "workspace %s states equal but drift entries exist", w.ID)
	}
	if w.SelectedSurfaceID != "" && !w.Observed.hasSurface(w.SelectedSurfaceID) {
		return Fail(CodeValidation, "workspace %s selects unknown surface %s", w.ID, w.SelectedSurfaceID)
	}
	return nil
}

func validAgent(a *Agent) error {
	switch a.Kind {
	case AgentClaude, AgentCodex, AgentOpenCode, AgentCustom:
	default:
		return Fail(CodeValidation, "agent %s has unknown kind %q", a.ID, a.Kind)
	}
	switch a.State {
	case AgentRunning, AgentNeedsInput, AgentWaiting, AgentDone:
	default:
		return Fail(CodeValidation, "agent %s has unknown state %q", a.ID, a.State)
	}
	if a.Progress < 0 || a.Progress > 99 {
		return Fail(CodeValidation, "agent %s progress %d out of 0..99", a.ID, a.Progress)
	}
	return nil
}

func validBackendStatus(b *Backend) error {
	switch b.Status {
	case BackendHealthy, BackendPreview, BackendUnavailable:
	default:
		return Fail(CodeValidation, "backend %s has unknown status %q", b.ID, b.Status)
	}
	if b.Default && !b.Enabled {
		return Fail(CodeValidation, "default backend %s cannot be disabled", b.ID)
	}
	return nil
}

func validRestoreOps(p *RestorePlan) error {
	for i, op := range p.Ops {
		switch op.Mode {
		case RestorePush, RestorePull, RestoreFreeze:
		default:
			return Fail(CodeValidation, "plan %s op %s has unknown mode %q", p.ID, op.ID, op.Mode)
		}
		switch op.Mode {
		case RestorePush, RestorePull:
			if op.Target.Kind != KindWorkspace || op.Target.ID != p.WorkspaceID {
				return Fail(CodeValidation, "plan %s op %s target must be the planned workspace", p.ID, op.ID)
			}
		case RestoreFreeze:
			if op.Target.Kind != KindSnapshot {
				return Fail(CodeValidation, "plan %s freeze op %s must target snapshots", p.ID, op.ID)
			}
		}
		if op.Field == "" {
			return Fail(CodeValidation, "plan %s op %s has empty field", p.ID, op.ID)
		}
		if i > 0 {
			prev := p.Ops[i-1]
			if restoreOpLess(prev, op) >= 0 {
				return Fail(CodeValidation, "plan %s ops not sorted by (mode,target,field,id) at %s", p.ID, op.ID)
			}
		}
	}
	return nil
}

// restoreOpLess compares ops by the fixed plan ordering
// (Mode, Target.Kind, Target.ID, Field, ID).
func restoreOpLess(a, b RestoreOp) int {
	if c := strings.Compare(string(a.Mode), string(b.Mode)); c != 0 {
		return c
	}
	if c := strings.Compare(string(a.Target.Kind), string(b.Target.Kind)); c != 0 {
		return c
	}
	if c := strings.Compare(a.Target.ID, b.Target.ID); c != 0 {
		return c
	}
	if c := strings.Compare(a.Field, b.Field); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

// SortRestoreOps sorts ops into the canonical plan order.
func SortRestoreOps(ops []RestoreOp) {
	sort.SliceStable(ops, func(i, j int) bool {
		return restoreOpLess(ops[i], ops[j]) < 0
	})
}
