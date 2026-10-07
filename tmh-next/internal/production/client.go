package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mark1708/tmh-next/internal/backend/zellij"
	"github.com/mark1708/tmh-next/internal/configsource"
	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

type Discoverer interface {
	Discover(context.Context) (*runtimegraph.Graph, error)
}

type CatalogStore interface {
	LoadCatalog(context.Context) (*domain.Catalog, error)
	SaveCatalog(context.Context, uint64, *domain.Catalog) error
	SaveMutationResult(context.Context, uint64, *domain.Catalog, string, domain.MutationResult) error
	CommandResult(context.Context, string) (domain.MutationResult, bool, error)
}

type Options struct {
	Discoverer       Discoverer
	Runner           zellij.Runner
	Store            CatalogStore
	ConfigPath       string
	CaptureHistory   bool
	ZellijBinary     string
	OperationTimeout time.Duration
	Now              func() time.Time
}

type Client struct {
	mu               sync.Mutex
	discoverer       Discoverer
	runner           zellij.Runner
	store            CatalogStore
	configPath       string
	captureHistory   bool
	zellijBinary     string
	operationTimeout time.Duration
	now              func() time.Time
	subs             map[uint64]chan control.WatchEvent
	subErrors        map[uint64]chan error
	nextSub          uint64
}

func NewClient(options Options) (*Client, error) {
	if options.Discoverer == nil {
		return nil, fmt.Errorf("production client: discoverer is nil")
	}
	if options.Store == nil {
		return nil, fmt.Errorf("production client: store is nil")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.ZellijBinary == "" {
		options.ZellijBinary = "zellij"
	}
	if options.OperationTimeout <= 0 {
		options.OperationTimeout = 30 * time.Second
	}
	return &Client{
		discoverer: options.Discoverer, runner: options.Runner, store: options.Store,
		configPath: options.ConfigPath, captureHistory: options.CaptureHistory,
		zellijBinary: options.ZellijBinary, operationTimeout: options.OperationTimeout, now: options.Now,
		subs: make(map[uint64]chan control.WatchEvent), subErrors: make(map[uint64]chan error),
	}, nil
}

func (c *Client) Snapshot(ctx context.Context) (control.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	catalog, event, err := c.reconcileLocked(ctx)
	if err != nil {
		return control.Snapshot{}, err
	}
	if event != nil {
		c.publishLocked(*event)
	}
	return control.Snapshot{Revision: catalog.Revision, EventSeq: eventSequence(catalog), Catalog: catalog.Clone()}, nil
}

func (c *Client) reconcileLocked(ctx context.Context) (*domain.Catalog, *control.WatchEvent, error) {
	previous, err := c.store.LoadCatalog(ctx)
	if err != nil {
		return nil, nil, err
	}
	desired, err := c.loadDesired()
	if err != nil {
		return nil, nil, err
	}
	now := c.now().UTC()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, nil, ctxErr
	}
	graph, discoveryErr := c.discoverer.Discover(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, nil, ctxErr
	}
	if discoveryErr != nil {
		graph = unavailableGraph()
	}
	if discoveryErr == nil {
		markConfiguredManaged(graph, desired)
	}
	candidate, err := ProjectRuntime(previous, graph, desired, now)
	if err != nil {
		return nil, nil, err
	}
	if discoveryErr != nil {
		for _, backend := range candidate.Backends {
			backend.Status = domain.BackendUnavailable
			backend.UnsupportedReasons = []string{discoveryErr.Error()}
		}
	}
	if discoveryErr == nil && c.captureHistory {
		c.captureScreens(ctx, candidate, graph)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, nil, ctxErr
	}
	if previous != nil && runtimeEqual(previous, candidate) {
		copy := previous.Clone()
		copy.Now = now
		return copy, nil, nil
	}

	base := uint64(0)
	if previous != nil {
		base = previous.Revision
	}
	candidate.Revision = base + 1
	candidate.Now = now
	message := "runtime discovered"
	severity := domain.SeverityInfo
	if discoveryErr != nil {
		message = "runtime discovery unavailable"
		severity = domain.SeverityWarn
	}
	appendAudit(candidate, domain.EventSystem, severity, domain.Ref(domain.KindBackend, candidate.Backends[0].ID), message, errorDetail(discoveryErr))
	if err := domain.ValidateCatalog(candidate); err != nil {
		return nil, nil, fmt.Errorf("reconciled catalog: %w", err)
	}
	if err := c.store.SaveCatalog(ctx, base, candidate); err != nil {
		return nil, nil, err
	}
	event := &control.WatchEvent{
		Seq: eventSequence(candidate), BaseRevision: base, Revision: candidate.Revision,
		Kind: control.EventCatalogChanged, Changed: []domain.ResourceRef{domain.Ref(domain.KindBackend, candidate.Backends[0].ID)},
	}
	return candidate, event, nil
}

func (c *Client) loadDesired() (map[string]domain.WorkspaceState, error) {
	if c.configPath == "" {
		return map[string]domain.WorkspaceState{}, nil
	}
	desired, err := loadDesiredConfig(c.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]domain.WorkspaceState{}, nil
	}
	return desired, err
}

func (c *Client) Search(ctx context.Context, query domain.SearchQuery) (domain.SearchResult, error) {
	snapshot, err := c.Snapshot(ctx)
	if err != nil {
		return domain.SearchResult{}, err
	}
	catalog := snapshot.Catalog
	limit := query.Limit
	if limit <= 0 {
		limit = 30
	}
	text := strings.ToLower(strings.TrimSpace(query.Text))
	var hits []domain.SearchHit
	add := func(scope string, ref domain.ResourceRef, title, snippet string) {
		if len(hits) >= limit {
			return
		}
		if text != "" && !strings.Contains(strings.ToLower(title), text) && !strings.Contains(strings.ToLower(snippet), text) {
			return
		}
		hits = append(hits, domain.SearchHit{Ref: ref, Title: title, Snippet: snippet, Scope: scope})
	}
	if query.Scope == "all" || query.Scope == "" || query.Scope == "agents" {
		for _, agent := range catalog.Agents {
			add("agents", domain.Ref(domain.KindAgent, agent.ID), agent.Name+" ("+string(agent.State)+")", fmt.Sprintf("%s agent on %s", agent.Kind, agent.WorkspaceID))
		}
	}
	if query.Scope == "all" || query.Scope == "" || query.Scope == "history" {
		for _, chunk := range catalog.History {
			add("history", domain.Ref(domain.KindHistory, chunk.ID), fmt.Sprintf("stream %s:%s", chunk.Scope.Kind, chunk.Scope.ID), strings.Join(chunk.Lines, " / "))
		}
	}
	if query.Scope == "all" || query.Scope == "" || query.Scope == "events" {
		for _, event := range catalog.Events {
			add("events", domain.Ref(domain.KindEvent, event.ID), event.Message, event.Detail)
		}
	}
	if query.Scope == "all" || query.Scope == "" {
		for _, workspace := range catalog.Workspaces {
			add("workspaces", domain.Ref(domain.KindWorkspace, workspace.ID), workspace.Name, fmt.Sprintf("workspace via %s (%s)", workspace.BackendID, workspace.Status))
		}
		for _, terminal := range catalog.Terminals {
			add("terminals", domain.Ref(domain.KindTerminal, terminal.ID), terminal.Name, terminal.Process+" "+terminal.CWD)
		}
	}
	return domain.SearchResult{Revision: catalog.Revision, Hits: hits}, nil
}

func (c *Client) Execute(ctx context.Context, action domain.Action) (domain.MutationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	result, event, err := c.executeLocked(ctx, action)
	if err != nil {
		return domain.MutationResult{}, err
	}
	if event != nil {
		c.publishLocked(*event)
	}
	return result, nil
}

func (c *Client) Watch(ctx context.Context, after uint64) (<-chan control.WatchEvent, <-chan error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextSub++
	id := c.nextSub
	events := make(chan control.WatchEvent, 64)
	errs := make(chan error, 1)
	c.subs[id], c.subErrors[id] = events, errs
	if catalog, err := c.store.LoadCatalog(ctx); err != nil {
		errs <- err
	} else if catalog != nil && eventSequence(catalog) > after {
		events <- control.WatchEvent{
			Seq: eventSequence(catalog), BaseRevision: catalog.Revision,
			Revision: catalog.Revision, Kind: control.EventHeartbeat,
		}
	}
	go func() {
		<-ctx.Done()
		c.mu.Lock()
		delete(c.subs, id)
		delete(c.subErrors, id)
		close(events)
		close(errs)
		c.mu.Unlock()
	}()
	return events, errs
}

func (c *Client) publishLocked(event control.WatchEvent) {
	for id, subscriber := range c.subs {
		select {
		case subscriber <- event:
		default:
			if errorsChannel := c.subErrors[id]; errorsChannel != nil {
				select {
				case errorsChannel <- fmt.Errorf("watch subscriber fell behind"):
				default:
				}
			}
		}
	}
}

func runtimeEqual(left, right *domain.Catalog) bool {
	type observed struct {
		Workspaces  []*domain.Workspace
		ActiveViews []*domain.ActiveView
		Terminals   []*domain.Terminal
		Agents      []*domain.Agent
		Backends    []*domain.Backend
		Machines    []*domain.Machine
		History     []*domain.HistoryChunk
		Snapshots   []*domain.Snapshot
		Plans       []*domain.RestorePlan
		Config      domain.Config
	}
	return reflect.DeepEqual(
		observed{left.Workspaces, left.ActiveViews, left.Terminals, left.Agents, left.Backends, left.Machines, left.History, left.Snapshots, left.RestorePlans, left.Config},
		observed{right.Workspaces, right.ActiveViews, right.Terminals, right.Agents, right.Backends, right.Machines, right.History, right.Snapshots, right.RestorePlans, right.Config},
	)
}

func eventSequence(catalog *domain.Catalog) uint64 {
	if catalog == nil || len(catalog.Events) == 0 {
		return 0
	}
	return uint64(catalog.Events[len(catalog.Events)-1].Seq)
}

func appendAudit(catalog *domain.Catalog, category domain.EventCategory, severity domain.EventSeverity, resource domain.ResourceRef, message, detail string) {
	seq := int(eventSequence(catalog)) + 1
	catalog.Events = append(catalog.Events, &domain.Event{
		ID: fmt.Sprintf("event-%08d", seq), Seq: seq, At: catalog.Now,
		Category: category, Severity: severity, Resource: resource, Message: message, Detail: detail,
	})
}

func errorDetail(err error) string {
	if err == nil {
		return "{}"
	}
	raw, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(raw)
}

func unavailableGraph() *runtimegraph.Graph {
	return &runtimegraph.Graph{
		Machines: []*runtimegraph.Machine{{ID: "machine-local", Name: "local", Kind: runtimegraph.MachineLocal}},
		BackendInstances: []*runtimegraph.BackendInstance{{
			ID: "backend-zellij", MachineID: "machine-local", Kind: runtimegraph.BackendZellij,
			Capabilities: []string{"sessions", "panes"},
		}},
	}
}

func markConfiguredManaged(graph *runtimegraph.Graph, desired map[string]domain.WorkspaceState) {
	if graph == nil {
		return
	}
	for _, workspace := range graph.Workspaces {
		if _, configured := desired[workspace.Name]; configured {
			workspace.Management = runtimegraph.ManagementManaged
		}
	}
}

func sortedStrings(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

var loadDesiredConfig = configsource.LoadDesired
