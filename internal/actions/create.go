package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/mark1708/tmh/internal/config"
	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/tmux"
)

// CreateSession materialises one ResolvedSession in tmux. Existing sessions
// are left untouched (ErrSessionExists surfaced as no-op).
func CreateSession(ctx context.Context, r tmux.Runner, s config.ResolvedSession, layouts map[string]config.Layout) error {
	if s.Name == "" {
		return fmt.Errorf("actions: session name required")
	}
	exists, err := r.HasSession(ctx, s.Name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if len(s.Windows) == 0 {
		return r.NewSession(ctx, tmux.NewSessionOpts{
			Name:     s.Name,
			Dir:      s.Dir,
			Env:      s.Env,
			Detached: true,
		})
	}

	first := s.Windows[0]
	if err := r.NewSession(ctx, tmux.NewSessionOpts{
		Name:       s.Name,
		Dir:        firstNonEmpty(first.Dir, s.Dir),
		WindowName: first.Name,
		Env:        s.Env,
		Detached:   true,
	}); err != nil && !errors.Is(err, errs.ErrSessionExists) {
		return err
	}
	firstTarget := fmt.Sprintf("%s:%d", s.Name, 1)
	if err := applyWindowLayout(ctx, r, firstTarget, first, layouts); err != nil {
		return fmt.Errorf("session %q window %q: %w", s.Name, first.Name, err)
	}

	for i := 1; i < len(s.Windows); i++ {
		w := s.Windows[i]
		win, err := r.NewWindow(ctx, tmux.NewWindowOpts{
			SessionTarget: s.Name + ":",
			Name:          w.Name,
			Dir:           firstNonEmpty(w.Dir, s.Dir),
			Env:           s.Env,
		})
		if err != nil {
			return fmt.Errorf("session %q window %q: %w", s.Name, w.Name, err)
		}
		target := fmt.Sprintf("%s:%d", s.Name, win.Index)
		if err := applyWindowLayout(ctx, r, target, w, layouts); err != nil {
			return fmt.Errorf("session %q window %q: %w", s.Name, w.Name, err)
		}
	}

	// Select the first focus:true window, otherwise leave first as active.
	for _, w := range s.Windows {
		if w.Focus {
			_ = r.SelectWindow(ctx, fmt.Sprintf("%s:%s", s.Name, w.Name))
			break
		}
	}
	return nil
}

// applyWindowLayout creates splits and applies the requested layout to an
// already-created window. The first pane is assumed to exist.
func applyWindowLayout(ctx context.Context, r tmux.Runner, target string, w config.ResolvedWindow, layouts map[string]config.Layout) error {
	layout := w.Layout
	if layout == "" {
		layout = "1-pane"
	}

	// Lock the window name so tmux's after-new-window hook can't rewrite it.
	if err := r.SetAutomaticRename(ctx, target, false); err != nil {
		return err
	}

	switch layout {
	case "1-pane":
		// nothing to split
	case "2-pane":
		if err := r.SplitWindow(ctx, tmux.SplitOpts{Target: target, Horizontal: true, Dir: w.Dir}); err != nil {
			return err
		}
	case "3-pane":
		if err := r.SplitWindow(ctx, tmux.SplitOpts{Target: target, Horizontal: true, Dir: w.Dir}); err != nil {
			return err
		}
		// right side: split vertically to get top/bottom
		if err := r.SplitWindow(ctx, tmux.SplitOpts{Target: target, Horizontal: false, Dir: w.Dir}); err != nil {
			return err
		}
		if err := r.SelectLayout(ctx, target, "main-vertical"); err != nil {
			return err
		}
	default:
		// custom layout: create panes matching expected count from layouts[], then apply hash
		lay, ok := layouts[layout]
		if !ok {
			return fmt.Errorf("%w: %s", errs.ErrUnknownLayout, layout)
		}
		// For custom layouts we rely on the user having recorded a hash that
		// matches a specific pane count; we honour explicit panes[] if given.
		for i := 1; i < len(w.Panes); i++ {
			if err := r.SplitWindow(ctx, tmux.SplitOpts{Target: target, Horizontal: true, Dir: w.Dir}); err != nil {
				return err
			}
		}
		if err := r.SelectLayout(ctx, target, lay.Hash); err != nil {
			return err
		}
	}

	// Apply explicit pane overrides if declared.
	for i, p := range w.Panes {
		if p.Command != "" {
			paneTarget := fmt.Sprintf("%s.%d", target, i+1)
			if err := r.SendKeys(ctx, paneTarget, p.Command, "Enter"); err != nil {
				return err
			}
		}
	}

	// Execute main command on pane 1 if specified.
	if w.Command != "" && len(w.Panes) == 0 {
		paneTarget := fmt.Sprintf("%s.%d", target, 1)
		if err := r.SendKeys(ctx, paneTarget, w.Command, "Enter"); err != nil {
			return err
		}
	}

	// Re-assert window name in case split-window changed it.
	if err := r.RenameWindow(ctx, target, w.Name); err != nil {
		return err
	}
	return nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
