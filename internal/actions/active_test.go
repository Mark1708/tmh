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
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

func TestPromoteActiveWindowDisabled(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)
	original := "source:1"

	target, err := PromoteActiveWindow(
		context.Background(), runner, store, config.ActiveSessionConfig{}, original, activeTestNow,
	)
	if err != nil {
		t.Fatalf("disabled promote: %v", err)
	}
	if target != original {
		t.Fatalf("target = %q, want %q", target, original)
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Fatalf("disabled promote made calls: %+v", calls)
	}
}

func TestPromoteActiveWindowFirstAndRepeated(t *testing.T) {
	runner, windowID := newActiveSource(t)
	store := newActiveTestStore(t)
	cfg := activeTestConfig()

	target, err := PromoteActiveWindow(context.Background(), runner, store, cfg, "source:1", activeTestNow)
	if err != nil {
		t.Fatalf("first promote: %v", err)
	}
	if target != ActiveSessionName+":"+windowID {
		t.Fatalf("target = %q", target)
	}
	marker, err := runner.ShowSessionOption(context.Background(), ActiveSessionName, ActiveOwnerOption)
	if err != nil || marker != ActiveOwnerValue {
		t.Fatalf("ownership marker = %q, err = %v", marker, err)
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); !ok {
		t.Fatal("active alias missing after promotion")
	}
	links, err := runner.ListWindowLinks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range links {
		if link.SessionName == ActiveSessionName && link.WindowName == activePlaceholderWindowName {
			t.Fatalf("placeholder was not cleaned up: %+v", link)
		}
	}

	epoch := currentActiveEpoch(t, runner)
	rows := listActiveRows(t, store, epoch.Value)
	if len(rows) != 1 || rows[0].WindowID != windowID || rows[0].State != state.ActiveWindowTracked {
		t.Fatalf("first promote rows: %+v", rows)
	}
	promotedAt := rows[0].PromotedAt
	runner.Reset()

	secondNow := activeTestNow.Add(time.Minute)
	secondTarget, err := PromoteActiveWindow(context.Background(), runner, store, cfg, "source:1", secondNow)
	if err != nil {
		t.Fatalf("repeated promote: %v", err)
	}
	if secondTarget != target {
		t.Fatalf("repeated target = %q, want %q", secondTarget, target)
	}
	if methodCount(runner.Calls(), "LinkWindow") != 0 || methodCount(runner.Calls(), "NewSession") != 0 {
		t.Fatalf("repeated promote duplicated session/link: %+v", runner.Calls())
	}
	rows = listActiveRows(t, store, epoch.Value)
	if len(rows) != 1 || !rows[0].PromotedAt.Equal(promotedAt) {
		t.Fatalf("promoted_at changed: %+v", rows)
	}
	if !rows[0].LastSelectedAt.Equal(secondNow) || !rows[0].ExpiresAt.Equal(secondNow.Add(5*time.Minute)) {
		t.Fatalf("sliding TTL not refreshed: %+v", rows[0])
	}
}

func TestPromoteActiveWindowDoesNotCleanUserWindowNamedPlaceholder(t *testing.T) {
	ctx := context.Background()
	runner := tmuxtest.New()
	if err := runner.NewSession(ctx, tmux.NewSessionOpts{
		Name: "source", WindowName: activePlaceholderWindowName, Detached: true,
	}); err != nil {
		t.Fatalf("create placeholder-named source: %v", err)
	}
	windowID, err := runner.WindowID(ctx, "source:1")
	if err != nil {
		t.Fatalf("source window id: %v", err)
	}
	store := newActiveTestStore(t)
	cfg := activeTestConfig()
	if _, err := PromoteActiveWindow(ctx, runner, store, cfg, "source:1", activeTestNow); err != nil {
		t.Fatalf("promote placeholder-named source: %v", err)
	}
	if err := runner.NewSession(ctx, tmux.NewSessionOpts{
		Name: "other", WindowName: "editor", Detached: true,
	}); err != nil {
		t.Fatalf("create other source: %v", err)
	}

	if _, err := PromoteActiveWindow(ctx, runner, store, cfg, "other:1", activeTestNow.Add(time.Minute)); err != nil {
		t.Fatalf("promote other source: %v", err)
	}

	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); !ok {
		t.Fatalf("active alias for user window %s named %q was removed", windowID, activePlaceholderWindowName)
	}
	if _, ok := activeLinkFor(t, runner, windowID, "source"); !ok {
		t.Fatalf("source link for user window %s named %q was removed", windowID, activePlaceholderWindowName)
	}
}

func TestPromoteActiveWindowCreatesMarkerAtomically(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)

	if _, err := PromoteActiveWindow(
		context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow,
	); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.Calls() {
		if call.Method != "NewSession" {
			continue
		}
		options, ok := call.Args["sessionOptions"].(map[string]string)
		if !ok || options[ActiveOwnerOption] != ActiveOwnerValue {
			t.Fatalf("NewSession options = %#v", call.Args["sessionOptions"])
		}
		if methodCount(runner.Calls(), "SetSessionOption") != 0 {
			t.Fatal("marker was set in a separate Runner call")
		}
		return
	}
	t.Fatal("NewSession call not found")
}

func TestPromoteActiveWindowCollisionFailsClosed(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)
	if err := runner.NewSession(context.Background(), tmux.NewSessionOpts{
		Name: ActiveSessionName, Detached: true,
	}); err != nil {
		t.Fatal(err)
	}
	runner.Reset()

	_, err := PromoteActiveWindow(
		context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow,
	)
	if !errors.Is(err, errs.ErrActiveSessionCollision) {
		t.Fatalf("promote error = %v, want collision", err)
	}
	assertNoMutatingCalls(t, runner.Calls())

	report, err := ActiveStatus(context.Background(), runner, store, activeTestConfig(), activeTestNow)
	if err != nil {
		t.Fatalf("collision status: %v", err)
	}
	if !report.Collision || report.Owned || report.CollisionReason == "" {
		t.Fatalf("collision report: %+v", report)
	}
	assertNoMutatingCalls(t, runner.Calls())
}

func TestPromoteActiveWindowUsesFreeIndexAndRetriesCollision(t *testing.T) {
	base, windowID := newActiveSource(t)
	base.Options()["base-index"] = "1"
	store := newActiveTestStore(t)
	runner := &conflictOnceLinkRunner{Runner: base}

	target, err := PromoteActiveWindow(
		context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow,
	)
	if err != nil {
		t.Fatalf("promote with conflict: %v", err)
	}
	if target != ActiveSessionName+":"+windowID {
		t.Fatalf("target = %q", target)
	}
	want := []string{"active:2", "active:3"}
	if len(runner.destinations) != len(want) {
		t.Fatalf("destinations = %v, want %v", runner.destinations, want)
	}
	for i := range want {
		if runner.destinations[i] != want[i] {
			t.Fatalf("destinations = %v, want %v", runner.destinations, want)
		}
	}
}

func TestPromoteActiveWindowSurvivesRenumberByWindowID(t *testing.T) {
	runner, store, windowID, _ := promoteActiveFixture(t)
	if err := runner.RenumberWindows(ActiveSessionName, 7); err != nil {
		t.Fatalf("renumber active: %v", err)
	}
	runner.Reset()

	target, err := PromoteActiveWindow(
		context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if target != ActiveSessionName+":"+windowID || methodCount(runner.Calls(), "LinkWindow") != 0 {
		t.Fatalf("renumbered promote target=%q calls=%+v", target, runner.Calls())
	}
}

func TestPromoteActiveWindowReconcilesServerEpoch(t *testing.T) {
	runner, windowID := newActiveSource(t)
	store := newActiveTestStore(t)
	current := currentActiveEpoch(t, runner)
	staleEpoch := current.Value + "-old"
	if err := store.UpsertActiveWindow(context.Background(), state.ActiveWindowInput{
		ServerKey: current.ServerKey, ServerEpoch: staleEpoch, WindowID: windowID,
		SourceSessionID: "old", SourceSessionName: "old", WindowName: "old",
	}, activeTestNow.Add(-time.Hour), time.Minute); err != nil {
		t.Fatal(err)
	}

	if _, err := PromoteActiveWindow(
		context.Background(), runner, store, activeTestConfig(), "source:1", activeTestNow,
	); err != nil {
		t.Fatal(err)
	}
	if rows := listActiveRows(t, store, staleEpoch); len(rows) != 0 {
		t.Fatalf("stale epoch rows remain: %+v", rows)
	}
	if rows := listActiveRows(t, store, current.Value); len(rows) != 1 {
		t.Fatalf("current epoch rows: %+v", rows)
	}
}

func TestPromoteActiveWindowRollsBackLinkOnDBFailure(t *testing.T) {
	runner, windowID := newActiveSource(t)
	store := newActiveTestStore(t)
	wantErr := errors.New("injected upsert failure")
	failing := failingUpsertStore{ActiveWindowStore: store, err: wantErr}

	_, err := PromoteActiveWindow(
		context.Background(), runner, failing, activeTestConfig(), "source:1", activeTestNow,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("promote error = %v, want injected failure", err)
	}
	if _, ok := activeLinkFor(t, runner, windowID, ActiveSessionName); ok {
		t.Fatal("active alias remained after DB rollback")
	}
	if _, ok := activeLinkFor(t, runner, windowID, "source"); !ok {
		t.Fatal("source link was lost during DB rollback")
	}
	if methodCount(runner.Calls(), "UnlinkWindow") != 1 || methodCount(runner.Calls(), "KillSession") != 0 {
		t.Fatalf("rollback calls: %+v", runner.Calls())
	}
}

func TestTouchActiveWindowRefreshesKnownWithoutPromotingUnknown(t *testing.T) {
	runner, store, windowID, epoch := promoteActiveFixture(t)
	touchNow := activeTestNow.Add(2 * time.Minute)
	runner.Reset()

	touched, err := TouchActiveWindow(
		context.Background(), runner, store, activeTestConfig(), windowID, touchNow,
	)
	if err != nil || !touched {
		t.Fatalf("known touch touched=%v err=%v", touched, err)
	}
	rows := listActiveRows(t, store, epoch.Value)
	if len(rows) != 1 || !rows[0].LastSelectedAt.Equal(touchNow) || !rows[0].ExpiresAt.Equal(touchNow.Add(5*time.Minute)) {
		t.Fatalf("known touch row: %+v", rows)
	}
	runner.Reset()

	touched, err = TouchActiveWindow(
		context.Background(), runner, store, activeTestConfig(), "@999", touchNow,
	)
	if err != nil || touched {
		t.Fatalf("unknown touch touched=%v err=%v", touched, err)
	}
	if methodCount(runner.Calls(), "LinkWindow") != 0 || methodCount(runner.Calls(), "NewSession") != 0 {
		t.Fatalf("unknown touch promoted: %+v", runner.Calls())
	}
}

func TestTouchActiveWindowRejectsInvalidIDAndDisabledFeature(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)

	if _, err := TouchActiveWindow(
		context.Background(), runner, store, activeTestConfig(), "not-an-id", activeTestNow,
	); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if _, err := TouchActiveWindow(
		context.Background(), runner, store, config.ActiveSessionConfig{}, "@1", activeTestNow,
	); !errors.Is(err, errs.ErrActiveSessionDisabled) {
		t.Fatalf("disabled touch error = %v", err)
	}
}

func TestActiveStatusReportsStableDomainData(t *testing.T) {
	runner, store, windowID, epoch := promoteActiveFixture(t)
	report, err := ActiveStatus(context.Background(), runner, store, activeTestConfig(), activeTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Enabled || !report.Owned || report.Collision || report.Session != ActiveSessionName {
		t.Fatalf("status ownership: %+v", report)
	}
	if report.TTL != "5m0s" || report.ServerEpoch != epoch.Value || report.Tracked != 1 || report.Orphaned != 0 {
		t.Fatalf("status summary: %+v", report)
	}
	if len(report.Entries) != 1 || report.Entries[0].WindowID != windowID || report.Entries[0].ActiveIndex == nil {
		t.Fatalf("status entries: %+v", report.Entries)
	}
	if len(report.Entries[0].SourceLinks) != 1 || report.Entries[0].SourceLinks[0].SessionName != "source" {
		t.Fatalf("status source links: %+v", report.Entries[0].SourceLinks)
	}
}

func TestActiveStatusDisabledIsReadOnly(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)
	report, err := ActiveStatus(
		context.Background(), runner, store, config.ActiveSessionConfig{}, activeTestNow,
	)
	if err != nil || report.Enabled {
		t.Fatalf("disabled status report=%+v err=%v", report, err)
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Fatalf("disabled status made calls: %+v", calls)
	}
}

func TestActiveLifecycleRejectsMalformedTTLAtActionBoundary(t *testing.T) {
	runner, _ := newActiveSource(t)
	store := newActiveTestStore(t)
	cfg := config.ActiveSessionConfig{Enabled: true, TTL: "invalid"}

	if _, err := PromoteActiveWindow(
		context.Background(), runner, store, cfg, "source:1", activeTestNow,
	); !errors.Is(err, errs.ErrInvalidTTL) {
		t.Fatalf("malformed TTL error = %v", err)
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Fatalf("malformed TTL made calls: %+v", calls)
	}
}

var _ ActiveWindowStore = (*state.DB)(nil)
var _ tmux.Runner = (*tmuxtest.MockRunner)(nil)
