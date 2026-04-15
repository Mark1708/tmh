package state

import (
	"context"
	"time"
)

// ReloadEntry is a pane that needs to be reloaded once idle.
type ReloadEntry struct {
	PaneID     string
	PaneTarget string
	QueuedAt   time.Time
	ExpiresAt  time.Time
	Action     string // "shell" | "tmux"
}

// EnqueueReload adds or replaces a pane in the reload queue.
func (d *DB) EnqueueReload(ctx context.Context, paneID, target, action string, ttl time.Duration) error {
	now := time.Now()
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO reload_queue(pane_id, pane_target, queued_at, expires_at, action)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(pane_id) DO UPDATE SET
		  pane_target=excluded.pane_target,
		  queued_at=excluded.queued_at,
		  expires_at=excluded.expires_at,
		  action=excluded.action
	`, paneID, target, now.Unix(), now.Add(ttl).Unix(), action)
	return err
}

// DequeueReload removes a pane from the queue.
func (d *DB) DequeueReload(ctx context.Context, paneID string) error {
	_, err := d.sql.ExecContext(ctx, "DELETE FROM reload_queue WHERE pane_id=?", paneID)
	return err
}

// PendingReloads returns all pending queue entries, newest first.
func (d *DB) PendingReloads(ctx context.Context) ([]ReloadEntry, error) {
	rows, err := d.sql.QueryContext(ctx,
		"SELECT pane_id, pane_target, queued_at, expires_at, action FROM reload_queue ORDER BY queued_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReloadEntry
	for rows.Next() {
		var e ReloadEntry
		var queued, expires int64
		if err := rows.Scan(&e.PaneID, &e.PaneTarget, &queued, &expires, &e.Action); err != nil {
			return nil, err
		}
		e.QueuedAt = time.Unix(queued, 0)
		e.ExpiresAt = time.Unix(expires, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ExpireReloads removes queue entries whose expires_at has passed.
// Returns the list of expired entries so callers can log them.
func (d *DB) ExpireReloads(ctx context.Context) ([]ReloadEntry, error) {
	rows, err := d.sql.QueryContext(ctx,
		"SELECT pane_id, pane_target, queued_at, expires_at, action FROM reload_queue WHERE expires_at < ?",
		time.Now().Unix())
	if err != nil {
		return nil, err
	}
	var expired []ReloadEntry
	for rows.Next() {
		var e ReloadEntry
		var q, x int64
		if err := rows.Scan(&e.PaneID, &e.PaneTarget, &q, &x, &e.Action); err != nil {
			rows.Close()
			return nil, err
		}
		e.QueuedAt = time.Unix(q, 0)
		e.ExpiresAt = time.Unix(x, 0)
		expired = append(expired, e)
	}
	rows.Close()
	if len(expired) == 0 {
		return nil, nil
	}
	_, err = d.sql.ExecContext(ctx, "DELETE FROM reload_queue WHERE expires_at < ?", time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return expired, nil
}
