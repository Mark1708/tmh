package production

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

func (c *Client) executeLocked(ctx context.Context, action domain.Action) (domain.MutationResult, *control.WatchEvent, error) {
	key := commandKey(action)
	if prior, ok, err := c.store.CommandResult(ctx, key); err != nil {
		return domain.MutationResult{}, nil, err
	} else if ok {
		return prior, nil, nil
	}
	current, err := c.store.LoadCatalog(ctx)
	if err != nil {
		return domain.MutationResult{}, nil, err
	}
	if current == nil {
		return domain.MutationResult{}, nil, domain.Fail(domain.CodeNotFound, "catalog is not initialized")
	}
	if action.ExpectedRevision != current.Revision {
		return domain.MutationResult{}, nil, domain.ErrRevisionConflict
	}
	graph, err := c.discoverer.Discover(ctx)
	if err != nil {
		return domain.MutationResult{}, nil, domain.Fail(domain.CodeUnreachable, "runtime discovery failed: %v", err)
	}
	desired, err := c.loadDesired()
	if err != nil {
		return domain.MutationResult{}, nil, err
	}
	markConfiguredManaged(graph, desired)
	work := current.Clone()
	work.Now = c.now().UTC()
	message, interactive, runtimeChanged, err := c.applyProduction(ctx, work, graph, action)
	if err != nil {
		return domain.MutationResult{}, nil, err
	}
	if runtimeChanged {
		observed, err := c.discoverer.Discover(ctx)
		if err != nil {
			return domain.MutationResult{}, nil, domain.Fail(domain.CodeUnreachable, "post-command discovery failed: %v", err)
		}
		desired, err = c.loadDesired()
		if err != nil {
			return domain.MutationResult{}, nil, err
		}
		markConfiguredManaged(observed, desired)
		work, err = ProjectRuntime(work, observed, desired, c.now().UTC())
		if err != nil {
			return domain.MutationResult{}, nil, err
		}
		if err := verifyRuntimeEffect(action, current, work); err != nil {
			return domain.MutationResult{}, nil, err
		}
	}
	base := current.Revision
	work.Revision = base + 1
	work.MutationSerial++
	appendAudit(work, categoryFor(action.Target.Kind), domain.SeverityInfo, auditRef(action), string(action.Kind), actionDetail(action))
	if err := domain.ValidateCatalog(work); err != nil {
		return domain.MutationResult{}, nil, domain.Fail(domain.CodeMalformedResult, "transition produced invalid catalog: %v", err)
	}
	result := domain.MutationResult{
		BaseRevision: base, NewRevision: work.Revision, Catalog: work.Clone(), Action: action,
		Message: message, InteractiveCommand: interactive,
	}
	if err := c.store.SaveMutationResult(ctx, base, work, key, result); err != nil {
		return domain.MutationResult{}, nil, err
	}
	event := &control.WatchEvent{
		Seq: eventSequence(work), BaseRevision: base, Revision: work.Revision,
		Kind: control.EventCommandResult, Changed: []domain.ResourceRef{auditRef(action)}, CommandID: key,
	}
	return result, event, nil
}

func (c *Client) applyProduction(ctx context.Context, catalog *domain.Catalog, graph *runtimegraph.Graph, action domain.Action) (string, []string, bool, error) {
	switch action.Kind {
	case domain.ActionAttach:
		session, _, err := resolveAttach(graph, catalog, action.Target)
		if err != nil {
			return "", nil, false, err
		}
		if workspace := workspaceForTarget(catalog, action.Target); workspace != nil {
			workspace.LastAttached, workspace.LastFocused = catalog.Now, catalog.Now
		}
		return "attaching to " + session, []string{c.zellijBinary, "attach", session}, false, nil

	case domain.ActionSplit:
		terminal, session, _, err := resolveTerminal(graph, catalog, action.Target)
		if err != nil {
			return "", nil, false, err
		}
		if err := requireManagedTerminal(graph, terminal.ID); err != nil {
			return "", nil, false, err
		}
		tab, err := terminalTabID(graph, terminal.ID)
		if err != nil {
			return "", nil, false, err
		}
		args := []string{"--session", session, "action", "new-pane", "--direction", "right", "--tab-id", tab, "--no-focus"}
		if terminal.CWD != "" {
			args = append(args, "--cwd", terminal.CWD)
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		args = append(args, "--", shell, "-il")
		if err := c.run(ctx, args...); err != nil {
			return "", nil, false, err
		}
		return "New terminal pane created in " + session, nil, true, nil

	case domain.ActionSend:
		terminal, session, pane, err := resolveTerminal(graph, catalog, action.Target)
		if err != nil {
			return "", nil, false, err
		}
		if err := requireManagedTerminal(graph, terminal.ID); err != nil {
			return "", nil, false, err
		}
		if strings.TrimSpace(action.Value) == "" {
			return "", nil, false, domain.Fail(domain.CodeEmptyValue, "send requires non-empty input")
		}
		if err := c.run(ctx, "--session", session, "action", "write-chars", "--pane-id", pane, action.Value); err != nil {
			return "", nil, false, err
		}
		terminal.LastIO = catalog.Now
		appendHistory(catalog, domain.KindTerminal, terminal.ID, []string{"$ " + action.Value})
		return fmt.Sprintf("sent %d bytes to %s", len(action.Value), terminal.ID), nil, true, nil

	case domain.ActionKill:
		terminal, session, pane, err := resolveTerminal(graph, catalog, action.Target)
		if err != nil {
			return "", nil, false, err
		}
		if err := requireManagedTerminal(graph, terminal.ID); err != nil {
			return "", nil, false, err
		}
		if !action.Confirmed {
			return "", nil, false, domain.Fail(domain.CodeConfirmationNeeded, "kill requires confirmation")
		}
		if err := c.run(ctx, "--session", session, "action", "close-pane", "--pane-id", pane); err != nil {
			return "", nil, false, err
		}
		return "closed pane " + terminal.Name, nil, true, nil

	case domain.ActionFreeze:
		workspace := catalog.WorkspaceByID(action.Target.ID)
		if action.Target.Kind != domain.KindWorkspace || workspace == nil {
			return "", nil, false, domain.Fail(domain.CodeNotFound, "workspace %s not found", action.Target.ID)
		}
		workspace.Desired = workspace.Observed.Clone()
		workspace.Drift = nil
		workspace.Status = domain.WorkspaceFrozen
		return "workspace frozen at observed state", nil, false, nil

	case domain.ActionLink:
		return "", nil, false, domain.Fail(domain.CodeProtected, "Zellij panes cannot move between sessions safely")

	case domain.ActionPin, domain.ActionUnpin, domain.ActionTouch, domain.ActionPrune:
		return mutateActive(catalog, action)

	case domain.ActionAgentFocus:
		agent := catalog.AgentByID(action.Target.ID)
		if action.Target.Kind != domain.KindAgent || agent == nil {
			return "", nil, false, domain.Fail(domain.CodeNotFound, "agent %s not found", action.Target.ID)
		}
		if agent.State == domain.AgentDone {
			return "", nil, false, domain.Fail(domain.CodeInvalidState, "agent %s is done", agent.ID)
		}
		agent.LastFocused, agent.LastUsed = catalog.Now, catalog.Now
		session, _, err := resolveAttach(graph, catalog, domain.Ref(domain.KindTerminal, agent.TerminalID))
		if err != nil {
			return "", nil, false, err
		}
		return "attaching to agent " + agent.Name, []string{c.zellijBinary, "attach", session}, false, nil

	case domain.ActionAgentPrompt:
		agent := catalog.AgentByID(action.Target.ID)
		if action.Target.Kind != domain.KindAgent || agent == nil {
			return "", nil, false, domain.Fail(domain.CodeNotFound, "agent %s not found", action.Target.ID)
		}
		if strings.TrimSpace(action.Value) == "" {
			return "", nil, false, domain.Fail(domain.CodeEmptyValue, "agent prompt requires non-empty input")
		}
		_, session, pane, err := resolveTerminal(graph, catalog, domain.Ref(domain.KindTerminal, agent.TerminalID))
		if err != nil {
			return "", nil, false, err
		}
		if err := requireManagedTerminal(graph, agent.TerminalID); err != nil {
			return "", nil, false, err
		}
		if err := c.run(ctx, "--session", session, "action", "write-chars", "--pane-id", pane, action.Value); err != nil {
			return "", nil, false, err
		}
		agent.State, agent.Prompt, agent.LastUsed = domain.AgentRunning, action.Value, catalog.Now
		agent.Timeline = append(agent.Timeline, domain.AgentEntry{At: catalog.Now, Kind: "prompt", Text: action.Value})
		appendHistory(catalog, domain.KindAgent, agent.ID, []string{"prompt: " + action.Value})
		return "prompt sent to " + agent.Name, nil, true, nil

	case domain.ActionAgentWait:
		agent := catalog.AgentByID(action.Target.ID)
		if agent == nil {
			return "", nil, false, domain.Fail(domain.CodeNotFound, "agent %s not found", action.Target.ID)
		}
		if agent.State != domain.AgentRunning {
			return "", nil, false, domain.Fail(domain.CodeInvalidState, "agent %s is not running", agent.ID)
		}
		agent.State = domain.AgentWaiting
		return "agent marked waiting", nil, false, nil

	case domain.ActionAgentClaim:
		agent := catalog.AgentByID(action.Target.ID)
		if agent == nil {
			return "", nil, false, domain.Fail(domain.CodeNotFound, "agent %s not found", action.Target.ID)
		}
		if agent.ClaimedBy != "" {
			return "", nil, false, domain.Fail(domain.CodeAlreadyClaimed, "agent %s claimed by %s", agent.ID, agent.ClaimedBy)
		}
		agent.ClaimedBy = currentUser()
		return "claimed " + agent.Name, nil, false, nil

	case domain.ActionHistoryExport:
		return c.exportHistory(catalog, action)

	case domain.ActionSnapshotCreate:
		return createSnapshots(catalog, action)
	case domain.ActionSnapshotTag:
		return tagSnapshot(catalog, action)
	case domain.ActionSnapshotDelete:
		return deleteSnapshot(catalog, action)
	case domain.ActionSnapshotPlan:
		return planSnapshot(catalog, action)
	case domain.ActionRestoreApply:
		return c.applyRestore(ctx, catalog, graph, action)
	case domain.ActionReconcileUndo:
		return "", nil, false, domain.Fail(domain.CodeUndoUnavailable, "production restore undo is unavailable after runtime commands")

	case domain.ActionBackendProbe, domain.ActionBackendDoctor:
		backend := catalog.BackendByID(action.Target.ID)
		if backend == nil {
			return "", nil, false, domain.Fail(domain.CodeNotFound, "backend %s not found", action.Target.ID)
		}
		started := time.Now()
		if err := c.run(ctx, "--version"); err != nil {
			backend.LastProbe = domain.ProbeResult{At: catalog.Now, OK: false, Findings: []string{err.Error()}}
			return "", nil, false, err
		}
		backend.LastProbe = domain.ProbeResult{At: catalog.Now, OK: true, LatencyMS: int(time.Since(started).Milliseconds()), Findings: append([]string(nil), backend.Capabilities...)}
		return fmt.Sprintf("%s ok (%dms)", action.Kind, backend.LastProbe.LatencyMS), nil, false, nil
	case domain.ActionBackendToggle:
		backend := catalog.BackendByID(action.Target.ID)
		if backend == nil {
			return "", nil, false, domain.Fail(domain.CodeNotFound, "backend %s not found", action.Target.ID)
		}
		if backend.Default {
			return "", nil, false, domain.Fail(domain.CodeProtected, "default backend cannot be disabled")
		}
		if backend.Enabled && !action.Confirmed {
			return "", nil, false, domain.Fail(domain.CodeConfirmationNeeded, "disabling backend requires confirmation")
		}
		backend.Enabled = !backend.Enabled
		return fmt.Sprintf("backend %s enabled=%v", backend.Name, backend.Enabled), nil, false, nil

	case domain.ActionMachineConnect, domain.ActionMachinePing, domain.ActionMachineDoctor:
		machine := catalog.MachineByID(action.Target.ID)
		if machine == nil {
			return "", nil, false, domain.Fail(domain.CodeNotFound, "machine %s not found", action.Target.ID)
		}
		if machine.Status != domain.MachineOnline {
			return "", nil, false, domain.Fail(domain.CodeUnreachable, "machine %s is offline", machine.ID)
		}
		machine.Heartbeat = catalog.Now
		machine.LastResult = domain.ProbeResult{At: catalog.Now, OK: true, Findings: []string{"local runtime reachable"}}
		return string(action.Kind) + " ok", nil, false, nil

	case domain.ActionPerfRecord, domain.ActionPerfBenchmark:
		started := time.Now()
		if _, err := c.discoverer.Discover(ctx); err != nil {
			return "", nil, false, err
		}
		elapsed := int(time.Since(started).Milliseconds())
		catalog.Performance.Available = true
		if action.Kind == domain.ActionPerfRecord {
			catalog.Performance.Traces = append(catalog.Performance.Traces, domain.PerfTrace{At: catalog.Now, Kind: "action", P99MS: elapsed, Detail: "live Zellij discovery"})
			return fmt.Sprintf("recorded live discovery trace (%dms)", elapsed), nil, false, nil
		}
		catalog.Performance.Benchmarks = append(catalog.Performance.Benchmarks, domain.PerfBenchmark{At: catalog.Now, Ops: 1, AvgMS: elapsed, P99MS: elapsed})
		return fmt.Sprintf("benchmark: one live discovery in %dms", elapsed), nil, false, nil

	case domain.ActionConfigValidate, domain.ActionConfigSave:
		var config domain.Config
		if err := json.Unmarshal([]byte(action.Value), &config); err != nil {
			return "", nil, false, domain.Fail(domain.CodeValidation, "decode config: %v", err)
		}
		if err := config.Validate(); err != nil {
			return "", nil, false, err
		}
		if action.Kind == domain.ActionConfigSave {
			catalog.Config = config
			return "control-plane config saved", nil, false, nil
		}
		return "draft config valid", nil, false, nil
	case domain.ActionConfigReload:
		catalog.Config = defaultConfig()
		return "control-plane config reset to defaults", nil, false, nil
	default:
		return "", nil, false, domain.Fail(domain.CodeValidation, "unknown action kind %q", action.Kind)
	}
}

func (c *Client) runOutput(ctx context.Context, args ...string) ([]byte, error) {
	if c.runner == nil {
		return nil, domain.Fail(domain.CodeUnreachable, "Zellij command runner is unavailable")
	}
	out, err := c.runner.Run(ctx, args...)
	if err != nil {
		return nil, domain.Fail(domain.CodeUnreachable, "zellij %s: %v", strings.Join(args, " "), err)
	}
	return out, nil
}

func (c *Client) run(ctx context.Context, args ...string) error {
	_, err := c.runOutput(ctx, args...)
	return err
}

func mutateActive(catalog *domain.Catalog, action domain.Action) (string, []string, bool, error) {
	view := catalog.ActiveViewByID(action.Target.ID)
	if action.Target.Kind != domain.KindActive || view == nil {
		return "", nil, false, domain.Fail(domain.CodeNotFound, "active view %s not found", action.Target.ID)
	}
	switch action.Kind {
	case domain.ActionPin:
		if view.Pinned {
			return "", nil, false, domain.Fail(domain.CodeInvalidState, "view already pinned")
		}
		view.Pinned, view.Kind, view.TTLSeconds = true, domain.ActivePinned, 0
	case domain.ActionUnpin:
		if !view.Pinned {
			return "", nil, false, domain.Fail(domain.CodeInvalidState, "view is not pinned")
		}
		view.Pinned, view.Kind, view.TTLSeconds = false, domain.ActiveRecent, 900
	case domain.ActionTouch:
		view.LastUsed = catalog.Now
	case domain.ActionPrune:
		if view.Pinned || !view.ExpiredAt(catalog.Now).Before(catalog.Now) {
			return "", nil, false, domain.Fail(domain.CodeProtected, "view is pinned or not expired")
		}
		if !action.Confirmed {
			return "", nil, false, domain.Fail(domain.CodeConfirmationNeeded, "prune requires confirmation")
		}
		return "", nil, false, domain.Fail(domain.CodeProtected, "Zellij client connections are read-only")
	}
	return string(action.Kind) + " " + view.Title, nil, false, nil
}

func createSnapshots(catalog *domain.Catalog, action domain.Action) (string, []string, bool, error) {
	var targets []*domain.Workspace
	if action.Value == "all" {
		targets = catalog.Workspaces
	} else {
		id := action.Value
		if id == "" && action.Target.Kind == domain.KindWorkspace {
			id = action.Target.ID
		}
		if workspace := catalog.WorkspaceByID(id); workspace != nil {
			targets = []*domain.Workspace{workspace}
		}
	}
	if len(targets) == 0 {
		return "", nil, false, domain.Fail(domain.CodeNotFound, "no workspace to snapshot")
	}
	for _, workspace := range targets {
		id := nextID("snapshot", snapshotIDs(catalog))
		catalog.Snapshots = append(catalog.Snapshots, &domain.Snapshot{
			ID: id, WorkspaceID: workspace.ID, CreatedAt: catalog.Now, BackendID: workspace.BackendID,
			Tags: []string{"manual"}, State: workspace.Observed.Clone(),
			WindowCount: len(workspace.Observed.Windows), SurfaceCount: workspace.Observed.SurfaceCount(),
		})
	}
	return fmt.Sprintf("created %d durable snapshot(s)", len(targets)), nil, false, nil
}

func tagSnapshot(catalog *domain.Catalog, action domain.Action) (string, []string, bool, error) {
	snapshot := catalog.SnapshotByID(action.Target.ID)
	if snapshot == nil {
		return "", nil, false, domain.Fail(domain.CodeNotFound, "snapshot %s not found", action.Target.ID)
	}
	if strings.TrimSpace(action.Value) == "" {
		return "", nil, false, domain.Fail(domain.CodeEmptyValue, "tag is empty")
	}
	for _, tag := range snapshot.Tags {
		if tag == action.Value {
			return "tag already present", nil, false, nil
		}
	}
	snapshot.Tags = append(snapshot.Tags, action.Value)
	return "snapshot tagged", nil, false, nil
}

func deleteSnapshot(catalog *domain.Catalog, action domain.Action) (string, []string, bool, error) {
	if catalog.SnapshotByID(action.Target.ID) == nil {
		return "", nil, false, domain.Fail(domain.CodeNotFound, "snapshot %s not found", action.Target.ID)
	}
	if !action.Confirmed {
		return "", nil, false, domain.Fail(domain.CodeConfirmationNeeded, "snapshot delete requires confirmation")
	}
	kept := catalog.Snapshots[:0]
	for _, snapshot := range catalog.Snapshots {
		if snapshot.ID != action.Target.ID {
			kept = append(kept, snapshot)
		}
	}
	catalog.Snapshots = kept
	plans := catalog.RestorePlans[:0]
	for _, plan := range catalog.RestorePlans {
		if plan.SnapshotID != action.Target.ID {
			plans = append(plans, plan)
		}
	}
	catalog.RestorePlans = plans
	return "snapshot deleted", nil, false, nil
}

func planSnapshot(catalog *domain.Catalog, action domain.Action) (string, []string, bool, error) {
	snapshot := catalog.SnapshotByID(action.Target.ID)
	if snapshot == nil {
		return "", nil, false, domain.Fail(domain.CodeNotFound, "snapshot %s not found", action.Target.ID)
	}
	workspace := catalog.WorkspaceByID(snapshot.WorkspaceID)
	if workspace == nil {
		return "", nil, false, domain.Fail(domain.CodeNotFound, "snapshot workspace is missing")
	}
	id := nextID("plan", planIDs(catalog))
	ops := []domain.RestoreOp{
		{ID: id + "-freeze", Mode: domain.RestoreFreeze, Target: domain.Ref(domain.KindSnapshot, snapshot.ID), Field: "workspace.state", Before: "observed", After: "snapshot", Included: false},
		{ID: id + "pull", Mode: domain.RestorePull, Target: domain.Ref(domain.KindWorkspace, workspace.ID), Field: "workspace.state", Before: "desired", After: "observed", Included: true},
		{ID: id + "push", Mode: domain.RestorePush, Target: domain.Ref(domain.KindWorkspace, workspace.ID), Field: "workspace.state", Before: "observed", After: "snapshot", Included: true},
	}
	domain.SortRestoreOps(ops)
	catalog.RestorePlans = append(catalog.RestorePlans, &domain.RestorePlan{
		ID: id, SnapshotID: snapshot.ID, WorkspaceID: workspace.ID, CreatedAt: catalog.Now,
		Status: domain.PlanPending, Ops: ops,
	})
	return fmt.Sprintf("restore plan %s created", id), nil, false, nil
}

func (c *Client) applyRestore(ctx context.Context, catalog *domain.Catalog, graph *runtimegraph.Graph, action domain.Action) (string, []string, bool, error) {
	plan := catalog.RestorePlanByID(action.Target.ID)
	if plan == nil {
		return "", nil, false, domain.Fail(domain.CodeNotFound, "plan %s not found", action.Target.ID)
	}
	if plan.Status != domain.PlanPending {
		return "", nil, false, domain.Fail(domain.CodeStalePlan, "plan is not pending")
	}
	if !action.Confirmed {
		return "", nil, false, domain.Fail(domain.CodeConfirmationNeeded, "restore apply requires confirmation")
	}
	mode := domain.RestoreMode(action.Value)
	workspace := catalog.WorkspaceByID(plan.WorkspaceID)
	if workspace == nil {
		return "", nil, false, domain.Fail(domain.CodeNotFound, "workspace is missing")
	}
	switch mode {
	case domain.RestoreFreeze:
		_, _, _, err := createSnapshots(catalog, domain.Action{Value: workspace.ID})
		if err != nil {
			return "", nil, false, err
		}
	case domain.RestorePull:
		workspace.Desired = workspace.Observed.Clone()
		workspace.Status, workspace.Drift = domain.WorkspaceOK, nil
	case domain.RestorePush:
		if err := requireManagedWorkspace(graph, workspace.ID); err != nil {
			return "", nil, false, err
		}
		snapshot := catalog.SnapshotByID(plan.SnapshotID)
		if snapshot == nil {
			return "", nil, false, domain.Fail(domain.CodeStalePlan, "snapshot is missing")
		}
		if err := c.pushSnapshot(ctx, graph, workspace, snapshot); err != nil {
			return "", nil, false, err
		}
	default:
		return "", nil, false, domain.Fail(domain.CodeValidation, "apply mode must be push, pull or freeze")
	}
	plan.Status, plan.AppliedAt, plan.AppliedSerial = domain.PlanApplied, catalog.Now, catalog.MutationSerial+1
	return "restore plan applied in " + string(mode) + " mode", nil, mode == domain.RestorePush, nil
}

func (c *Client) pushSnapshot(ctx context.Context, graph *runtimegraph.Graph, workspace *domain.Workspace, snapshot *domain.Snapshot) error {
	session := nativeWorkspace(graph, workspace.ID)
	if session == "" {
		return domain.Fail(domain.CodeProtected, "creating absent Zellij sessions requires a generated layout")
	}
	existing := map[string]bool{}
	for _, view := range graph.Views {
		if view.WorkspaceID == workspace.ID {
			existing[view.Name] = true
		}
	}
	for _, window := range snapshot.State.Windows {
		name := strings.TrimPrefix(window.ID, "window:")
		if existing[name] {
			continue
		}
		args := []string{"--session", session, "action", "new-tab", "--name", name}
		var cwd string
		if len(window.Surfaces) > 0 {
			cwd = window.Surfaces[0].CWD
		}
		if cwd != "" {
			args = append(args, "--cwd", cwd)
		}
		rawTabID, err := c.runOutput(ctx, args...)
		if err != nil {
			return err
		}
		tabID := strings.TrimSpace(string(rawTabID))
		if _, err := strconv.Atoi(tabID); err != nil {
			return domain.Fail(domain.CodeMalformedResult, "new-tab returned invalid tab id %q", tabID)
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		paneArgs := []string{"--session", session, "action", "new-pane", "--tab-id", tabID, "--no-focus"}
		if cwd != "" {
			paneArgs = append(paneArgs, "--cwd", cwd)
		}
		paneArgs = append(paneArgs, "--", shell, "-il")
		if err := c.run(ctx, paneArgs...); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) exportHistory(catalog *domain.Catalog, action domain.Action) (string, []string, bool, error) {
	if len(action.Selection) == 0 {
		return "", nil, false, domain.Fail(domain.CodeEmptySelection, "export requires selected history rows")
	}
	selected := map[string]bool{}
	for _, id := range action.Selection {
		selected[id] = true
	}
	var chunks []*domain.HistoryChunk
	scopes := map[string]bool{}
	rows := 0
	for _, chunk := range catalog.History {
		if selected[chunk.ID] {
			chunks = append(chunks, chunk)
			rows += len(chunk.Lines)
			scopes[string(chunk.Scope.Kind)+":"+chunk.Scope.ID] = true
		}
	}
	if rows == 0 {
		return "", nil, false, domain.Fail(domain.CodeEmptySelection, "selection matched no history")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, false, err
	}
	dir := filepath.Join(home, ".local", "state", "tmh", "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, false, err
	}
	filename := fmt.Sprintf("tmh-history-rev%d.json", catalog.Revision+1)
	raw, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		return "", nil, false, err
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", nil, false, err
	}
	catalog.LastExport = &domain.ExportDescriptor{At: catalog.Now, Rows: rows, Scopes: sortedStrings(scopes), Filename: path}
	return fmt.Sprintf("exported %d rows to %s", rows, path), nil, false, nil
}

func resolveAttach(graph *runtimegraph.Graph, catalog *domain.Catalog, target domain.ResourceRef) (string, string, error) {
	if target.Kind == domain.KindTerminal {
		_, session, pane, err := resolveTerminal(graph, catalog, target)
		return session, pane, err
	}
	if target.Kind != domain.KindWorkspace {
		return "", "", domain.Fail(domain.CodeBadTarget, "attach targets workspace or terminal")
	}
	if catalog.WorkspaceByID(target.ID) == nil {
		return "", "", domain.Fail(domain.CodeNotFound, "workspace %s not found", target.ID)
	}
	session := nativeWorkspace(graph, target.ID)
	if session == "" {
		return "", "", domain.Fail(domain.CodeNotLive, "workspace has no live Zellij session")
	}
	return session, "", nil
}

func resolveTerminal(graph *runtimegraph.Graph, catalog *domain.Catalog, target domain.ResourceRef) (*domain.Terminal, string, string, error) {
	terminal := catalog.TerminalByID(target.ID)
	if target.Kind != domain.KindTerminal || terminal == nil {
		return nil, "", "", domain.Fail(domain.CodeNotFound, "terminal %s not found", target.ID)
	}
	if terminal.State != domain.TerminalLive {
		return nil, "", "", domain.Fail(domain.CodeNotLive, "terminal %s is not live", terminal.ID)
	}
	for _, runtimeTerminal := range graph.Terminals {
		if runtimeTerminal.ID != terminal.ID {
			continue
		}
		parts := strings.Split(runtimeTerminal.NativeID, "/")
		if len(parts) != 4 || parts[0] != "session" || parts[1] == "" || parts[2] != "terminal" || parts[3] == "" {
			return nil, "", "", domain.Fail(domain.CodeMalformedResult, "terminal native binding is malformed")
		}
		return terminal, parts[1], parts[3], nil
	}
	return nil, "", "", domain.Fail(domain.CodeNotLive, "terminal binding is no longer observed")
}

func terminalTabID(graph *runtimegraph.Graph, terminalID string) (string, error) {
	for _, surface := range graph.Surfaces {
		if surface.TerminalID != terminalID {
			continue
		}
		view := graph.ViewByID(surface.ViewID)
		if view == nil || view.NativeID == "" {
			return "", domain.Fail(domain.CodeMalformedResult, "terminal view binding is malformed")
		}
		return view.NativeID, nil
	}
	return "", domain.Fail(domain.CodeNotFound, "terminal %s has no runtime surface", terminalID)
}

func nativeWorkspace(graph *runtimegraph.Graph, id string) string {
	for _, workspace := range graph.Workspaces {
		if workspace.ID == id {
			return workspace.NativeID
		}
	}
	return ""
}

func requireManagedTerminal(graph *runtimegraph.Graph, terminalID string) error {
	for _, surface := range graph.Surfaces {
		if surface.TerminalID != terminalID {
			continue
		}
		view := graph.ViewByID(surface.ViewID)
		if view == nil {
			break
		}
		return requireManagedWorkspace(graph, view.WorkspaceID)
	}
	return domain.Fail(domain.CodeNotLive, "terminal %s has no live workspace", terminalID)
}

func requireManagedWorkspace(graph *runtimegraph.Graph, workspaceID string) error {
	workspace := graph.WorkspaceByID(workspaceID)
	if workspace == nil {
		return domain.Fail(domain.CodeNotLive, "workspace %s is not observed", workspaceID)
	}
	if workspace.Management != runtimegraph.ManagementManaged {
		return domain.Fail(domain.CodeProtected, "workspace %s is observed but not managed by tmh", workspace.Name)
	}
	return nil
}

func verifyRuntimeEffect(action domain.Action, before, after *domain.Catalog) error {
	switch action.Kind {
	case domain.ActionSplit:
		terminal := before.TerminalByID(action.Target.ID)
		if terminal == nil {
			return domain.Fail(domain.CodeNotFound, "split terminal disappeared")
		}
		if liveTerminalCount(after, terminal.WorkspaceID) <= liveTerminalCount(before, terminal.WorkspaceID) {
			return domain.Fail(domain.CodeInvalidState, "Zellij accepted split but no persistent pane was observed; attach the session and retry")
		}
	case domain.ActionKill:
		if terminal := after.TerminalByID(action.Target.ID); terminal != nil && terminal.State == domain.TerminalLive {
			return domain.Fail(domain.CodeInvalidState, "Zellij accepted close but the pane is still live")
		}
	}
	return nil
}

func liveTerminalCount(catalog *domain.Catalog, workspaceID string) int {
	count := 0
	for _, terminal := range catalog.Terminals {
		if terminal.WorkspaceID == workspaceID && terminal.State == domain.TerminalLive {
			count++
		}
	}
	return count
}

func workspaceForTarget(catalog *domain.Catalog, target domain.ResourceRef) *domain.Workspace {
	if target.Kind == domain.KindWorkspace {
		return catalog.WorkspaceByID(target.ID)
	}
	if target.Kind == domain.KindTerminal {
		if terminal := catalog.TerminalByID(target.ID); terminal != nil {
			return catalog.WorkspaceByID(terminal.WorkspaceID)
		}
	}
	return nil
}

func appendHistory(catalog *domain.Catalog, kind domain.ResourceKind, id string, lines []string) {
	seq := 1
	for _, chunk := range catalog.History {
		if chunk.Scope.Kind == kind && chunk.Scope.ID == id && chunk.Seq >= seq {
			seq = chunk.Seq + 1
		}
	}
	catalog.History = append(catalog.History, &domain.HistoryChunk{
		ID: fmt.Sprintf("history-%s-%04d", id, seq), Scope: domain.HistoryScope{Kind: kind, ID: id},
		Seq: seq, At: catalog.Now, Lines: append([]string(nil), lines...), Kind: "stream",
	})
}

func commandKey(action domain.Action) string {
	raw, _ := json.Marshal(action)
	sum := sha256.Sum256(raw)
	return "cmd-" + hex.EncodeToString(sum[:16])
}

func actionDetail(action domain.Action) string {
	raw, _ := json.Marshal(action)
	return string(raw)
}

func auditRef(action domain.Action) domain.ResourceRef {
	if action.Target.Kind != "" && action.Target.ID != "" {
		return action.Target
	}
	return domain.Ref(domain.KindConfig, "control")
}

func categoryFor(kind domain.ResourceKind) domain.EventCategory {
	switch kind {
	case domain.KindWorkspace, domain.KindActive:
		return domain.EventWorkspace
	case domain.KindTerminal:
		return domain.EventTerminal
	case domain.KindAgent:
		return domain.EventAgent
	case domain.KindSnapshot:
		return domain.EventSnapshot
	case domain.KindPlan:
		return domain.EventRestore
	case domain.KindBackend:
		return domain.EventBackend
	case domain.KindMachine:
		return domain.EventMachine
	case domain.KindConfig:
		return domain.EventConfig
	default:
		return domain.EventSystem
	}
}

func currentUser() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "operator"
}

func nextID(prefix string, ids []string) string {
	maxID := 0
	for _, id := range ids {
		value, err := strconv.Atoi(strings.TrimPrefix(id, prefix+"-"))
		if err == nil && value > maxID {
			maxID = value
		}
	}
	return fmt.Sprintf("%s-%04d", prefix, maxID+1)
}

func snapshotIDs(catalog *domain.Catalog) []string {
	ids := make([]string, 0, len(catalog.Snapshots))
	for _, item := range catalog.Snapshots {
		ids = append(ids, item.ID)
	}
	return ids
}

func planIDs(catalog *domain.Catalog) []string {
	ids := make([]string, 0, len(catalog.RestorePlans))
	for _, item := range catalog.RestorePlans {
		ids = append(ids, item.ID)
	}
	return ids
}
