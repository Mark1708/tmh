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
