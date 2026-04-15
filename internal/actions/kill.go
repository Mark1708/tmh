package actions

import (
	"context"
	"fmt"
	"strings"

	"git.mark1708.ru/me/tmh/internal/tmux"
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
		if err := r.KillSession(ctx, s.Name); err != nil {
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
