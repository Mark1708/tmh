package daemonapi_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/daemonapi"
	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/remote"
)

type serviceStub struct {
	catalog    *domain.Catalog
	events     chan control.WatchEvent
	executeErr error
}

func (s *serviceStub) Snapshot(context.Context) (control.Snapshot, error) {
	return control.Snapshot{Revision: s.catalog.Revision, EventSeq: 1, Catalog: s.catalog.Clone()}, nil
}
func (s *serviceStub) Search(context.Context, domain.SearchQuery) (domain.SearchResult, error) {
	return domain.SearchResult{Revision: s.catalog.Revision}, nil
}
func (s *serviceStub) Execute(_ context.Context, action domain.Action) (domain.MutationResult, error) {
	if s.executeErr != nil {
		return domain.MutationResult{}, s.executeErr
	}
	if action.ExpectedRevision != s.catalog.Revision {
		return domain.MutationResult{}, domain.ErrRevisionConflict
	}
	base := s.catalog.Revision
	s.catalog = s.catalog.Clone()
	s.catalog.Revision++
	return domain.MutationResult{BaseRevision: base, NewRevision: s.catalog.Revision, Catalog: s.catalog.Clone(), Action: action, Message: "ok"}, nil
}
func (s *serviceStub) Watch(ctx context.Context, _ uint64) (<-chan control.WatchEvent, <-chan error) {
	errs := make(chan error)
	return s.events, errs
}

func TestUnixHTTPRoundTripSnapshotExecuteAndWatch(t *testing.T) {
	catalog := &domain.Catalog{
		Revision: 1, Scenario: domain.Scenario("production"), Now: time.Now().UTC(),
		Machines: []*domain.Machine{{ID: "machine-local", Name: "local", Kind: "local", Status: domain.MachineOnline}},
		Backends: []*domain.Backend{{ID: "backend-zellij", Name: "Zellij", Status: domain.BackendHealthy, Enabled: true, Default: true}},
		Config:   domain.Config{DefaultPage: "dashboard", LeaderDisplay: domain.LeaderPath, HistoryMode: domain.HistoryPersistent, RetentionHours: 168, Overflow: domain.OverflowArchiveOldest, RedactSecrets: true, RemoteTrust: domain.TrustLocalOnly},
	}
	service := &serviceStub{catalog: catalog, events: make(chan control.WatchEvent, 1)}
	api, err := daemonapi.New(service)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("tmhd-test-%d.sock", time.Now().UnixNano()))
	defer os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: api.Handler()}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	client, err := remote.New(socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 1 {
		t.Fatalf("snapshot revision = %d", snapshot.Revision)
	}

	events, errs := client.Watch(ctx, snapshot.EventSeq)
	service.events <- control.WatchEvent{Seq: 2, BaseRevision: 1, Revision: 2, Kind: control.EventCatalogChanged}
	select {
	case event := <-events:
		if event.Seq != 2 {
			t.Fatalf("watch event = %+v", event)
		}
	case err := <-errs:
		t.Fatalf("watch error: %v", err)
	case <-ctx.Done():
		t.Fatal("watch timed out")
	}

	action := domain.Action{Kind: domain.ActionConfigReload, ExpectedRevision: 1}
	result, err := client.Execute(ctx, action)
	if err != nil {
		t.Fatal(err)
	}
	if result.NewRevision != 2 || result.Action.Kind != action.Kind {
		t.Fatalf("execute result = %+v", result)
	}

	service.executeErr = domain.Fail(domain.CodeProtected, "session is read-only")
	_, err = client.Execute(ctx, domain.Action{Kind: domain.ActionSend, ExpectedRevision: 2})
	if err == nil || err.Error() != "protected: session is read-only" {
		t.Fatalf("remote typed error = %v", err)
	}
}
