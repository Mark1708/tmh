package ui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

func TestActivePaletteActionEligibilityAndWindowIDBinding(t *testing.T) {
	model := New(Deps{})
	index := 4
	model.activeStatus = actions.ActiveStatusReport{
		Enabled: true,
		Owned:   true,
		Entries: []actions.ActiveWindowStatus{
			{WindowID: "@42", State: state.ActiveWindowTracked, ActiveIndex: &index, SourceLinks: []actions.ActiveSourceLink{{SessionName: "work"}}},
			{WindowID: "@77", State: state.ActiveWindowOrphaned, ActiveIndex: &index},
		},
	}

	actionsByID := activeActionsByWindowID(model.activePaletteActions())
	if got := actionsByID["@42"]; len(got) != 1 || got[0].Kind != activeActionRemove || got[0].WindowID != "@42" {
		t.Fatalf("healthy tracked actions = %+v", got)
	}
	if got := actionsByID["@77"]; len(got) != 1 || got[0].Kind != activeActionRecover || !got[0].NeedsParam || got[0].WindowID != "@77" {
		t.Fatalf("orphan actions = %+v", got)
	}

	model.activeStatus.Collision = true
	if got := model.activePaletteActions(); len(got) != 0 {
		t.Fatalf("collision must disable destructive active actions: %+v", got)
	}
}

func TestActiveStatusWorkerReturnsMessageWithoutMutatingModel(t *testing.T) {
	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runner := tmuxtest.New()
	model := New(Deps{Runner: runner, State: db, LoadConfig: func() (*config.Config, error) {
		return config.Parse([]byte("version: 1\ndefaults:\n  tmux_integration:\n    active:\n      enabled: true\n"))
	}})
	before := model.activeStatus

	msg := model.loadActiveStatusCmd(9)()
	if !reflect.DeepEqual(model.activeStatus, before) {
		t.Fatalf("worker mutated model: before=%+v after=%+v", before, model.activeStatus)
	}
	loaded, ok := msg.(activeStatusLoadedMsg)
	if !ok || loaded.Seq != 9 || loaded.Err != nil || !loaded.Report.Enabled {
		t.Fatalf("worker message = %#v", msg)
	}
}

func TestActiveAttachTargetCmdUsesActiveNavigation(t *testing.T) {
	model, runner, db, windowID := activeNavigationModel(t)
	runner.SetInTmux(true)

	msg := model.attachTargetCmd("source:1")()
	if msg != nil {
		t.Fatalf("attach message = %#v", msg)
	}
	if !hasWindowLink(t, runner, windowID, actions.ActiveSessionName) {
		t.Fatalf("missing active alias for %s", windowID)
	}
	rows, err := db.ListActiveWindows(context.Background(), currentEpoch(t, runner).Value)
	if err != nil {
		t.Fatalf("list active rows: %v", err)
	}
	if len(rows) != 1 || rows[0].WindowID != windowID {
		t.Fatalf("active rows = %+v, want %s", rows, windowID)
	}
}

func TestAttachTargetCmdOutsideTmuxExecsTmhAttach(t *testing.T) {
	model, runner, _, _ := activeNavigationModel(t)
	runner.SetInTmux(false)
	model.deps.ConfigPath = "/tmp/tmh-config.yml"
	model.deps.Profile = "work"

	oldExecProcess := execProcess
	t.Cleanup(func() { execProcess = oldExecProcess })
	var got *exec.Cmd
	execProcess = func(cmd *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		got = cmd
		return func() tea.Msg { return fn(nil) }
	}

	msg := model.attachTargetCmd("source:1")()
	if msg != nil {
		t.Fatalf("attach message = %#v", msg)
	}
	wantArgs := []string{got.Path, "--config", "/tmp/tmh-config.yml", "--profile", "work", "attach", "source:1"}
	if !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("attach command args = %#v, want %#v", got.Args, wantArgs)
	}
	if len(runner.Calls()) != 0 {
		t.Fatalf("outside-tmux attach mutated runner instead of execing child tmh: %+v", runner.Calls())
	}
}

func TestActiveKillWindowCmdCleansAliasBeforeKill(t *testing.T) {
	model, runner, db, windowID := activeNavigationModel(t)
	if _, err := actions.PromoteActiveWindow(context.Background(), runner, db, model.cfg.Defaults.TmuxIntegration.Active, "source:1", time.Unix(1_800_000_000, 0)); err != nil {
		t.Fatalf("promote: %v", err)
	}
	runner.Reset()

	msg := model.killWindowCmd("source:1")()
	if _, ok := msg.(errorMsg); ok {
		t.Fatalf("kill returned error: %#v", msg)
	}
	calls := runner.Calls()
	unlinkAt, killAt := firstUICallIndex(calls, "UnlinkWindow"), firstUICallIndex(calls, "KillWindow")
	if unlinkAt < 0 || killAt < 0 || unlinkAt > killAt {
		t.Fatalf("calls = %+v, want active unlink before window kill", calls)
	}
	if hasWindowLink(t, runner, windowID, actions.ActiveSessionName) {
		t.Fatalf("active alias for %s remained after kill", windowID)
	}
}

func activeNavigationModel(t *testing.T) (*Model, *tmuxtest.MockRunner, *state.DB, string) {
	t.Helper()
	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runner := tmuxtest.New()
	if err := runner.NewSession(context.Background(), tmux.NewSessionOpts{Name: "source", WindowName: "editor", Detached: true}); err != nil {
		t.Fatalf("create source: %v", err)
	}
	windowID, err := runner.WindowID(context.Background(), "source:1")
	if err != nil {
		t.Fatalf("window id: %v", err)
	}
	cfg := &config.Config{}
	cfg.Defaults.TmuxIntegration.Active = config.ActiveSessionConfig{Enabled: true, TTL: "5h"}
	model := New(Deps{Runner: runner, State: db})
	model.cfg = cfg
	runner.Reset()
	return model, runner, db, windowID
}

func currentEpoch(t *testing.T, runner *tmuxtest.MockRunner) tmux.ServerEpoch {
	t.Helper()
	epoch, err := runner.ServerEpoch(context.Background())
	if err != nil {
		t.Fatalf("server epoch: %v", err)
	}
	return epoch
}

func hasWindowLink(t *testing.T, runner *tmuxtest.MockRunner, windowID, session string) bool {
	t.Helper()
	links, err := runner.ListWindowLinks(context.Background())
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	for _, link := range links {
		if link.WindowID == windowID && link.SessionName == session {
			return true
		}
	}
	return false
}

func firstUICallIndex(calls []tmuxtest.Call, method string) int {
	for i, call := range calls {
		if call.Method == method {
			return i
		}
	}
	return -1
}

func TestActiveStatusStaleResultGuard(t *testing.T) {
	model := New(Deps{})
	model.activeSeq = 2
	model.Update(activeStatusLoadedMsg{Seq: 1, Report: actions.ActiveStatusReport{Enabled: true, Tracked: 99}})
	if model.activeStatus.Enabled || model.activeStatus.Tracked != 0 {
		t.Fatalf("stale status applied: %+v", model.activeStatus)
	}
	model.Update(activeStatusLoadedMsg{Seq: 2, Report: actions.ActiveStatusReport{Enabled: true, Tracked: 1}})
	if !model.activeStatus.Enabled || model.activeStatus.Tracked != 1 {
		t.Fatalf("fresh status not applied: %+v", model.activeStatus)
	}
}

func TestActiveLocaleKeyParity(t *testing.T) {
	keys := []string{
		"tui.settings.field.active_enabled",
		"tui.settings.field.active_ttl",
		"tui.settings.error.active_ttl",
		"tui.dashboard.active.title",
		"tui.dashboard.active.collision",
		"tui.dashboard.active.tracked",
		"tui.dashboard.active.orphaned",
		"tui.dashboard.active.expired",
		"tui.palette.action.active_prune.title",
		"tui.palette.action.active_remove.title",
		"tui.palette.action.active_recover.title",
	}
	en := readLocaleMap(t, "../i18n/locales/en.json")
	ru := readLocaleMap(t, "../i18n/locales/ru.json")
	for _, key := range keys {
		if en[key] == "" || ru[key] == "" {
			t.Errorf("locale key %q parity: en=%q ru=%q", key, en[key], ru[key])
		}
	}
}

func activeActionsByWindowID(in []activePaletteAction) map[string][]activePaletteAction {
	out := make(map[string][]activePaletteAction)
	for _, action := range in {
		if action.WindowID != "" {
			out[action.WindowID] = append(out[action.WindowID], action)
		}
	}
	return out
}

func readLocaleMap(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	return values
}
