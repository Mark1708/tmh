package pane

import (
	"testing"
	"time"
)

func TestProvider_SetAllAndGet(t *testing.T) {
	p := New(5 * time.Second)
	p.SetAll(map[string]Info{
		"atlas:0.0": {Command: "nvim", Path: "/home/mark"},
		"atlas:0.1": {Command: "zsh", Path: "/home/mark"},
	})
	got, ok := p.Get("atlas:0.0")
	if !ok {
		t.Fatal("expected cache hit for atlas:0.0")
	}
	if got.Command != "nvim" {
		t.Errorf("expected nvim, got %q", got.Command)
	}
}

func TestProvider_Get_ExpiredEntry(t *testing.T) {
	p := New(1 * time.Millisecond)
	p.SetAll(map[string]Info{"s:0.0": {Command: "nvim"}})
	time.Sleep(5 * time.Millisecond)
	_, ok := p.Get("s:0.0")
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestProvider_Invalidate(t *testing.T) {
	p := New(5 * time.Second)
	p.SetAll(map[string]Info{"s:0.0": {Command: "nvim"}})
	p.Invalidate()
	if _, ok := p.Get("s:0.0"); ok {
		t.Fatal("expected cache miss after Invalidate")
	}
}

func TestProvider_SetAll_ReplacesOldEntries(t *testing.T) {
	p := New(5 * time.Second)
	p.SetAll(map[string]Info{"s:0.0": {Command: "old"}})
	p.SetAll(map[string]Info{"s:0.0": {Command: "new"}, "s:0.1": {Command: "bash"}})
	got, ok := p.Get("s:0.0")
	if !ok || got.Command != "new" {
		t.Errorf("expected new, got %q (ok=%v)", got.Command, ok)
	}
}

func TestIsIdleShell(t *testing.T) {
	idle := []string{"zsh", "-zsh", "bash", "-bash", "sh", "-sh", "fish", "-fish"}
	for _, s := range idle {
		if !IsIdleShell(s) {
			t.Errorf("IsIdleShell(%q) should be true", s)
		}
	}
	busy := []string{"nvim", "claude", "node", ""}
	for _, s := range busy {
		if IsIdleShell(s) {
			t.Errorf("IsIdleShell(%q) should be false", s)
		}
	}
}

func TestProvider_ZeroTTL_NeverExpires(t *testing.T) {
	p := New(0) // zero TTL = never expires
	p.SetAll(map[string]Info{"s:0.0": {Command: "nvim"}})
	time.Sleep(2 * time.Millisecond)
	if _, ok := p.Get("s:0.0"); !ok {
		t.Fatal("expected cache hit with zero TTL")
	}
}
