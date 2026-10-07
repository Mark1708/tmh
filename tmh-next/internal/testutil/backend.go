package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
)

// RecordingBackend is a deterministic fake over an injected base catalog: it
// records every call, accepts any action with a correct revision chain and
// supports failure injection for conflict/error paths.
type RecordingBackend struct {
	mu         sync.Mutex
	cat        *domain.Catalog
	nextRev    uint64
	loads      int
	searches   []domain.SearchQuery
	ticks      int
	actions    []domain.Action
	execErr    error
	tickErr    error
	searchRevs []uint64
	tickForce  *uint64
}

// NewRecordingBackend builds a recording backend over a deep copy of base.
func NewRecordingBackend(base *domain.Catalog) *RecordingBackend {
	return &RecordingBackend{cat: base.Clone()}
}

// SetExecErr injects the next Execute error.
func (r *RecordingBackend) SetExecErr(err error) { r.mu.Lock(); defer r.mu.Unlock(); r.execErr = err }

// ClearExecErr removes the injected Execute error.
func (r *RecordingBackend) ClearExecErr() { r.SetExecErr(nil) }

// SetExecRevsForTick pins the revision the next Tick expects, forcing a
// conflict when it differs from the catalog.
func (r *RecordingBackend) SetExecRevsForTick(rev uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tickForce = &rev
}

// SetSearchRevs pins the revision sequence returned by successive searches.
func (r *RecordingBackend) SetSearchRevs(revs ...uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.searchRevs = revs
}

// Load supports focused mock-backend tests.
func (r *RecordingBackend) Load(_ context.Context, _ domain.Scenario) (domain.Catalog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads++
	return *r.cat.Clone(), nil
}

// Snapshot implements control.Client.
func (r *RecordingBackend) Snapshot(_ context.Context) (control.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads++
	catalog := r.cat.Clone()
	return control.Snapshot{Revision: catalog.Revision, Catalog: catalog}, nil
}

// Search implements control.Client.
func (r *RecordingBackend) Search(_ context.Context, q domain.SearchQuery) (domain.SearchResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.searches = append(r.searches, q)
	rev := r.cat.Revision
	if len(r.searchRevs) > 0 {
		rev = r.searchRevs[0]
		r.searchRevs = r.searchRevs[1:]
	}
	return domain.SearchResult{Revision: rev, Hits: []domain.SearchHit{
		{Ref: domain.Ref(domain.KindWorkspace, "base"), Title: "hit for " + q.Text, Snippet: "snippet", Scope: "workspaces"},
	}}, nil
}

// Execute implements control.Client with a one-step revision chain.
func (r *RecordingBackend) Execute(_ context.Context, a domain.Action) (domain.MutationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, a)
	if r.execErr != nil {
		err := r.execErr
		r.execErr = nil // one-shot
		return domain.MutationResult{}, err
	}
	base := r.cat.Revision
	cat := r.cat.Clone()
	cat.Revision = base + 1
	r.cat = cat // commit so the next action chains from the new revision
	return domain.MutationResult{
		BaseRevision: base,
		NewRevision:  cat.Revision,
		Catalog:      cat.Clone(),
		Action:       a,
		Message:      fmt.Sprintf("mock %s accepted", a.Kind),
	}, nil
}

// Tick implements control.DemoTicker with revision validation.
func (r *RecordingBackend) Tick(_ context.Context, expected uint64) (domain.MutationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ticks++
	base := r.cat.Revision
	if r.tickForce != nil {
		base = *r.tickForce
		r.tickForce = nil
	}
	if expected != base {
		return domain.MutationResult{}, domain.ErrRevisionConflict
	}
	cat := r.cat.Clone()
	cat.Revision = base + 1
	r.cat = cat // commit the tick chain
	return domain.MutationResult{
		BaseRevision: base,
		NewRevision:  cat.Revision,
		Catalog:      cat.Clone(),
		IsTick:       true,
		Message:      "tick accepted",
	}, nil
}

// Loads returns the Load call count.
func (r *RecordingBackend) Loads() int { r.mu.Lock(); defer r.mu.Unlock(); return r.loads }

// Ticks returns the Tick call count.
func (r *RecordingBackend) Ticks() int { r.mu.Lock(); defer r.mu.Unlock(); return r.ticks }

// Actions returns every recorded action in order.
func (r *RecordingBackend) Actions() []domain.Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Action, len(r.actions))
	copy(out, r.actions)
	return out
}

// Searches returns every recorded query in order.
func (r *RecordingBackend) Searches() []domain.SearchQuery {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.SearchQuery, len(r.searches))
	copy(out, r.searches)
	return out
}

// GatedBackend wraps a RecordingBackend with blocking gates so tests can
// hold Execute or Tick in flight and prove the mutation lane never overlaps.
type GatedBackend struct {
	// Inner is the wrapped recording backend, exposed for test injection.
	Inner *RecordingBackend

	execGate chan struct{}
	tickGate chan struct{}

	mu         sync.Mutex
	inFlight   map[string]int
	overlapped map[string]int
}

// NewGatedBackend wraps inner with both gates open.
func NewGatedBackend(inner *RecordingBackend) *GatedBackend {
	open := func() chan struct{} { ch := make(chan struct{}); close(ch); return ch }
	return &GatedBackend{
		Inner:      inner,
		execGate:   open(),
		tickGate:   open(),
		inFlight:   map[string]int{},
		overlapped: map[string]int{},
	}
}

// CloseExecGate blocks Execute until ReleaseExecGate.
func (g *GatedBackend) CloseExecGate() { g.execGate = make(chan struct{}) }

// ReleaseExecGate unblocks Execute.
func (g *GatedBackend) ReleaseExecGate() { close(g.execGate) }

// CloseTickGate blocks Tick until ReleaseTickGate.
func (g *GatedBackend) CloseTickGate() { g.tickGate = make(chan struct{}) }

// ReleaseTickGate unblocks Tick.
func (g *GatedBackend) ReleaseTickGate() { close(g.tickGate) }

// Overlaps returns per-lane overlap counts; any non-zero is a violation.
func (g *GatedBackend) Overlaps() map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]int, len(g.overlapped))
	for k, v := range g.overlapped {
		out[k] = v
	}
	return out
}

func (g *GatedBackend) enter(lane string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inFlight[lane]++
	if g.inFlight[lane] > 1 {
		g.overlapped[lane]++
	}
}

func (g *GatedBackend) leave(lane string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inFlight[lane]--
}

// Load forwards to the inner backend.
func (g *GatedBackend) Load(ctx context.Context, sc domain.Scenario) (domain.Catalog, error) {
	return g.Inner.Load(ctx, sc)
}

// Snapshot forwards to the inner backend.
func (g *GatedBackend) Snapshot(ctx context.Context) (control.Snapshot, error) {
	return g.Inner.Snapshot(ctx)
}

// Search forwards to the inner backend.
func (g *GatedBackend) Search(ctx context.Context, q domain.SearchQuery) (domain.SearchResult, error) {
	return g.Inner.Search(ctx, q)
}

// Execute gates and forwards.
func (g *GatedBackend) Execute(ctx context.Context, a domain.Action) (domain.MutationResult, error) {
	g.enter("action")
	defer g.leave("action")
	<-g.execGate
	return g.Inner.Execute(ctx, a)
}

// Tick gates and forwards.
func (g *GatedBackend) Tick(ctx context.Context, expected uint64) (domain.MutationResult, error) {
	g.enter("tick")
	defer g.leave("tick")
	<-g.tickGate
	return g.Inner.Tick(ctx, expected)
}
