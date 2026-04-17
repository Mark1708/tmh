package actions

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux"
)

// FreezeOptions tunes Freeze behaviour.
type FreezeOptions struct {
	// Session, when non-empty, restricts freeze to the named live session.
	Session string
	// DryRun reports what would change without mutating cfg.Node.
	DryRun bool
}

// FreezeReport summarises what Freeze did (or would do, under DryRun).
//
// The semantic contract is non-destructive: sessions or windows absent
// from the config are added with inferred roots (when they fit an
// existing roots entry) or as absolute-dir shorthand; sessions and
// windows that already exist in the config are left untouched, and
// windows whose live dir differs from the configured dir are reported
// as Conflicts — the user resolves them explicitly via `tmh sync --pull
// --all` or by hand editing.
type FreezeReport struct {
	AddedSessions []string // "session-name"
	AddedWindows  []string // "session/window"
	Conflicts     []string // "session/window — live=<a> config=<b>"
	Unchanged     []string // "session/window"
}

// Freeze inserts live tmux sessions/windows into cfg, preserving the
// existing YAML tree (comments, templates, profiles, hooks) via
// config.PathSet. The caller is responsible for calling config.Write to
// persist cfg.Node.
//
// Freeze is the authoring complement to Diff: build your layout by
// hand, freeze it into YAML, then drift detection becomes meaningful.
func Freeze(ctx context.Context, r tmux.Runner, cfg *config.Config, opts FreezeOptions) (*FreezeReport, error) {
	if cfg == nil || cfg.Node == nil {
		return nil, fmt.Errorf("freeze: nil config (run tmh init first)")
	}
	live, err := collectLive(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("freeze: collect live: %w", err)
	}

	// Merge live roots with existing roots: inferred roots supplement
	// user-declared ones but never override them.
	existingRoots := cfg.Roots
	inferred := inferRoots(live)
	mergedRoots := make(map[string]string, len(existingRoots)+len(inferred))
	for k, v := range existingRoots {
		mergedRoots[k] = v
	}
	for k, v := range inferred {
		if _, alreadyDeclared := mergedRoots[k]; alreadyDeclared {
			continue
		}
		mergedRoots[k] = v
	}

	rep := &FreezeReport{}
	for _, ls := range live.Sessions {
		if opts.Session != "" && ls.Name != opts.Session {
			continue
		}
		_, configHasSession := cfg.Sessions[ls.Name]
		if !configHasSession {
			rep.AddedSessions = append(rep.AddedSessions, ls.Name)
			if !opts.DryRun {
				if err := freezeSession(cfg, mergedRoots, ls); err != nil {
					return rep, err
				}
			}
			continue
		}
		// Session exists — walk windows.
		if err := freezeWindows(cfg, mergedRoots, ls, rep, opts.DryRun); err != nil {
			return rep, err
		}
	}

	// Persist any newly inferred roots before the caller writes the file.
	if !opts.DryRun {
		for name, base := range mergedRoots {
			if _, existed := existingRoots[name]; existed {
				continue
			}
			if err := config.PathSet(cfg.Node, "roots."+name, base); err != nil {
				return rep, fmt.Errorf("freeze: set roots.%s: %w", name, err)
			}
		}
	}
	return rep, nil
}

func freezeSession(cfg *config.Config, roots map[string]string, ls config.LiveSession) error {
	for _, w := range ls.Windows {
		base := fmt.Sprintf("sessions.%s.windows.%s", ls.Name, w.Name)
		if w.Dir == "" {
			continue
		}
		rootName, rel := matchRoot(roots, w.Dir)
		if rootName != "" {
			if err := config.PathSet(cfg.Node, base+".root", rootName); err != nil {
				return fmt.Errorf("freeze: set %s.root: %w", base, err)
			}
			if err := config.PathSet(cfg.Node, base+".path", rel); err != nil {
				return fmt.Errorf("freeze: set %s.path: %w", base, err)
			}
			continue
		}
		if err := config.PathSet(cfg.Node, base, w.Dir); err != nil {
			return fmt.Errorf("freeze: set %s: %w", base, err)
		}
	}
	return nil
}

func freezeWindows(cfg *config.Config, roots map[string]string, ls config.LiveSession, rep *FreezeReport, dryRun bool) error {
	cs := cfg.Sessions[ls.Name]
	for _, lw := range ls.Windows {
		cw, exists := cs.Windows.Entries[lw.Name]
		entry := ls.Name + "/" + lw.Name
		if !exists {
			rep.AddedWindows = append(rep.AddedWindows, entry)
			if dryRun || lw.Dir == "" {
				continue
			}
			base := fmt.Sprintf("sessions.%s.windows.%s", ls.Name, lw.Name)
			rootName, rel := matchRoot(roots, lw.Dir)
			if rootName != "" {
				if err := config.PathSet(cfg.Node, base+".root", rootName); err != nil {
					return err
				}
				if err := config.PathSet(cfg.Node, base+".path", rel); err != nil {
					return err
				}
				continue
			}
			if err := config.PathSet(cfg.Node, base, lw.Dir); err != nil {
				return err
			}
			continue
		}
		// Window exists: compare dirs.
		cwDir := cw.Dir
		if cwDir == "" && cw.Root != "" {
			if base, ok := roots[cw.Root]; ok {
				cwDir = joinClean(base, cw.Path)
			}
		}
		switch {
		case cwDir == lw.Dir, cwDir == "" && lw.Dir == "":
			rep.Unchanged = append(rep.Unchanged, entry)
		default:
			rep.Conflicts = append(rep.Conflicts,
				fmt.Sprintf("%s — live=%q config=%q", entry, lw.Dir, cwDir))
		}
	}
	return nil
}

// joinClean is a freeze-local wrapper that mirrors config.JoinClean for
// cases where we need to compare the configured <root, path> pair
// against a live absolute path. Matches the behaviour in
// config/resolver.go without re-exporting a private helper.
func joinClean(base, rel string) string {
	if rel == "" {
		return base
	}
	return fmt.Sprintf("%s/%s", base, rel)
}
