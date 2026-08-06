package actions

import (
	"context"
	"testing"

	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

// TestShowHook_IndexedSlot verifies that ShowHook works correctly with indexed slots.
func TestShowHook_IndexedSlot(t *testing.T) {
	ctx := context.Background()
	mock := tmuxtest.New()

	// Test with unset slot
	result, err := mock.ShowHook(ctx, "session-window-changed[1708]")
	if err != nil {
		t.Fatalf("ShowHook failed: %v", err)
	}
	if result != "" {
		t.Errorf("unset slot should return empty string, got %q", result)
	}

	// Test with exact slot match
	mock.Hooks()["session-window-changed[1708]"] = ActiveHookCommand
	result, err = mock.ShowHook(ctx, "session-window-changed[1708]")
	if err != nil {
		t.Fatalf("ShowHook failed: %v", err)
	}
	if result != ActiveHookCommand {
		t.Errorf("expected %q, got %q", ActiveHookCommand, result)
	}

	// Test with foreign command
	foreign := `run-shell "echo foreign"`
	mock.Hooks()["session-window-changed[1708]"] = foreign
	result, err = mock.ShowHook(ctx, "session-window-changed[1708]")
	if err != nil {
		t.Fatalf("ShowHook failed: %v", err)
	}
	if result != foreign {
		t.Errorf("expected %q, got %q", foreign, result)
	}
}

// TestShowHook_StandardHook verifies ShowHook works with standard hooks too.
func TestShowHook_StandardHook(t *testing.T) {
	ctx := context.Background()
	mock := tmuxtest.New()

	// Test standard hook without index
	mock.Hooks()["after-new-window"] = `run-shell "echo after"`
	result, err := mock.ShowHook(ctx, "after-new-window")
	if err != nil {
		t.Fatalf("ShowHook failed: %v", err)
	}
	if result != `run-shell "echo after"` {
		t.Errorf("expected %q, got %q", `run-shell "echo after"`, result)
	}

	// Test unset standard hook
	result, err = mock.ShowHook(ctx, "after-split-window")
	if err != nil {
		t.Fatalf("ShowHook failed: %v", err)
	}
	if result != "" {
		t.Errorf("unset standard hook should return empty string, got %q", result)
	}
}

// TestShowHook_DifferentIndexedSlots verifies different indices don't conflict.
func TestShowHook_DifferentIndexedSlots(t *testing.T) {
	ctx := context.Background()
	mock := tmuxtest.New()

	mock.Hooks()["session-window-changed[1708]"] = ActiveHookCommand
	mock.Hooks()["session-window-changed[1709]"] = `run-shell "echo other"`

	// First slot
	result, err := mock.ShowHook(ctx, "session-window-changed[1708]")
	if err != nil {
		t.Fatalf("ShowHook failed: %v", err)
	}
	if result != ActiveHookCommand {
		t.Errorf("expected %q, got %q", ActiveHookCommand, result)
	}

	// Second slot
	result, err = mock.ShowHook(ctx, "session-window-changed[1709]")
	if err != nil {
		t.Fatalf("ShowHook failed: %v", err)
	}
	if result != `run-shell "echo other"` {
		t.Errorf("expected %q, got %q", `run-shell "echo other"`, result)
	}
}
