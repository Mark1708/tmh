package actions

import (
	"context"
	"testing"

	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/tmux/tmuxtest"
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
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "epcp", Dir: "/tmp/lk", WindowName: "lk", Detached: true})
	_, _ = m.NewWindow(context.Background(), tmux.NewWindowOpts{SessionTarget: "epcp:", Name: "preview", Dir: "/tmp/preview"})

	cfg := parseConfig(t, `
version: 1
sessions:
  epcp:
    windows:
      lk: /tmp/lk
`)
	rep, err := Pull(context.Background(), m, cfg, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 1 || rep.Created[0] != "epcp/preview" {
		t.Fatalf("expected epcp/preview, got %+v", rep)
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
		Name: "epcp", Dir: "/Users/mark/Projects/otr/products/epcp/repos/lk",
		WindowName: "lk", Detached: true,
	})
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{
		Name: "kb", Dir: "/Users/mark/Projects/me/products/kb/bases/claude",
		WindowName: "claude", Detached: true,
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
		"/Users/mark/Projects/otr/x/a",
		"/Users/mark/Projects/otr/x/b",
		"/Users/mark/Projects/me/y/a",
		"/Users/mark/Projects/me/y/b",
	)
	roots := inferRoots(snap)
	// expect 2 clusters
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %+v", roots)
	}
	wantBases := map[string]bool{
		"/Users/mark/Projects/otr": true,
		"/Users/mark/Projects/me":  true,
	}
	for _, base := range roots {
		if !wantBases[base] {
			t.Fatalf("unexpected root base %q in %+v", base, roots)
		}
	}
}
