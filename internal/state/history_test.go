package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tmpHistoryStore(t *testing.T, opts HistoryOptions) *HistoryStore {
	t.Helper()
	dir := t.TempDir()
	opts.Path = filepath.Join(dir, "history.jsonl")
	return NewHistoryStore(opts)
}

func TestHistory_AppendAndLoad(t *testing.T) {
	s := tmpHistoryStore(t, HistoryOptions{})
	e := HistoryEntry{Action: "kill_session", Target: "atlas", Result: "ok", Details: "done"}
	if err := s.Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Action != "kill_session" {
		t.Errorf("unexpected action %q", got[0].Action)
	}
	if got[0].Ts == "" {
		t.Error("Ts should be set automatically")
	}
}

func TestHistory_SizeCap_AmortizedRewrite(t *testing.T) {
	// maxEntries=10; threshold=12; writing 13 entries should trigger rewrite.
	s := tmpHistoryStore(t, HistoryOptions{MaxEntries: 10})
	for i := 0; i < 13; i++ {
		if err := s.Append(HistoryEntry{Action: "a", Target: strings.Repeat("x", i), Result: "ok"}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load after rewrite: %v", err)
	}
	// After amortized rewrite the file should have exactly maxEntries records.
	if len(got) != 10 {
		t.Fatalf("expected 10 entries after size-cap rewrite, got %d", len(got))
	}
	// The newest entries should be kept (last 10 of 13, so Target = "xxxxxxxxx" to "xxxxxxxxxxxx").
	if got[0].Target != strings.Repeat("x", 3) {
		t.Errorf("expected oldest kept entry to be #3, got Target=%q", got[0].Target)
	}
}

func TestHistory_AgeCap_PruneAge(t *testing.T) {
	s := tmpHistoryStore(t, HistoryOptions{Retention: 24 * time.Hour})
	old := HistoryEntry{Ts: time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339), Action: "old", Result: "ok"}
	recent := HistoryEntry{Ts: time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339), Action: "recent", Result: "ok"}
	if err := s.Append(old); err != nil {
		t.Fatalf("Append old: %v", err)
	}
	if err := s.Append(recent); err != nil {
		t.Fatalf("Append recent: %v", err)
	}
	if err := s.PruneAge(time.Now()); err != nil {
		t.Fatalf("PruneAge: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load after prune: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry after age prune, got %d", len(got))
	}
	if got[0].Action != "recent" {
		t.Errorf("expected recent entry, got %q", got[0].Action)
	}
}

func TestHistory_CorruptFile_MovedAndEmptyReturned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	// Write a corrupt file (not valid JSONL).
	if err := os.WriteFile(path, []byte("not json\nalso not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewHistoryStore(HistoryOptions{Path: path})
	got, err := s.Load()
	// Corrupt file means malformed lines are skipped — should return empty slice, no error.
	if err != nil {
		t.Fatalf("expected no error on malformed lines, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 valid entries from corrupt file, got %d", len(got))
	}
}

func TestHistory_UnreadableFile_QuarantinedAndEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	if err := os.WriteFile(path, []byte(`{"ts":"2024-01-01T00:00:00Z","action":"a","result":"ok"}`+"\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	s := NewHistoryStore(HistoryOptions{Path: path})
	got, err := s.Load()
	// On permission-denied, file is moved to .corrupt-<ts> and error returned.
	if err == nil {
		// On some CI environments the test runs as root; skip the check.
		if os.Getuid() == 0 {
			t.Skip("running as root, permission check not applicable")
		}
		t.Fatal("expected error for unreadable file")
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 entries on unreadable file, got %d", len(got))
	}
	// Original file should be gone.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("original file should have been renamed away")
	}
}

func TestHistory_Clear_ArchiveOnClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	s := NewHistoryStore(HistoryOptions{Path: path, ArchiveOnClear: true})
	_ = s.Append(HistoryEntry{Action: "a", Result: "ok"})

	archivePath, err := s.Clear()
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if archivePath == "" {
		t.Fatal("expected non-empty archivePath")
	}
	if _, statErr := os.Stat(archivePath); os.IsNotExist(statErr) {
		t.Errorf("archive file %s should exist", archivePath)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("original file should not exist after clear+archive")
	}
}

func TestHistory_Clear_Truncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	s := NewHistoryStore(HistoryOptions{Path: path, ArchiveOnClear: false})
	_ = s.Append(HistoryEntry{Action: "a", Result: "ok"})

	archivePath, err := s.Clear()
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if archivePath != "" {
		t.Errorf("expected empty archivePath without ArchiveOnClear, got %q", archivePath)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file should exist after truncate-clear: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("expected file size 0 after truncate, got %d", info.Size())
	}
}
