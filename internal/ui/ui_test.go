package ui

import (
	"context"
	"strings"
	"testing"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/tmux/tmuxtest"
	"git.mark1708.ru/me/tmh/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"

	"git.mark1708.ru/me/tmh/internal/ui/toast"
)

func TestModel_RendersWithoutPanic(t *testing.T) {
	m := tmuxtest.New()
	_ = m.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", Detached: true})

	deps := Deps{
		Runner:     m,
		ConfigPath: "/tmp/c.yml",
		LoadConfig: func() (*config.Config, error) {
			return config.Parse([]byte("version: 1\nsessions:\n  s:\n    windows:\n      w: /tmp\n"))
		},
	}
	model := New(deps)
	// Drive a window-size message so the dashboard knows its dimensions.
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Force-load data synchronously by triggering the data command and
	// posting its result back through Update.
	cmd := model.loadDataCmd()
	if cmd == nil {
		t.Fatal("loadDataCmd returned nil")
	}
	msg := cmd()
	model.Update(msg)

	view := model.View()
	if view == "" {
		t.Fatal("View returned empty string")
	}
	if !strings.Contains(view, "tmh") {
		t.Fatalf("expected header to mention tmh:\n%s", view)
	}
}

func TestThemeCycle(t *testing.T) {
	got := theme.Cycle(theme.Mocha).Name
	if got != "macchiato" {
		t.Fatalf("expected macchiato after mocha, got %q", got)
	}
	last := theme.Cycle(theme.Latte).Name
	if last != "mocha" {
		t.Fatalf("cycle should wrap, got %q", last)
	}
}

// TestToast_TagCompare verifies that an old expiry Tick cannot dismiss a newer
// toast. Two back-to-back showToast calls should leave the second text visible
// even after the first expiry message arrives.
func TestToast_TagCompare(t *testing.T) {
	deps := Deps{
		ConfigPath: "/tmp/c.yml",
		LoadConfig: func() (*config.Config, error) { return &config.Config{Version: 1}, nil },
	}
	m := New(deps)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Show first toast — captures seq=1 in its Tick closure (cmd discarded).
	_ = m.showToast(toast.KindSuccess, "first")
	if m.toast != "first" {
		t.Fatalf("expected toast=first, got %q", m.toast)
	}
	seq1 := m.toastSeq // should be 1

	// Show second toast before first expires — seq bumped to 2.
	m.showToast(toast.KindSuccess, "second")
	if m.toast != "second" {
		t.Fatalf("expected toast=second after second show, got %q", m.toast)
	}
	if m.toastSeq == seq1 {
		t.Fatal("toastSeq should have incremented")
	}

	// Fire the expiry message with seq1 (the old Tick).
	m.Update(toastExpiredMsg{Seq: seq1})
	// Toast must NOT be cleared — the stale Tick should be ignored.
	if m.toast == "" {
		t.Fatal("stale Tick should not have dismissed the newer toast")
	}

	// Fire the expiry message with the current seq (new Tick).
	m.Update(toastExpiredMsg{Seq: m.toastSeq})
	if m.toast != "" {
		t.Fatalf("current Tick should have dismissed the toast, but got %q", m.toast)
	}
}

func TestDashboard_NavigationAndAttachTarget(t *testing.T) {
	mock := tmuxtest.New()
	_ = mock.NewSession(context.Background(), tmux.NewSessionOpts{Name: "s", WindowName: "w", Detached: true})

	listing := &actions.Listing{
		Sessions: []actions.ListedSession{{
			Name: "s", Live: true, Configured: true,
			Windows: []actions.ListedWindow{{Name: "w", Live: true, Configured: true}},
		}},
	}
	d := newDashboard(DefaultKeys(), theme.New(theme.Mocha), LoadStrings())
	d.Resize(80, 20)
	d.SetData(listing, nil)

	// Cursor on session row.
	if got := d.SelectedTarget(); got != "s" {
		t.Fatalf("expected session target, got %q", got)
	}

	// Move down → window row.
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := d.SelectedTarget(); got != "s:w" {
		t.Fatalf("expected s:w, got %q", got)
	}
}
