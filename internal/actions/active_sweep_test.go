package actions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mark1708/tmh/internal/config"
	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
)

func TestPruneActiveWindowsSafeExpiry(t *testing.T) {
	runner, store, windowID, epoch := promoteActiveFixture(t)
	runner.Reset()

	report, err := PruneActiveWindows(
		context.Background(), runner, store, activeTestConfig(), activeTestNow.Add(6*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Unlinked != 1 || report.Deleted != 1 || report.Orphaned != 0 {
		t.Fatalf("prune report: %+v", report)
	}
	if rows := listActiveRows(t, store, epoch.Value); len(rows) != 0 {
		t.Fatalf("expired row remains: %+v", rows)
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); ok {
		t.Fatal("active alias remains after safe expiry")
	}
	if _, ok := activeLinkFor(t, runner, windowID, "source"); !ok {
		t.Fatal("source link lost during safe expiry")
	}
	if methodCount(runner.Calls(), "UnlinkWindow") != 1 || methodCount(runner.Calls(), "KillWindow") != 0 || methodCount(runner.Calls(), "KillSession") != 0 {
		t.Fatalf("safe expiry calls: %+v", runner.Calls())
	}
}

func TestPruneActiveWindowsOrphansLastLink(t *testing.T) {
	runner, store, windowID, epoch := promoteActiveFixture(t)
	if err := runner.KillSession(context.Background(), "source"); err != nil {
		t.Fatal(err)
	}
	runner.Reset()

	report, err := PruneActiveWindows(
		context.Background(), runner, store, activeTestConfig(), activeTestNow.Add(6*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Orphaned != 1 || report.Unlinked != 0 || report.Deleted != 0 {
		t.Fatalf("orphan report: %+v", report)
	}
	rows := listActiveRows(t, store, epoch.Value)
	if len(rows) != 1 || rows[0].State != state.ActiveWindowOrphaned || rows[0].OrphanedAt.IsZero() {
		t.Fatalf("orphan row: %+v", rows)
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); !ok {
		t.Fatal("last active link was lost")
	}
	assertNoUnsafeCalls(t, runner.Calls())
}

func TestPruneActiveWindowsDeletesStaleRowWithLiveSource(t *testing.T) {
	runner, store, windowID, epoch := promoteActiveFixture(t)
	if err := runner.UnlinkWindow(context.Background(), ActiveSessionName, windowID); err != nil {
		t.Fatal(err)
	}
	runner.Reset()

	report, err := PruneActiveWindows(
		context.Background(), runner, store, activeTestConfig(), activeTestNow.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Deleted != 1 || report.Unlinked != 0 {
		t.Fatalf("stale-row report: %+v", report)
	}
	if rows := listActiveRows(t, store, epoch.Value); len(rows) != 0 {
		t.Fatalf("stale row remains: %+v", rows)
	}
	assertNoUnsafeCalls(t, runner.Calls())
}

func TestPruneActiveWindowsDeletesGoneWindowRow(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)
	epoch := currentActiveEpoch(t, runner)
	if err := store.UpsertActiveWindow(context.Background(), state.ActiveWindowInput{
		ServerKey: epoch.ServerKey, ServerEpoch: epoch.Value, WindowID: "@999",
		SourceSessionID: "gone", SourceSessionName: "gone", WindowName: "gone",
	}, activeTestNow, time.Minute); err != nil {
		t.Fatal(err)
	}

	report, err := PruneActiveWindows(
		context.Background(), runner, store, activeTestConfig(), activeTestNow,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Deleted != 1 || len(listActiveRows(t, store, epoch.Value)) != 0 {
		t.Fatalf("gone-window report: %+v", report)
	}
	assertNoUnsafeCalls(t, runner.Calls())
}

func TestRemoveActiveWindowRequiresVerifiedNonActiveLink(t *testing.T) {
	t.Run("safe source", func(t *testing.T) {
		runner, store, windowID, epoch := promoteActiveFixture(t)
		runner.Reset()

		if err := RemoveActiveWindow(
			context.Background(), runner, store, activeTestConfig(), windowID,
		); err != nil {
			t.Fatal(err)
		}
		if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); ok {
			t.Fatal("active alias remains after remove")
		}
		if _, ok := activeLinkFor(t, runner, windowID, "source"); !ok {
			t.Fatal("source link lost during remove")
		}
		if rows := listActiveRows(t, store, epoch.Value); len(rows) != 0 {
			t.Fatalf("removed row remains: %+v", rows)
		}
		if methodCount(runner.Calls(), "UnlinkWindow") != 1 || methodCount(runner.Calls(), "KillWindow") != 0 {
			t.Fatalf("remove calls: %+v", runner.Calls())
		}
	})

	t.Run("orphan refused", func(t *testing.T) {
		runner, store, windowID, epoch := promoteActiveFixture(t)
		if err := runner.KillSession(context.Background(), "source"); err != nil {
			t.Fatal(err)
		}
		runner.Reset()

		err := RemoveActiveWindow(context.Background(), runner, store, activeTestConfig(), windowID)
		if !errors.Is(err, errs.ErrUnsafeActiveWindow) {
			t.Fatalf("remove error = %v, want unsafe", err)
		}
		if rows := listActiveRows(t, store, epoch.Value); len(rows) != 1 {
			t.Fatalf("orphan row mutated: %+v", rows)
		}
		assertNoUnsafeCalls(t, runner.Calls())
	})
}

func TestActiveLifecycleMutationsRejectDisabledFeature(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)
	disabled := config.ActiveSessionConfig{}

	if _, err := PruneActiveWindows(context.Background(), runner, store, disabled, activeTestNow); !errors.Is(err, errs.ErrActiveSessionDisabled) {
		t.Fatalf("disabled prune error = %v", err)
	}
	if err := RemoveActiveWindow(context.Background(), runner, store, disabled, "@1"); !errors.Is(err, errs.ErrActiveSessionDisabled) {
		t.Fatalf("disabled remove error = %v", err)
	}
	if err := RecoverActiveWindow(context.Background(), runner, store, disabled, "@1", "source"); !errors.Is(err, errs.ErrActiveSessionDisabled) {
		t.Fatalf("disabled recover error = %v", err)
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Fatalf("disabled mutations made calls: %+v", calls)
	}
}

func TestPruneActiveWindowsCollisionHasNoMutations(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)
	if err := runner.NewSession(context.Background(), tmux.NewSessionOpts{Name: ActiveSessionName, Detached: true}); err != nil {
		t.Fatal(err)
	}
	runner.Reset()

	_, err := PruneActiveWindows(context.Background(), runner, store, activeTestConfig(), activeTestNow)
	if !errors.Is(err, errs.ErrActiveSessionCollision) {
		t.Fatalf("prune error = %v", err)
	}
	assertNoMutatingCalls(t, runner.Calls())
}
