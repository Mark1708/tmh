package config

import (
	"strings"
	"testing"
)

func TestPathGet_Scalar(t *testing.T) {
	src := `
version: 1
sessions:
  epcp:
    env:
      KUBE: epcp-dev
`
	c := mustParse(t, src)
	n, err := PathGet(c.Node, "sessions.epcp.env.KUBE")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if n.Value != "epcp-dev" {
		t.Fatalf("value = %q", n.Value)
	}
}

func TestPathGet_Missing(t *testing.T) {
	src := `
sessions:
  epcp: {}
`
	c := mustParse(t, src)
	_, err := PathGet(c.Node, "sessions.ghost")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "segment") {
		t.Fatalf("unexpected error format: %v", err)
	}
}

func TestPathSet_UpdateScalar(t *testing.T) {
	src := `
sessions:
  epcp:
    env:
      KUBE: old
`
	c := mustParse(t, src)
	if err := PathSet(c.Node, "sessions.epcp.env.KUBE", "new"); err != nil {
		t.Fatalf("set: %v", err)
	}
	n, _ := PathGet(c.Node, "sessions.epcp.env.KUBE")
	if n.Value != "new" {
		t.Fatalf("value = %q", n.Value)
	}
}

func TestPathSet_CreateNested(t *testing.T) {
	src := `
sessions:
  epcp: {}
`
	c := mustParse(t, src)
	if err := PathSet(c.Node, "sessions.epcp.env.NEW_KEY", "hello"); err != nil {
		t.Fatalf("set: %v", err)
	}
	n, err := PathGet(c.Node, "sessions.epcp.env.NEW_KEY")
	if err != nil {
		t.Fatalf("get after set: %v", err)
	}
	if n.Value != "hello" {
		t.Fatalf("value = %q", n.Value)
	}
}

func TestPathDelete(t *testing.T) {
	src := `
sessions:
  epcp:
    env:
      KEEP: a
      REMOVE: b
`
	c := mustParse(t, src)
	if err := PathDelete(c.Node, "sessions.epcp.env.REMOVE"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := PathGet(c.Node, "sessions.epcp.env.REMOVE"); err == nil {
		t.Fatalf("should be removed")
	}
	if n, err := PathGet(c.Node, "sessions.epcp.env.KEEP"); err != nil || n.Value != "a" {
		t.Fatalf("sibling gone: err=%v value=%q", err, n.Value)
	}
}

func TestPathRename(t *testing.T) {
	src := `
sessions:
  old:
    root: x
`
	c := mustParse(t, src)
	if err := PathRename(c.Node, "sessions", "old", "new"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := PathGet(c.Node, "sessions.old"); err == nil {
		t.Fatalf("old still exists")
	}
	if _, err := PathGet(c.Node, "sessions.new"); err != nil {
		t.Fatalf("new not found: %v", err)
	}
}

func TestPathRename_Conflict(t *testing.T) {
	src := `
sessions:
  a: {root: x}
  b: {root: y}
`
	c := mustParse(t, src)
	err := PathRename(c.Node, "sessions", "a", "b")
	if err == nil || !strings.Contains(err.Error(), "target already exists") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}
