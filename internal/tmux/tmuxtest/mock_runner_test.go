package tmuxtest

import (
	"context"
	"errors"
	"testing"

	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/tmux"
)

func TestMock_SessionLifecycle(t *testing.T) {
	ctx := context.Background()
	m := New()

	if err := m.NewSession(ctx, tmux.NewSessionOpts{Name: "epcp", Dir: "/tmp", Detached: true}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.HasSession(ctx, "epcp"); !ok {
		t.Fatal("HasSession false after NewSession")
	}
	if err := m.NewSession(ctx, tmux.NewSessionOpts{Name: "epcp"}); !errors.Is(err, errs.ErrSessionExists) {
		t.Fatalf("expected ErrSessionExists, got %v", err)
	}
	sessions, _ := m.ListSessions(ctx)
	if len(sessions) != 1 || sessions[0].Name != "epcp" {
		t.Fatalf("ListSessions: %+v", sessions)
	}
	if err := m.KillSession(ctx, "epcp"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.HasSession(ctx, "epcp"); ok {
		t.Fatal("HasSession true after KillSession")
	}
}

func TestMock_WindowOps(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "s", Dir: "/tmp", Detached: true, WindowName: "first"})
	w, err := m.NewWindow(ctx, tmux.NewWindowOpts{SessionTarget: "s:", Name: "second", Dir: "/tmp/second"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Index != 2 {
		t.Fatalf("expected index 2, got %d", w.Index)
	}
	windows, _ := m.ListWindows(ctx, "s")
	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(windows))
	}
	if err := m.RenameWindow(ctx, "s:1", "renamed"); err != nil {
		t.Fatal(err)
	}
	windows, _ = m.ListWindows(ctx, "s")
	if windows[0].Name != "renamed" {
		t.Fatalf("rename did not apply: %+v", windows)
	}
}

func TestMock_SplitAndPanes(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "s", Dir: "/tmp", Detached: true})
	if err := m.SplitWindow(ctx, tmux.SplitOpts{Target: "s:1", Horizontal: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.SplitWindow(ctx, tmux.SplitOpts{Target: "s:1", Horizontal: false}); err != nil {
		t.Fatal(err)
	}
	panes, _ := m.ListPanes(ctx, "s:1")
	if len(panes) != 3 {
		t.Fatalf("expected 3 panes, got %d: %+v", len(panes), panes)
	}
}

func TestMock_CallRecording(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "s", Detached: true})
	_ = m.AttachSession(ctx, "s")
	names := m.MethodNames()
	if len(names) != 2 || names[0] != "NewSession" || names[1] != "AttachSession" {
		t.Fatalf("got call sequence %v", names)
	}
	m.Reset()
	if len(m.Calls()) != 0 {
		t.Fatal("Reset should clear calls")
	}
}

func TestMock_InTmuxToggle(t *testing.T) {
	m := New()
	if m.InTmux() {
		t.Fatal("default should be false")
	}
	m.SetInTmux(true)
	if !m.InTmux() {
		t.Fatal("SetInTmux(true) not honoured")
	}
}
