package app

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/mock"
	"github.com/mark1708/tmh-next/internal/ui"
)

type snapshotClient struct {
	snapshot control.Snapshot
	calls    int
}

func (c *snapshotClient) Snapshot(context.Context) (control.Snapshot, error) {
	c.calls++
	return c.snapshot, nil
}

func (*snapshotClient) Search(context.Context, domain.SearchQuery) (domain.SearchResult, error) {
	return domain.SearchResult{}, nil
}

func (*snapshotClient) Execute(context.Context, domain.Action) (domain.MutationResult, error) {
	return domain.MutationResult{}, nil
}

func TestRootInitializesThroughRuntimeClientSnapshot(t *testing.T) {
	catalog, err := mock.Fixture(domain.ScenarioDefault)
	if err != nil {
		t.Fatal(err)
	}
	client := &snapshotClient{snapshot: control.Snapshot{
		Revision: catalog.Revision, EventSeq: 9, Catalog: catalog,
	}}
	root := NewRoot(Options{
		Client: client, Stack: []ui.Location{{Route: ui.RouteDashboard}},
	})

	message := root.Init()()
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init returned %T, want tea.BatchMsg", message)
	}
	var loaded *domain.Catalog
	for _, command := range batch {
		if command == nil {
			continue
		}
		if result, ok := command().(LoadCatalogMsg); ok {
			loaded = result.Catalog
		}
	}
	if client.calls != 1 {
		t.Fatalf("snapshot calls = %d, want 1", client.calls)
	}
	if loaded == nil || loaded.Revision != catalog.Revision {
		t.Fatalf("loaded catalog = %#v", loaded)
	}
}
