package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

func TestSnapshot_SaveAndRestore(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", WindowName: "w", Dir: "/tmp", Detached: true})
	_, _ = m.NewWindow(context.Background(), tmux.NewWindowOpts{SessionTarget: "s:", Name: "w2", Dir: "/tmp/2"})

	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := SaveSnapshot(context.Background(), m, db, "before"); err != nil {
		t.Fatal(err)
	}

	if err := m.KillSession(context.Background(), "s"); err != nil {
		t.Fatal(err)
	}
	if exists, _ := m.HasSession(context.Background(), "s"); exists {
		t.Fatal("kill failed")
	}

	if _, err := RestoreSnapshot(context.Background(), m, db, "before"); err != nil {
		t.Fatal(err)
	}
	if exists, _ := m.HasSession(context.Background(), "s"); !exists {
		t.Fatal("session not restored")
	}
	wins, _ := m.ListWindows(context.Background(), "s")
	if len(wins) != 2 {
		t.Fatalf("expected 2 windows after restore, got %d", len(wins))
	}
}

func TestUndo_RestoresKilledSession(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", WindowName: "w", Dir: "/tmp", Detached: true})

	db, _ := state.Open(":memory:")
	defer db.Close()

	// Capture pre-kill snapshot and record an event mimicking what `tmh kill`
	// would do.
	live, _ := CaptureLive(context.Background(), m)
	if len(live) != 1 || live[0].Name != "s" {
		t.Fatalf("unexpected live snapshot: %+v", live)
	}
	payload := mustJSON(live[0])
	if _, err := db.InsertEvent(context.Background(), "kill_session", "s", payload); err != nil {
		t.Fatal(err)
	}
	_ = m.KillSession(context.Background(), "s")

	target, err := UndoLast(context.Background(), m, db)
	if err != nil {
		t.Fatal(err)
	}
	if target != "s" {
		t.Fatalf("undo target = %q", target)
	}
	if exists, _ := m.HasSession(context.Background(), "s"); !exists {
		t.Fatal("undo did not restore session")
	}
}

func TestUndo_UnsupportedKind(t *testing.T) {
	db, _ := state.Open(":memory:")
	defer db.Close()
	_, _ = db.InsertEvent(context.Background(), "rename", "x->y", `{}`)
	_, err := UndoLast(context.Background(), tmuxtest.New(), db)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected unsupported error, got %v", err)
	}
}

func TestUndo_NothingToUndo(t *testing.T) {
	db, _ := state.Open(":memory:")
	defer db.Close()
	_, err := UndoLast(context.Background(), tmuxtest.New(), db)
	if err == nil || !errors.Is(err, err) {
		// just ensure error returned
		t.Fatal("expected error on empty events")
	}
}

func mustJSON(v any) string {
	b, err := jsonMarshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// jsonMarshal indirected so the helper can be reused in other tests later
// without forcing a new import block on each one.
func jsonMarshal(v any) ([]byte, error) {
	return jsonMarshalImpl(v)
}
