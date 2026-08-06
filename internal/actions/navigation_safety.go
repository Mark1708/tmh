package actions

import (
	"context"
	"fmt"

	"github.com/mark1708/tmh/internal/tmux"
)

// CleanupActiveAliasesBeforeKill removes active session aliases for a source
// session (and optionally window) before the source is killed. This ensures
// that tmh-managed kills never leave orphaned aliases in the "active" session.
//
// The function must use fresh links and only unlink while a source/non-active
// link is still verified. On any cleanup failure, the original kill must be
// aborted by the caller.
//
// windowName may be empty for session-level kills; only windows matching
// the specified name are cleaned for window-level kills.
//
// Returns nil on success or if no active aliases exist. Returns an error
// on cleanup failures that should abort the kill.
func CleanupActiveAliasesBeforeKill(
	ctx context.Context,
	runner tmux.Runner,
	sourceSession string,
	windowName string,
) error {
	// Verify active session ownership first
	ownership, err := inspectActiveOwnership(ctx, runner)
	if err != nil {
		return fmt.Errorf("cleanup active aliases: inspect ownership: %w", err)
	}
	if ownership.collision {
		return fmt.Errorf("cleanup active aliases: %w", activeCollisionError(ownership))
	}
	if !ownership.exists || !ownership.owned {
		// No owned active session, nothing to clean
		return nil
	}

	// Get fresh link snapshot
	links, err := runner.ListWindowLinks(ctx)
	if err != nil {
		return fmt.Errorf("cleanup active aliases: list links: %w", err)
	}

	// Group links by window_id
	linksByWindow := make(map[string][]tmux.WindowLink)
	for _, link := range links {
		linksByWindow[link.WindowID] = append(linksByWindow[link.WindowID], link)
	}

	// Find and unlink active aliases for matching source windows
	for _, windowLinks := range linksByWindow {
		// Check if there's an active alias
		var activeLink *tmux.WindowLink
		var sourceLinks []tmux.WindowLink

		for _, link := range windowLinks {
			if link.SessionName == ActiveSessionName {
				activeLink = &link
				continue
			}
			// Check if this source matches
			if link.SessionName == sourceSession {
				if windowName == "" || link.WindowName == windowName {
					sourceLinks = append(sourceLinks, link)
				}
			}
		}

		// No active alias or no matching source, skip
		if activeLink == nil || len(sourceLinks) == 0 {
			continue
		}

		// Safety: only unlink if there's at least one non-active source link
		// for this window (last-link preservation)
		nonActiveLinks := 0
		for _, link := range windowLinks {
			if link.SessionName != ActiveSessionName {
				nonActiveLinks++
			}
		}

		if nonActiveLinks == 0 {
			// Last link would be removed - preserve as orphan
			continue
		}

		// Verify source still exists (fresh verification)
		sourceVerified := false
		for _, source := range sourceLinks {
			if source.SessionID != "" {
				sourceVerified = true
				break
			}
		}

		if !sourceVerified {
			continue
		}

		// Unlink the active alias
		if err := contextError(ctx, "unlink active alias"); err != nil {
			return err
		}
		if err := runner.UnlinkWindow(ctx, ActiveSessionName, activeLink.WindowID); err != nil {
			return fmt.Errorf("cleanup active aliases: unlink %s from %s: %w",
				activeLink.WindowID, ActiveSessionName, err)
		}
	}

	return nil
}
