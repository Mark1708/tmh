package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrite_RoundTripPreservesComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	original := `# top-level comment
version: 1
roots:
  # comment before otr
  otr: /tmp/otr
sessions:
  atlas:
    root: otr
    # comment before env
    env:
      KUBE: old
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := PathSet(c.Node, "sessions.atlas.env.KUBE", "new"); err != nil {
		t.Fatal(err)
	}
	if err := Write(c, path, WriteOptions{}); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "# top-level comment") {
		t.Fatalf("lost top-level comment:\n%s", s)
	}
	if !strings.Contains(s, "# comment before otr") {
		t.Fatalf("lost otr comment:\n%s", s)
	}
	if !strings.Contains(s, "KUBE: new") {
		t.Fatalf("value not updated:\n%s", s)
	}
}

func TestWrite_AtomicAndBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	backup := filepath.Join(dir, "backups")

	original := "version: 1\nroots: {}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := PathSet(c.Node, "version", "2"); err != nil {
		t.Fatal(err)
	}
	if err := Write(c, path, WriteOptions{BackupDir: backup, MaxBackups: 5}); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := os.ReadDir(backup)
	if err != nil {
		t.Fatalf("backup dir missing: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}
	data, _ := os.ReadFile(filepath.Join(backup, entries[0].Name()))
	if string(data) != original {
		t.Fatalf("backup content:\n%s", string(data))
	}
}

func TestWrite_BlankLineReinjection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	original := `version: 1

roots:
  otr: /tmp/otr

sessions:
  s:
    root: otr
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(c, path, WriteOptions{PreserveBlanks: true}); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	s := string(out)
	if !strings.Contains(s, "version: 1\n\nroots:") {
		t.Fatalf("blank line between version and roots lost:\n%s", s)
	}
	if !strings.Contains(s, "\n\nsessions:") {
		t.Fatalf("blank line before sessions lost:\n%s", s)
	}
}

func TestWrite_AddAndDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	original := `version: 1
sessions:
  atlas:
    root: otr
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := PathSet(c.Node, "sessions.atlas.path", "products/x"); err != nil {
		t.Fatal(err)
	}
	if err := PathDelete(c.Node, "sessions.atlas.root"); err != nil {
		t.Fatal(err)
	}

	if err := Write(c, path, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	s := string(out)
	if strings.Contains(s, "root:") {
		t.Fatalf("root should be deleted:\n%s", s)
	}
	if !strings.Contains(s, "path: products/x") {
		t.Fatalf("path not added:\n%s", s)
	}
}
