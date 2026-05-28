package actions

import (
	"context"
	"testing"

	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

func TestPush_CreatesMissing(t *testing.T) {
	m := tmuxtest.New()
	cfg := parseConfig(t, `
version: 1
sessions:
  a:
    windows:
      w1: /tmp/a
  b:
    windows:
      w1: /tmp/b
`)
	rep, err := Push(context.Background(), m, cfg, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 2 {
		t.Fatalf("expected 2 created, got %+v", rep)
	}
	sessions, _ := m.ListSessions(context.Background())
	if len(sessions) != 2 {
		t.Fatalf("expected 2 live sessions, got %d", len(sessions))
	}
}

func TestPush_AddsMissingWindowToExistingSession(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", Detached: true, WindowName: "first"})
	cfg := parseConfig(t, `
version: 1
sessions:
  s:
    windows:
      first: /tmp/a
      second: /tmp/b
`)
	rep, err := Push(context.Background(), m, cfg, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 1 || rep.Created[0] != "s/second" {
		t.Fatalf("expected s/second created, got %+v", rep)
	}
	wins, _ := m.ListWindows(context.Background(), "s")
	if len(wins) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(wins))
	}
}

func TestPush_DryRun(t *testing.T) {
	m := tmuxtest.New()
	cfg := parseConfig(t, `
version: 1
sessions:
  a:
    windows: {w: /tmp/a}
`)
	rep, err := Push(context.Background(), m, cfg, SyncOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 1 {
		t.Fatalf("%+v", rep)
	}
	sessions, _ := m.ListSessions(context.Background())
	if len(sessions) != 0 {
		t.Fatalf("dry-run should create nothing, got %d live", len(sessions))
	}
}

func TestPull_AddsNewWindowFromLive(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "atlas", Dir: "/tmp/web", WindowName: "web", Detached: true})
	_, _ = m.NewWindow(context.Background(), tmux.NewWindowOpts{SessionTarget: "atlas:", Name: "preview", Dir: "/tmp/preview"})

	cfg := parseConfig(t, `
version: 1
sessions:
  atlas:
    windows:
      web: /tmp/web
`)
	rep, err := Pull(context.Background(), m, cfg, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 1 || rep.Created[0] != "atlas/preview" {
		t.Fatalf("expected atlas/preview, got %+v", rep)
	}
}

func TestPull_SkipsDriftWithoutApplyAll(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", Dir: "/tmp/live", WindowName: "w", Detached: true})
	cfg := parseConfig(t, `
version: 1
sessions:
  s:
    windows:
      w: /tmp/config
`)
	rep, err := Pull(context.Background(), m, cfg, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Updated) != 0 || len(rep.Skipped) != 1 {
		t.Fatalf("%+v", rep)
	}
	rep, _ = Pull(context.Background(), m, cfg, SyncOptions{ApplyAll: true})
	if len(rep.Updated) != 1 {
		t.Fatalf("expected 1 updated with ApplyAll, got %+v", rep)
	}
}

func TestBootstrap_InfersRoots(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{
		Name: "alpha", Dir: "/home/user/work/orgA/services/api/repos/web",
		WindowName: "web", Detached: true,
	})
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{
		Name: "beta", Dir: "/home/user/work/personal/notes/bases/kb",
		WindowName: "notes", Detached: true,
	})

	cfg := parseConfig(t, `version: 1`)
	rep, err := Bootstrap(context.Background(), m, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) < 2 {
		t.Fatalf("expected ≥2 created, got %+v", rep)
	}
}

func TestInferRoots_LCP(t *testing.T) {
	snap := liveSnapshotFromPaths(
		"/home/user/work/orgA/x/a",
		"/home/user/work/orgA/x/b",
		"/home/user/work/personal/y/a",
		"/home/user/work/personal/y/b",
	)
	roots := inferRoots(snap)
	// expect 2 clusters
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %+v", roots)
	}
	wantBases := map[string]bool{
		"/home/user/work/orgA":     true,
		"/home/user/work/personal": true,
	}
	for _, base := range roots {
		if !wantBases[base] {
			t.Fatalf("unexpected root base %q in %+v", base, roots)
		}
	}
}
