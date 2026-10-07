package configsource

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDesiredResolvesRootsTemplatesAndOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	content := `version: 1
roots:
  home: /Users/mark
  projects: /work

templates:
  cluster:
    command: k9s
sessions:
  base:
    windows:
      root:
        dir: /Users/mark
      Downloads:
        root: home
        path: Downloads
  infra:
    windows:
      homelab:
        root: projects
        path: homelab
      cluster:
        template: cluster
        root: projects
        path: homelab
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDesired(path)
	if err != nil {
		t.Fatal(err)
	}
	base := got["base"]
	if len(base.Windows) != 2 || base.Windows[0].ID != "window:root" || base.Windows[1].ID != "window:Downloads" {
		t.Fatalf("base windows = %+v", base.Windows)
	}
	if cwd := base.Windows[1].Surfaces[0].CWD; cwd != "/Users/mark/Downloads" {
		t.Fatalf("Downloads cwd = %q", cwd)
	}
	infra := got["infra"]
	if title := infra.Windows[1].Surfaces[0].Title; title != "k9s" {
		t.Fatalf("cluster command = %q", title)
	}
}

func TestLoadDesiredRejectsPathEscapingRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	content := "version: 1\nroots:\n  projects: /work\nsessions:\n  bad:\n    windows:\n      escape:\n        root: projects\n        path: ../../etc\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDesired(path); err == nil {
		t.Fatal("expected escaping path to be rejected")
	}
}
