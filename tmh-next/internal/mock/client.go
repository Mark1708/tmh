package mock

import (
	"context"
	"sync/atomic"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
)

// Client adapts the deterministic backend to the production-shaped control
// boundary. Scenario selection is construction-time configuration rather than
// a wire-level Snapshot argument.
type Client struct {
	backend  *Backend
	scenario domain.Scenario
	eventSeq atomic.Uint64
}

func NewClient(scenario domain.Scenario) *Client {
	return &Client{backend: New(), scenario: scenario}
}

func (c *Client) Snapshot(ctx context.Context) (control.Snapshot, error) {
	catalog, err := c.backend.Load(ctx, c.scenario)
	if err != nil {
		return control.Snapshot{}, err
	}
	return control.Snapshot{Revision: catalog.Revision, EventSeq: c.eventSeq.Load(), Catalog: &catalog}, nil
}

func (c *Client) Search(ctx context.Context, query domain.SearchQuery) (domain.SearchResult, error) {
	return c.backend.Search(ctx, query)
}

func (c *Client) Execute(ctx context.Context, action domain.Action) (domain.MutationResult, error) {
	result, err := c.backend.Execute(ctx, action)
	if err == nil {
		c.eventSeq.Add(1)
	}
	return result, err
}

func (c *Client) Tick(ctx context.Context, expectedRevision uint64) (domain.MutationResult, error) {
	result, err := c.backend.Tick(ctx, expectedRevision)
	if err == nil {
		c.eventSeq.Add(1)
	}
	return result, err
}

// Backend exposes the deterministic store for focused demo tests only.
func (c *Client) Backend() *Backend { return c.backend }
