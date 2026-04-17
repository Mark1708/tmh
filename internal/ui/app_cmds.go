package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/shell"
	appstate "github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/ui/toast"

	tea "github.com/charmbracelet/bubbletea"
)

// loadHistoryCmd asynchronously reads persistent history from disk.
func (m *Model) loadHistoryCmd() tea.Cmd {
	if m.historyStore == nil {
		return nil
	}
	hs := m.historyStore
	return func() tea.Msg {
		entries, err := hs.Load()
		if err != nil {
			return historyLoadedMsg{Err: err}
		}
		disk := make([]historyDiskEntry, 0, len(entries))
		for _, e := range entries {
			disk = append(disk, historyDiskEntry{
				Ts: e.Ts, Action: e.Action, Target: e.Target,
				Result: e.Result, Details: e.Details,
			})
		}
		return historyLoadedMsg{Entries: disk}
	}
}

// appendHistoryCmd persists a single action to disk asynchronously.
func (m *Model) appendHistoryCmd(action, target, result, details string) tea.Cmd {
	if m.historyStore == nil {
		return nil
	}
	hs := m.historyStore
	e := appstate.HistoryEntry{
		Ts:      time.Now().UTC().Format(time.RFC3339),
		Action:  action,
		Target:  target,
		Result:  result,
		Details: details,
	}
	return func() tea.Msg {
		_ = hs.Append(e) // best-effort; errors are not surfaced for writes
		return nil
	}
}

// clearHistoryCmd wipes all persistent history.
func (m *Model) clearHistoryCmd() tea.Cmd {
	if m.historyStore == nil {
		return func() tea.Msg { return historyClearedMsg{} }
	}
	hs := m.historyStore
	return func() tea.Msg {
		archivePath, err := hs.Clear()
		return historyClearedMsg{ArchivePath: archivePath, Err: err}
	}
}

func (m *Model) loadDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cfg, err := m.deps.LoadConfig()
		if err != nil {
			cfg = &config.Config{Version: 1}
		}
		listing, err := actions.BuildListing(ctx, m.deps.Runner, cfg, m.deps.Profile)
		if err != nil {
			return dataLoadedMsg{Err: err}
		}
		resolved, err := config.Resolve(cfg, m.deps.Profile)
		if err != nil {
			resolved = &config.Resolved{}
		}
		snap, err := collectLive(ctx, m.deps.Runner)
		if err != nil {
			return dataLoadedMsg{Err: err}
		}
		drift := config.Diff(resolved, snap)
		// Auto-detect pane-base-index so rows match what tmux reports,
		// regardless of whether the user configured it in tmh settings.
		paneBaseIndex := 0
		if raw, err := m.deps.Runner.ShowOption(ctx, "pane-base-index"); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
				paneBaseIndex = n
			}
		}
		// Do NOT write m.cfg here — tea.Cmd runs on a worker goroutine and m is
		// not synchronised. Return cfg in the message; Update assigns it on the
		// main goroutine.
		return dataLoadedMsg{Listing: listing, Drift: drift, Cfg: cfg, PaneBaseIndex: paneBaseIndex}
	}
}

func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.pollEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) reloadAllCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rcFile := shell.DefaultRCFile()
		_, err := actions.Reload(ctx, m.deps.Runner, m.deps.State, rcFile,
			actions.ReloadOptions{Shell: true, Tmux: true, Busy: true, RcFile: rcFile})
		if err != nil {
			return errorMsg{Err: err}
		}
		return actionDoneMsg{Text: m.str.Toast.ReloadTriggered}
	}
}

func (m *Model) syncPushCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg, err := m.deps.LoadConfig()
		if err != nil {
			return errorMsg{Err: err}
		}
		rep, err := actions.Push(ctx, m.deps.Runner, cfg, actions.SyncOptions{Profile: m.deps.Profile})
		if err != nil {
			return errorMsg{Err: err}
		}
		text := i18n.Tf("tui.toast.sync_report", map[string]any{
			"created": len(rep.Created),
			"updated": len(rep.Updated),
		})
		return actionDoneMsg{Text: text}
	}
}

// attachCmd hands the controlling terminal over to tmux for an
// attach/switch-client. tea.ExecProcess properly suspends the bubbletea
// event loop, restores the alt-screen on return, and gives the child
// process direct access to stdin/stdout/stderr — without this, tmux
// receives a useless pipe and the user can't type into the attached
// session.
func attachCmd(r tmux.Runner, inTmux bool, target string) tea.Cmd {
	args := []string{"attach-session", "-t", target}
	if inTmux {
		// switch-client doesn't take over the terminal; it sends a tmux
		// command to the running client, then returns immediately. Run via
		// runner so the parent process keeps its TTY.
		return func() tea.Msg {
			if err := r.SwitchClient(context.Background(), target); err != nil {
				return errorMsg{Err: fmt.Errorf("attach: %w", err)}
			}
			return nil
		}
	}
	cmd := exec.Command("tmux", args...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return errorMsg{Err: fmt.Errorf("attach: %w", err)}
		}
		return nil
	})
}

func (m *Model) killTargetCmd(target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		// Snapshot before kill so undo can restore.
		if m.deps.State != nil {
			if live, err := actions.CaptureLive(ctx, m.deps.Runner); err == nil {
				for _, s := range live {
					if s.Name == target {
						payload, _ := jsonMarshal(s)
						_, _ = m.deps.State.InsertEvent(ctx, "kill_session", target, string(payload))
						break
					}
				}
			}
		}
		if err := m.deps.Runner.KillSession(ctx, target); err != nil {
			return errorMsg{Err: err}
		}
		// Invalidate any marks that pointed at this target.
		if m.marksStore != nil {
			m.marksStore.InvalidateMark(target)
		}
		killed := i18n.Tf("tui.toast.session_killed", map[string]any{"name": target})
		hint := "kill session " + target
		return tea.Batch(
			func() tea.Msg { return actionDoneMsg{Text: killed} },
			func() tea.Msg { return undoHintMsg{Text: hint} },
			m.loadDataCmd(),
			func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} },
		)()
	}
}

func (m *Model) killWindowCmd(target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := m.deps.Runner.KillWindow(ctx, target); err != nil {
			return errorMsg{Err: err}
		}
		if m.marksStore != nil {
			m.marksStore.InvalidateMark(target)
		}
		killed := i18n.Tf("tui.toast.session_killed", map[string]any{"name": target})
		hint := "kill window " + target
		return tea.Batch(
			func() tea.Msg { return actionDoneMsg{Text: killed} },
			func() tea.Msg { return undoHintMsg{Text: hint} },
			m.loadDataCmd(),
			func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} },
		)()
	}
}

func (m *Model) killPaneCmd(target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := m.deps.Runner.KillPane(ctx, target); err != nil {
			return errorMsg{Err: err}
		}
		if m.marksStore != nil {
			m.marksStore.InvalidateMark(target)
		}
		killed := i18n.Tf("tui.toast.session_killed", map[string]any{"name": target})
		return tea.Batch(
			func() tea.Msg { return actionDoneMsg{Text: killed} },
			m.loadDataCmd(),
			func() tea.Msg { return switchScreenMsg{Screen: ScreenDashboard} },
		)()
	}
}

// gotoProcCmd finds the first pane running the given process name.
// It returns a gotoProcMsg so that the actual cursor mutation happens on the
// main goroutine inside Update (Bubble Tea's message-passing model — tea.Cmd
// closures must not access unsynchronised Model fields directly).
func (m *Model) gotoProcCmd(procName string) tea.Cmd {
	pp := m.paneProvider // capture safe handle; Provider is internally locked
	return func() tea.Msg {
		if pp == nil {
			return toastMsg{Kind: toast.KindError, Text: "goto: no pane data available"}
		}
		target := pp.FindByCommand(strings.ToLower(procName))
		if target == "" {
			return toastMsg{Kind: toast.KindError,
				Text: i18n.Tf("tui.toast.goto_not_found", map[string]any{"name": procName})}
		}
		return gotoProcMsg{Target: target}
	}
}

func (m *Model) undoCmd() tea.Cmd {
	return func() tea.Msg {
		if m.deps.State == nil {
			return errorMsg{Err: fmt.Errorf("%s", m.str.Toast.UndoUnavailable)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		target, err := actions.UndoLast(ctx, m.deps.Runner, m.deps.State)
		if err != nil {
			return errorMsg{Err: err}
		}
		restored := i18n.Tf("tui.toast.session_restored", map[string]any{"name": target})
		return tea.Batch(
			func() tea.Msg { return actionDoneMsg{Text: restored} },
			func() tea.Msg { return undoHintMsg{} }, // clear undo hint
			m.loadDataCmd(),
		)()
	}
}

// newSessionCmd launches the `tmh new` wizard as a subprocess. Bubbletea
// owns the controlling TTY in alt-screen mode, so huh can't render its form
// inside this process; tea.ExecProcess suspends the event loop, hands the
// terminal over to the child, then restores the alt-screen when the child
// exits. The next polling tick picks up any newly-created session.
func (m *Model) newSessionCmd() tea.Cmd {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = os.Args[0]
	}
	cmd := exec.Command(exe, "new")
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return errorMsg{Err: fmt.Errorf("new: %w", err)}
		}
		return actionDoneMsg{Text: i18n.T("tui.toast.session_created")}
	})
}

// initCmd runs actions.Init so the palette can create every configured
// session that isn't already live. Toasts success count or error.
func (m *Model) initCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cfg, err := m.deps.LoadConfig()
		if err != nil {
			return errorMsg{Err: err}
		}
		if err := actions.Init(ctx, m.deps.Runner, cfg, actions.InitOptions{Profile: m.deps.Profile}); err != nil {
			return errorMsg{Err: err}
		}
		return actionDoneMsg{Text: "init: " + i18n.T("tui.toast.reload_triggered")}
	}
}

// snapshotSaveCmd captures the current live state under an auto-timestamped
// name (tmh-YYYYMMDD-HHMMSS). The user can inspect / restore via CLI.
func (m *Model) snapshotSaveCmd() tea.Cmd {
	return func() tea.Msg {
		if m.deps.State == nil {
			return errorMsg{Err: fmt.Errorf("%s", m.str.Toast.UndoUnavailable)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		name := "tmh-" + time.Now().Format("20060102-150405")
		if err := actions.SaveSnapshot(ctx, m.deps.Runner, m.deps.State, name); err != nil {
			return errorMsg{Err: err}
		}
		return actionDoneMsg{Text: "snapshot: " + name}
	}
}

// doctorCmd runs the tmux audit in-process and pushes a one-line summary
// (✓n ⚠n ✗n) to the history so the palette user can inspect results.
func (m *Model) doctorCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		findings := actions.AuditTmuxConfig(ctx, m.deps.Runner)
		var ok, warn, errs int
		for _, f := range findings {
			switch f.Level {
			case actions.AuditOK:
				ok++
			case actions.AuditWarn:
				warn++
			case actions.AuditError:
				errs++
			}
		}
		text := fmt.Sprintf("doctor: ✓%d ⚠%d ✗%d", ok, warn, errs)
		return actionDoneMsg{Text: text}
	}
}

// maybeLoadPreview triggers an async CapturePane for the current selection
// when the dashboard's cached preview doesn't match. Returns nil when no
// fetch is needed (no selection, or cache is fresh).
func (m *Model) maybeLoadPreview() tea.Cmd {
	if m.dashboard == nil {
		return nil
	}
	target, stale := m.dashboard.PreviewStale()
	if !stale || target == "" {
		return nil
	}
	return m.loadPreviewCmd(target)
}

func (m *Model) loadPreviewCmd(target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		// tmux accepts `session` or `session:window`; for session-level rows
		// we capture the active window's first pane.
		data, err := m.deps.Runner.CapturePane(ctx, target, 200)
		if err != nil {
			return previewLoadedMsg{Target: target, Err: err}
		}
		return previewLoadedMsg{Target: target, Data: string(data)}
	}
}
