package tmuxtest

import (
	"context"
	"errors"
	"strings"
	"testing"

	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/tmux"
)

func TestMock_SessionLifecycle(t *testing.T) {
	ctx := context.Background()
	m := New()

	if err := m.NewSession(ctx, tmux.NewSessionOpts{Name: "atlas", Dir: "/tmp", Detached: true}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.HasSession(ctx, "atlas"); !ok {
		t.Fatal("HasSession false after NewSession")
	}
	if err := m.NewSession(ctx, tmux.NewSessionOpts{Name: "atlas"}); !errors.Is(err, errs.ErrSessionExists) {
		t.Fatalf("expected ErrSessionExists, got %v", err)
	}
	sessions, _ := m.ListSessions(ctx)
	if len(sessions) != 1 || sessions[0].Name != "atlas" {
		t.Fatalf("ListSessions: %+v", sessions)
	}
	if err := m.KillSession(ctx, "atlas"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.HasSession(ctx, "atlas"); ok {
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

func TestMock_WindowID(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "s", Detached: true})

	id, err := m.WindowID(ctx, "s:1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("got empty window id")
	}
	if !strings.HasPrefix(id, "@") {
		t.Fatalf("window id should start with @, got %q", id)
	}

	_, err = m.WindowID(ctx, "nonexistent:1")
	if err == nil {
		t.Fatal("expected error for non-existent window")
	}
}

func TestMock_ListWindowLinks(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "source", Detached: true, WindowName: "main"})
	_, _ = m.NewWindow(ctx, tmux.NewWindowOpts{SessionTarget: "source:", Name: "shared"})

	links, err := m.ListWindowLinks(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}

	mainID := ""
	for _, link := range links {
		if link.WindowName == "main" {
			mainID = link.WindowID
		}
	}
	if mainID == "" {
		t.Fatal("main window not found")
	}

	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "dest", Detached: true})
	_ = m.LinkWindow(ctx, mainID, "dest")

	links, err = m.ListWindowLinks(ctx)
	if err != nil {
		t.Fatalf("unexpected error after link: %v", err)
	}

	mainLinkCount := 0
	for _, link := range links {
		if link.WindowID == mainID {
			mainLinkCount++
		}
	}
	if mainLinkCount != 2 {
		t.Fatalf("expected 2 links of main window, got %d", mainLinkCount)
	}
}

func TestMock_LinkWindow(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "source", Detached: true})

	sourceID, err := m.WindowID(ctx, "source:1")
	if err != nil {
		t.Fatalf("unexpected error getting window ID: %v", err)
	}

	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "dest", Detached: true})
	err = m.LinkWindow(ctx, sourceID, "dest")
	if err != nil {
		t.Fatalf("unexpected error linking window: %v", err)
	}

	destWindows, _ := m.ListWindows(ctx, "dest")
	if len(destWindows) != 2 {
		t.Fatalf("expected 2 windows in dest after link, got %d", len(destWindows))
	}
}

func TestMock_UnlinkWindow(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "source", Detached: true})
	windows, _ := m.ListWindows(ctx, "source")
	sourceID := windows[0].Name

	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "dest", Detached: true})
	_ = m.LinkWindow(ctx, sourceID, "dest")

	err := m.UnlinkWindow(ctx, "dest", sourceID)
	if err != nil {
		t.Fatalf("unexpected error unlinking window: %v", err)
	}

	destWindows, _ := m.ListWindows(ctx, "dest")
	if len(destWindows) != 1 {
		t.Fatalf("expected 1 window in dest after unlink, got %d", len(destWindows))
	}

	sourceWindows, _ := m.ListWindows(ctx, "source")
	if len(sourceWindows) != 1 {
		t.Fatal("source window should still exist after unlink from dest")
	}
}

func TestMock_UnlinkWindow_LastLinkRefused(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "solo", Detached: true})

	soloID, err := m.WindowID(ctx, "solo:1")
	if err != nil {
		t.Fatalf("unexpected error getting window ID: %v", err)
	}

	err = m.UnlinkWindow(ctx, "solo", soloID)
	if err == nil {
		t.Fatal("expected error when unlinking last link")
	}
}

func TestMock_ServerEpoch(t *testing.T) {
	ctx := context.Background()
	m := New()

	epoch, err := m.ServerEpoch(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if epoch.ServerKey == "" {
		t.Fatal("server key should not be empty")
	}
	if epoch.Value == "" {
		t.Fatal("epoch value should not be empty")
	}

	m.server = false
	_, err = m.ServerEpoch(ctx)
	if err == nil {
		t.Fatal("expected error when server not running")
	}
}

func TestMock_SessionOptions(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "test", Detached: true})

	value, err := m.ShowSessionOption(ctx, "test", "@tmh-active-owner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "" {
		t.Fatalf("expected empty value for unset option, got %q", value)
	}

	err = m.SetSessionOption(ctx, "test", "@tmh-active-owner", "tmh/v1")
	if err != nil {
		t.Fatalf("unexpected error setting option: %v", err)
	}

	value, err = m.ShowSessionOption(ctx, "test", "@tmh-active-owner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "tmh/v1" {
		t.Fatalf("expected tmh/v1, got %q", value)
	}

	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "other", Detached: true})
	value, err = m.ShowSessionOption(ctx, "other", "@tmh-active-owner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "" {
		t.Fatal("session option should not leak to other sessions")
	}
}

func TestMock_NewSessionAppliesOptionsAtomically(t *testing.T) {
	ctx := context.Background()
	m := New()
	err := m.NewSession(ctx, tmux.NewSessionOpts{
		Name: "active",
		SessionOptions: map[string]string{
			"@tmh-active-owner": "tmh/v1",
		},
		Detached: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := m.ShowSessionOption(ctx, "active", "@tmh-active-owner")
	if err != nil || value != "tmh/v1" {
		t.Fatalf("created option = %q, err = %v", value, err)
	}
	calls := m.Calls()
	options, ok := calls[0].Args["sessionOptions"].(map[string]string)
	if !ok || options["@tmh-active-owner"] != "tmh/v1" {
		t.Fatalf("recorded options = %#v", calls[0].Args["sessionOptions"])
	}
}

func TestMock_LinkWindowUsesExplicitFreeIndex(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "source", Detached: true})
	windowID, _ := m.WindowID(ctx, "source:1")
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "dest", Detached: true})

	if err := m.LinkWindow(ctx, windowID, "dest:0"); err != nil {
		t.Fatal(err)
	}
	links, _ := m.ListWindowLinks(ctx)
	for _, link := range links {
		if link.SessionName == "dest" && link.WindowID == windowID {
			if link.WindowIndex != 0 {
				t.Fatalf("link index = %d, want 0", link.WindowIndex)
			}
			return
		}
	}
	t.Fatal("explicit-index link missing")
}

func TestMock_RenumberWindowsPreservesPhysicalID(t *testing.T) {
	ctx := context.Background()
	m := New()
	_ = m.NewSession(ctx, tmux.NewSessionOpts{Name: "source", Detached: true})
	windowID, _ := m.WindowID(ctx, "source:1")

	if err := m.RenumberWindows("source", 7); err != nil {
		t.Fatal(err)
	}
	links, _ := m.ListWindowLinks(ctx)
	if len(links) != 1 || links[0].WindowID != windowID || links[0].WindowIndex != 7 {
		t.Fatalf("renumbered links: %+v", links)
	}
}
