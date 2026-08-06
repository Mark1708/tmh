package actions

import (
	"context"
	"errors"
	"testing"
	"time"

	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

func orphanFixture(t *testing.T) (*tmuxtest.MockRunner, *state.DB, string, tmux.ServerEpoch) {
	t.Helper()
	runner, store, windowID, epoch := promoteActiveFixture(t)
	if err := runner.NewSession(context.Background(), tmux.NewSessionOpts{
		Name: "recovered", WindowName: "existing", Detached: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := runner.KillSession(context.Background(), "source"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkActiveWindowOrphaned(context.Background(), epoch.Value, windowID, activeTestNow.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	runner.Reset()
	return runner, store, windowID, epoch
}

func TestRecoverActiveWindowLinksDestinationBeforeUnlink(t *testing.T) {
	runner, store, windowID, epoch := orphanFixture(t)

	if err := RecoverActiveWindow(
		context.Background(), runner, store, activeTestConfig(), windowID, "recovered",
	); err != nil {
		t.Fatal(err)
	}
	if _, ok := activeLinkFor(t, runner, windowID, "recovered"); !ok {
		t.Fatal("recovery destination link missing")
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); ok {
		t.Fatal("active alias remains after recovery")
	}
	if rows := listActiveRows(t, store, epoch.Value); len(rows) != 0 {
		t.Fatalf("recovered row remains: %+v", rows)
	}
	calls := runner.Calls()
	linkAt, unlinkAt := -1, -1
	for i, call := range calls {
		switch call.Method {
		case "LinkWindow":
			linkAt = i
		case "UnlinkWindow":
			unlinkAt = i
		}
	}
	if linkAt < 0 || unlinkAt <= linkAt {
		t.Fatalf("recovery order is unsafe: %+v", calls)
	}
	if methodCount(calls, "KillWindow") != 0 || methodCount(calls, "KillSession") != 0 {
		t.Fatalf("recovery used destructive calls: %+v", calls)
	}
}

func TestRecoverActiveWindowPartialFailureIsRetryable(t *testing.T) {
	base, store, windowID, epoch := orphanFixture(t)
	runner := &failOnceUnlinkRunner{Runner: base}

	err := RecoverActiveWindow(context.Background(), runner, store, activeTestConfig(), windowID, "recovered")
	if err == nil {
		t.Fatal("expected injected unlink failure")
	}
	if _, ok := activeLinkFor(t, base, windowID, "recovered"); !ok {
		t.Fatal("safe destination link was rolled back")
	}
	if _, ok := activeLinkFor(t, base, windowID, ActiveSessionName); !ok {
		t.Fatal("active alias missing after failed unlink")
	}
	if rows := listActiveRows(t, store, epoch.Value); len(rows) != 1 || rows[0].State != state.ActiveWindowOrphaned {
		t.Fatalf("partial recovery row: %+v", rows)
	}
	base.Reset()

	if err := RecoverActiveWindow(context.Background(), runner, store, activeTestConfig(), windowID, "recovered"); err != nil {
		t.Fatalf("retry recovery: %v", err)
	}
	if methodCount(base.Calls(), "LinkWindow") != 0 {
		t.Fatalf("retry duplicated destination link: %+v", base.Calls())
	}
	if rows := listActiveRows(t, store, epoch.Value); len(rows) != 0 {
		t.Fatalf("retry did not delete row: %+v", rows)
	}
}

func TestRecoverActiveWindowRetriesDestinationIndexCollision(t *testing.T) {
	base, store, windowID, _ := orphanFixture(t)
	base.Options()["base-index"] = "1"
	runner := &conflictOnceLinkRunner{Runner: base}

	if err := RecoverActiveWindow(
		context.Background(), runner, store, activeTestConfig(), windowID, "recovered",
	); err != nil {
		t.Fatal(err)
	}
	want := []string{"recovered:2", "recovered:3"}
	if len(runner.destinations) != len(want) {
		t.Fatalf("destinations = %v, want %v", runner.destinations, want)
	}
	for i := range want {
		if runner.destinations[i] != want[i] {
			t.Fatalf("destinations = %v, want %v", runner.destinations, want)
		}
	}
}

func TestRecoverActiveWindowContextCancellationLeavesRetryableState(t *testing.T) {
	base, store, windowID, epoch := orphanFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	runner := &cancelAfterLinkRunner{Runner: base, cancel: cancel}

	err := RecoverActiveWindow(ctx, runner, store, activeTestConfig(), windowID, "recovered")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("recover error = %v, want context canceled", err)
	}
	if _, ok := activeLinkFor(t, base, windowID, "recovered"); !ok {
		t.Fatal("destination link missing after cancellation")
	}
	if _, ok := activeLinkFor(t, base, windowID, ActiveSessionName); !ok {
		t.Fatal("active alias was unlinked after cancellation")
	}
	if rows := listActiveRows(t, store, epoch.Value); len(rows) != 1 {
		t.Fatalf("cancellation deleted tracking row: %+v", rows)
	}
	if methodCount(base.Calls(), "UnlinkWindow") != 0 {
		t.Fatalf("cancellation performed unlink: %+v", base.Calls())
	}

	base.Reset()
	if err := RecoverActiveWindow(
		context.Background(), base, store, activeTestConfig(), windowID, "recovered",
	); err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
}

func TestRecoverActiveWindowValidatesIDAndDestination(t *testing.T) {
	runner, store, windowID, _ := orphanFixture(t)
	tests := []struct {
		name        string
		windowID    string
		destination string
	}{
		{name: "malformed ID", windowID: "1", destination: "recovered"},
		{name: "active destination", windowID: windowID, destination: ActiveSessionName},
		{name: "destination context", windowID: windowID, destination: "recovered:1"},
		{name: "missing destination", windowID: windowID, destination: "missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner.Reset()
			err := RecoverActiveWindow(
				context.Background(), runner, store, activeTestConfig(), test.windowID, test.destination,
			)
			if err == nil {
				t.Fatal("invalid recovery accepted")
			}
			assertNoUnsafeCalls(t, runner.Calls())
		})
	}
}

func TestRecoverActiveWindowRefusesTrackedNonOrphan(t *testing.T) {
	runner, store, windowID, _ := promoteActiveFixture(t)
	if err := runner.NewSession(context.Background(), tmux.NewSessionOpts{Name: "recovered", Detached: true}); err != nil {
		t.Fatal(err)
	}
	runner.Reset()

	err := RecoverActiveWindow(context.Background(), runner, store, activeTestConfig(), windowID, "recovered")
	if !errors.Is(err, errs.ErrUnsafeActiveWindow) {
		t.Fatalf("recover tracked error = %v", err)
	}
	assertNoUnsafeCalls(t, runner.Calls())
}
