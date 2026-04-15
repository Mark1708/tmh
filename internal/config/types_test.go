package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWindow_UnmarshalShortForm(t *testing.T) {
	var w Window
	if err := yaml.Unmarshal([]byte(`"repos/lk"`), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Dir != "repos/lk" {
		t.Fatalf("dir = %q, want %q", w.Dir, "repos/lk")
	}
}

func TestWindow_UnmarshalFullForm(t *testing.T) {
	src := `
dir: repos/lk
layout: 3-pane
command: pnpm dev
env:
  NODE_ENV: development
focus: true
`
	var w Window
	if err := yaml.Unmarshal([]byte(src), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Dir != "repos/lk" || w.Layout != "3-pane" || w.Command != "pnpm dev" {
		t.Fatalf("unexpected window: %+v", w)
	}
	if !w.Focus {
		t.Fatalf("focus not set")
	}
	if w.Env["NODE_ENV"] != "development" {
		t.Fatalf("env lost: %+v", w.Env)
	}
}

func TestWindows_PreservesOrder(t *testing.T) {
	src := `
lk: lk-mosru-epcp
mdr: mdr
filings: filings
kb:
  layout: 2-pane
  dir: kb-dir
`
	var ws Windows
	if err := yaml.Unmarshal([]byte(src), &ws); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"lk", "mdr", "filings", "kb"}
	if !reflect.DeepEqual(ws.Order, want) {
		t.Fatalf("order = %v, want %v", ws.Order, want)
	}
	if ws.Entries["kb"].Layout != "2-pane" {
		t.Fatalf("kb layout lost")
	}
	if ws.Entries["lk"].Dir != "lk-mosru-epcp" {
		t.Fatalf("lk short form lost")
	}
}

func TestHooks_ScalarShorthand(t *testing.T) {
	src := `
on_attach: "mise use"
on_create:
  - docker compose up -d
  - sleep 1
`
	var h Hooks
	if err := yaml.Unmarshal([]byte(src), &h); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(h.OnAttach) != 1 || h.OnAttach[0] != "mise use" {
		t.Fatalf("on_attach scalar not coerced: %+v", h.OnAttach)
	}
	if len(h.OnCreate) != 2 {
		t.Fatalf("on_create list wrong length: %+v", h.OnCreate)
	}
}

func TestConfig_RoundTrip(t *testing.T) {
	src := `
version: 1
roots:
  otr: /tmp/otr
sessions:
  epcp:
    group: [work]
    root: otr
    windows:
      lk: repos/lk
      mdr:
        dir: repos/mdr
        layout: 3-pane
`
	var c Config
	if err := yaml.Unmarshal([]byte(src), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Version != 1 {
		t.Fatalf("version lost")
	}
	if c.Roots["otr"] != "/tmp/otr" {
		t.Fatalf("roots lost")
	}
	sess, ok := c.Sessions["epcp"]
	if !ok {
		t.Fatalf("session epcp missing")
	}
	if sess.Root != "otr" {
		t.Fatalf("session.root = %q", sess.Root)
	}
	if sess.Windows.Entries["lk"].Dir != "repos/lk" {
		t.Fatalf("window short form broken")
	}
	if sess.Windows.Entries["mdr"].Layout != "3-pane" {
		t.Fatalf("window full form broken")
	}
}
