package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
)

func TestSQLiteCatalogCompareAndSwapPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	ctx := context.Background()

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	catalog := storeTestCatalog(1)
	if err := db.SaveCatalog(ctx, 0, catalog); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	if err := db.SaveCatalog(ctx, 0, catalog); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save error = %v, want ErrConflict", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	got, err := db.LoadCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Revision != 1 || got.Config.RetentionHours != 168 {
		t.Fatalf("reloaded catalog = %#v", got)
	}
}

func TestSQLiteCommandResultIsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	result := domain.MutationResult{BaseRevision: 3, NewRevision: 4, Message: "done"}
	if err := db.SaveCommandResult(ctx, "cmd-1", result); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.CommandResult(ctx, "cmd-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.NewRevision != 4 || got.Message != "done" {
		t.Fatalf("command result = %+v, %v", got, ok)
	}
}

func TestSQLiteMutationCommitsCatalogAndResultTogether(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	initial := storeTestCatalog(1)
	if err := db.SaveCatalog(ctx, 0, initial); err != nil {
		t.Fatal(err)
	}
	next := initial.Clone()
	next.Revision = 2
	result := domain.MutationResult{BaseRevision: 1, NewRevision: 2, Catalog: next.Clone(), Message: "committed"}
	if err := db.SaveMutationResult(ctx, 1, next, "cmd-atomic", result); err != nil {
		t.Fatal(err)
	}
	stored, err := db.LoadCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.CommandResult(ctx, "cmd-atomic")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != 2 || !ok || got.NewRevision != 2 {
		t.Fatalf("catalog/result = rev %d, %+v, %v", stored.Revision, got, ok)
	}
}

func storeTestCatalog(revision uint64) *domain.Catalog {
	return &domain.Catalog{
		Revision: revision,
		Now:      time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC),
		Scenario: domain.Scenario("production"),
		Machines: []*domain.Machine{{
			ID: "machine-local", Name: "local", Kind: "local", Status: domain.MachineOnline,
		}},
		Backends: []*domain.Backend{{
			ID: "backend-zellij", Name: "Zellij", Status: domain.BackendHealthy,
			Enabled: true, Default: true,
		}},
		Config: domain.Config{
			DefaultPage: "dashboard", LeaderDisplay: domain.LeaderPath,
			HistoryMode: domain.HistoryPersistent, RetentionHours: 168,
			Overflow: domain.OverflowArchiveOldest, RedactSecrets: true,
			RemoteTrust: domain.TrustLocalOnly,
		},
	}
}
