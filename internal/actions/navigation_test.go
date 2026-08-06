package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

func TestNavigateWithActiveFeatureDisabled(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			TmuxIntegration: config.TmuxIntegrationConfig{
				Active: config.ActiveSessionConfig{Enabled: false},
			},
		},
	}
	runner := tmuxtest.New()
	runner.SetInTmux(false)

	ctx := context.Background()
	if err := runner.NewSession(ctx, tmux.NewSessionOpts{Name: "test-session"}); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	runner.Reset()

	err := NavigateWithActive(ctx, runner, cfg, nil, "test-session", time.Now())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := runner.Calls()
	if len(calls) != 1 || calls[0].Method != "AttachSession" {
		t.Errorf("expected AttachSession call, got %v", runner.MethodNames())
	}
}

func TestNavigateWithActiveNilConfig(t *testing.T) {
	runner := tmuxtest.New()
	runner.SetInTmux(false)

	ctx := context.Background()
	if err := runner.NewSession(ctx, tmux.NewSessionOpts{Name: "test-session"}); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	runner.Reset()

	err := NavigateWithActive(ctx, runner, nil, nil, "test-session", time.Now())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := runner.Calls()
	if len(calls) != 1 || calls[0].Method != "AttachSession" {
		t.Errorf("expected AttachSession call, got %v", runner.MethodNames())
	}
}

func TestNavigateWithActiveEnabledRequiresStoreBeforeMutation(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			TmuxIntegration: config.TmuxIntegrationConfig{
				Active: activeTestConfig(),
			},
		},
	}
	runner, _ := newActiveSource(t)

	err := NavigateWithActive(context.Background(), runner, cfg, nil, "source:1", activeTestNow)
	if err == nil {
		t.Fatal("expected nil store error")
	}
	if got := err.Error(); got != "active windows feature enabled but state store is nil" {
		t.Fatalf("error = %q", got)
	}
	assertNoMutatingCalls(t, runner.Calls())
}

func TestNavigateWithActiveFirstAttachCreatesAndPromotes(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			TmuxIntegration: config.TmuxIntegrationConfig{
				Active: activeTestConfig(),
			},
		},
	}
	runner, windowID := newActiveSource(t)
	store := newActiveTestStore(t)
	runner.SetInTmux(false)
	epoch, err := runner.ServerEpoch(context.Background())
	if err != nil {
		t.Fatalf("server epoch: %v", err)
	}
	runner.Reset()

	err = NavigateWithActive(context.Background(), runner, cfg, store, "source:1", activeTestNow)
	if err != nil {
		t.Fatalf("navigate with active: %v", err)
	}
	calls := runner.Calls()
	if methodCount(calls, "NewSession") != 1 || methodCount(calls, "LinkWindow") != 1 {
		t.Fatalf("promotion calls = %+v", calls)
	}
	last := calls[len(calls)-1]
	if last.Method != "AttachSession" || last.Args["name"] != ActiveSessionName+":"+windowID {
		t.Fatalf("last call = %+v, want AttachSession to active alias", last)
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); !ok {
		t.Fatalf("missing active link for %s", windowID)
	}
	if _, ok := activeLinkFor(t, runner, windowID, "source"); !ok {
		t.Fatalf("missing source link for %s", windowID)
	}
	rows := listActiveRows(t, store, epoch.Value)
	if len(rows) != 1 || rows[0].WindowID != windowID {
		t.Fatalf("stored rows = %+v, want one row for %s", rows, windowID)
	}
}

func TestNavigateWithActiveExistingAliasTargetsActiveLink(t *testing.T) {
	runner, store, windowID, _ := promoteActiveFixture(t)
	cfg := &config.Config{
		Defaults: config.Defaults{
			TmuxIntegration: config.TmuxIntegrationConfig{
				Active: activeTestConfig(),
			},
		},
	}
	runner.SetInTmux(true)
	runner.Reset()

	err := NavigateWithActive(context.Background(), runner, cfg, store, "source:1", activeTestNow.Add(time.Minute))
	if err != nil {
		t.Fatalf("navigate existing alias: %v", err)
	}
	calls := runner.Calls()
	if methodCount(calls, "LinkWindow") != 0 || methodCount(calls, "NewSession") != 0 {
		t.Fatalf("existing alias duplicated mutations: %+v", calls)
	}
	last := calls[len(calls)-1]
	if last.Method != "SwitchClient" || last.Args["target"] != ActiveSessionName+":"+windowID {
		t.Fatalf("last call = %+v, want SwitchClient to active alias", last)
	}
}

func TestPromoteActiveWindowCleanupFailureRollsBack(t *testing.T) {
	base, windowID := newActiveSource(t)
	store := newActiveTestStore(t)
	runner := failingKillRunner{Runner: base, err: errors.New("injected cleanup failure")}

	_, err := PromoteActiveWindow(context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow)
	if err == nil {
		t.Fatal("expected cleanup failure")
	}
	if !strings.Contains(err.Error(), "injected cleanup failure") {
		t.Fatalf("error = %v, want cleanup failure context", err)
	}
	if _, ok := activeLinkFor(t, base, windowID, ActiveSessionName); ok {
		t.Fatalf("active alias for %s was not rolled back", windowID)
	}
	if _, ok := activeLinkFor(t, base, windowID, "source"); !ok {
		t.Fatalf("source link for %s was removed", windowID)
	}
	rows := listActiveRows(t, store, currentActiveEpoch(t, base).Value)
	if len(rows) != 0 {
		t.Fatalf("cleanup failure persisted rows: %+v", rows)
	}
}

func TestPromoteActiveWindowCleanupAndRollbackErrorsAreJoined(t *testing.T) {
	base, windowID := newActiveSource(t)
	store := newActiveTestStore(t)
	runner := cleanupAndRollbackFailureRunner{Runner: base}

	_, err := PromoteActiveWindow(context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow)
	if err == nil {
		t.Fatal("expected joined cleanup and rollback errors")
	}
	errText := err.Error()
	for _, want := range []string{"injected cleanup failure", "injected rollback failure"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
	if _, ok := activeLinkFor(t, base, windowID, ActiveSessionName); !ok {
		t.Fatalf("rollback failure should leave active alias for retry inspection")
	}
	if _, ok := activeLinkFor(t, base, windowID, "source"); !ok {
		t.Fatalf("source link for %s was removed", windowID)
	}
}

func TestCleanupActiveAliasesBeforeKill(t *testing.T) {
	runner := tmuxtest.New()

	ctx := context.Background()
	err := CleanupActiveAliasesBeforeKill(ctx, runner, "test-session", "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKillSessionCleansActiveAliasBeforeKill(t *testing.T) {
	runner, _, windowID, _ := promoteActiveFixture(t)
	runner.Reset()

	if err := KillSession(context.Background(), runner, "source"); err != nil {
		t.Fatalf("kill session: %v", err)
	}
	calls := runner.Calls()
	unlinkAt, killAt := firstCallIndex(calls, "UnlinkWindow"), firstCallIndex(calls, "KillSession")
	if unlinkAt < 0 || killAt < 0 || unlinkAt > killAt {
		t.Fatalf("calls = %+v, want active unlink before session kill", calls)
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); ok {
		t.Fatalf("active alias for %s remained after session kill", windowID)
	}
}

func TestKillWindowCleansActiveAliasBeforeKill(t *testing.T) {
	runner, _, windowID, _ := promoteActiveFixture(t)
	runner.Reset()

	if err := KillWindow(context.Background(), runner, "source:1"); err != nil {
		t.Fatalf("kill window: %v", err)
	}
	calls := runner.Calls()
	unlinkAt, killAt := firstCallIndex(calls, "UnlinkWindow"), firstCallIndex(calls, "KillWindow")
	if unlinkAt < 0 || killAt < 0 || unlinkAt > killAt {
		t.Fatalf("calls = %+v, want active unlink before window kill", calls)
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); ok {
		t.Fatalf("active alias for %s remained after window kill", windowID)
	}
}

func TestKillPaneKeepsActiveAliasForMultiPaneWindow(t *testing.T) {
	runner, windowID := newActiveSource(t)
	if err := runner.SplitWindow(context.Background(), tmux.SplitOpts{Target: "source:1"}); err != nil {
		t.Fatalf("split source: %v", err)
	}
	store := newActiveTestStore(t)
	if _, err := PromoteActiveWindow(context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow); err != nil {
		t.Fatalf("promote: %v", err)
	}
	runner.Reset()

	if err := KillPane(context.Background(), runner, "source:1.1"); err != nil {
		t.Fatalf("kill pane: %v", err)
	}
	if methodCount(runner.Calls(), "UnlinkWindow") != 0 {
		t.Fatalf("multi-pane kill removed active alias: %+v", runner.Calls())
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); !ok {
		t.Fatalf("multi-pane kill removed active alias for %s", windowID)
	}
}

func TestKillPaneCleansActiveAliasForLastPaneWindow(t *testing.T) {
	runner, _, windowID, _ := promoteActiveFixture(t)
	runner.Reset()

	if err := KillPane(context.Background(), runner, "source:1.1"); err != nil {
		t.Fatalf("kill last pane: %v", err)
	}
	calls := runner.Calls()
	unlinkAt, killAt := firstCallIndex(calls, "UnlinkWindow"), firstCallIndex(calls, "KillPane")
	if unlinkAt < 0 || killAt < 0 || unlinkAt > killAt {
		t.Fatalf("calls = %+v, want active unlink before last pane kill", calls)
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); ok {
		t.Fatalf("active alias for %s remained after last-pane kill", windowID)
	}
}

func firstCallIndex(calls []tmuxtest.Call, method string) int {
	for i, call := range calls {
		if call.Method == method {
			return i
		}
	}
	return -1
}

type failingKillRunner struct {
	tmux.Runner
	err error
}

func (r failingKillRunner) KillWindow(context.Context, string) error {
	return r.err
}

type cleanupAndRollbackFailureRunner struct {
	tmux.Runner
}

func (r cleanupAndRollbackFailureRunner) KillWindow(context.Context, string) error {
	return errors.New("injected cleanup failure")
}

func (r cleanupAndRollbackFailureRunner) UnlinkWindow(context.Context, string, string) error {
	return errors.New("injected rollback failure")
}
