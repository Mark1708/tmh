package state

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mark1708/tmh/internal/xdg"
)

// HistoryEntry is one record in the persistent action history JSONL file.
type HistoryEntry struct {
	// Ts is the RFC3339 timestamp of the event.
	Ts string `json:"ts"`
	// Action is a short machine-readable action name (e.g. "kill_session").
	Action string `json:"action"`
	// Target identifies the primary object of the action (e.g. "epcp").
	Target string `json:"target,omitempty"`
	// Result is "ok" or "err".
	Result string `json:"result"`
	// Details is a human-readable description or error message.
	Details string `json:"details,omitempty"`
}

// HistoryOptions tunes persistence behaviour. Zero value produces safe defaults.
type HistoryOptions struct {
	// MaxEntries caps the file at this many records (default 1000).
	MaxEntries int
	// Retention prunes entries older than this duration (default 30 days).
	// A zero Retention means "keep forever".
	Retention time.Duration
	// ArchiveOnClear renames the file before truncating on manual clear.
	// Defaults to true.
	ArchiveOnClear bool
	// Path overrides xdg.HistoryPath(); used in tests.
	Path string
}

func (o HistoryOptions) path() string {
	if o.Path != "" {
		return o.Path
	}
	return xdg.HistoryPath()
}

func (o HistoryOptions) maxEntries() int {
	if o.MaxEntries > 0 {
		return o.MaxEntries
	}
	return 1000
}

// HistoryStore manages the append-only JSONL history file.
type HistoryStore struct {
	opts HistoryOptions
}

// NewHistoryStore creates a store with the given options.
func NewHistoryStore(opts HistoryOptions) *HistoryStore {
	return &HistoryStore{opts: opts}
}

// Load reads all valid history entries from disk. Malformed lines are silently
// skipped. If the file is unreadable (permission error, etc.) the corrupt file
// is renamed to history.jsonl.corrupt-<ts> and an empty slice is returned.
func (s *HistoryStore) Load() ([]HistoryEntry, error) {
	p := s.opts.path()
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		// File exists but is unreadable — quarantine it.
		corrupt := p + ".corrupt-" + strconv.FormatInt(time.Now().Unix(), 10)
		_ = os.Rename(p, corrupt)
		return nil, fmt.Errorf("history: unreadable file moved to %s: %w", filepath.Base(corrupt), err)
	}
	defer f.Close()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			// Skip malformed lines gracefully.
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Append writes one entry to the end of the history file and triggers an
// amortized rewrite when the file exceeds maxEntries*1.2.
func (s *HistoryStore) Append(e HistoryEntry) error {
	if e.Ts == "" {
		e.Ts = time.Now().UTC().Format(time.RFC3339)
	}
	p := s.opts.path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("history: mkdir: %w", err)
	}

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("history: marshal: %w", err)
	}

	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("history: open for append: %w", err)
	}
	if _, werr := fmt.Fprintf(f, "%s\n", line); werr != nil {
		f.Close()
		return fmt.Errorf("history: write: %w", werr)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("history: close after write: %w", err)
	}

	// Amortized rewrite: count lines and rewrite if > maxEntries*1.2.
	return s.maybeRewrite()
}

// PruneAge removes entries older than the configured Retention and rewrites
// the file atomically. No-op when Retention is zero.
func (s *HistoryStore) PruneAge(now time.Time) error {
	if s.opts.Retention == 0 {
		return nil
	}
	cutoff := now.Add(-s.opts.Retention)
	entries, err := s.Load()
	if err != nil || len(entries) == 0 {
		return err
	}
	var kept []HistoryEntry
	for _, e := range entries {
		t, perr := time.Parse(time.RFC3339, e.Ts)
		if perr != nil || !t.Before(cutoff) {
			kept = append(kept, e)
		}
	}
	if len(kept) == len(entries) {
		return nil
	}
	return s.rewrite(kept)
}

// Clear removes all history. When ArchiveOnClear is true the current file is
// first renamed to history.jsonl.archived-<ts>. Returns the archive path if
// one was created (empty string otherwise).
func (s *HistoryStore) Clear() (archivePath string, err error) {
	p := s.opts.path()
	if _, statErr := os.Stat(p); os.IsNotExist(statErr) {
		return "", nil
	}
	if s.opts.ArchiveOnClear {
		archivePath = p + ".archived-" + strconv.FormatInt(time.Now().Unix(), 10)
		if renameErr := os.Rename(p, archivePath); renameErr != nil {
			return "", fmt.Errorf("history: archive rename: %w", renameErr)
		}
		return archivePath, nil
	}
	return "", os.Truncate(p, 0)
}

// maybeRewrite counts lines in the file and rewrites (keeping last maxEntries)
// when the count exceeds maxEntries*1.2.
func (s *HistoryStore) maybeRewrite() error {
	max := s.opts.maxEntries()
	threshold := int(float64(max) * 1.2)

	entries, err := s.Load()
	if err != nil || len(entries) <= threshold {
		return err
	}
	// Keep only the newest max entries.
	tail := entries
	if len(tail) > max {
		tail = tail[len(tail)-max:]
	}
	return s.rewrite(tail)
}

// rewrite atomically replaces the history file with the provided entries.
// It uses a write-to-tmp + fsync + rename sequence for crash safety.
func (s *HistoryStore) rewrite(entries []HistoryEntry) error {
	p := s.opts.path()
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("history: rewrite mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "history.*.tmp")
	if err != nil {
		return fmt.Errorf("history: rewrite create temp: %w", err)
	}
	tmpPath := tmp.Name()

	w := bufio.NewWriter(tmp)
	for _, e := range entries {
		line, merr := json.Marshal(e)
		if merr != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("history: rewrite marshal: %w", merr)
		}
		if _, werr := fmt.Fprintf(w, "%s\n", line); werr != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("history: rewrite write: %w", werr)
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("history: rewrite flush: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("history: rewrite sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("history: rewrite close: %w", err)
	}
	if err := os.Rename(tmpPath, p); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("history: rewrite rename: %w", err)
	}
	return nil
}
