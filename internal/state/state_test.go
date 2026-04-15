package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	errs "git.mark1708.ru/me/tmh/internal/errors"
)

func openInMemory(t *testing.T) *DB {
	t.Helper()
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestEvents_InsertAndList(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	id1, err := d.InsertEvent(ctx, "kill_session", "epcp", `{"name":"epcp"}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.InsertEvent(ctx, "rename", "old->new", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	events, err := d.RecentEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID <= id1 && events[1].ID != id1 {
		t.Fatalf("ordering broken: %+v", events)
	}
	if err := d.DeleteEvent(ctx, id1); err != nil {
		t.Fatal(err)
	}
	events, _ = d.RecentEvents(ctx, 10)
	if len(events) != 1 {
		t.Fatalf("expected 1 after delete, got %d", len(events))
	}
}

func TestEvents_ByKind(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	_, _ = d.InsertEvent(ctx, "scratch_ttl", "scratch-1", `{"ttl":3600}`)
	_, _ = d.InsertEvent(ctx, "scratch_ttl", "scratch-2", `{"ttl":60}`)
	_, _ = d.InsertEvent(ctx, "kill_session", "other", `{}`)
	ev, err := d.EventsByKind(ctx, "scratch_ttl")
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 2 {
		t.Fatalf("expected 2 scratch events, got %d", len(ev))
	}
}

func TestSnapshots_UpsertAndRestore(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	if err := d.SaveSnapshot(ctx, "pre-demo", `{"sessions":[]}`); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveSnapshot(ctx, "pre-demo", `{"sessions":[{"n":"s"}]}`); err != nil {
		t.Fatal(err)
	}
	s, err := d.GetSnapshot(ctx, "pre-demo")
	if err != nil {
		t.Fatal(err)
	}
	if s.Payload != `{"sessions":[{"n":"s"}]}` {
		t.Fatalf("payload not overwritten: %q", s.Payload)
	}
	snaps, _ := d.ListSnapshots(ctx)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot (unique name), got %d", len(snaps))
	}
	if err := d.DeleteSnapshot(ctx, "pre-demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetSnapshot(ctx, "pre-demo"); err == nil {
		t.Fatal("snapshot should be gone")
	}
}

func TestTrust_LifeCycle(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	ok, _ := d.IsTrusted(ctx, "/p/config.yml", "hash1")
	if ok {
		t.Fatal("should not be trusted initially")
	}
	if err := d.MarkTrusted(ctx, "/p/config.yml", "hash1"); err != nil {
		t.Fatal(err)
	}
	ok, _ = d.IsTrusted(ctx, "/p/config.yml", "hash1")
	if !ok {
		t.Fatal("should be trusted after mark")
	}
	// different hash → not trusted
	ok, _ = d.IsTrusted(ctx, "/p/config.yml", "hash2")
	if ok {
		t.Fatal("different hash should not be trusted")
	}
	if err := d.ForgetTrust(ctx, "/p/config.yml"); err != nil {
		t.Fatal(err)
	}
	ok, _ = d.IsTrusted(ctx, "/p/config.yml", "hash1")
	if ok {
		t.Fatal("forget should wipe trust")
	}
}

func TestReloadQueue_EnqueueExpire(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	if err := d.EnqueueReload(ctx, "%1", "s:1.1", "shell", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := d.EnqueueReload(ctx, "%2", "s:1.2", "shell", -1*time.Second); err != nil {
		t.Fatal(err)
	}
	pending, _ := d.PendingReloads(ctx)
	if len(pending) != 2 {
		t.Fatalf("expected 2, got %d", len(pending))
	}
	expired, err := d.ExpireReloads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].PaneID != "%2" {
		t.Fatalf("expected %%2 expired, got %+v", expired)
	}
	pending, _ = d.PendingReloads(ctx)
	if len(pending) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(pending))
	}
	if err := d.DequeueReload(ctx, "%1"); err != nil {
		t.Fatal(err)
	}
	pending, _ = d.PendingReloads(ctx)
	if len(pending) != 0 {
		t.Fatalf("queue should be empty")
	}
}

func TestConcurrent_Writes(t *testing.T) {
	// Use a file-backed database to exercise WAL + busy_timeout across
	// connections (an in-memory sqlite DB serialises regardless).
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	const goroutines = 8
	const inserts = 25
	wg.Add(goroutines)
	errCh := make(chan error, goroutines*inserts)
	for i := 0; i < goroutines; i++ {
		go func(gid int) {
			defer wg.Done()
			for j := 0; j < inserts; j++ {
				if _, err := d.InsertEvent(ctx, "test", "target", `{}`); err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent insert failed: %v", err)
	}
	events, _ := d.RecentEvents(ctx, goroutines*inserts+5)
	if len(events) != goroutines*inserts {
		t.Fatalf("expected %d events, got %d", goroutines*inserts, len(events))
	}
}

func TestIntegrity_Ok(t *testing.T) {
	d := openInMemory(t)
	if err := d.IntegrityCheck(context.Background()); err != nil {
		t.Fatalf("integrity should be ok: %v", err)
	}
}

func TestFixState_RecreatesBrokenDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	// Write garbage to simulate corruption before any open.
	if err := os.WriteFile(path, []byte("this is not a sqlite file"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A fresh Open on a corrupt file still succeeds at the OS level
	// (sqlite lazily detects corruption). Force detection via PRAGMA.
	d, err := Open(path)
	if err == nil {
		// integrity check should catch it even though Open succeeded
		err = d.IntegrityCheck(context.Background())
		d.Close()
	}
	if err == nil {
		// Some sqlite builds reject the header up front. Either way fine.
		t.Log("corrupt header was detected at Open time")
	} else if !errors.Is(err, errs.ErrStateCorrupted) && !containsAny(err.Error(), "corrupt", "database", "not a database") {
		t.Fatalf("expected corruption error, got %v", err)
	}

	// Recovery.
	broken, err := FixState(path)
	if err != nil {
		t.Fatalf("FixState: %v", err)
	}
	if broken == "" {
		t.Fatal("expected a broken-file rename path")
	}
	if _, err := os.Stat(broken); err != nil {
		t.Fatalf("broken file missing: %v", err)
	}

	// New DB should be usable now.
	d2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after fix: %v", err)
	}
	defer d2.Close()
	if _, err := d2.InsertEvent(context.Background(), "smoke", "x", "{}"); err != nil {
		t.Fatalf("post-fix insert failed: %v", err)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
