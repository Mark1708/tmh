// Package mock implements the deterministic in-memory tmh-next backend:
// fixed fixtures, a logical clock, monotonic ID counters and the exact action
// transition table. No real processes, files, sockets or wall-clock reads.
package mock

import (
	"fmt"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
)

// epoch is the logical clock origin: the backend starts here and advances
// exactly one second per accepted mutation.
var epoch = time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC)

// minutesAgo returns a fixture timestamp before the epoch.
func minutesAgo(m int) time.Time { return epoch.Add(-time.Duration(m) * time.Minute) }

func secondsAgo(s int) time.Time { return epoch.Add(-time.Duration(s) * time.Second) }

// surface builds a surface spec.
func surface(id, title, cwd string) domain.SurfaceSpec {
	return domain.SurfaceSpec{ID: id, Title: title, CWD: cwd, Split: "none"}
}

func baseMachines(degraded bool) []*domain.Machine {
	buildStatus := domain.MachineOnline
	hb := minutesAgo(1)
	lat := 24
	instances := 1
	if degraded {
		buildStatus = domain.MachineOffline
		hb = minutesAgo(34)
		lat = -1
		instances = 0
	}
	return []*domain.Machine{
		{ID: "local", Name: "local", Host: "localhost", Kind: "local", Status: domain.MachineOnline,
			Instances: 2, Heartbeat: secondsAgo(2), LatencyMS: 0,
			LastResult: domain.ProbeResult{At: minutesAgo(5), OK: true, LatencyMS: 0}},
		{ID: "homelab", Name: "homelab", Host: "homelab.lan", Kind: "remote", Status: domain.MachineOnline,
			Instances: 1, Heartbeat: secondsAgo(9), LatencyMS: 8,
			LastResult: domain.ProbeResult{At: minutesAgo(7), OK: true, LatencyMS: 8, Findings: []string{"ssh ok", "tmux 3.4"}}},
		{ID: "build-01", Name: "build-01", Host: "build-01.ci", Kind: "remote", Status: buildStatus,
			Instances: instances,
			Heartbeat: hb, LatencyMS: lat,
			LastResult: func() domain.ProbeResult {
				if degraded {
					return domain.ProbeResult{At: minutesAgo(34), OK: false}
				}
				return domain.ProbeResult{At: minutesAgo(6), OK: true, LatencyMS: 24, Findings: []string{"ssh ok"}}
			}()},
	}
}

func baseBackends() []*domain.Backend {
	return []*domain.Backend{
		{ID: "tmux", Name: "tmux", Status: domain.BackendHealthy, Enabled: true, Default: true,
			Capabilities: []string{"windows", "panes", "session-share", "sockets", "layouts"},
			Instances:    4, LastProbe: domain.ProbeResult{At: minutesAgo(4), OK: true, LatencyMS: 3, Findings: []string{"server 3.4", "6 sessions"}}},
		{ID: "zellij", Name: "zellij", Status: domain.BackendPreview, Enabled: true, Default: false,
			Capabilities:       []string{"windows", "panes", "layouts", "plugins"},
			UnsupportedReasons: []string{"session-share: preview"},
			Instances:          1, LastProbe: domain.ProbeResult{At: minutesAgo(12), OK: true, LatencyMS: 9, Findings: []string{"0.41.2"}}},
		{ID: "native", Name: "native", Status: domain.BackendUnavailable, Enabled: false, Default: false,
			Capabilities:       []string{"windows"},
			UnsupportedReasons: []string{"panes: not implemented", "session-share: not available", "probe times out on this fixture"},
			Instances:          0, LastProbe: domain.ProbeResult{At: minutesAgo(20), OK: false}},
	}
}

func baseConfig() domain.Config {
	return domain.Config{
		DefaultPage:    "dashboard",
		LeaderDisplay:  domain.LeaderIcon,
		DefaultBackend: "tmux",
		HistoryMode:    domain.HistoryPersistent,
		RetentionHours: 720,
		Overflow:       domain.OverflowReject,
		RedactSecrets:  true,
		AllowRawInput:  false,
		RemoteTrust:    domain.TrustLocalOnly,
	}
}

func perfSeries(available bool, n int) domain.Performance {
	samples := make([]domain.PerfSample, 0, n)
	for i := range n {
		t := minutesAgo(n - i)
		samples = append(samples, domain.PerfSample{
			At: t, InputP50: 4 + i%3, InputP95: 9 + i%4, InputP99: 12 + i%5,
			RedrawP99: 16 + i%6, ActionP99: 34 + i%7, QueueDepth: i % 3,
		})
	}
	return domain.Performance{
		Available: available,
		Samples:   samples,
		Targets:   domain.PerfTargets{InputP99MS: 15, RedrawP99MS: 20, ActionP99MS: 50},
	}
}

// defaultTopology builds workspaces/terminals/agents/views/history/events/
// snapshots for the default (and degraded) scenario.
func defaultTopology(cat *domain.Catalog, degraded bool) {
	ws := func(id, name, machine string, windows []domain.WindowSpec, ephemeral bool) *domain.Workspace {
		w := &domain.Workspace{
			ID: id, Name: name, MachineID: machine, BackendID: "tmux",
			Desired:  domain.WorkspaceState{Windows: cloneWindows(windows)},
			Observed: domain.WorkspaceState{Windows: cloneWindows(windows)},
			Status:   domain.WorkspaceOK, LastAttached: minutesAgo(12), LastFocused: minutesAgo(3),
			Ephemeral: ephemeral,
		}
		if len(w.Observed.Windows) > 0 && len(w.Observed.Windows[0].Surfaces) > 0 {
			w.SelectedSurfaceID = w.Observed.Windows[0].Surfaces[0].ID
		}
		cat.Workspaces = append(cat.Workspaces, w)
		return w
	}

	baseWin := []domain.WindowSpec{{
		ID: "win-0001", Index: 0, Layout: "main-horizontal",
		Surfaces: []domain.SurfaceSpec{
			surface("sf-0001", "editor", "~/work/base"),
			surface("sf-0002", "server", "~/work/base"),
		},
	}, {
		ID: "win-0002", Index: 1, Layout: "even-horizontal",
		Surfaces: []domain.SurfaceSpec{surface("sf-0003", "logs", "~/work/base/logs")},
	}}
	ws("base", "base", "local", baseWin, false)

	ws("ecp", "ecp", "local", []domain.WindowSpec{{
		ID: "win-0004", Index: 0, Layout: "tiled",
		Surfaces: []domain.SurfaceSpec{
			surface("sf-0004", "ledger", "~/ecp/ledger"),
			surface("sf-0005", "tests", "~/ecp/tests"),
		},
	}}, false)

	ws("infra", "infra", "homelab", []domain.WindowSpec{{
		ID: "win-0005", Index: 0, Layout: "main-vertical",
		Surfaces: []domain.SurfaceSpec{surface("sf-0006", "cluster", "~/infra/cluster")},
	}}, false)

	// kb carries drift: desired wants an extra docs surface that reality lacks.
	kbWin := []domain.WindowSpec{{
		ID: "win-0006", Index: 0, Layout: "even-vertical",
		Surfaces: []domain.SurfaceSpec{surface("sf-0007", "search", "~/kb")},
	}}
	kb := ws("kb", "kb", "local", kbWin, false)
	kb.Desired.Windows[0].Surfaces = append(kb.Desired.Windows[0].Surfaces, surface("sf-0008", "docs", "~/kb/docs"))
	kb.Drift = []domain.DriftEntry{{Field: "window/win-0006/surface/sf-0008", Desired: "present", Observed: "absent"}}
	kb.Status = domain.WorkspaceDrift

	ws("my-projects", "my-projects", "local", []domain.WindowSpec{{
		ID: "win-0007", Index: 0, Layout: "main-horizontal",
		Surfaces: []domain.SurfaceSpec{surface("sf-0009", "ui-kit", "~/p/ui-kit")},
	}}, false)

	ws("platform", "platform", "build-01", []domain.WindowSpec{{
		ID: "win-0008", Index: 0, Layout: "tiled",
		Surfaces: []domain.SurfaceSpec{surface("sf-0010", "deploy", "~/platform/deploy")},
	}}, false)

	ws("active", "active", "local", []domain.WindowSpec{{
		ID: "win-0009", Index: 0, Layout: "even-horizontal",
		Surfaces: []domain.SurfaceSpec{surface("sf-0011", "scratch", "~/tmp")},
	}}, true)

	// terminals ----------------------------------------------------------------
	term := func(n int, name, wsID, sfID, process, cwd string, state domain.TerminalState, caps ...string) *domain.Terminal {
		t := &domain.Terminal{
			ID: fmt.Sprintf("term-%04d", n), Name: name, WorkspaceID: wsID, SurfaceID: sfID,
			BackendID: "tmux", State: state, Process: process, CWD: cwd,
			Capabilities: caps, LastIO: minutesAgo(2), CreatedAt: minutesAgo(90),
		}
		cat.Terminals = append(cat.Terminals, t)
		return t
	}
	term(1, "lk", "base", "sf-0001", "zsh", "~/work/base", domain.TerminalLive, "pty", "mouse", "color256")
	term(2, "mdr", "base", "sf-0002", "make dev", "~/work/base", domain.TerminalLive, "pty", "color256")
	term(3, "filings", "ecp", "sf-0004", "bt", "~/ecp/ledger", domain.TerminalLive, "pty")
	term(4, "ui-kit", "my-projects", "sf-0009", "pnpm dev", "~/p/ui-kit", domain.TerminalLive, "pty", "mouse")
	term(5, "cluster", "infra", "sf-0006", "kubectl", "~/infra/cluster", domain.TerminalLive, "pty")
	term(6, "terminal", "platform", "sf-0010", "zsh", "~/platform/deploy", domain.TerminalExited, "pty")
	term(7, "sandbox", "active", "sf-0011", "zsh", "~/tmp", domain.TerminalLive, "pty")

	link := func(wsID, termID string) {
		for _, w := range cat.Workspaces {
			if w.ID == wsID {
				w.TerminalIDs = append(w.TerminalIDs, termID)
			}
		}
	}
	link("base", "term-0001")
	link("base", "term-0002")
	link("ecp", "term-0003")
	link("my-projects", "term-0004")
	link("infra", "term-0005")
	link("platform", "term-0006")
	link("active", "term-0007")

	// agents ---------------------------------------------------------------------
	agent := func(n int, name string, kind domain.AgentKind, state domain.AgentState, wsID, termID string, progress int, mAgo int) *domain.Agent {
		a := &domain.Agent{
			ID: fmt.Sprintf("agent-%04d", n), Name: name, Kind: kind, State: state,
			WorkspaceID: wsID, TerminalID: termID, Progress: progress,
			LastUsed: minutesAgo(mAgo), LastFocused: minutesAgo(mAgo),
		}
		cat.Agents = append(cat.Agents, a)
		for _, w := range cat.Workspaces {
			if w.ID == wsID {
				w.AgentIDs = append(w.AgentIDs, a.ID)
			}
		}
		return a
	}
	a1 := agent(1, "refactor", domain.AgentClaude, domain.AgentRunning, "base", "term-0001", 42, 6)
	a1.Timeline = []domain.AgentEntry{
		{At: minutesAgo(9), Kind: "prompt", Text: "extract config loader"},
		{At: minutesAgo(7), Kind: "result", Text: "3 files changed, tests green"},
	}
	a2 := agent(2, "ledger-audit", domain.AgentCodex, domain.AgentNeedsInput, "ecp", "term-0003", 61, 14)
	a2.Timeline = []domain.AgentEntry{
		{At: minutesAgo(16), Kind: "prompt", Text: "audit ledger entries"},
		{At: minutesAgo(14), Kind: "state", Text: "needs input: choose fiscal year"},
	}
	agent(3, "cluster-plan", domain.AgentOpenCode, domain.AgentWaiting, "infra", "term-0005", 23, 22)
	a4 := agent(4, "release-notes", domain.AgentClaude, domain.AgentDone, "platform", "", 99, 40)
	a4.Timeline = []domain.AgentEntry{{At: minutesAgo(44), Kind: "prompt", Text: "draft release notes"}}
	agent(5, "scratch-helper", domain.AgentCustom, domain.AgentRunning, "active", "term-0007", 8, 2)

	// active views -------------------------------------------------------------------
	pinned := &domain.ActiveView{ID: "view-0001", Title: "ecp/ledger", Kind: domain.ActivePinned,
		WorkspaceID: "ecp", Owner: "mark", LastUsed: minutesAgo(4), TTLSeconds: 3600, Pinned: true}
	shared := &domain.ActiveView{ID: "view-0002", Title: "base/editor", Kind: domain.ActiveShared,
		WorkspaceID: "base", Owner: "mark", LastUsed: minutesAgo(1), TTLSeconds: 900}
	recent := &domain.ActiveView{ID: "view-0003", Title: "kb/search", Kind: domain.ActiveRecent,
		WorkspaceID: "kb", Owner: "mark", LastUsed: minutesAgo(10), TTLSeconds: 900}
	// view-0004 is unpinned and long expired: the prune target.
	expired := &domain.ActiveView{ID: "view-0004", Title: "platform/deploy", Kind: domain.ActiveRecent,
		WorkspaceID: "platform", Owner: "ci", LastUsed: minutesAgo(120), TTLSeconds: 600}
	cat.ActiveViews = append(cat.ActiveViews, pinned, shared, recent, expired)

	// history --------------------------------------------------------------------------
	hist := func(n int, scopeKind domain.ResourceKind, scopeID string, seq int, lines []string, gap bool) {
		cat.History = append(cat.History, &domain.HistoryChunk{
			ID: fmt.Sprintf("hist-%04d", n), Scope: domain.HistoryScope{Kind: scopeKind, ID: scopeID},
			Seq: seq, At: minutesAgo(30 - n), Lines: lines, Kind: "stream", Gap: gap,
		})
	}
	hist(1, domain.KindTerminal, "term-0001", 1, []string{"$ make test", "ok  4.2s", "coverage 81%"}, false)
	hist(2, domain.KindTerminal, "term-0003", 1, []string{"$ bt reconcile", "dry-run: 12 ops"}, false)
	hist(3, domain.KindAgent, "agent-0001", 1, []string{"prompt: extract config loader", "result: 3 files changed"}, false)
	hist(4, domain.KindTerminal, "term-0007", 1, []string{"$ echo scratch", "scratch"}, degraded) // gap marker in degraded
	hist(5, domain.KindTerminal, "term-0002", 1, []string{"$ make dev", "listening :5173"}, false)

	// events -----------------------------------------------------------------------------
	seq := 0
	evt := func(category domain.EventCategory, sev domain.EventSeverity, res domain.ResourceRef, msg string) {
		seq++
		cat.Events = append(cat.Events, &domain.Event{
			ID: fmt.Sprintf("evt-%04d", seq), Seq: seq, At: minutesAgo(60 - seq),
			Category: category, Severity: sev, Resource: res, Message: msg,
			Detail: fmt.Sprintf(`{"cat":%q,"msg":%q}`, category, msg),
		})
	}
	evt(domain.EventSystem, domain.SeverityInfo, domain.Ref(domain.KindConfig, "config"), "demo catalog loaded")
	evt(domain.EventWorkspace, domain.SeverityInfo, domain.Ref(domain.KindWorkspace, "base"), "workspace attached")
	evt(domain.EventTerminal, domain.SeverityInfo, domain.Ref(domain.KindTerminal, "term-0003"), "terminal linked to ecp")
	evt(domain.EventAgent, domain.SeverityWarn, domain.Ref(domain.KindAgent, "agent-0002"), "agent needs input")
	evt(domain.EventSnapshot, domain.SeverityInfo, domain.Ref(domain.KindSnapshot, "snap-0002"), "nightly snapshot created")
	evt(domain.EventBackend, domain.SeverityWarn, domain.Ref(domain.KindBackend, "native"), "probe failed: timeout")
	if degraded {
		evt(domain.EventMachine, domain.SeverityError, domain.Ref(domain.KindMachine, "build-01"), "machine unreachable")
	} else {
		evt(domain.EventMachine, domain.SeverityInfo, domain.Ref(domain.KindMachine, "build-01"), "ping 24ms")
	}
	evt(domain.EventWorkspace, domain.SeverityWarn, domain.Ref(domain.KindWorkspace, "kb"), "drift detected: 1 field")

	// snapshots -----------------------------------------------------------------------------
	snap := func(n int, wsID string, mAgo int, tags []string, windows []domain.WindowSpec) {
		state := domain.WorkspaceState{Windows: cloneWindows(windows)}
		cat.Snapshots = append(cat.Snapshots, &domain.Snapshot{
			ID: fmt.Sprintf("snap-%04d", n), WorkspaceID: wsID, CreatedAt: minutesAgo(mAgo),
			BackendID: "tmux", Tags: tags, State: state,
			WindowCount: len(state.Windows), SurfaceCount: state.SurfaceCount(),
		})
	}
	snap(1, "base", 240, []string{"baseline", "gold"}, baseWin)
	snap(2, "ecp", 720, []string{"nightly"}, []domain.WindowSpec{{
		ID: "win-0004", Index: 0, Layout: "tiled",
		Surfaces: []domain.SurfaceSpec{surface("sf-0004", "ledger", "~/ecp/ledger")},
	}})
	snap(3, "infra", 60, nil, []domain.WindowSpec{{
		ID: "win-0005", Index: 0, Layout: "main-vertical",
		Surfaces: []domain.SurfaceSpec{surface("sf-0006", "cluster", "~/infra/cluster")},
	}})
	snap(4, "kb", 1440, []string{"pre-migration"}, kbWin)
}

// cloneWindows deep-copies window specs.
func cloneWindows(in []domain.WindowSpec) []domain.WindowSpec {
	out := make([]domain.WindowSpec, len(in))
	for i, w := range in {
		out[i] = w
		out[i].Surfaces = append([]domain.SurfaceSpec(nil), w.Surfaces...)
	}
	return out
}

// Fixture returns the pristine catalog for a scenario. Revision starts at 1
// and the logical clock at the epoch.
func Fixture(sc domain.Scenario) (*domain.Catalog, error) {
	cat := &domain.Catalog{Revision: 1, Now: epoch, Scenario: sc, MutationSerial: 0}
	switch sc {
	case domain.ScenarioDefault:
		cat.Machines = baseMachines(false)
		cat.Backends = baseBackends()
		cat.Performance = perfSeries(true, 24)
		cat.Config = baseConfig()
		defaultTopology(cat, false)
	case domain.ScenarioDegraded:
		cat.Machines = baseMachines(true)
		cat.Backends = baseBackends()
		cat.Performance = perfSeries(false, 12) // performance service down → busy
		cat.Config = baseConfig()
		defaultTopology(cat, true)
	case domain.ScenarioEmpty:
		cat.Machines = baseMachines(false)
		cat.Backends = baseBackends()
		cat.Performance = perfSeries(true, 8)
		cat.Config = baseConfig()
	default:
		return nil, domain.Fail(domain.CodeNotFound, "unknown scenario %q", sc)
	}
	if err := domain.ValidateCatalog(cat); err != nil {
		return nil, fmt.Errorf("fixture %s invalid: %w", sc, err)
	}
	return cat, nil
}
