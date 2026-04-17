package actions

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mark1708/tmh/internal/shell"

	"github.com/fsnotify/fsnotify"
)

// WatchEvent is emitted when a watched dotfile changes after the debounce.
type WatchEvent struct {
	Kind string // "zshrc" | "tmuxconf" | "config"
	Path string
}

// WatchPaths returns the default dotfiles tmh watch monitors. The shell
// rc-file is derived from $SHELL so bash and fish users see their own
// dotfile in the watchlist instead of a hard-coded ~/.zshrc.
func WatchPaths(configPath string) []string {
	home := os.Getenv("HOME")
	return []string{
		shell.DefaultRCFile(),
		filepath.Join(home, ".tmux.conf"),
		configPath,
	}
}

// Watch monitors paths and emits one WatchEvent per debounced change. The
// debounce window is 300ms, matching the plan §6 requirement.
//
// Watch returns when ctx is cancelled or the watcher errors. Progress is
// written to logOut (pass io.Discard to silence).
func Watch(ctx context.Context, paths []string, events chan<- WatchEvent, logOut io.Writer) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	defer w.Close()

	// fsnotify on macOS requires watching the file's parent directory for
	// atomic saves (vim writes a temp file and renames).
	dirs := make(map[string]struct{})
	for _, p := range paths {
		if p == "" {
			continue
		}
		dirs[filepath.Dir(p)] = struct{}{}
	}
	for d := range dirs {
		if err := w.Add(d); err != nil {
			fmt.Fprintln(logOut, "watch: cannot watch", d, err)
		}
	}

	pathSet := make(map[string]string, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		pathSet[p] = classifyWatchPath(p)
	}

	const debounce = 300 * time.Millisecond
	var timer *time.Timer
	var pendingKind, pendingPath string

	fire := func() {
		if pendingKind == "" {
			return
		}
		ev := WatchEvent{Kind: pendingKind, Path: pendingPath}
		pendingKind, pendingPath = "", ""
		select {
		case events <- ev:
		case <-ctx.Done():
		}
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return nil
		case err := <-w.Errors:
			if err == nil {
				return nil
			}
			fmt.Fprintln(logOut, "watch error:", err)
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			kind, matched := pathSet[ev.Name]
			if !matched {
				continue
			}
			pendingKind = kind
			pendingPath = ev.Name
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, fire)
		}
	}
}

func classifyWatchPath(p string) string {
	switch filepath.Base(p) {
	case ".zshrc":
		return "zshrc"
	case ".tmux.conf":
		return "tmuxconf"
	default:
		return "config"
	}
}
