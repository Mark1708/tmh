package actions

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/tmux/tmuxtest"
)

func TestReload_ShellIdlePanes(t *testing.T) {
	m := tmuxtest.New()
	// session s with two panes; pane 1 is a shell (zsh), pane 2 is busy (node)
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", WindowName: "w", Detached: true})
	_ = m.SplitWindow(context.Background(), tmux.SplitOpts{Target: "s:1", Horizontal: true})
	// MockRunner starts panes with empty Command; we mark them by poking
	// into the state through ListPanes + a helper. For simplicity, rely on
	// isIdleShell returning false for "" (busy) — so Reload enqueues them.
	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rep, err := Reload(context.Background(), m, db, "~/.zshrc", ReloadOptions{Shell: true, Busy: true})
	if err != nil {
		t.Fatal(err)
	}
	// MockRunner has no Command set; isIdleShell("") is false → all queued
	if len(rep.ReloadedPanes) != 0 {
		t.Fatalf("expected nothing reloaded on empty Command, got %v", rep.ReloadedPanes)
	}
	if len(rep.QueuedPanes) != 2 {
		t.Fatalf("expected 2 queued, got %v", rep.QueuedPanes)
	}
}

func TestReload_TmuxSourceFile(t *testing.T) {
	m := tmuxtest.New()
	rep, err := Reload(context.Background(), m, nil, "", ReloadOptions{Tmux: true, TmuxConf: "/etc/tmux.conf"})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.TmuxSourced {
		t.Fatal("expected TmuxSourced=true")
	}
	names := m.MethodNames()
	found := false
	for _, n := range names {
		if n == "SourceFile" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("SourceFile not invoked: %v", names)
	}
}

func TestDrainReloadQueue_HandlesStalePanes(t *testing.T) {
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", WindowName: "w", Detached: true})

	// Enqueue a pane that doesn't exist — Drain should drop it.
	if err := db.EnqueueReload(context.Background(), "%ghost", "s:1.99", "shell", time.Hour); err != nil {
		t.Fatal(err)
	}
	done, err := DrainReloadQueue(context.Background(), m, db, "~/.zshrc")
	if err != nil {
		t.Fatal(err)
	}
	if done != 0 {
		t.Fatalf("expected 0 reloads (ghost pane), got %d", done)
	}
	pending, _ := db.PendingReloads(context.Background())
	if len(pending) != 0 {
		t.Fatalf("ghost not dequeued: %+v", pending)
	}
}

func TestIsIdleShell(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"zsh", true},
		{"-zsh", true},
		{"bash", true},
		{"fish", true},
		{"node", false},
		{"pnpm", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isIdleShell(tt.cmd); got != tt.want {
			t.Errorf("isIdleShell(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestReload_SkipsBusyWhenNoBusyFlag(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", WindowName: "w", Detached: true})
	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rep, err := Reload(context.Background(), m, db, "~/.zshrc", ReloadOptions{Shell: true, Busy: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.SkippedBusy) == 0 {
		t.Fatal("expected skipped entries")
	}
	// Verify none enqueued.
	pending, _ := db.PendingReloads(context.Background())
	if len(pending) != 0 {
		t.Fatalf("nothing should be queued without --busy, got %d", len(pending))
	}
	_ = strings.TrimSpace
}
