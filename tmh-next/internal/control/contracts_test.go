package control_test

import (
	"testing"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/mock"
)

func TestSnapshotValidationBindsEnvelopeToCatalogRevision(t *testing.T) {
	catalog, err := mock.Fixture(domain.ScenarioDefault)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := control.Snapshot{Revision: catalog.Revision, EventSeq: 11, Catalog: catalog}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}

	snapshot.Revision++
	if err := snapshot.Validate(); err == nil {
		t.Fatal("revision mismatch accepted")
	}
}

func TestWatchEventValidationRejectsRevisionRegression(t *testing.T) {
	event := control.WatchEvent{Seq: 3, BaseRevision: 7, Revision: 8, Kind: control.EventCatalogChanged}
	if err := event.Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	event.Revision = 6
	if err := event.Validate(); err == nil {
		t.Fatal("revision regression accepted")
	}
}
