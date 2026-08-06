package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ActiveWindowState is the lifecycle state of one tracked physical tmux window
// linked into the runtime "active" session.
type ActiveWindowState string

const (
	// ActiveWindowTracked means the window has a live alias in "active" and is
	// within its sliding TTL window.
	ActiveWindowTracked ActiveWindowState = "tracked"
	// ActiveWindowOrphaned means "active" is the last link to the physical
	// window; the alias must not be unlinked until a recovery destination is
	// verified.
	ActiveWindowOrphaned ActiveWindowState = "orphaned"
)

// ActiveWindow is an immutable snapshot of one tracked physical tmux window.
// It is always returned by value so callers cannot mutate store internals.
// OrphanedAt is the zero time when State == ActiveWindowTracked.
type ActiveWindow struct {
	ServerKey         string
	ServerEpoch       string
	WindowID          string
	SourceSessionID   string
	SourceSessionName string
	WindowName        string
	PromotedAt        time.Time
	LastSelectedAt    time.Time
	ExpiresAt         time.Time
	State             ActiveWindowState
	OrphanedAt        time.Time
}

// ActiveWindowInput is the caller-supplied identity and source metadata for an
// upsert. Timestamps are computed by the store from now and ttl so callers
// cannot pass inconsistent values; only the identity fields are validated.
type ActiveWindowInput struct {
	ServerKey         string
	ServerEpoch       string
	WindowID          string
	SourceSessionID   string
	SourceSessionName string
	WindowName        string
}

// UpsertActiveWindow inserts a new tracked window or re-promotes an existing
// one. On re-promote the original promoted_at is preserved, last_selected_at
// and expires_at are refreshed from now+ttl, source metadata is updated, and
// the row is reset to the tracked state (clearing any prior orphaned_at). The
// single INSERT ... ON CONFLICT statement is atomic.
func (d *DB) UpsertActiveWindow(ctx context.Context, in ActiveWindowInput, now time.Time, ttl time.Duration) error {
	switch "" {
	case in.ServerKey:
		return fmt.Errorf("state: upsert active window: server_key is required")
	case in.ServerEpoch:
		return fmt.Errorf("state: upsert active window: server_epoch is required")
	case in.WindowID:
		return fmt.Errorf("state: upsert active window: window_id is required")
	}
	if ttl <= 0 {
		return fmt.Errorf("state: upsert active window: ttl must be positive, got %v", ttl)
	}
	expires := now.Add(ttl)
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO active_windows(
			server_key, server_epoch, window_id,
			source_session_id, source_session_name, window_name,
			promoted_at, last_selected_at, expires_at, state, orphaned_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 'tracked', NULL)
		ON CONFLICT(server_epoch, window_id) DO UPDATE SET
			source_session_id   = excluded.source_session_id,
			source_session_name = excluded.source_session_name,
			window_name         = excluded.window_name,
			last_selected_at    = excluded.last_selected_at,
			expires_at          = excluded.expires_at,
			state               = 'tracked',
			orphaned_at         = NULL
	`, in.ServerKey, in.ServerEpoch, in.WindowID,
		in.SourceSessionID, in.SourceSessionName, in.WindowName,
		now.Unix(), now.Unix(), expires.Unix())
	if err != nil {
		return fmt.Errorf("state: upsert active window %s/%s: %w", in.ServerEpoch, in.WindowID, err)
	}
	return nil
}

// TouchActiveWindow refreshes last_selected_at and expires_at for an existing
// row only when the incoming selection time is not older than the saved one.
// It returns touched=false when no row matches (epoch, window_id) or when a
// stale out-of-order hook arrives, so hooks cannot move TTL backwards.
// Promoted_at and state are untouched. Safe under concurrent touches.
func (d *DB) TouchActiveWindow(ctx context.Context, serverEpoch, windowID string, now time.Time, ttl time.Duration) (bool, error) {
	if serverEpoch == "" || windowID == "" {
		return false, fmt.Errorf("state: touch active window: server_epoch and window_id are required")
	}
	if ttl <= 0 {
		return false, fmt.Errorf("state: touch active window: ttl must be positive, got %v", ttl)
	}
	expires := now.Add(ttl)
	res, err := d.sql.ExecContext(ctx, `
		UPDATE active_windows
		SET last_selected_at = ?, expires_at = ?
		WHERE server_epoch = ? AND window_id = ? AND last_selected_at <= ?
	`, now.Unix(), expires.Unix(), serverEpoch, windowID, now.Unix())
	if err != nil {
		return false, fmt.Errorf("state: touch active window %s/%s: %w", serverEpoch, windowID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("state: touch active window %s/%s: rows: %w", serverEpoch, windowID, err)
	}
	return n > 0, nil
}

// MarkActiveWindowOrphaned transitions a tracked window to the orphaned state.
// The operation is idempotent: COALESCE keeps the original orphaned_at on a
// repeat call, and marking an absent row is a safe no-op (returns nil).
func (d *DB) MarkActiveWindowOrphaned(ctx context.Context, serverEpoch, windowID string, now time.Time) error {
	if serverEpoch == "" || windowID == "" {
		return fmt.Errorf("state: mark active window orphaned: server_epoch and window_id are required")
	}
	_, err := d.sql.ExecContext(ctx, `
		UPDATE active_windows
		SET state = 'orphaned',
		    orphaned_at = COALESCE(orphaned_at, ?)
		WHERE server_epoch = ? AND window_id = ?
	`, now.Unix(), serverEpoch, windowID)
	if err != nil {
		return fmt.Errorf("state: mark active window orphaned %s/%s: %w", serverEpoch, windowID, err)
	}
	return nil
}

// DeleteActiveWindow removes one tracked window row. Idempotent: deleting an
// already-absent row is not an error.
func (d *DB) DeleteActiveWindow(ctx context.Context, serverEpoch, windowID string) error {
	if serverEpoch == "" || windowID == "" {
		return fmt.Errorf("state: delete active window: server_epoch and window_id are required")
	}
	_, err := d.sql.ExecContext(ctx,
		"DELETE FROM active_windows WHERE server_epoch = ? AND window_id = ?",
		serverEpoch, windowID)
	if err != nil {
		return fmt.Errorf("state: delete active window %s/%s: %w", serverEpoch, windowID, err)
	}
	return nil
}

// ListActiveWindows returns every tracked/orphaned window for the given server
// epoch, ordered by promoted_at then window_id for stable iteration.
func (d *DB) ListActiveWindows(ctx context.Context, serverEpoch string) ([]ActiveWindow, error) {
	if serverEpoch == "" {
		return nil, fmt.Errorf("state: list active windows: server_epoch is required")
	}
	rows, err := d.sql.QueryContext(ctx, activeWindowSelect+`
		WHERE server_epoch = ?
		ORDER BY promoted_at ASC, window_id ASC
	`, serverEpoch)
	if err != nil {
		return nil, fmt.Errorf("state: list active windows %s: %w", serverEpoch, err)
	}
	return scanActiveWindows(rows)
}

// ListExpiredActiveWindows returns windows whose expires_at is at or before
// now for the given server epoch. A window expiring exactly at now counts as
// expired. Results are ordered by expires_at then window_id.
func (d *DB) ListExpiredActiveWindows(ctx context.Context, serverEpoch string, now time.Time) ([]ActiveWindow, error) {
	if serverEpoch == "" {
		return nil, fmt.Errorf("state: list expired active windows: server_epoch is required")
	}
	rows, err := d.sql.QueryContext(ctx, activeWindowSelect+`
		WHERE server_epoch = ? AND expires_at <= ?
		ORDER BY expires_at ASC, window_id ASC
	`, serverEpoch, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("state: list expired active windows %s: %w", serverEpoch, err)
	}
	return scanActiveWindows(rows)
}

// DeleteStaleActiveEpochs removes every row for serverKey whose epoch differs
// from currentEpoch. This invalidates window_ids from a restarted tmux server
// without touching rows that belong to a different socket. It returns the
// number of deleted rows. A zero currentEpoch deletes all non-empty-epoch rows
// for serverKey, which is a legitimate "clear this socket" sweep.
func (d *DB) DeleteStaleActiveEpochs(ctx context.Context, serverKey, currentEpoch string) (int64, error) {
	if serverKey == "" {
		return 0, fmt.Errorf("state: delete stale active epochs: server_key is required")
	}
	res, err := d.sql.ExecContext(ctx, `
		DELETE FROM active_windows
		WHERE server_key = ? AND server_epoch <> ?
	`, serverKey, currentEpoch)
	if err != nil {
		return 0, fmt.Errorf("state: delete stale active epochs for %s: %w", serverKey, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("state: delete stale active epochs for %s: rows: %w", serverKey, err)
	}
	return n, nil
}

// activeWindowSelect is the shared column list for read queries.
const activeWindowSelect = `
	SELECT server_key, server_epoch, window_id,
	       source_session_id, source_session_name, window_name,
	       promoted_at, last_selected_at, expires_at, state, orphaned_at
	FROM active_windows`

// scanActiveWindows drains rows into immutable ActiveWindow values. The rows
// handle is always closed, including on early scan errors.
func scanActiveWindows(rows *sql.Rows) ([]ActiveWindow, error) {
	defer rows.Close()
	var out []ActiveWindow
	for rows.Next() {
		var w ActiveWindow
		var promoted, lastSel, expires int64
		var state string
		var orphaned sql.NullInt64
		if err := rows.Scan(
			&w.ServerKey, &w.ServerEpoch, &w.WindowID,
			&w.SourceSessionID, &w.SourceSessionName, &w.WindowName,
			&promoted, &lastSel, &expires, &state, &orphaned,
		); err != nil {
			return nil, err
		}
		w.PromotedAt = time.Unix(promoted, 0)
		w.LastSelectedAt = time.Unix(lastSel, 0)
		w.ExpiresAt = time.Unix(expires, 0)
		w.State = ActiveWindowState(state)
		if orphaned.Valid {
			w.OrphanedAt = time.Unix(orphaned.Int64, 0)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
