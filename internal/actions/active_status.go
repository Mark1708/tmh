package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
)

func ActiveStatus(
	ctx context.Context,
	runner tmux.Runner,
	store ActiveWindowStore,
	cfg config.ActiveSessionConfig,
	now time.Time,
) (ActiveStatusReport, error) {
	report := ActiveStatusReport{Enabled: cfg.Enabled, Session: ActiveSessionName}
	if !cfg.Enabled {
		return report, nil
	}
	if store == nil {
		return report, fmt.Errorf("active integration enabled but state database unavailable")
	}
	ttl, err := activeTTL(cfg)
	if err != nil {
		return ActiveStatusReport{}, err
	}
	report.TTL = ttl.String()
	ownership, err := inspectActiveOwnership(ctx, runner)
	if err != nil {
		return ActiveStatusReport{}, err
	}
	report.Owned = ownership.owned
	report.Collision = ownership.collision
	report.CollisionReason = ownership.reason
	epoch, err := runner.ServerEpoch(ctx)
	if err != nil {
		return ActiveStatusReport{}, fmt.Errorf("read active status epoch: %w", err)
	}
	report.ServerEpoch = epoch.Value
	rows, err := store.ListActiveWindows(ctx, epoch.Value)
	if err != nil {
		return ActiveStatusReport{}, fmt.Errorf("list active status rows: %w", err)
	}
	snapshot, err := loadActiveSnapshot(ctx, runner)
	if err != nil {
		return ActiveStatusReport{}, err
	}
	report.Entries = make([]ActiveWindowStatus, 0, len(rows))
	for _, row := range rows {
		entry := buildActiveStatusEntry(row, snapshot.forWindow(row.WindowID), now)
		report.Entries = append(report.Entries, entry)
		if row.State == state.ActiveWindowOrphaned {
			report.Orphaned++
		} else {
			report.Tracked++
		}
	}
	return report, nil
}

func buildActiveStatusEntry(row state.ActiveWindow, links activeLinkSet, now time.Time) ActiveWindowStatus {
	entry := ActiveWindowStatus{
		WindowID:       row.WindowID,
		WindowName:     row.WindowName,
		State:          row.State,
		SourceLinks:    make([]ActiveSourceLink, 0, len(links.sources)),
		PromotedAt:     row.PromotedAt,
		LastSelectedAt: row.LastSelectedAt,
		ExpiresAt:      row.ExpiresAt,
		OrphanedAt:     row.OrphanedAt,
		Expired:        !row.ExpiresAt.After(now),
	}
	if links.hasActive {
		index := links.active.WindowIndex
		entry.ActiveIndex = &index
	}
	for _, source := range links.sources {
		entry.SourceLinks = append(entry.SourceLinks, ActiveSourceLink{
			SessionID: source.SessionID, SessionName: source.SessionName, WindowIndex: source.WindowIndex,
		})
	}
	return entry
}
