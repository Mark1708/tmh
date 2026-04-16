// Package slogx initialises the application-wide structured logger.
//
// Usage:
//
//	slogx.Init()           // call once at program start
//	slog.Info("msg", "k", v)
//
// Logging is disabled (no-op) unless TMH_LOG is set. Supported values:
// "debug", "info", "warn", "error". Any non-empty value enables at least
// Info level.
//
// Log output goes to $XDG_STATE_HOME/tmh/tmh.log, rotated when the file
// exceeds 5 MB. Up to 3 rotated files are kept (tmh.log.1, .2, .3);
// older ones are removed.
package slogx

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"git.mark1708.ru/me/tmh/internal/xdg"
)

// Init reads TMH_LOG and configures the default slog logger accordingly.
// Safe to call multiple times — subsequent calls are no-ops.
var initialized bool

// Init sets up the global slog logger. Must be called before any slog usage.
func Init() {
	if initialized {
		return
	}
	initialized = true

	raw := strings.ToLower(strings.TrimSpace(os.Getenv("TMH_LOG")))
	if raw == "" {
		// Logging disabled — keep the default discard-level handler.
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: slog.LevelError + 100, // effectively disabled
		})))
		return
	}

	level := slog.LevelInfo
	switch raw {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logPath := xdg.LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		// Can't create dir — fall back to stderr.
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
		return
	}

	w := &rotatingWriter{path: logPath, maxBytes: 5 * 1024 * 1024, keep: 3}
	slog.SetDefault(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug,
	})))
}

// rotatingWriter is a size-capped io.Writer that rotates log files when the
// current file exceeds maxBytes. Up to keep rotated files are retained.
type rotatingWriter struct {
	path     string
	maxBytes int64
	keep     int
	f        *os.File
	size     int64
}

func (r *rotatingWriter) Write(p []byte) (int, error) {
	if r.f == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	if r.size+int64(len(p)) > r.maxBytes {
		r.rotate()
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingWriter) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	info, _ := f.Stat()
	if info != nil {
		r.size = info.Size()
	}
	r.f = f
	return nil
}

func (r *rotatingWriter) rotate() {
	if r.f != nil {
		_ = r.f.Close()
		r.f = nil
	}
	// Shift existing rotated files: .3 deleted, .2→.3, .1→.2, current→.1.
	for i := r.keep; i >= 1; i-- {
		src := r.numberedPath(i - 1)
		dst := r.numberedPath(i)
		if i == r.keep {
			_ = os.Remove(dst)
		}
		_ = os.Rename(src, dst)
	}
	r.size = 0
	_ = r.open()
}

// numberedPath returns the rotated filename. 0 → base path, n>0 → path.n.
func (r *rotatingWriter) numberedPath(n int) string {
	if n == 0 {
		return r.path
	}
	return r.path + "." + string(rune('0'+n))
}
