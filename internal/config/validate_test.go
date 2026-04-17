package config

import (
	"errors"
	"testing"

	errs "github.com/mark1708/tmh/internal/errors"
)

func TestValidate_OK(t *testing.T) {
	src := `
version: 1
roots:
  otr: /tmp/otr
templates:
  kb_base:
    layout: 2-pane
layouts:
  my-ide:
    hash: "abc"
sessions:
  s:
    root: otr
    windows:
      a:
        dir: x
        layout: 3-pane
      b:
        extends: kb_base
        dir: y
      c:
        layout: my-ide
        dir: z
`
	c := mustParse(t, src)
	if err := Validate(c); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidate_UnknownRoot(t *testing.T) {
	src := `
version: 1
sessions:
  s:
    root: nope
    windows:
      a: /tmp
`
	c := mustParse(t, src)
	err := Validate(c)
	if !errors.Is(err, errs.ErrUnknownRoot) {
		t.Fatalf("got %v", err)
	}
}

func TestValidate_UnknownTemplate(t *testing.T) {
	src := `
version: 1
sessions:
  s:
    windows:
      a:
        extends: ghost
        dir: /tmp
`
	c := mustParse(t, src)
	err := Validate(c)
	if !errors.Is(err, errs.ErrUnknownTemplate) {
		t.Fatalf("got %v", err)
	}
}

func TestValidate_TemplateExtendsChain(t *testing.T) {
	src := `
version: 1
templates:
  a:
    extends: b
  b:
    layout: 1-pane
sessions: {}
`
	c := mustParse(t, src)
	err := Validate(c)
	if !errors.Is(err, errs.ErrTemplateChain) {
		t.Fatalf("got %v", err)
	}
}

func TestValidate_UnknownLayout(t *testing.T) {
	src := `
version: 1
sessions:
  s:
    windows:
      a:
        layout: made-up
        dir: /tmp
`
	c := mustParse(t, src)
	err := Validate(c)
	if !errors.Is(err, errs.ErrUnknownLayout) {
		t.Fatalf("got %v", err)
	}
}

func TestValidate_LayoutMismatch(t *testing.T) {
	src := `
version: 1
sessions:
  s:
    windows:
      a:
        layout: 2-pane
        dir: /tmp
        panes:
          - dir: a
          - dir: b
          - dir: c
`
	c := mustParse(t, src)
	err := Validate(c)
	if !errors.Is(err, errs.ErrLayoutMismatch) {
		t.Fatalf("got %v", err)
	}
}
