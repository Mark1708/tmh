package actions

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"git.mark1708.ru/me/tmh/internal/config"
	errs "git.mark1708.ru/me/tmh/internal/errors"
	"git.mark1708.ru/me/tmh/internal/tmux"
)

// SyncOptions controls sync behaviour.
type SyncOptions struct {
	Profile  string
	DryRun   bool
	ApplyAll bool // for --push: update drifted dirs too; for --pull: replace drifted entries
}

// SyncReport summarises what sync did (or would do, under DryRun).
type SyncReport struct {
	Created []string // "session" or "session/window"
	Updated []string
	Deleted []string
	Skipped []string
}

// Push reconciles live tmux to match the resolved config. Missing sessions
// and windows are created; existing ones are left alone unless ApplyAll is
// set (in which case drifted dirs are not rewritten — tmux can't move a pane
// non-destructively — but the drift is recorded for reporting).
func Push(ctx context.Context, r tmux.Runner, cfg *config.Config, opts SyncOptions) (*SyncReport, error) {
	resolved, err := config.Resolve(cfg, opts.Profile)
	if err != nil {
		return nil, err
	}
	rep := &SyncReport{}
	for _, s := range resolved.Sessions {
		exists, err := r.HasSession(ctx, s.Name)
		if err != nil {
			return rep, err
		}
		if !exists {
			rep.Created = append(rep.Created, s.Name)
			if opts.DryRun {
				continue
			}
			if err := CreateSession(ctx, r, s, cfg.Layouts); err != nil && !errors.Is(err, errs.ErrSessionExists) {
				return rep, fmt.Errorf("push session %q: %w", s.Name, err)
			}
			continue
		}
		// session exists — ensure configured windows exist
		liveWins, err := r.ListWindows(ctx, s.Name)
		if err != nil {
			return rep, err
		}
		byName := make(map[string]tmux.Window, len(liveWins))
		for _, w := range liveWins {
			byName[w.Name] = w
		}
		for _, cw := range s.Windows {
			if _, ok := byName[cw.Name]; ok {
				continue
			}
			rep.Created = append(rep.Created, s.Name+"/"+cw.Name)
			if opts.DryRun {
				continue
			}
			win, err := r.NewWindow(ctx, tmux.NewWindowOpts{
				SessionTarget: s.Name + ":",
				Name:          cw.Name,
				Dir:           cw.Dir,
				Env:           s.Env,
			})
			if err != nil {
				return rep, fmt.Errorf("push window %q/%q: %w", s.Name, cw.Name, err)
			}
			target := fmt.Sprintf("%s:%d", s.Name, win.Index)
			if err := applyWindowLayout(ctx, r, target, cw, cfg.Layouts); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}

// Pull updates the config to match live tmux. `new` windows become config
// entries; `drift` entries (window dir changed) have their dir rewritten.
// `gone` entries are preserved unless opts.ApplyAll is true, in which case
// they are removed.
//
// Write the returned Config back to disk via config.Write to persist.
func Pull(ctx context.Context, r tmux.Runner, cfg *config.Config, opts SyncOptions) (*SyncReport, error) {
	rep := &SyncReport{}
	live, err := collectLive(ctx, r)
	if err != nil {
		return rep, err
	}
	resolved, err := config.Resolve(cfg, opts.Profile)
	if err != nil {
		return rep, err
	}
	drift := config.Diff(resolved, live)

	for _, d := range drift {
		switch d.Status {
		case config.StatusNew:
			path := fmt.Sprintf("sessions.%s.windows.%s", d.Session, d.Window)
			rep.Created = append(rep.Created, d.ConfigEntry)
			if opts.DryRun {
				continue
			}
			if err := config.PathSet(cfg.Node, path, d.LiveDir); err != nil {
				return rep, fmt.Errorf("pull add %s: %w", path, err)
			}
		case config.StatusDrift:
			if !opts.ApplyAll {
				rep.Skipped = append(rep.Skipped, d.ConfigEntry+" (dir drift, use --all to apply)")
				continue
			}
			path := fmt.Sprintf("sessions.%s.windows.%s", d.Session, d.Window)
			rep.Updated = append(rep.Updated, d.ConfigEntry)
			if opts.DryRun {
				continue
			}
			// Window may currently be the shorthand scalar form — to set
			// dir we need to replace the shorthand with the new path.
			if err := config.PathSet(cfg.Node, path, d.LiveDir); err != nil {
				return rep, err
			}
		case config.StatusGone:
			if !opts.ApplyAll {
				rep.Skipped = append(rep.Skipped, d.ConfigEntry+" (gone, use --all to delete)")
				continue
			}
			path := fmt.Sprintf("sessions.%s.windows.%s", d.Session, d.Window)
			rep.Deleted = append(rep.Deleted, d.ConfigEntry)
			if opts.DryRun {
				continue
			}
			if err := config.PathDelete(cfg.Node, path); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}

// Bootstrap imports every live session into a fresh config, inferring roots
// by longest-common-prefix across first-pane paths. Intended for first-run
// when config.yml is empty.
func Bootstrap(ctx context.Context, r tmux.Runner, cfg *config.Config) (*SyncReport, error) {
	live, err := collectLive(ctx, r)
	if err != nil {
		return nil, err
	}
	roots := inferRoots(live)
	rep := &SyncReport{}

	// Populate roots map.
	for name, base := range roots {
		if err := config.PathSet(cfg.Node, "roots."+name, base); err != nil {
			return rep, fmt.Errorf("bootstrap roots.%s: %w", name, err)
		}
	}

	for _, s := range live.Sessions {
		for _, w := range s.Windows {
			entry := s.Name + "/" + w.Name
			if w.Dir == "" {
				// tmux did not report a cwd (e.g. a brand-new pane before
				// its shell wrote one); record the window with an empty
				// mapping so the user can fill it in later.
				rep.Skipped = append(rep.Skipped, entry+" (no live cwd)")
				continue
			}
			rep.Created = append(rep.Created, entry)
			rootName, rel := matchRoot(roots, w.Dir)
			basePath := fmt.Sprintf("sessions.%s.windows.%s", s.Name, w.Name)
			if rootName != "" {
				if err := config.PathSet(cfg.Node, basePath+".root", rootName); err != nil {
					return rep, err
				}
				if err := config.PathSet(cfg.Node, basePath+".path", rel); err != nil {
					return rep, err
				}
				continue
			}
			// fall back to absolute dir as a scalar shorthand
			if err := config.PathSet(cfg.Node, basePath, w.Dir); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}

// collectLive builds a LiveSnapshot from the runner.
//
// `tmux list-panes -t SESSION` only returns panes in the session's currently
// active window; to get every pane we call list-panes with an empty target
// (which translates to `-a` inside the runner) and filter client-side.
func collectLive(ctx context.Context, r tmux.Runner) (config.LiveSnapshot, error) {
	var snap config.LiveSnapshot
	sessions, err := r.ListSessions(ctx)
	if err != nil {
		return snap, err
	}
	allPanes, err := r.ListPanes(ctx, "")
	if err != nil {
		return snap, err
	}
	type winKey struct {
		session string
		window  int
	}
	firstDir := make(map[winKey]string, len(allPanes))
	for _, p := range allPanes {
		k := winKey{p.Session, p.Window}
		if _, set := firstDir[k]; !set {
			firstDir[k] = p.Path
		}
	}
	for _, s := range sessions {
		wins, err := r.ListWindows(ctx, s.Name)
		if err != nil {
			return snap, err
		}
		ls := config.LiveSession{Name: s.Name}
		for _, w := range wins {
			ls.Windows = append(ls.Windows, config.LiveWindow{
				Name: w.Name,
				Dir:  firstDir[winKey{s.Name, w.Index}],
			})
		}
		snap.Sessions = append(snap.Sessions, ls)
	}
	return snap, nil
}

// inferRoots finds the shallowest fork point in the trie of session paths
// and promotes every distinct child at that fork into a root entry. See
// plan §11.
//
// Example — paths
//
//	/Users/mark/Projects/otr/x/a
//	/Users/mark/Projects/otr/x/b
//	/Users/mark/Projects/me/y/a
//
// share /Users/mark/Projects, which forks into {otr, me}. The resulting
// roots are otr=/Users/mark/Projects/otr and me=/Users/mark/Projects/me.
func inferRoots(snap config.LiveSnapshot) map[string]string {
	paths := make([]string, 0)
	for _, s := range snap.Sessions {
		for _, w := range s.Windows {
			if w.Dir != "" && filepath.IsAbs(w.Dir) {
				paths = append(paths, w.Dir)
			}
		}
	}
	if len(paths) == 0 {
		return nil
	}

	commonParts := splitPath(paths[0])
	for _, p := range paths[1:] {
		commonParts = lcpSegments(commonParts, splitPath(p))
		if len(commonParts) == 0 {
			return nil
		}
	}

	// A single path (or all identical) yields no fork — nothing to infer.
	if len(paths) == 1 {
		return nil
	}

	roots := make(map[string]string)
	usedBase := make(map[string]string)
	for _, p := range paths {
		parts := splitPath(p)
		if len(parts) <= len(commonParts) {
			continue
		}
		childSeg := parts[len(commonParts)]
		base := joinPath(append(append([]string(nil), commonParts...), childSeg))

		name := childSeg
		original := name
		i := 2
		for existing, ok := usedBase[name]; ok && existing != base; existing, ok = usedBase[name] {
			name = fmt.Sprintf("%s%d", original, i)
			i++
		}
		if _, seen := roots[name]; !seen {
			roots[name] = base
			usedBase[name] = base
		}
	}
	return roots
}

// splitPath returns the segments of an absolute path starting with an empty
// string for the leading "/". joinPath is its inverse.
func splitPath(p string) []string {
	return strings.Split(p, string(filepath.Separator))
}

func joinPath(parts []string) string {
	return strings.Join(parts, string(filepath.Separator))
}

// lcpSegments returns the longest common prefix of two split-path slices.
func lcpSegments(a, b []string) []string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	out := a[:0]
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			break
		}
		out = append(out, a[i])
	}
	return out
}

// matchRoot finds the longest root that is a prefix of dir and returns
// (root-name, relative-path). If none matches, returns ("", dir).
func matchRoot(roots map[string]string, dir string) (string, string) {
	type pair struct{ name, base string }
	var all []pair
	for n, b := range roots {
		all = append(all, pair{n, b})
	}
	// sort descending by base length so the most specific wins
	sort.Slice(all, func(i, j int) bool { return len(all[i].base) > len(all[j].base) })
	for _, p := range all {
		if strings.HasPrefix(dir, p.base+"/") || dir == p.base {
			rel := strings.TrimPrefix(dir, p.base)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "" {
				rel = "."
			}
			return p.name, rel
		}
	}
	return "", dir
}
