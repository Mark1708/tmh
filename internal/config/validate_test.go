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

func TestValidate_ActiveSessionConfig_DisabledByDefault(t *testing.T) {
	src := `
version: 1
sessions:
  my-session:
    windows:
      a: /tmp
`
	c := mustParse(t, src)
	if err := Validate(c); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if c.Defaults.TmuxIntegration.Active.Enabled != false {
		t.Fatalf("expected active.enabled to default to false, got %v", c.Defaults.TmuxIntegration.Active.Enabled)
	}
}

func TestValidate_ActiveSessionConfig_EnabledWithOmittedTTLEffective5h(t *testing.T) {
	src := `
version: 1
defaults:
  tmux_integration:
    active:
      enabled: true
sessions:
  my-session:
    windows:
      a: /tmp
`
	c := mustParse(t, src)
	if err := Validate(c); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if c.Defaults.TmuxIntegration.Active.TTL != "" {
		t.Fatalf("expected TTL to be omitted when not specified, got %q", c.Defaults.TmuxIntegration.Active.TTL)
	}
	effectiveTTL := c.Defaults.TmuxIntegration.Active.EffectiveTTL()
	if effectiveTTL != "5h" {
		t.Fatalf("expected effective TTL to be 5h, got %q", effectiveTTL)
	}
}

func TestValidate_ActiveSessionConfig_ExplicitTTLEffective(t *testing.T) {
	tests := []struct {
		name     string
		ttl      string
		expected string
	}{
		{"1 hour", "1h", "1h"},
		{"30 minutes", "30m", "30m"},
		{"maximum 720 hours", "720h", "720h"},
		{"minimum positive", "1ns", "1ns"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := `
version: 1
defaults:
  tmux_integration:
    active:
      enabled: true
      ttl: ` + tt.ttl + `
sessions:
  my-session:
    windows:
      a: /tmp
`
			c := mustParse(t, src)
			if err := Validate(c); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if c.Defaults.TmuxIntegration.Active.TTL != tt.ttl {
				t.Fatalf("expected TTL to be %q, got %q", tt.ttl, c.Defaults.TmuxIntegration.Active.TTL)
			}
			effectiveTTL := c.Defaults.TmuxIntegration.Active.EffectiveTTL()
			if effectiveTTL != tt.expected {
				t.Fatalf("expected effective TTL to be %q, got %q", tt.expected, effectiveTTL)
			}
		})
	}
}

func TestValidate_ActiveSessionConfig_InvalidTTL(t *testing.T) {
	tests := []struct {
		name        string
		ttl         string
		expectError error
	}{
		{"zero duration", "0h", errs.ErrInvalidTTL},
		{"negative duration", "-1h", errs.ErrInvalidTTL},
		{"malformed", "not-a-duration", errs.ErrInvalidTTL},
		{"exceeds maximum", "721h", errs.ErrInvalidTTL},
		{"exceeds maximum with minutes", "720h1m", errs.ErrInvalidTTL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := `
version: 1
defaults:
  tmux_integration:
    active:
      enabled: true
      ttl: ` + tt.ttl + `
sessions:
  my-session:
    windows:
      a: /tmp
`
			c := mustParse(t, src)
			err := Validate(c)
			if !errors.Is(err, tt.expectError) {
				t.Fatalf("expected %v, got %v", tt.expectError, err)
			}
		})
	}
}

func TestValidate_ActiveSessionConfig_InvalidTTLEvenWhenDisabled(t *testing.T) {
	tests := []struct {
		name        string
		ttl         string
		expectError error
	}{
		{"zero duration when disabled", "0h", errs.ErrInvalidTTL},
		{"negative duration when disabled", "-1h", errs.ErrInvalidTTL},
		{"malformed when disabled", "not-a-duration", errs.ErrInvalidTTL},
		{"exceeds maximum when disabled", "721h", errs.ErrInvalidTTL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := `
version: 1
defaults:
  tmux_integration:
    active:
      enabled: false
      ttl: ` + tt.ttl + `
sessions:
  my-session:
    windows:
      a: /tmp
`
			c := mustParse(t, src)
			err := Validate(c)
			if !errors.Is(err, tt.expectError) {
				t.Fatalf("expected %v even when disabled, got %v", tt.expectError, err)
			}
		})
	}
}

func TestValidate_ActiveSessionConfig_ReservedSessionName(t *testing.T) {
	src := `
version: 1
defaults:
  tmux_integration:
    active:
      enabled: true
      ttl: 5h
sessions:
  active:
    windows:
      a: /tmp
`
	c := mustParse(t, src)
	err := Validate(c)
	if !errors.Is(err, errs.ErrReservedSessionName) {
		t.Fatalf("expected %v, got %v", errs.ErrReservedSessionName, err)
	}
}

func TestValidate_ActiveSessionConfig_ReservedNameAllowedWhenDisabled(t *testing.T) {
	src := `
version: 1
defaults:
  tmux_integration:
    active:
      enabled: false
sessions:
  active:
    windows:
      a: /tmp
`
	c := mustParse(t, src)
	if err := Validate(c); err != nil {
		t.Fatalf("expected no error when disabled, got %v", err)
	}
}

func TestValidate_ActiveSessionConfig_BackwardCompatibility(t *testing.T) {
	src := `
version: 1
roots:
  otr: /tmp/otr
sessions:
  atlas:
    root: otr
    windows:
      web: web-frontend
      api:
        dir: api
        layout: 3-pane
`
	c := mustParse(t, src)
	if err := Validate(c); err != nil {
		t.Fatalf("expected no error for backward compatibility, got %v", err)
	}
	if c.Defaults.TmuxIntegration.Active.Enabled != false {
		t.Fatalf("expected active.enabled to default to false, got %v", c.Defaults.TmuxIntegration.Active.Enabled)
	}
}
