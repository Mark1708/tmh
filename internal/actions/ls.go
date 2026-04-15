package actions

import (
	"context"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/tmux"
)

// Listing is a merged view of live tmux sessions and the configured ones.
// It's what `tmh ls` renders and what the TUI dashboard consumes.
type Listing struct {
	Sessions []ListedSession
}

// ListedSession combines config + live data for one session.
type ListedSession struct {
	Name       string
	Groups     []string
	Live       bool
	Attached   bool
	Configured bool
	Windows    []ListedWindow
}

// ListedWindow combines config + live data for one window.
type ListedWindow struct {
	Name       string
	Index      int
	Live       bool
	Configured bool
	Layout     string
	Panes      int
	Dir        string // pane_current_path of first pane when live
}

// BuildListing merges a resolved config view with live tmux state.
func BuildListing(ctx context.Context, r tmux.Runner, cfg *config.Config, profile string) (*Listing, error) {
	resolved, err := config.Resolve(cfg, profile)
	if err != nil {
		return nil, err
	}

	live, err := r.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	liveMap := make(map[string]tmux.Session, len(live))
	for _, s := range live {
		liveMap[s.Name] = s
	}

	out := &Listing{}
	seen := make(map[string]bool)

	for _, rs := range resolved.Sessions {
		seen[rs.Name] = true
		ls, _ := liveMap[rs.Name]
		sess := ListedSession{
			Name:       rs.Name,
			Groups:     rs.Group,
			Live:       ls.Name != "",
			Attached:   ls.Attached,
			Configured: true,
		}
		liveWindows, _ := r.ListWindows(ctx, rs.Name)
		liveByName := make(map[string]tmux.Window, len(liveWindows))
		for _, w := range liveWindows {
			liveByName[w.Name] = w
		}
		for _, rw := range rs.Windows {
			w := ListedWindow{
				Name:       rw.Name,
				Configured: true,
				Layout:     rw.Layout,
			}
			if lw, ok := liveByName[rw.Name]; ok {
				w.Live = true
				w.Index = lw.Index
				w.Panes = lw.Panes
			}
			sess.Windows = append(sess.Windows, w)
		}
		// windows present live but not in config ⇒ new
		for _, lw := range liveWindows {
			found := false
			for _, rw := range rs.Windows {
				if rw.Name == lw.Name {
					found = true
					break
				}
			}
			if !found {
				sess.Windows = append(sess.Windows, ListedWindow{
					Name:  lw.Name,
					Index: lw.Index,
					Live:  true,
					Panes: lw.Panes,
				})
			}
		}
		out.Sessions = append(out.Sessions, sess)
	}

	// live-only (ad-hoc) sessions appear at the end
	for _, ls := range live {
		if seen[ls.Name] {
			continue
		}
		sess := ListedSession{Name: ls.Name, Live: true, Attached: ls.Attached}
		liveWindows, _ := r.ListWindows(ctx, ls.Name)
		for _, lw := range liveWindows {
			sess.Windows = append(sess.Windows, ListedWindow{
				Name: lw.Name, Index: lw.Index, Live: true, Panes: lw.Panes,
			})
		}
		out.Sessions = append(out.Sessions, sess)
	}

	return out, nil
}
