package state

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// activeInput is a minimal ActiveWindowInput builder for tests. All metadata
// fields are non-empty so tests focus on lifecycle behaviour rather than data.
func activeInput(serverKey, epoch, windowID string) ActiveWindowInput {
	return ActiveWindowInput{
		ServerKey:         serverKey,
		ServerEpoch:       epoch,
		WindowID:          windowID,
		SourceSessionID:   "$1",
		SourceSessionName: "src",
		WindowName:        "w-" + windowID,
	}
}

func TestActive_UpsertInsertThenReUpsertPreservesPromotedAt(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	t1 := time.Unix(1_000, 0)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), t1, 5*time.Hour); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	got, err := firstActive(d, "e", "@1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.PromotedAt.Equal(t1) {
		t.Fatalf("promoted_at = %v, want %v", got.PromotedAt, t1)
	}
	if got.State != ActiveWindowTracked {
		t.Fatalf("state = %q, want tracked", got.State)
	}
	if got.LastSelectedAt.Equal(t1) == false {
		t.Fatalf("last_selected_at = %v, want %v", got.LastSelectedAt, t1)
	}

	// Re-promote later: promoted_at preserved, expiry extended, tracked reset.
	t2 := t1.Add(2 * time.Hour)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), t2, 5*time.Hour); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got, err = firstActive(d, "e", "@1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.PromotedAt.Equal(t1) {
		t.Fatalf("promoted_at changed = %v, want preserved %v", got.PromotedAt, t1)
	}
	if !got.LastSelectedAt.Equal(t2) {
		t.Fatalf("last_selected_at = %v, want %v", got.LastSelectedAt, t2)
	}
	wantExp := t2.Add(5 * time.Hour)
	if !got.ExpiresAt.Equal(wantExp) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, wantExp)
	}
}

func TestActive_UpsertRePromoteClearsOrphanedState(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	t0 := time.Unix(1_000, 0)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), t0, 5*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := d.MarkActiveWindowOrphaned(ctx, "e", "@1", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := firstActive(d, "e", "@1")
	if got.State != ActiveWindowOrphaned {
		t.Fatalf("expected orphaned before re-promote, got %q", got.State)
	}

	// Re-promote must reset to tracked and clear orphaned_at, keep promoted_at.
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), t0.Add(2*time.Hour), 5*time.Hour); err != nil {
		t.Fatal(err)
	}
	got, _ = firstActive(d, "e", "@1")
	if got.State != ActiveWindowTracked {
		t.Fatalf("state = %q, want tracked after re-promote", got.State)
	}
	if !got.OrphanedAt.IsZero() {
		t.Fatalf("orphaned_at = %v, want zero after re-promote", got.OrphanedAt)
	}
	if !got.PromotedAt.Equal(t0) {
		t.Fatalf("promoted_at = %v, want preserved %v", got.PromotedAt, t0)
	}
}

func TestActive_TouchUnknownIDDoesNotPromote(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	touched, err := d.TouchActiveWindow(ctx, "e", "@9", time.Unix(1_000, 0), time.Hour)
	if err != nil {
		t.Fatalf("touch unknown: %v", err)
	}
	if touched {
		t.Fatal("touched=true for unknown id, expected false")
	}
	ws, err := d.ListActiveWindows(ctx, "e")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 0 {
		t.Fatalf("touch created a row: %+v", ws)
	}
}

func TestActive_TouchUpdatesTimestamps(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	t0 := time.Unix(1_000, 0)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), t0, time.Hour); err != nil {
		t.Fatal(err)
	}
	t1 := t0.Add(30 * time.Minute)
	touched, err := d.TouchActiveWindow(ctx, "e", "@1", t1, 5*time.Hour)
	if err != nil {
		t.Fatalf("touch: %v", err)
	}
	if !touched {
		t.Fatal("touched=false for existing id")
	}
	got, _ := firstActive(d, "e", "@1")
	if !got.LastSelectedAt.Equal(t1) {
		t.Fatalf("last_selected_at = %v, want %v", got.LastSelectedAt, t1)
	}
	if !got.ExpiresAt.Equal(t1.Add(5 * time.Hour)) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, t1.Add(5*time.Hour))
	}
	// Touch must not flip promoted_at.
	if !got.PromotedAt.Equal(t0) {
		t.Fatalf("promoted_at = %v, want %v", got.PromotedAt, t0)
	}
}

func TestActive_TouchIgnoresOlderSelection(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	t0 := time.Unix(1_000, 0)
	newer := t0.Add(10 * time.Minute)
	older := t0.Add(5 * time.Minute)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), t0, time.Hour); err != nil {
		t.Fatal(err)
	}
	if touched, err := d.TouchActiveWindow(ctx, "e", "@1", newer, 5*time.Hour); err != nil || !touched {
		t.Fatalf("newer touch: touched=%v err=%v", touched, err)
	}
	if touched, err := d.TouchActiveWindow(ctx, "e", "@1", older, 5*time.Hour); err != nil || touched {
		t.Fatalf("older touch: touched=%v err=%v, want stale no-op", touched, err)
	}
	got, _ := firstActive(d, "e", "@1")
	if !got.LastSelectedAt.Equal(newer) {
		t.Fatalf("last_selected_at = %v, want %v", got.LastSelectedAt, newer)
	}
	if !got.ExpiresAt.Equal(newer.Add(5 * time.Hour)) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, newer.Add(5*time.Hour))
	}
}

func TestActive_ConcurrentTouchKeepsNewestShuffledSelection(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx := context.Background()
	base := time.Unix(1_000, 0)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), base, time.Hour); err != nil {
		t.Fatal(err)
	}

	offsets := []time.Duration{9, 1, 7, 3, 10, 2, 8, 4, 6, 5}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(len(offsets))
	errCh := make(chan error, len(offsets))
	for _, offset := range offsets {
		offset := offset
		go func() {
			defer wg.Done()
			<-start
			now := base.Add(offset * time.Minute)
			if _, err := d.TouchActiveWindow(ctx, "e", "@1", now, time.Hour); err != nil {
				errCh <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent touch: %v", err)
	}

	want := base.Add(10 * time.Minute)
	got, _ := firstActive(d, "e", "@1")
	if !got.LastSelectedAt.Equal(want) {
		t.Fatalf("last_selected_at = %v, want newest %v", got.LastSelectedAt, want)
	}
	if !got.ExpiresAt.Equal(want.Add(time.Hour)) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, want.Add(time.Hour))
	}
}

func TestActive_MarkOrphanedIdempotent(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	t0 := time.Unix(1_000, 0)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), t0, time.Hour); err != nil {
		t.Fatal(err)
	}
	markT := t0.Add(time.Hour)
	if err := d.MarkActiveWindowOrphaned(ctx, "e", "@1", markT); err != nil {
		t.Fatal(err)
	}
	// Second call later in time: orphaned_at must stay at the first mark.
	if err := d.MarkActiveWindowOrphaned(ctx, "e", "@1", markT.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := firstActive(d, "e", "@1")
	if got.State != ActiveWindowOrphaned {
		t.Fatalf("state = %q, want orphaned", got.State)
	}
	if !got.OrphanedAt.Equal(markT) {
		t.Fatalf("orphaned_at = %v, want idempotent %v", got.OrphanedAt, markT)
	}
}

func TestActive_MarkOrphanedOnAbsentRowIsNoOp(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	if err := d.MarkActiveWindowOrphaned(ctx, "e", "@9", time.Unix(1_000, 0)); err != nil {
		t.Fatalf("mark absent row should not error: %v", err)
	}
	ws, _ := d.ListActiveWindows(ctx, "e")
	if len(ws) != 0 {
		t.Fatalf("expected no rows, got %d", len(ws))
	}
}

func TestActive_DeleteIdempotent(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), time.Unix(1_000, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteActiveWindow(ctx, "e", "@1"); err != nil {
		t.Fatalf("delete existing: %v", err)
	}
	// Deleting again (now absent) must not error.
	if err := d.DeleteActiveWindow(ctx, "e", "@1"); err != nil {
		t.Fatalf("delete absent should be idempotent: %v", err)
	}
}

func TestActive_ListOrdersByPromotedAtThenWindowID(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	// Insert out of order; expect stable order (promoted_at ASC, window_id ASC).
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@3"), time.Unix(300, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@2"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	ws, err := d.ListActiveWindows(ctx, "e")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 3 {
		t.Fatalf("expected 3, got %d", len(ws))
	}
	want := []string{"@1", "@2", "@3"}
	for i, w := range ws {
		if w.WindowID != want[i] {
			t.Fatalf("ws[%d].WindowID = %q, want %q (order: %+v)", i, w.WindowID, want[i], ws)
		}
	}
}

func TestActive_ListIsScopedByEpoch(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e1", "@1"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e2", "@1"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	ws, _ := d.ListActiveWindows(ctx, "e1")
	if len(ws) != 1 || ws[0].ServerEpoch != "e1" {
		t.Fatalf("list leaked across epochs: %+v", ws)
	}
}

func TestActive_ListExpiredBoundary(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	now := time.Unix(1_000, 0)
	// @1 expires exactly at now (boundary: considered expired).
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), now.Add(-time.Hour), time.Hour); err != nil {
		t.Fatal(err)
	}
	// @2 expires in the future.
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@2"), now.Add(-time.Hour), 2*time.Hour); err != nil {
		t.Fatal(err)
	}
	exp, err := d.ListExpiredActiveWindows(ctx, "e", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(exp) != 1 || exp[0].WindowID != "@1" {
		t.Fatalf("expected only @1 expired, got %+v", exp)
	}
}

func TestActive_ListExpiredExcludesOrphansInOtherEpoch(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	now := time.Unix(1_000, 0)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), now.Add(-2*time.Hour), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "other", "@1"), now.Add(-2*time.Hour), time.Hour); err != nil {
		t.Fatal(err)
	}
	exp, _ := d.ListExpiredActiveWindows(ctx, "e", now)
	if len(exp) != 1 || exp[0].ServerEpoch != "e" {
		t.Fatalf("expired list leaked across epochs: %+v", exp)
	}
}

func TestActive_DeleteStaleEpochsScopedByServerKey(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()

	// server A: stale epoch e1 and current epoch e2.
	if err := d.UpsertActiveWindow(ctx, activeInput("A", "e1", "@1"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertActiveWindow(ctx, activeInput("A", "e2", "@2"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	// server B: any epoch must survive the A-scoped sweep.
	if err := d.UpsertActiveWindow(ctx, activeInput("B", "e3", "@3"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}

	n, err := d.DeleteStaleActiveEpochs(ctx, "A", "e2")
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	// A/e1 gone, A/e2 kept, B/e3 untouched.
	if ws, _ := d.ListActiveWindows(ctx, "e1"); len(ws) != 0 {
		t.Fatalf("stale epoch e1 should be gone, got %+v", ws)
	}
	if ws, _ := d.ListActiveWindows(ctx, "e2"); len(ws) != 1 {
		t.Fatalf("current epoch e2 should be kept, got %d", len(ws))
	}
	if ws, _ := d.ListActiveWindows(ctx, "e3"); len(ws) != 1 {
		t.Fatalf("other-server epoch e3 must survive, got %d", len(ws))
	}
}

func TestActive_DeleteStaleEpochsNoMatchReturnsZero(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	if err := d.UpsertActiveWindow(ctx, activeInput("A", "e2", "@1"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	n, err := d.DeleteStaleActiveEpochs(ctx, "A", "e2")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("deleted = %d, want 0", n)
	}
}

func TestActive_GenericEventsIsolation(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := d.TouchActiveWindow(ctx, "e", "@1", time.Unix(200, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := d.MarkActiveWindowOrphaned(ctx, "e", "@1", time.Unix(300, 0)); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteActiveWindow(ctx, "e", "@1"); err != nil {
		t.Fatal(err)
	}
	// Active lifecycle must not write to the generic undo events table.
	events, err := d.RecentEvents(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("active lifecycle leaked into events: %+v", events)
	}
}

func TestActive_ExistingDBMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	// Simulate a pre-active_windows database: open raw and create only the
	// legacy events table, then seed a sentinel row.
	raw, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := `
		CREATE TABLE IF NOT EXISTS events (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  ts INTEGER NOT NULL,
		  kind TEXT NOT NULL,
		  target TEXT NOT NULL,
		  payload TEXT NOT NULL
		);`
	if _, err := raw.Exec(legacySchema); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec("INSERT INTO events(ts, kind, target, payload) VALUES(1,'k','t','p')"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen via the real Open: additive migrate must create active_windows.
	d, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after migration: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx := context.Background()
	// Legacy data must survive the additive migration.
	events, err := d.RecentEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("legacy event lost after migration: got %d", len(events))
	}
	// The new table must be usable immediately.
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), time.Unix(100, 0), time.Hour); err != nil {
		t.Fatalf("upsert after migration: %v", err)
	}
	ws, err := d.ListActiveWindows(ctx, "e")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || ws[0].WindowID != "@1" {
		t.Fatalf("expected migrated active window @1, got %+v", ws)
	}
}

func TestActive_UpsertRejectsEmptyIdentity(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	cases := []struct {
		name string
		in   ActiveWindowInput
	}{
		{"empty server_key", ActiveWindowInput{ServerEpoch: "e", WindowID: "@1", SourceSessionID: "$1", SourceSessionName: "s", WindowName: "w"}},
		{"empty server_epoch", ActiveWindowInput{ServerKey: "k", WindowID: "@1", SourceSessionID: "$1", SourceSessionName: "s", WindowName: "w"}},
		{"empty window_id", ActiveWindowInput{ServerKey: "k", ServerEpoch: "e", SourceSessionID: "$1", SourceSessionName: "s", WindowName: "w"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := d.UpsertActiveWindow(ctx, tc.in, time.Unix(100, 0), time.Hour)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestActive_UpsertRejectsNonPositiveTTL(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), time.Unix(100, 0), 0); err == nil {
		t.Fatal("expected error for zero ttl")
	}
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), time.Unix(100, 0), -time.Second); err == nil {
		t.Fatal("expected error for negative ttl")
	}
}

func TestActive_CheckConstraintRejectsBogusState(t *testing.T) {
	d := openInMemory(t)
	ctx := context.Background()
	// Bypass the typed API to prove the DB-level CHECK constraint is active.
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO active_windows(
			server_key, server_epoch, window_id,
			source_session_id, source_session_name, window_name,
			promoted_at, last_selected_at, expires_at, state, orphaned_at)
		VALUES('k','e','@1','$1','s','w',1,1,1,'bogus',NULL)
	`)
	if err == nil {
		t.Fatal("CHECK constraint did not reject bogus state")
	}
}

func TestActive_ConcurrentTouchRace(t *testing.T) {
	// File-backed DB exercises WAL + busy_timeout across concurrent touches.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx := context.Background()
	base := time.Unix(1_000, 0)
	if err := d.UpsertActiveWindow(ctx, activeInput("k", "e", "@1"), base, time.Hour); err != nil {
		t.Fatal(err)
	}

	const goroutines = 8
	const touches = 25
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(gid int) {
			defer wg.Done()
			for j := 0; j < touches; j++ {
				now := base.Add(time.Duration(gid*touches+j) * time.Second)
				if _, err := d.TouchActiveWindow(ctx, "e", "@1", now, time.Hour); err != nil {
					errCh <- fmt.Errorf("g%d touch %d: %w", gid, j, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent touch failed: %v", err)
	}

	ws, err := d.ListActiveWindows(ctx, "e")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 {
		t.Fatalf("expected 1 window after concurrent touches, got %d", len(ws))
	}
	// Expiry must reflect the winning touch's sliding window.
	minExp := base.Add(time.Hour)
	if ws[0].ExpiresAt.Before(minExp) {
		t.Fatalf("expires_at = %v, want >= %v", ws[0].ExpiresAt, minExp)
	}
}

// firstActive returns the single active window for (epoch, windowID), failing
// the test if it is absent or not unique.
func firstActive(d *DB, epoch, windowID string) (ActiveWindow, error) {
	ws, err := d.ListActiveWindows(context.Background(), epoch)
	if err != nil {
		return ActiveWindow{}, err
	}
	for _, w := range ws {
		if w.WindowID == windowID {
			return w, nil
		}
	}
	return ActiveWindow{}, fmt.Errorf("active window %s/%s not found", epoch, windowID)
}
