package config

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type stubZoxide struct{ paths []string }

func (s stubZoxide) Run(_ context.Context, limit int) ([]string, error) {
	if limit > 0 && len(s.paths) > limit {
		return s.paths[:limit], nil
	}
	return s.paths, nil
}

func TestExpandDiscoverRules_GlobPath(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"api", "web", "infra"} {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// non-dir: should be skipped by expandGlob
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{Discover: []DiscoverRule{{Path: filepath.Join(dir, "*")}}}
	got, err := ExpandDiscoverRules(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(got))
	for _, d := range got {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	want := []string{"api", "infra", "web"}
	if len(names) != len(want) {
		t.Fatalf("expected 3 discovered, got %v", names)
	}
	for i, n := range names {
		if n != want[i] {
			t.Fatalf("name[%d]=%q, want %q", i, n, want[i])
		}
	}
}

func TestExpandDiscoverRules_DeclaredSessionsWin(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "api"), 0o755)
	cfg := &Config{
		Sessions: map[string]Session{"api": {}},
		Discover: []DiscoverRule{{Path: filepath.Join(dir, "*")}},
	}
	got, _ := ExpandDiscoverRules(context.Background(), cfg, nil)
	for _, d := range got {
		if d.Name == "api" {
			t.Fatalf("api should be suppressed by declared session, got %+v", got)
		}
	}
}

func TestExpandDiscoverRules_ZoxideSupplements(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "alpha"), 0o755)
	cfg := &Config{
		Discover: []DiscoverRule{
			{Path: filepath.Join(dir, "*"), Zoxide: true, ZoxideLimit: 2},
		},
	}
	zox := stubZoxide{paths: []string{"/home/user/frequent_one", "/home/user/frequent_two"}}
	got, _ := ExpandDiscoverRules(context.Background(), cfg, zox)

	sawGlob, sawZox := false, false
	for _, d := range got {
		if d.Name == "alpha" && !d.FromZoxide {
			sawGlob = true
		}
		if d.FromZoxide {
			sawZox = true
		}
	}
	if !sawGlob {
		t.Fatalf("glob match missing: %+v", got)
	}
	if !sawZox {
		t.Fatalf("zoxide entries missing: %+v", got)
	}
}

func TestExpandDiscoverRules_DedupesDuplicates(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "x"), 0o755)
	cfg := &Config{
		Discover: []DiscoverRule{
			{Path: filepath.Join(dir, "*")},
			{Path: filepath.Join(dir, "*")}, // duplicate rule
		},
	}
	got, _ := ExpandDiscoverRules(context.Background(), cfg, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 deduped, got %d: %+v", len(got), got)
	}
}
