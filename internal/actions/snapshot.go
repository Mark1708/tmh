package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/tmux"
)

// SessionSnapshot is what we persist for snapshot/restore and undo.
type SessionSnapshot struct {
	Name    string           `json:"name"`
	Windows []WindowSnapshot `json:"windows"`
}

// WindowSnapshot captures enough to recreate a window's panes + dirs.
type WindowSnapshot struct {
	Name   string         `json:"name"`
	Layout string         `json:"layout"`
	Panes  []PaneSnapshot `json:"panes"`
}

// PaneSnapshot is per-pane cwd + last-running command (for hint surfacing).
type PaneSnapshot struct {
	Path    string `json:"path"`
	Command string `json:"command"`
}

// CaptureLive snapshots every live session into the SessionSnapshot list.
// Used as the "before" state for destructive actions and as the payload of
// named snapshots.
func CaptureLive(ctx context.Context, r tmux.Runner) ([]SessionSnapshot, error) {
	sessions, err := r.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	allPanes, err := r.ListPanes(ctx, "")
	if err != nil {
		return nil, err
	}
	type winKey struct {
		session string
		window  int
	}
	panesByWin := make(map[winKey][]tmux.Pane, len(allPanes))
	for _, p := range allPanes {
		k := winKey{p.Session, p.Window}
		panesByWin[k] = append(panesByWin[k], p)
	}

	out := make([]SessionSnapshot, 0, len(sessions))
	for _, s := range sessions {
		wins, err := r.ListWindows(ctx, s.Name)
		if err != nil {
			return nil, err
		}
		ss := SessionSnapshot{Name: s.Name}
		for _, w := range wins {
			ws := WindowSnapshot{Name: w.Name, Layout: w.Layout}
			for _, p := range panesByWin[winKey{s.Name, w.Index}] {
				ws.Panes = append(ws.Panes, PaneSnapshot{Path: p.Path, Command: p.Command})
			}
			ss.Windows = append(ss.Windows, ws)
		}
		out = append(out, ss)
	}
	return out, nil
}

// SaveSnapshot serialises and stores live state under name.
func SaveSnapshot(ctx context.Context, r tmux.Runner, db *state.DB, name string) error {
	if db == nil {
		return fmt.Errorf("snapshot: nil db")
	}
	live, err := CaptureLive(ctx, r)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(live)
	if err != nil {
		return err
	}
	return db.SaveSnapshot(ctx, name, string(payload))
}

// RestoreSnapshot recreates sessions and windows from a saved snapshot.
// Existing sessions are skipped to avoid overwriting in-flight work.
//
// Pane commands are NOT replayed — only structure (layout + cwd). Callers
// surface a hint listing past commands so the user can re-run them.
func RestoreSnapshot(ctx context.Context, r tmux.Runner, db *state.DB, name string) ([]SessionSnapshot, error) {
	snap, err := db.GetSnapshot(ctx, name)
	if err != nil {
		return nil, err
	}
	var sessions []SessionSnapshot
	if err := json.Unmarshal([]byte(snap.Payload), &sessions); err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if err := recreateSnapshot(ctx, r, s); err != nil {
			return sessions, err
		}
	}
	return sessions, nil
}

func recreateSnapshot(ctx context.Context, r tmux.Runner, s SessionSnapshot) error {
	exists, err := r.HasSession(ctx, s.Name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if len(s.Windows) == 0 {
		return r.NewSession(ctx, tmux.NewSessionOpts{Name: s.Name, Detached: true})
	}
	first := s.Windows[0]
	firstDir := ""
	if len(first.Panes) > 0 {
		firstDir = first.Panes[0].Path
	}
	if err := r.NewSession(ctx, tmux.NewSessionOpts{
		Name: s.Name, Dir: firstDir, WindowName: first.Name, Detached: true,
	}); err != nil {
		return err
	}
	if first.Layout != "" {
		_ = r.SelectLayout(ctx, fmt.Sprintf("%s:1", s.Name), first.Layout)
	}
	for i := 1; i < len(s.Windows); i++ {
		w := s.Windows[i]
		dir := ""
		if len(w.Panes) > 0 {
			dir = w.Panes[0].Path
		}
		win, err := r.NewWindow(ctx, tmux.NewWindowOpts{
			SessionTarget: s.Name + ":", Name: w.Name, Dir: dir,
		})
		if err != nil {
			return err
		}
		if w.Layout != "" {
			_ = r.SelectLayout(ctx, fmt.Sprintf("%s:%d", s.Name, win.Index), w.Layout)
		}
	}
	return nil
}

// UndoLast pops the most recent destructive event and replays it to undo.
// Currently supports kill_session events; other kinds are skipped with a
// no-op return.
func UndoLast(ctx context.Context, r tmux.Runner, db *state.DB) (string, error) {
	events, err := db.RecentEvents(ctx, 1)
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return "", fmt.Errorf("undo: nothing to undo")
	}
	e := events[0]
	switch e.Kind {
	case "kill_session":
		var snap SessionSnapshot
		if err := json.Unmarshal([]byte(e.Payload), &snap); err != nil {
			return "", err
		}
		if err := recreateSnapshot(ctx, r, snap); err != nil {
			return "", err
		}
		_ = db.DeleteEvent(ctx, e.ID)
		return e.Target, nil
	default:
		return "", fmt.Errorf("undo: kind %q not supported yet", e.Kind)
	}
}
