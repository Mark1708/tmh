package state

import (
	"context"
	"time"
)

// Event is one destructive-action record used for undo.
type Event struct {
	ID      int64
	TS      time.Time
	Kind    string // kill_session | kill_window | delete_config | rename | scratch_ttl
	Target  string
	Payload string // JSON-encoded state snapshot for restoration
}

// InsertEvent records an event. Returns the assigned ID.
func (d *DB) InsertEvent(ctx context.Context, kind, target, payload string) (int64, error) {
	res, err := d.sql.ExecContext(ctx,
		"INSERT INTO events(ts, kind, target, payload) VALUES(?, ?, ?, ?)",
		time.Now().Unix(), kind, target, payload,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RecentEvents returns the most recent events up to limit.
func (d *DB) RecentEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.sql.QueryContext(ctx,
		"SELECT id, ts, kind, target, payload FROM events ORDER BY ts DESC, id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Kind, &e.Target, &e.Payload); err != nil {
			return nil, err
		}
		e.TS = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteEvent removes an event by id.
func (d *DB) DeleteEvent(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, "DELETE FROM events WHERE id=?", id)
	return err
}

// EventsByKind returns events of a given kind. Used for scratch TTL sweeps.
func (d *DB) EventsByKind(ctx context.Context, kind string) ([]Event, error) {
	rows, err := d.sql.QueryContext(ctx,
		"SELECT id, ts, kind, target, payload FROM events WHERE kind=? ORDER BY ts DESC", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Kind, &e.Target, &e.Payload); err != nil {
			return nil, err
		}
		e.TS = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}
