package actions

import (
	"context"
	"testing"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/tmux/tmuxtest"
)

func TestAttach_OutsideTmux(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "epcp", Detached: true})
	m.Reset()
	if err := Attach(context.Background(), m, "epcp"); err != nil {
		t.Fatal(err)
	}
	names := m.MethodNames()
	if len(names) != 1 || names[0] != "AttachSession" {
		t.Fatalf("got %v", names)
	}
}

func TestAttach_InsideTmux_SwitchesClient(t *testing.T) {
	m := tmuxtest.New()
	m.SetInTmux(true)
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "epcp", Detached: true})
	m.Reset()
	if err := Attach(context.Background(), m, "epcp"); err != nil {
		t.Fatal(err)
	}
	names := m.MethodNames()
	if len(names) != 1 || names[0] != "SwitchClient" {
		t.Fatalf("got %v", names)
	}
}

func TestCreateSession_TwoPaneLayout(t *testing.T) {
	m := tmuxtest.New()
	sess := config.ResolvedSession{
		Name: "s", Dir: "/tmp",
		Windows: []config.ResolvedWindow{{Name: "w", Dir: "/tmp", Layout: "2-pane"}},
	}
	if err := CreateSession(context.Background(), m, sess, nil); err != nil {
		t.Fatal(err)
	}
	panes, _ := m.ListPanes(context.Background(), "s:1")
	if len(panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(panes))
	}
}

func TestCreateSession_ThreePaneLayout(t *testing.T) {
	m := tmuxtest.New()
	sess := config.ResolvedSession{
		Name: "s", Dir: "/tmp",
		Windows: []config.ResolvedWindow{{Name: "w", Dir: "/tmp", Layout: "3-pane"}},
	}
	if err := CreateSession(context.Background(), m, sess, nil); err != nil {
		t.Fatal(err)
	}
	panes, _ := m.ListPanes(context.Background(), "s:1")
	if len(panes) != 3 {
		t.Fatalf("expected 3 panes, got %d", len(panes))
	}
}

func TestCreateSession_SkipsExisting(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", Detached: true})
	m.Reset()
	if err := CreateSession(context.Background(), m, config.ResolvedSession{Name: "s"}, nil); err != nil {
		t.Fatal(err)
	}
	names := m.MethodNames()
	if len(names) != 1 || names[0] != "HasSession" {
		t.Fatalf("got %v", names)
	}
}

func TestInit_BulkCreate(t *testing.T) {
	m := tmuxtest.New()
	src := `
version: 1
sessions:
  a:
    windows:
      w1: /tmp/a
  b:
    windows:
      w1: /tmp/b
`
	cfg := parseConfig(t, src)
	if err := Init(context.Background(), m, cfg, InitOptions{}); err != nil {
		t.Fatal(err)
	}
	sessions, _ := m.ListSessions(context.Background())
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestInit_OnlyFilter(t *testing.T) {
	m := tmuxtest.New()
	src := `
version: 1
sessions:
  a:
    windows: {w: /tmp/a}
  b:
    windows: {w: /tmp/b}
`
	cfg := parseConfig(t, src)
	if err := Init(context.Background(), m, cfg, InitOptions{Only: []string{"b"}}); err != nil {
		t.Fatal(err)
	}
	sessions, _ := m.ListSessions(context.Background())
	if len(sessions) != 1 || sessions[0].Name != "b" {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}
}

func TestKillMatching_Substring(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "epcp", Detached: true})
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "epcp-preview", Detached: true})
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "kb", Detached: true})
	killed, err := KillMatching(context.Background(), m, "epcp")
	if err != nil {
		t.Fatal(err)
	}
	if len(killed) != 2 {
		t.Fatalf("expected 2 killed, got %v", killed)
	}
	sessions, _ := m.ListSessions(context.Background())
	if len(sessions) != 1 || sessions[0].Name != "kb" {
		t.Fatalf("remaining: %+v", sessions)
	}
}

func TestBuildListing_MergesConfigAndLive(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "epcp", Detached: true})
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "scratch", Detached: true})
	src := `
version: 1
sessions:
  epcp:
    windows:
      lk: /tmp/lk
  kb:
    windows:
      root: /tmp/kb
`
	cfg := parseConfig(t, src)
	listing, err := BuildListing(context.Background(), m, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ListedSession{}
	for _, s := range listing.Sessions {
		byName[s.Name] = s
	}
	if !byName["epcp"].Live || !byName["epcp"].Configured {
		t.Fatalf("epcp should be both live and configured: %+v", byName["epcp"])
	}
	if byName["kb"].Live || !byName["kb"].Configured {
		t.Fatalf("kb should be configured-only: %+v", byName["kb"])
	}
	if !byName["scratch"].Live || byName["scratch"].Configured {
		t.Fatalf("scratch should be ad-hoc: %+v", byName["scratch"])
	}
}

// --- helpers ---

func parseConfig(t *testing.T, src string) *config.Config {
	t.Helper()
	c, err := config.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return c
}
