package actions

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatch_DebouncesMultipleWrites(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(target, []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	events := make(chan WatchEvent, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- Watch(ctx, []string{target}, events, io.Discard) }()

	// Give the watcher a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Simulate an atomic-save burst: write twice within the debounce window.
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(target, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}

	select {
	case ev := <-events:
		if ev.Kind != "zshrc" {
			t.Fatalf("unexpected kind: %v", ev)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("no event received")
	}

	// No second event should arrive within the debounce window for the
	// quiet period that follows.
	select {
	case extra := <-events:
		t.Fatalf("got unexpected additional event: %+v", extra)
	case <-time.After(400 * time.Millisecond):
		// good — debounce collapsed the burst
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch did not exit after cancel")
	}
}

func TestClassifyWatchPath(t *testing.T) {
	tests := map[string]string{
		"/a/.zshrc":            "zshrc",
		"/a/.tmux.conf":        "tmuxconf",
		"/a/tmh/config.yml":    "config",
		"/somewhere/other.txt": "config",
	}
	for path, want := range tests {
		if got := classifyWatchPath(path); got != want {
			t.Errorf("classify(%q) = %q, want %q", path, got, want)
		}
	}
}
