package actions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

func TestScratch_CreateAndSweep(t *testing.T) {
	m := tmuxtest.New()
	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// TTL 1 second.
	name, err := CreateScratch(context.Background(), m, db, ScratchOptions{
		Dir: "/tmp", TTL: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !IsScratchName(name) {
		t.Fatalf("name not prefixed: %q", name)
	}

	// Forge an "old" event so the sweep treats it as expired.
	events, _ := db.EventsByKind(context.Background(), scratchTTLKind)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	var p scratchPayload
	_ = json.Unmarshal([]byte(events[0].Payload), &p)
	p.CreatedAt = time.Now().Add(-2 * time.Second).Unix()
	patched, _ := json.Marshal(p)
	_ = db.DeleteEvent(context.Background(), events[0].ID)
	_, _ = db.InsertEvent(context.Background(), scratchTTLKind, name, string(patched))

	killed, err := SweepExpiredScratch(context.Background(), m, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(killed) != 1 || killed[0] != name {
		t.Fatalf("expected sweep to kill %q, got %v", name, killed)
	}
	exists, _ := m.HasSession(context.Background(), name)
	if exists {
		t.Fatal("session should have been killed")
	}
}

func TestScratch_NoTTL(t *testing.T) {
	m := tmuxtest.New()
	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	name, err := CreateScratch(context.Background(), m, db, ScratchOptions{Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	events, _ := db.EventsByKind(context.Background(), scratchTTLKind)
	if len(events) != 0 {
		t.Fatalf("no TTL event expected, got %d", len(events))
	}
	exists, _ := m.HasSession(context.Background(), name)
	if !exists {
		t.Fatal("scratch session should exist")
	}
}
