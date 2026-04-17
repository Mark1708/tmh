package actions

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux"
)

// InitOptions filters which sessions Init creates.
type InitOptions struct {
	Profile string   // matches config.Profiles[<name>]
	Only    []string // if non-empty, only these session names are created
}

// Init materialises every session in the resolved config subject to filters.
// Existing sessions are skipped. Returns the first error (if any) but does
// continue to attempt remaining sessions before returning, so partial setup
// isn't lost on a single failure.
func Init(ctx context.Context, r tmux.Runner, cfg *config.Config, opts InitOptions) error {
	resolved, err := config.Resolve(cfg, opts.Profile)
	if err != nil {
		return err
	}
	only := stringSet(opts.Only)

	var firstErr error
	for _, s := range resolved.Sessions {
		if len(only) > 0 && !only[s.Name] {
			continue
		}
		if err := CreateSession(ctx, r, s, cfg.Layouts); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("init %q: %w", s.Name, err)
			}
		}
	}
	return firstErr
}

func stringSet(in []string) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for _, s := range in {
		out[s] = true
	}
	return out
}
