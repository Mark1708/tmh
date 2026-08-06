package actions

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

var activeTestNow = time.Unix(1_800_000_000, 0)

func activeTestConfig() config.ActiveSessionConfig {
	return config.ActiveSessionConfig{Enabled: true, TTL: "5m"}
}

func newActiveTestStore(t *testing.T) *state.DB {
	t.Helper()
	db, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newActiveSource(t *testing.T) (*tmuxtest.MockRunner, string) {
	t.Helper()
	ctx := context.Background()
	runner := tmuxtest.New()
	if err := runner.NewSession(ctx, tmux.NewSessionOpts{
		Name: "source", WindowName: "editor", Detached: true,
	}); err != nil {
		t.Fatalf("create source: %v", err)
	}
	windowID, err := runner.WindowID(ctx, "source:1")
	if err != nil {
		t.Fatalf("source window id: %v", err)
	}
	runner.Reset()
	return runner, windowID
}

func currentActiveEpoch(t *testing.T, runner tmux.Runner) tmux.ServerEpoch {
	t.Helper()
	epoch, err := runner.ServerEpoch(context.Background())
	if err != nil {
		t.Fatalf("server epoch: %v", err)
	}
	if resetter, ok := runner.(interface{ Reset() }); ok {
		resetter.Reset()
	}
	return epoch
}

func listActiveRows(t *testing.T, store ActiveWindowStore, epoch string) []state.ActiveWindow {
	t.Helper()
	rows, err := store.ListActiveWindows(context.Background(), epoch)
	if err != nil {
		t.Fatalf("list active rows: %v", err)
	}
	return rows
}

func promoteActiveFixture(t *testing.T) (*tmuxtest.MockRunner, *state.DB, string, tmux.ServerEpoch) {
	t.Helper()
	runner, windowID := newActiveSource(t)
	store := newActiveTestStore(t)
	target, err := PromoteActiveWindow(
		context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow,
	)
	if err != nil {
		t.Fatalf("promote fixture: %v", err)
	}
	if target != ActiveSessionName+":"+windowID {
		t.Fatalf("promotion target = %q, want active ID target", target)
	}
	epoch := currentActiveEpoch(t, runner)
	return runner, store, windowID, epoch
}

func activeLinkFor(t *testing.T, runner tmux.Runner, windowID, session string) (tmux.WindowLink, bool) {
	t.Helper()
	links, err := runner.ListWindowLinks(context.Background())
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	for _, link := range links {
		if link.WindowID == windowID && link.SessionName == session {
			return link, true
		}
	}
	return tmux.WindowLink{}, false
}

func methodCount(calls []tmuxtest.Call, method string) int {
	count := 0
	for _, call := range calls {
		if call.Method == method {
			count++
		}
	}
	return count
}

func assertNoUnsafeCalls(t *testing.T, calls []tmuxtest.Call) {
	t.Helper()
	for _, call := range calls {
		switch call.Method {
		case "UnlinkWindow", "KillWindow", "KillSession":
			t.Fatalf("unsafe runner call: %s %+v", call.Method, call.Args)
		}
	}
}

func assertNoMutatingCalls(t *testing.T, calls []tmuxtest.Call) {
	t.Helper()
	for _, call := range calls {
		switch call.Method {
		case "NewSession", "NewWindow", "LinkWindow", "UnlinkWindow",
			"KillWindow", "KillSession", "SetSessionOption", "SetOption":
			t.Fatalf("mutating runner call: %s %+v", call.Method, call.Args)
		}
	}
}

type failingUpsertStore struct {
	ActiveWindowStore
	err error
}

func (s failingUpsertStore) UpsertActiveWindow(
	context.Context,
	state.ActiveWindowInput,
	time.Time,
	time.Duration,
) error {
	return s.err
}

type failOnceUnlinkRunner struct {
	tmux.Runner
	failed bool
	calls  int
}

func (r *failOnceUnlinkRunner) UnlinkWindow(ctx context.Context, session, windowID string) error {
	r.calls++
	if !r.failed {
		r.failed = true
		return errors.New("injected unlink failure")
	}
	return r.Runner.UnlinkWindow(ctx, session, windowID)
}

type cancelAfterLinkRunner struct {
	tmux.Runner
	cancel context.CancelFunc
}

func (r *cancelAfterLinkRunner) LinkWindow(ctx context.Context, windowID, destination string) error {
	if err := r.Runner.LinkWindow(ctx, windowID, destination); err != nil {
		return err
	}
	r.cancel()
	return nil
}

type conflictOnceLinkRunner struct {
	tmux.Runner
	destinations []string
	failed       bool
}

func (r *conflictOnceLinkRunner) LinkWindow(ctx context.Context, windowID, destination string) error {
	r.destinations = append(r.destinations, destination)
	if !r.failed {
		r.failed = true
		session := strings.SplitN(destination, ":", 2)[0]
		if _, err := r.Runner.NewWindow(ctx, tmux.NewWindowOpts{
			SessionTarget: session + ":",
			Name:          "index-conflict",
		}); err != nil {
			return fmt.Errorf("inject index conflict: %w", err)
		}
		return errors.New("index in use")
	}
	return r.Runner.LinkWindow(ctx, windowID, destination)
}
