package config

import (
	"errors"
	"reflect"
	"testing"

	errs "git.mark1708.ru/me/tmh/internal/errors"
)

func TestResolve_RootsAndPath(t *testing.T) {
	src := `
version: 1
roots:
  otr: /tmp/otr
sessions:
  epcp:
    root: otr
    path: products/epcp/repos
    windows:
      lk: lk-mosru-epcp
      mdr:
        dir: mdr
`
	c := mustParse(t, src)
	r, err := Resolve(c, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(r.Sessions) != 1 {
		t.Fatalf("sessions count = %d", len(r.Sessions))
	}
	s := r.Sessions[0]
	if s.Dir != "/tmp/otr/products/epcp/repos" {
		t.Fatalf("session dir = %q", s.Dir)
	}
	if len(s.Windows) != 2 {
		t.Fatalf("windows count = %d", len(s.Windows))
	}
	if s.Windows[0].Name != "lk" || s.Windows[0].Dir != "/tmp/otr/products/epcp/repos/lk-mosru-epcp" {
		t.Fatalf("window 0: %+v", s.Windows[0])
	}
	if s.Windows[1].Dir != "/tmp/otr/products/epcp/repos/mdr" {
		t.Fatalf("window 1 dir: %q", s.Windows[1].Dir)
	}
}

func TestResolve_WindowOverridesRoot(t *testing.T) {
	src := `
version: 1
roots:
  otr: /tmp/otr
  kb:  /tmp/kb
sessions:
  epcp:
    root: otr
    windows:
      lk: repos/lk
      notes:
        root: kb
        path: epcp
`
	c := mustParse(t, src)
	r, err := Resolve(c, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	s := r.Sessions[0]
	if s.Windows[0].Dir != "/tmp/otr/repos/lk" {
		t.Fatalf("lk: %q", s.Windows[0].Dir)
	}
	if s.Windows[1].Dir != "/tmp/kb/epcp" {
		t.Fatalf("notes: %q", s.Windows[1].Dir)
	}
}

func TestResolve_EnvMerge(t *testing.T) {
	src := `
version: 1
defaults:
  env:
    EDITOR: nvim
    SHARED: base
profiles:
  work:
    env:
      AWS_REGION: eu-central-1
      SHARED: profile
sessions:
  epcp:
    group: [work]
    env:
      KUBE: epcp
      SHARED: session
    windows:
      lk:
        dir: .
        env:
          SHARED: window
`
	c := mustParse(t, src)
	r, err := Resolve(c, "work")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(r.Sessions) != 1 {
		t.Fatalf("profile filter broke; sessions = %d", len(r.Sessions))
	}
	win := r.Sessions[0].Windows[0]
	want := map[string]string{
		"EDITOR":     "nvim",
		"AWS_REGION": "eu-central-1",
		"KUBE":       "epcp",
		"SHARED":     "window",
	}
	if !reflect.DeepEqual(win.Env, want) {
		t.Fatalf("env = %v, want %v", win.Env, want)
	}
}

func TestResolve_ProfileGroupFilter(t *testing.T) {
	src := `
version: 1
profiles:
  work:
    groups: [work]
sessions:
  epcp:
    group: [work]
    windows:
      lk: /tmp/x
  kb:
    group: [kb]
    windows:
      root: /tmp/y
`
	c := mustParse(t, src)
	r, err := Resolve(c, "work")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(r.Sessions) != 1 || r.Sessions[0].Name != "epcp" {
		t.Fatalf("expected only epcp, got %+v", r.Sessions)
	}
}

func TestResolve_UnknownRoot(t *testing.T) {
	src := `
version: 1
sessions:
  epcp:
    root: nonexistent
    windows:
      lk: .
`
	c := mustParse(t, src)
	_, err := Resolve(c, "")
	if !errors.Is(err, errs.ErrUnknownRoot) {
		t.Fatalf("expected ErrUnknownRoot, got %v", err)
	}
}

func TestResolve_TemplateExtendsChain(t *testing.T) {
	src := `
version: 1
templates:
  a:
    layout: 2-pane
    extends: b
  b:
    layout: 1-pane
sessions:
  s:
    windows:
      x:
        extends: a
        dir: /tmp
`
	c := mustParse(t, src)
	_, err := Resolve(c, "")
	if !errors.Is(err, errs.ErrTemplateChain) {
		t.Fatalf("expected ErrTemplateChain, got %v", err)
	}
}

func TestResolve_TemplateApplied(t *testing.T) {
	src := `
version: 1
templates:
  kb_base:
    layout: 2-pane
    command: nvim .
sessions:
  kb:
    windows:
      claude:
        extends: kb_base
        dir: /tmp/claude
`
	c := mustParse(t, src)
	r, err := Resolve(c, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	w := r.Sessions[0].Windows[0]
	if w.Layout != "2-pane" {
		t.Fatalf("layout = %q", w.Layout)
	}
	if w.Command != "nvim ." {
		t.Fatalf("command = %q", w.Command)
	}
	if w.Dir != "/tmp/claude" {
		t.Fatalf("dir = %q", w.Dir)
	}
}

func TestExpandShorthand(t *testing.T) {
	roots := map[string]string{"otr": "/tmp/otr"}
	tests := []struct {
		in       string
		wantRoot string
		wantPath string
		wantOk   bool
	}{
		{"$otr/products/x", "otr", "products/x", true},
		{"$otr", "otr", "", true},
		{"$$otr/literal", "", "", false},
		{"plain/path", "", "", false},
		{"$unknown/x", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			r, p, ok := ExpandShorthand(tt.in, roots)
			if r != tt.wantRoot || p != tt.wantPath || ok != tt.wantOk {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", r, p, ok, tt.wantRoot, tt.wantPath, tt.wantOk)
			}
		})
	}
}

func TestLintWindow_ShorthandToCanonical(t *testing.T) {
	roots := map[string]string{"otr": "/tmp/otr"}
	w := Window{Dir: "$otr/products/x"}
	LintWindow(&w, roots)
	if w.Dir != "" {
		t.Fatalf("dir should be cleared, got %q", w.Dir)
	}
	if w.Root != "otr" || w.Path != "products/x" {
		t.Fatalf("got root=%q path=%q", w.Root, w.Path)
	}
}

func mustParse(t *testing.T, src string) *Config {
	t.Helper()
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return c
}
