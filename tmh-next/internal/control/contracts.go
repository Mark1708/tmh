// Package control defines the application-facing control-plane contracts.
// Transport clients and the deterministic demo implement these interfaces;
// the Bubble Tea shell depends only on this package and domain snapshots.
package control

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh-next/internal/domain"
)

type Snapshot struct {
	Revision uint64          `json:"revision"`
	EventSeq uint64          `json:"event_seq"`
	Catalog  *domain.Catalog `json:"catalog"`
}

func (s Snapshot) Validate() error {
	if s.Catalog == nil {
		return fmt.Errorf("snapshot catalog is nil")
	}
	if s.Catalog.Revision != s.Revision {
		return fmt.Errorf("snapshot revision %d differs from catalog revision %d", s.Revision, s.Catalog.Revision)
	}
	if err := domain.ValidateCatalog(s.Catalog); err != nil {
		return fmt.Errorf("snapshot catalog: %w", err)
	}
	return nil
}

type EventKind string

const (
	EventCatalogChanged EventKind = "catalog.changed"
	EventBackendState   EventKind = "backend.state"
	EventCommandResult  EventKind = "command.result"
	EventHeartbeat      EventKind = "heartbeat"
)

type WatchEvent struct {
	Seq          uint64               `json:"seq"`
	BaseRevision uint64               `json:"base_revision"`
	Revision     uint64               `json:"revision"`
	Kind         EventKind            `json:"kind"`
	Changed      []domain.ResourceRef `json:"changed,omitempty"`
	CommandID    string               `json:"command_id,omitempty"`
}

func (e WatchEvent) Validate() error {
	if e.Seq == 0 {
		return fmt.Errorf("watch event sequence must be positive")
	}
	switch e.Kind {
	case EventCatalogChanged, EventBackendState, EventCommandResult:
		if e.Revision < e.BaseRevision {
			return fmt.Errorf("watch event revision %d precedes base revision %d", e.Revision, e.BaseRevision)
		}
	case EventHeartbeat:
		if e.Revision != e.BaseRevision {
			return fmt.Errorf("heartbeat cannot change revision")
		}
	default:
		return fmt.Errorf("unknown watch event kind %q", e.Kind)
	}
	return nil
}

// Client is the request/response boundary required by the TUI. A production
// implementation speaks the versioned daemon protocol; the demo client wraps
// the deterministic in-memory backend.
type Client interface {
	Snapshot(context.Context) (Snapshot, error)
	Search(context.Context, domain.SearchQuery) (domain.SearchResult, error)
	Execute(context.Context, domain.Action) (domain.MutationResult, error)
}

// Watcher is the optional streaming extension. Keeping it separate permits
// deterministic clients without goroutines while the root transitions from
// demo ticks to daemon events.
type Watcher interface {
	Watch(context.Context, uint64) (<-chan WatchEvent, <-chan error)
}

// DemoTicker is intentionally outside Client: production transports never
// expose mock time progression as a control-plane operation.
type DemoTicker interface {
	Tick(context.Context, uint64) (domain.MutationResult, error)
}
