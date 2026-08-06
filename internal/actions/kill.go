package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark1708/tmh/internal/tmux"
)

// KillMatching kills every live session whose name matches the given pattern.
// Pattern is a plain substring match by default; prefix `re:` switches to
// Go's regexp syntax.
//
// Returns the list of killed session names. Errors from individual
// KillSession calls are collected and returned as a joined error but do not
// stop processing.
func KillMatching(ctx context.Context, r tmux.Runner, pattern string) ([]string, error) {
	sessions, err := r.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	var (
		killed []string
		errs   []error
	)
	for _, s := range sessions {
		if !sessionMatch(s.Name, pattern) {
			continue
		}
		if err := KillSession(ctx, r, s.Name); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name, err))
			continue
		}
		killed = append(killed, s.Name)
	}
	if len(errs) > 0 {
		return killed, joinErrs(errs)
	}
	return killed, nil
}

func KillSession(ctx context.Context, r tmux.Runner, session string) error {
	if err := CleanupActiveAliasesBeforeKill(ctx, r, session, ""); err != nil {
		return fmt.Errorf("cleanup active aliases: %w", err)
	}
	if err := r.KillSession(ctx, session); err != nil {
		return err
	}
	return nil
}

func KillWindow(ctx context.Context, r tmux.Runner, target string) error {
	if err := CleanupActiveAliasBeforeWindowKill(ctx, r, target); err != nil {
		return fmt.Errorf("cleanup active alias: %w", err)
	}
	if err := r.KillWindow(ctx, target); err != nil {
		return err
	}
	return nil
}

func KillPane(ctx context.Context, r tmux.Runner, target string) error {
	windowTarget := paneWindowTarget(target)
	panes, err := r.ListPanes(ctx, windowTarget)
	if err != nil {
		return err
	}
	if len(panes) <= 1 {
		if err := CleanupActiveAliasBeforeWindowKill(ctx, r, windowTarget); err != nil {
			return fmt.Errorf("cleanup active alias: %w", err)
		}
	}
	if err := r.KillPane(ctx, target); err != nil {
		return err
	}
	return nil
}

func CleanupActiveAliasBeforeWindowKill(ctx context.Context, r tmux.Runner, target string) error {
	ownership, err := inspectActiveOwnership(ctx, r)
	if err != nil {
		return fmt.Errorf("inspect active ownership: %w", err)
	}
	if ownership.collision {
		return activeCollisionError(ownership)
	}
	if !ownership.exists || !ownership.owned {
		return nil
	}
	windowID, err := r.WindowID(ctx, target)
	if err != nil {
		return fmt.Errorf("resolve window target %q: %w", target, err)
	}
	snapshot, err := loadActiveSnapshot(ctx, r)
	if err != nil {
		return err
	}
	if !snapshot.forWindow(windowID).hasActive {
		return nil
	}
	if _, err := unlinkVerifiedActiveAlias(ctx, r, windowID, snapshot); err != nil {
		return err
	}
	return nil
}

func paneWindowTarget(target string) string {
	colon := strings.LastIndexByte(target, ':')
	dot := strings.LastIndexByte(target, '.')
	if dot > colon && dot >= 0 {
		return target[:dot]
	}
	return target
}

func sessionMatch(name, pattern string) bool {
	if pattern == "" {
		return true
	}
	return strings.Contains(name, pattern)
}

type multiErr struct{ items []error }

func (m *multiErr) Error() string {
	parts := make([]string, len(m.items))
	for i, e := range m.items {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}
func (m *multiErr) Unwrap() []error { return m.items }

func joinErrs(items []error) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		return items[0]
	}
	return &multiErr{items: items}
}
