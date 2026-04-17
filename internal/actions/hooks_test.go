package actions

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/state"
)

func TestEnsureTrusted_PromptApproval(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	hc := HookContext{ConfigPath: cfgPath}
	called := 0
	prompter := func(commands []string) (bool, error) {
		called++
		if len(commands) != 1 {
			t.Fatalf("expected 1 command, got %v", commands)
		}
		return true, nil
	}

	if err := EnsureTrusted(context.Background(), db, hc, []string{"echo ok"}, prompter); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatal("prompter should be called once")
	}

	// Same hash → no prompt this time.
	if err := EnsureTrusted(context.Background(), db, hc, []string{"echo ok"}, prompter); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatal("prompter should not be called again on same hash")
	}

	// Modify config → re-prompt.
	if err := os.WriteFile(cfgPath, []byte("version: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureTrusted(context.Background(), db, hc, []string{"echo ok"}, prompter); err != nil {
		t.Fatal(err)
	}
	if called != 2 {
		t.Fatalf("prompter should be called again on changed config, got %d", called)
	}
}

func TestEnsureTrusted_DenialIsTyped(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yml")
	_ = os.WriteFile(cfgPath, []byte("version: 1\n"), 0o644)
	db, _ := state.Open(":memory:")
	defer db.Close()
	err := EnsureTrusted(context.Background(), db, HookContext{ConfigPath: cfgPath}, []string{"rm -rf /"}, func([]string) (bool, error) { return false, nil })
	if !errors.Is(err, errs.ErrHookDenied) {
		t.Fatalf("expected ErrHookDenied, got %v", err)
	}
}

func TestEnsureTrusted_NoPrompterFails(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yml")
	_ = os.WriteFile(cfgPath, []byte("version: 1\n"), 0o644)
	db, _ := state.Open(":memory:")
	defer db.Close()
	err := EnsureTrusted(context.Background(), db, HookContext{ConfigPath: cfgPath}, []string{"x"}, nil)
	if !errors.Is(err, errs.ErrHookDenied) {
		t.Fatalf("expected ErrHookDenied, got %v", err)
	}
}

func TestRunHooks_ExecutesInOrder(t *testing.T) {
	out := &bytes.Buffer{}
	hooks := []string{"echo first", "echo second"}
	err := RunHooks(context.Background(), HookContext{}, HookOptions{}, hooks, out, out)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if got != "first\nsecond\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRunHooks_NoHooksFlag(t *testing.T) {
	out := &bytes.Buffer{}
	err := RunHooks(context.Background(), HookContext{}, HookOptions{NoHooks: true}, []string{"echo nope"}, out, out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty output, got %q", out.String())
	}
}

func TestRunHooks_EnvOverrides(t *testing.T) {
	out := &bytes.Buffer{}
	err := RunHooks(context.Background(), HookContext{Env: map[string]string{"FOO": "bar"}}, HookOptions{}, []string{"echo $FOO"}, out, out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "bar\n" {
		t.Fatalf("got %q", out.String())
	}
}

func TestRunScopedHooks_ErrorIncludesScopeAndEvent(t *testing.T) {
	out := &bytes.Buffer{}
	err := RunScopedHooks(
		context.Background(), HookContext{}, HookOptions{},
		ScopeWindow, EventOnCreate,
		[]string{"false"}, out, out,
	)
	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	if !strings.Contains(err.Error(), "window") || !strings.Contains(err.Error(), "on_create") {
		t.Fatalf("error missing scope/event context: %v", err)
	}
}

func TestRunScopedHooks_EmptyScopeMatchesLegacyFormat(t *testing.T) {
	out := &bytes.Buffer{}
	err := RunScopedHooks(
		context.Background(), HookContext{}, HookOptions{},
		"", "", []string{"false"}, out, out,
	)
	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	// Legacy callers (RunHooks) pass empty scope/event — error should start
	// with "hook", not "<scope>".
	if !strings.HasPrefix(err.Error(), "hook ") {
		t.Fatalf("expected legacy-style error prefix, got %v", err)
	}
}

func TestCollectHookCommands_IncludesWindowAndPaneScopes(t *testing.T) {
	cfg := parseConfig(t, `
version: 1
sessions:
  api:
    hooks:
      on_create: ["session-hook"]
    windows:
      server:
        dir: /tmp
        hooks:
          on_create: ["window-hook"]
        panes:
          - dir: /tmp
            hooks:
              on_create: ["pane-hook"]
`)
	got := CollectHookCommands(cfg, "")
	want := map[string]bool{"session-hook": true, "window-hook": true, "pane-hook": true}
	for _, h := range got {
		delete(want, h)
	}
	if len(want) != 0 {
		t.Fatalf("missing scope(s) in collected hooks: %v (got %v)", want, got)
	}
}
