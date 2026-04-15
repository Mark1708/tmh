package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/tmux"
)

// expandHome rewrites a leading ~ to $HOME. tmux source-file and
// `source <path>` need an absolute path — the shell normally expands ~
// before exec, but exec.Command does not.
func expandHome(p string) string {
	if p == "" {
		return p
	}
	if p == "~" {
		return os.Getenv("HOME")
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(os.Getenv("HOME"), p[2:])
	}
	return p
}

// ReloadOptions selects what to reload and how.
type ReloadOptions struct {
	Shell    bool   // source ~/.zshrc in idle shell panes
	Tmux     bool   // tmux source-file ~/.tmux.conf
	Busy     bool   // enqueue busy panes for deferred reload
	Respawn  bool   // kill-server + init from snapshot (not wired yet)
	RcFile   string // overrides ~/.zshrc for `--shell`
	TmuxConf string // overrides ~/.tmux.conf for `--tmux`
	BusyTTL  time.Duration
}

// ReloadReport summarises what reload touched.
type ReloadReport struct {
	ReloadedPanes []string
	QueuedPanes   []string
	SkippedBusy   []string
	TmuxSourced   bool
}

// Reload performs both --shell and --tmux actions as requested. `db` is
// optional; required only for --busy (deferred queue).
func Reload(ctx context.Context, r tmux.Runner, db *state.DB, rc string, opts ReloadOptions) (*ReloadReport, error) {
	rep := &ReloadReport{}
	if opts.RcFile == "" {
		opts.RcFile = rc
	}
	if opts.BusyTTL == 0 {
		opts.BusyTTL = 10 * time.Minute
	}

	rcExpanded := expandHome(opts.RcFile)
	if opts.Shell {
		panes, err := r.ListPanes(ctx, "")
		if err != nil {
			return rep, err
		}
		for _, p := range panes {
			if isIdleShell(p.Command) {
				target := paneTarget(p)
				if err := r.SendKeys(ctx, target, fmt.Sprintf("source %s", rcExpanded), "Enter"); err != nil {
					return rep, err
				}
				rep.ReloadedPanes = append(rep.ReloadedPanes, target)
				continue
			}
			if opts.Busy && db != nil {
				target := paneTarget(p)
				if err := db.EnqueueReload(ctx, p.ID, target, "shell", opts.BusyTTL); err != nil {
					return rep, err
				}
				rep.QueuedPanes = append(rep.QueuedPanes, target)
			} else {
				rep.SkippedBusy = append(rep.SkippedBusy, paneTarget(p)+" ("+p.Command+")")
			}
		}
	}

	if opts.Tmux {
		path := opts.TmuxConf
		if path == "" {
			path = "~/.tmux.conf"
		}
		if err := r.SourceFile(ctx, expandHome(path)); err != nil {
			return rep, err
		}
		rep.TmuxSourced = true
	}

	return rep, nil
}

// DrainReloadQueue walks the deferred queue once: any entry whose pane has
// gone idle gets its reload executed and is removed.
func DrainReloadQueue(ctx context.Context, r tmux.Runner, db *state.DB, rcFile string) (int, error) {
	if db == nil {
		return 0, nil
	}
	entries, err := db.PendingReloads(ctx)
	if err != nil {
		return 0, err
	}
	if len(entries) == 0 {
		return 0, nil
	}
	panes, err := r.ListPanes(ctx, "")
	if err != nil {
		return 0, err
	}
	byID := make(map[string]tmux.Pane, len(panes))
	for _, p := range panes {
		byID[p.ID] = p
	}
	done := 0
	for _, e := range entries {
		p, ok := byID[e.PaneID]
		if !ok {
			// pane gone entirely — drop it
			_ = db.DequeueReload(ctx, e.PaneID)
			continue
		}
		if !isIdleShell(p.Command) {
			continue
		}
		expanded := expandHome(rcFile)
		switch e.Action {
		case "shell":
			if err := r.SendKeys(ctx, e.PaneTarget, fmt.Sprintf("source %s", expanded), "Enter"); err != nil {
				return done, err
			}
		case "tmux":
			if err := r.SourceFile(ctx, expanded); err != nil {
				return done, err
			}
		}
		_ = db.DequeueReload(ctx, e.PaneID)
		done++
	}
	// ExpireReloads handles TTL outside of this drain path.
	if _, err := db.ExpireReloads(ctx); err != nil {
		return done, err
	}
	return done, nil
}

// PendingReloads returns the current deferred queue depth.
func PendingReloads(ctx context.Context, db *state.DB) ([]state.ReloadEntry, error) {
	if db == nil {
		return nil, nil
	}
	return db.PendingReloads(ctx)
}

func isIdleShell(cmd string) bool {
	switch strings.ToLower(cmd) {
	case "zsh", "-zsh", "bash", "-bash", "sh", "-sh", "fish", "-fish":
		return true
	}
	return false
}

func paneTarget(p tmux.Pane) string {
	return fmt.Sprintf("%s:%d.%d", p.Session, p.Window, p.Index)
}
