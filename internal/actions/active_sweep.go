package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux"
)

type ActivePruneResult struct {
	WindowID   string
	Transition string
}

type ActivePruneReport struct {
	Unlinked int                 `json:"unlinked"`
	Deleted  int                 `json:"deleted"`
	Orphaned int                 `json:"orphaned"`
	Results  []ActivePruneResult `json:"results,omitempty"`
}

func PruneActiveWindows(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	cfg config.ActiveSessionConfig,
	now time.Time,
) (ActivePruneReport, error) {
	if _, err := activeTTL(cfg); err != nil {
		return ActivePruneReport{}, err
	}
	if _, err := requireOwnedActiveSession(ctx, runner); err != nil {
		return ActivePruneReport{}, err
	}
	epoch, err := reconcileActiveEpoch(ctx, runner, store)
	if err != nil {
		return ActivePruneReport{}, err
	}
	rows, err := store.ListActiveWindows(ctx, epoch.Value)
	if err != nil {
		return ActivePruneReport{}, fmt.Errorf("list active rows for prune: %w", err)
	}
	snapshot, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return ActivePruneReport{}, err
	}
	classified := classifyActiveRows(rows, snapshot, now)
	return applyActiveTransitions(ctx, runner, store, epoch.Value, snapshot, classified, now)
}

func applyActiveTransitions(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	epoch string,
	snapshot activeSnapshot,
	classified []classifiedActiveWindow,
	now time.Time,
) (ActivePruneReport, error) {
	report := ActivePruneReport{Results: make([]ActivePruneResult, 0, len(classified))}
	for _, item := range classified {
		if err := contextError(ctx, "prune active windows"); err != nil {
			return report, err
		}
		var err error
		snapshot, err = applyActiveTransition(ctx, runner, store, epoch, snapshot, item, now, &report)
		if err != nil {
			return report, err
		}
		report.Results = append(report.Results, ActivePruneResult{
			WindowID: item.row.WindowID, Transition: string(item.transition),
		})
	}
	return report, nil
}

func applyActiveTransition(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	epoch string,
	snapshot activeSnapshot,
	item classifiedActiveWindow,
	now time.Time,
	report *ActivePruneReport,
) (activeSnapshot, error) {
	switch item.transition {
	case activeKeep:
		return snapshot, nil
	case activeExpireOrphan:
		if err := store.MarkActiveWindowOrphaned(ctx, epoch, item.row.WindowID, now); err != nil {
			return snapshot, fmt.Errorf("mark active window %s orphaned: %w", item.row.WindowID, err)
		}
		report.Orphaned++
		return snapshot, nil
	case activeExpireSafe:
		fresh, err := unlinkVerifiedActiveAlias(ctx, runner, item.row.WindowID, snapshot)
		if err != nil {
			return snapshot, err
		}
		if err := deleteActiveRow(ctx, store, epoch, item.row.WindowID); err != nil {
			return fresh, err
		}
		report.Unlinked++
		report.Deleted++
		return fresh, nil
	case activeStaleRow, activeGoneWindow:
		if err := deleteActiveRow(ctx, store, epoch, item.row.WindowID); err != nil {
			return snapshot, err
		}
		report.Deleted++
		return snapshot, nil
	default:
		return snapshot, fmt.Errorf("unknown active transition %q", item.transition)
	}
}

func deleteActiveRow(ctx context.Context, store ActiveWindowStore, epoch, windowID string) error {
	if err := store.DeleteActiveWindow(ctx, epoch, windowID); err != nil {
		return fmt.Errorf("delete active row %s: %w", windowID, err)
	}
	return nil
}
