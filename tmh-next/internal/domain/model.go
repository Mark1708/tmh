// Package domain defines the canonical tmh-next ontology: the catalog
// snapshot, stable IDs, resource references, actions, mutation results and
// typed errors. It depends on the standard library only.
package domain

import (
	"time"
)

// Scenario names the deterministic fixture set loaded by a backend.
type Scenario string

const (
	ScenarioDefault  Scenario = "default"
	ScenarioEmpty    Scenario = "empty"
	ScenarioDegraded Scenario = "degraded"
)

// ParseScenario validates a scenario slug.
func ParseScenario(s string) (Scenario, bool) {
	switch Scenario(s) {
	case ScenarioDefault, ScenarioEmpty, ScenarioDegraded:
		return Scenario(s), true
	default:
		return "", false
	}
}

// ResourceKind enumerates every addressable entity kind.
type ResourceKind string

const (
	KindWorkspace ResourceKind = "workspace"
	KindActive    ResourceKind = "active"
	KindTerminal  ResourceKind = "terminal"
	KindAgent     ResourceKind = "agent"
	KindHistory   ResourceKind = "history"
	KindEvent     ResourceKind = "event"
	KindSnapshot  ResourceKind = "snapshot"
	KindPlan      ResourceKind = "plan"
	KindBackend   ResourceKind = "backend"
	KindMachine   ResourceKind = "machine"
	KindConfig    ResourceKind = "config"
)

// ValidResourceKind reports whether kind is a known kind.
func ValidResourceKind(kind ResourceKind) bool {
	switch kind {
	case KindWorkspace, KindActive, KindTerminal, KindAgent, KindHistory,
		KindEvent, KindSnapshot, KindPlan, KindBackend, KindMachine, KindConfig:
		return true
	default:
		return false
	}
}

// ResourceRef is a typed (kind, id) pair used by routes, actions and events.
type ResourceRef struct {
	Kind ResourceKind `json:"kind"`
	ID   string       `json:"id"`
}

// Ref is a small constructor helper.
func Ref(kind ResourceKind, id string) ResourceRef {
	return ResourceRef{Kind: kind, ID: id}
}

// Catalog is the single canonical UI snapshot. Backends return deep copies of
// the full post-state; pages treat it as read-only.
type Catalog struct {
	Revision       uint64    `json:"revision"`
	Now            time.Time `json:"now"` // logical clock
	Scenario       Scenario  `json:"scenario"`
	MutationSerial uint64    `json:"mutation_serial"` // user mutations only

	Workspaces   []*Workspace    `json:"workspaces"`
	ActiveViews  []*ActiveView   `json:"active_views"`
	Terminals    []*Terminal     `json:"terminals"`
	Agents       []*Agent        `json:"agents"`
	History      []*HistoryChunk `json:"history"`
	Events       []*Event        `json:"events"`
	Snapshots    []*Snapshot     `json:"snapshots"`
	RestorePlans []*RestorePlan  `json:"restore_plans"`
	Backends     []*Backend      `json:"backends"`
	Machines     []*Machine      `json:"machines"`
	Performance  Performance     `json:"performance"`
	Config       Config          `json:"config"`

	// LastExport describes the most recent mock history export (no file).
	LastExport *ExportDescriptor `json:"last_export,omitempty"`
}

// WorkspaceStatus classifies workspace reconciliation state.
type WorkspaceStatus string

const (
	WorkspaceOK     WorkspaceStatus = "ok"
	WorkspaceDrift  WorkspaceStatus = "drift"
	WorkspaceFrozen WorkspaceStatus = "frozen"
)

// SurfaceSpec is one pane surface inside a window.
type SurfaceSpec struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	CWD   string `json:"cwd"`
	Split string `json:"split"` // none|vertical|horizontal
}

// WindowSpec is one window of a workspace state.
type WindowSpec struct {
	ID       string        `json:"id"`
	Index    int           `json:"index"`
	Layout   string        `json:"layout"`
	Surfaces []SurfaceSpec `json:"surfaces"`
}

// WorkspaceState is a desired or observed workspace shape.
type WorkspaceState struct {
	Windows []WindowSpec `json:"windows"`
}

// HasSurface reports whether the state contains the surface id.
func (s WorkspaceState) HasSurface(id string) bool { return s.hasSurface(id) }

// Equal reports deep equality of two workspace states.
func (s WorkspaceState) Equal(o WorkspaceState) bool {
	if len(s.Windows) != len(o.Windows) {
		return false
	}
	for i, w := range s.Windows {
		ow := o.Windows[i]
		if w.ID != ow.ID || w.Index != ow.Index || w.Layout != ow.Layout {
			return false
		}
		if len(w.Surfaces) != len(ow.Surfaces) {
			return false
		}
		for j, sf := range w.Surfaces {
			osf := ow.Surfaces[j]
			if sf != osf {
				return false
			}
		}
	}
	return true
}

// SurfaceCount returns the total number of surfaces.
func (s WorkspaceState) SurfaceCount() int {
	n := 0
	for _, w := range s.Windows {
		n += len(w.Surfaces)
	}
	return n
}

// DriftEntry describes one desired-vs-observed difference.
type DriftEntry struct {
	Field    string `json:"field"`
	Desired  string `json:"desired"`
	Observed string `json:"observed"`
}

// Workspace is a managed terminal workspace.
type Workspace struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Desired           WorkspaceState  `json:"desired"`
	Observed          WorkspaceState  `json:"observed"`
	Drift             []DriftEntry    `json:"drift,omitempty"`
	Status            WorkspaceStatus `json:"status"`
	MachineID         string          `json:"machine_id"`
	BackendID         string          `json:"backend_id"`
	TerminalIDs       []string        `json:"terminal_ids"`
	AgentIDs          []string        `json:"agent_ids"`
	LastAttached      time.Time       `json:"last_attached"`
	LastFocused       time.Time       `json:"last_focused"`
	SelectedSurfaceID string          `json:"selected_surface_id"`
	Ephemeral         bool            `json:"ephemeral"`
}

// Live reports whether the workspace has at least one live terminal.
func (w *Workspace) Live(terminals []*Terminal) bool {
	for _, id := range w.TerminalIDs {
		for _, t := range terminals {
			if t.ID == id && t.State == TerminalLive {
				return true
			}
		}
	}
	return false
}

// ActiveViewKind classifies an active view entry.
type ActiveViewKind string

const (
	ActiveShared ActiveViewKind = "shared"
	ActiveRecent ActiveViewKind = "recent"
	ActivePinned ActiveViewKind = "pinned"
)

// ActiveView is a shared/recent/pinned view card.
type ActiveView struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Kind        ActiveViewKind `json:"kind"`
	WorkspaceID string         `json:"workspace_id"`
	Owner       string         `json:"owner"`
	LastUsed    time.Time      `json:"last_used"`
	TTLSeconds  int            `json:"ttl_seconds"`
	Pinned      bool           `json:"pinned"`
}

// ExpiredAt returns the expiry moment of the view.
func (v *ActiveView) ExpiredAt(now time.Time) time.Time {
	return v.LastUsed.Add(time.Duration(v.TTLSeconds) * time.Second)
}

// TerminalState is live or exited.
type TerminalState string

const (
	TerminalLive   TerminalState = "live"
	TerminalExited TerminalState = "exited"
)

// Terminal is one managed terminal surface.
type Terminal struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	WorkspaceID  string        `json:"workspace_id"`
	SurfaceID    string        `json:"surface_id"`
	BackendID    string        `json:"backend_id"`
	State        TerminalState `json:"state"`
	Process      string        `json:"process"`
	CWD          string        `json:"cwd"`
	Capabilities []string      `json:"capabilities"`
	LastIO       time.Time     `json:"last_io"`
	CreatedAt    time.Time     `json:"created_at"`
}

// AgentKind enumerates supported agent runtimes.
type AgentKind string

const (
	AgentClaude   AgentKind = "claude"
	AgentCodex    AgentKind = "codex"
	AgentOpenCode AgentKind = "opencode"
	AgentCustom   AgentKind = "custom"
)

// AgentState is the agent lifecycle state.
type AgentState string

const (
	AgentRunning    AgentState = "running"
	AgentNeedsInput AgentState = "needs_input"
	AgentWaiting    AgentState = "waiting"
	AgentDone       AgentState = "done"
)

// AgentEntry is one timeline record inside an agent.
type AgentEntry struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // prompt|result|state
	Text string    `json:"text"`
}

// Agent is an AI agent attached (optionally) to a terminal.
type Agent struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Kind        AgentKind    `json:"kind"`
	State       AgentState   `json:"state"`
	WorkspaceID string       `json:"workspace_id"`
	TerminalID  string       `json:"terminal_id,omitempty"`
	Progress    int          `json:"progress"`
	ClaimedBy   string       `json:"claimed_by,omitempty"`
	Timeline    []AgentEntry `json:"timeline"`
	LastUsed    time.Time    `json:"last_used"`
	LastFocused time.Time    `json:"last_focused"`
	Prompt      string       `json:"prompt,omitempty"`
}

// HistoryScope is a terminal or agent stream scope.
type HistoryScope struct {
	Kind ResourceKind `json:"kind"` // terminal|agent
	ID   string       `json:"id"`
}

// HistoryChunk is one bounded chunk of terminal/agent stream history.
type HistoryChunk struct {
	ID    string       `json:"id"`
	Scope HistoryScope `json:"scope"`
	Seq   int          `json:"seq"`
	At    time.Time    `json:"at"`
	Lines []string     `json:"lines"`
	Kind  string       `json:"kind"`          // stream|export
	Gap   bool         `json:"gap,omitempty"` // gap marker before this chunk
}

// EventCategory groups audit events.
type EventCategory string

const (
	EventWorkspace EventCategory = "workspace"
	EventTerminal  EventCategory = "terminal"
	EventAgent     EventCategory = "agent"
	EventSnapshot  EventCategory = "snapshot"
	EventRestore   EventCategory = "restore"
	EventBackend   EventCategory = "backend"
	EventMachine   EventCategory = "machine"
	EventConfig    EventCategory = "config"
	EventSystem    EventCategory = "system"
)

// EventSeverity is info, warn or error.
type EventSeverity string

const (
	SeverityInfo  EventSeverity = "info"
	SeverityWarn  EventSeverity = "warn"
	SeverityError EventSeverity = "error"
)

// Event is one durable audit record.
type Event struct {
	ID       string        `json:"id"`
	Seq      int           `json:"seq"`
	At       time.Time     `json:"at"`
	Category EventCategory `json:"category"`
	Severity EventSeverity `json:"severity"`
	Resource ResourceRef   `json:"resource"`
	Message  string        `json:"message"`
	Detail   string        `json:"detail"` // compact JSON for the raw view
}

// Snapshot captures a workspace state at a moment.
type Snapshot struct {
	ID           string         `json:"id"`
	WorkspaceID  string         `json:"workspace_id"`
	CreatedAt    time.Time      `json:"created_at"`
	BackendID    string         `json:"backend_id"`
	Tags         []string       `json:"tags"`
	State        WorkspaceState `json:"state"`
	WindowCount  int            `json:"window_count"`
	SurfaceCount int            `json:"surface_count"`
}

// RestoreMode is push, pull or freeze.
type RestoreMode string

const (
	RestorePush   RestoreMode = "push"
	RestorePull   RestoreMode = "pull"
	RestoreFreeze RestoreMode = "freeze"
)

// RestorePlanStatus is pending, applied or undone.
type RestorePlanStatus string

const (
	PlanPending RestorePlanStatus = "pending"
	PlanApplied RestorePlanStatus = "applied"
	PlanUndone  RestorePlanStatus = "undone"
)

// RestoreOp is one field-level reconcile operation.
type RestoreOp struct {
	ID           string      `json:"id"`
	Mode         RestoreMode `json:"mode"`
	Target       ResourceRef `json:"target"`
	Field        string      `json:"field"`
	Before       string      `json:"before"`
	After        string      `json:"after"`
	BeforeAbsent bool        `json:"before_absent"`
	Included     bool        `json:"included"`
	AppliedAt    time.Time   `json:"applied_at,omitempty"`
}

// RestorePlan is a snapshot reconcile plan.
type RestorePlan struct {
	ID            string            `json:"id"`
	SnapshotID    string            `json:"snapshot_id"`
	WorkspaceID   string            `json:"workspace_id"`
	Status        RestorePlanStatus `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	Ops           []RestoreOp       `json:"ops"`
	AppliedAt     time.Time         `json:"applied_at,omitempty"`
	UndoneAt      time.Time         `json:"undone_at,omitempty"`
	AppliedSerial uint64            `json:"applied_serial,omitempty"`
	UndoOps       []RestoreOp       `json:"undo_ops,omitempty"`
}

// IncludedOps returns ops of the given mode that are included.
func (p *RestorePlan) IncludedOps(mode RestoreMode) []*RestoreOp {
	var out []*RestoreOp
	for i := range p.Ops {
		if p.Ops[i].Mode == mode && p.Ops[i].Included {
			out = append(out, &p.Ops[i])
		}
	}
	return out
}

// BackendStatus is the backend health classification.
type BackendStatus string

const (
	BackendHealthy     BackendStatus = "healthy"
	BackendPreview     BackendStatus = "preview"
	BackendUnavailable BackendStatus = "unavailable"
)

// ProbeResult captures the latest probe/doctor outcome.
type ProbeResult struct {
	At        time.Time `json:"at"`
	OK        bool      `json:"ok"`
	LatencyMS int       `json:"latency_ms"`
	Findings  []string  `json:"findings,omitempty"`
}

// Backend is a terminal multiplexer backend.
type Backend struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Status             BackendStatus `json:"status"`
	Enabled            bool          `json:"enabled"`
	Default            bool          `json:"default"`
	Capabilities       []string      `json:"capabilities"`
	UnsupportedReasons []string      `json:"unsupported_reasons,omitempty"`
	LastProbe          ProbeResult   `json:"last_probe"`
	Instances          int           `json:"instances"`
}

// MachineStatus is online or offline.
type MachineStatus string

const (
	MachineOnline  MachineStatus = "online"
	MachineOffline MachineStatus = "offline"
)

// Machine is a local or remote node.
type Machine struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Host       string        `json:"host"`
	Kind       string        `json:"kind"` // local|remote
	Status     MachineStatus `json:"status"`
	Instances  int           `json:"instances"`
	Heartbeat  time.Time     `json:"heartbeat"`
	LatencyMS  int           `json:"latency_ms"`
	LastResult ProbeResult   `json:"last_result"`
}

// PerfSample is one bounded performance sample.
type PerfSample struct {
	At         time.Time `json:"at"`
	InputP50   int       `json:"input_p50"`
	InputP95   int       `json:"input_p95"`
	InputP99   int       `json:"input_p99"`
	RedrawP99  int       `json:"redraw_p99"`
	ActionP99  int       `json:"action_p99"`
	QueueDepth int       `json:"queue_depth"`
}

// PerfTargets are the performance SLO targets.
type PerfTargets struct {
	InputP99MS  int `json:"input_p99_ms"`
	RedrawP99MS int `json:"redraw_p99_ms"`
	ActionP99MS int `json:"action_p99_ms"`
}

// Performance carries bounded latency series and targets.
type Performance struct {
	Available  bool            `json:"available"`
	Samples    []PerfSample    `json:"samples"`
	Targets    PerfTargets     `json:"targets"`
	Traces     []PerfTrace     `json:"traces"`
	Benchmarks []PerfBenchmark `json:"benchmarks"`
}

// PerfTrace is one recorded input/redraw trace.
type PerfTrace struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"` // input|redraw|action
	P99MS  int       `json:"p99_ms"`
	Detail string    `json:"detail"`
}

// PerfBenchmark is one benchmark result.
type PerfBenchmark struct {
	At    time.Time `json:"at"`
	Ops   int       `json:"ops"`
	AvgMS int       `json:"avg_ms"`
	P99MS int       `json:"p99_ms"`
}

// ExportDescriptor describes a mock history export.
type ExportDescriptor struct {
	At       time.Time `json:"at"`
	Rows     int       `json:"rows"`
	Scopes   []string  `json:"scopes"`
	Filename string    `json:"filename"`
}

// Lookup helpers (linear scans are fine at fixture scale).

// WorkspaceByID returns the workspace or nil.
func (c *Catalog) WorkspaceByID(id string) *Workspace {
	for _, w := range c.Workspaces {
		if w.ID == id {
			return w
		}
	}
	return nil
}

// TerminalByID returns the terminal or nil.
func (c *Catalog) TerminalByID(id string) *Terminal {
	for _, t := range c.Terminals {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// TerminalBySurface returns the terminal bound to a surface, or nil.
func (c *Catalog) TerminalBySurface(sfID string) *Terminal {
	for _, t := range c.Terminals {
		if t.SurfaceID == sfID {
			return t
		}
	}
	return nil
}

// AgentByID returns the agent or nil.
func (c *Catalog) AgentByID(id string) *Agent {
	for _, a := range c.Agents {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// ActiveViewByID returns the active view or nil.
func (c *Catalog) ActiveViewByID(id string) *ActiveView {
	for _, v := range c.ActiveViews {
		if v.ID == id {
			return v
		}
	}
	return nil
}

// SnapshotByID returns the snapshot or nil.
func (c *Catalog) SnapshotByID(id string) *Snapshot {
	for _, s := range c.Snapshots {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// RestorePlanByID returns the restore plan or nil.
func (c *Catalog) RestorePlanByID(id string) *RestorePlan {
	for _, p := range c.RestorePlans {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// HistoryMode is the history persistence mode.
type HistoryMode string

const (
	HistoryOff        HistoryMode = "off"
	HistoryMemory     HistoryMode = "memory"
	HistoryPersistent HistoryMode = "persistent"
)

// OverflowPolicy is the history overflow behavior.
type OverflowPolicy string

const (
	OverflowReject        OverflowPolicy = "reject"
	OverflowArchiveOldest OverflowPolicy = "archive-oldest"
)

// RemoteTrust is the remote connection trust policy.
type RemoteTrust string

const (
	TrustLocalOnly   RemoteTrust = "local-only"
	TrustPrompt      RemoteTrust = "prompt"
	TrustAllowlisted RemoteTrust = "allowlisted"
)

// LeaderDisplay controls the status leader rendering.
type LeaderDisplay string

const (
	LeaderIcon LeaderDisplay = "icon"
	LeaderPath LeaderDisplay = "path"
	LeaderNone LeaderDisplay = "none"
)

// Config is the in-memory application configuration. Cross-field rules are
// enforced by Validate and re-checked defensively by the backend on save.
type Config struct {
	DefaultPage         string         `json:"default_page"`
	LeaderDisplay       LeaderDisplay  `json:"leader_display"`
	DefaultBackend      string         `json:"default_backend"`
	HistoryMode         HistoryMode    `json:"history_mode"`
	RetentionHours      int            `json:"retention_hours"`
	Overflow            OverflowPolicy `json:"overflow"`
	RedactSecrets       bool           `json:"redact_secrets"`
	AllowRawInput       bool           `json:"allow_raw_input"`
	RemoteTrust         RemoteTrust    `json:"remote_trust"`
	AllowlistedMachines []string       `json:"allowlisted_machines"`
}

// Validate enforces the config cross-field rules:
// persistent 1h–365d; memory ≤24h; archive-oldest requires persistent; raw
// input requires local-only; allowlisted requires configured machines.
func (c Config) Validate() error {
	switch c.HistoryMode {
	case HistoryOff:
		if c.RetentionHours != 0 {
			return Fail(CodeValidation, "history mode off requires retention 0h")
		}
	case HistoryMemory:
		if c.RetentionHours < 1 || c.RetentionHours > 24 {
			return Fail(CodeValidation, "memory history retention must be 1h–24h")
		}
	case HistoryPersistent:
		if c.RetentionHours < 1 || c.RetentionHours > 365*24 {
			return Fail(CodeValidation, "persistent history retention must be 1h–8760h")
		}
	default:
		return Fail(CodeValidation, "unknown history mode %q", c.HistoryMode)
	}
	if c.Overflow == OverflowArchiveOldest && c.HistoryMode != HistoryPersistent {
		return Fail(CodeValidation, "archive-oldest overflow requires persistent history")
	}
	if c.Overflow != OverflowReject && c.Overflow != OverflowArchiveOldest {
		return Fail(CodeValidation, "unknown overflow policy %q", c.Overflow)
	}
	if c.AllowRawInput && c.RemoteTrust != TrustLocalOnly {
		return Fail(CodeValidation, "raw input requires local-only remote trust")
	}
	if c.RemoteTrust == TrustAllowlisted && len(c.AllowlistedMachines) == 0 {
		return Fail(CodeValidation, "allowlisted remote trust requires configured machines")
	}
	switch c.LeaderDisplay {
	case LeaderIcon, LeaderPath, LeaderNone:
	default:
		return Fail(CodeValidation, "unknown leader display %q", c.LeaderDisplay)
	}
	return nil
}

// BackendByID returns the backend or nil.
func (c *Catalog) BackendByID(id string) *Backend {
	for _, b := range c.Backends {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// MachineByID returns the machine or nil.
func (c *Catalog) MachineByID(id string) *Machine {
	for _, m := range c.Machines {
		if m.ID == id {
			return m
		}
	}
	return nil
}

// EventByID returns the event or nil.
func (c *Catalog) EventByID(id string) *Event {
	for _, e := range c.Events {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// HistoryForScope returns chunks of one scope sorted by seq.
func (c *Catalog) HistoryForScope(kind ResourceKind, id string) []*HistoryChunk {
	var out []*HistoryChunk
	for _, h := range c.History {
		if h.Scope.Kind == kind && h.Scope.ID == id {
			out = append(out, h)
		}
	}
	return out
}

// Clone returns a deep copy of the entire catalog. Backends return clones of
// their post-state; callers must never share slices or maps with the store.
func (c *Catalog) Clone() *Catalog {
	if c == nil {
		return nil
	}
	out := &Catalog{
		Revision:       c.Revision,
		Now:            c.Now,
		Scenario:       c.Scenario,
		MutationSerial: c.MutationSerial,
		Performance: Performance{
			Available: c.Performance.Available,
			Targets:   c.Performance.Targets,
		},
		Config: c.Config.clone(),
	}
	out.Workspaces = cloneSlice(c.Workspaces, func(w *Workspace) *Workspace {
		cp := *w
		cp.Desired = w.Desired.clone()
		cp.Observed = w.Observed.clone()
		cp.Drift = append([]DriftEntry(nil), w.Drift...)
		cp.TerminalIDs = append([]string(nil), w.TerminalIDs...)
		cp.AgentIDs = append([]string(nil), w.AgentIDs...)
		return &cp
	})
	out.ActiveViews = cloneSlice(c.ActiveViews, func(v *ActiveView) *ActiveView {
		cp := *v
		return &cp
	})
	out.Terminals = cloneSlice(c.Terminals, func(t *Terminal) *Terminal {
		cp := *t
		cp.Capabilities = append([]string(nil), t.Capabilities...)
		return &cp
	})
	out.Agents = cloneSlice(c.Agents, func(a *Agent) *Agent {
		cp := *a
		cp.Timeline = append([]AgentEntry(nil), a.Timeline...)
		return &cp
	})
	out.History = cloneSlice(c.History, func(h *HistoryChunk) *HistoryChunk {
		cp := *h
		cp.Lines = append([]string(nil), h.Lines...)
		return &cp
	})
	out.Events = cloneSlice(c.Events, func(e *Event) *Event {
		cp := *e
		return &cp
	})
	out.Snapshots = cloneSlice(c.Snapshots, func(s *Snapshot) *Snapshot {
		cp := *s
		cp.Tags = append([]string(nil), s.Tags...)
		cp.State = s.State.clone()
		return &cp
	})
	out.RestorePlans = cloneSlice(c.RestorePlans, func(p *RestorePlan) *RestorePlan {
		cp := *p
		cp.Ops = append([]RestoreOp(nil), p.Ops...)
		return &cp
	})
	out.Backends = cloneSlice(c.Backends, func(b *Backend) *Backend {
		cp := *b
		cp.Capabilities = append([]string(nil), b.Capabilities...)
		cp.UnsupportedReasons = append([]string(nil), b.UnsupportedReasons...)
		cp.LastProbe.Findings = append([]string(nil), b.LastProbe.Findings...)
		return &cp
	})
	out.Machines = cloneSlice(c.Machines, func(m *Machine) *Machine {
		cp := *m
		cp.LastResult.Findings = append([]string(nil), m.LastResult.Findings...)
		return &cp
	})
	out.Performance.Samples = append([]PerfSample(nil), c.Performance.Samples...)
	out.Performance.Traces = append([]PerfTrace(nil), c.Performance.Traces...)
	out.Performance.Benchmarks = append([]PerfBenchmark(nil), c.Performance.Benchmarks...)
	if c.LastExport != nil {
		cp := *c.LastExport
		cp.Scopes = append([]string(nil), c.LastExport.Scopes...)
		out.LastExport = &cp
	}
	return out
}

func cloneSlice[T any](in []*T, cp func(*T) *T) []*T {
	if in == nil {
		return nil
	}
	out := make([]*T, len(in))
	for i, v := range in {
		out[i] = cp(v)
	}
	return out
}

// Clone returns a deep copy of the workspace state.
func (s WorkspaceState) Clone() WorkspaceState {
	return s.clone()
}

func (s WorkspaceState) clone() WorkspaceState {
	out := WorkspaceState{Windows: make([]WindowSpec, len(s.Windows))}
	for i, w := range s.Windows {
		out.Windows[i] = WindowSpec{
			ID:       w.ID,
			Index:    w.Index,
			Layout:   w.Layout,
			Surfaces: append([]SurfaceSpec(nil), w.Surfaces...),
		}
	}
	return out
}

func (c Config) clone() Config {
	cp := c
	cp.AllowlistedMachines = append([]string(nil), c.AllowlistedMachines...)
	return cp
}
