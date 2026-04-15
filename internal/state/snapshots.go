package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Snapshot is a named live-state snapshot used for restore-points.
type Snapshot struct {
	ID      int64
	Name    string
	TS      time.Time
	Payload string // JSON-encoded SessionSnapshot list
}

// SaveSnapshot upserts a named snapshot.
func (d *DB) SaveSnapshot(ctx context.Context, name, payload string) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO snapshots(name, ts, payload) VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET ts=excluded.ts, payload=excluded.payload
	`, name, time.Now().Unix(), payload)
	return err
}

// GetSnapshot returns the snapshot with the given name.
func (d *DB) GetSnapshot(ctx context.Context, name string) (Snapshot, error) {
	var s Snapshot
	var ts int64
	err := d.sql.QueryRowContext(ctx,
		"SELECT id, name, ts, payload FROM snapshots WHERE name=?", name,
	).Scan(&s.ID, &s.Name, &ts, &s.Payload)
	if err == sql.ErrNoRows {
		return s, fmt.Errorf("snapshot %q not found", name)
	}
	if err != nil {
		return s, err
	}
	s.TS = time.Unix(ts, 0)
	return s, nil
}

// ListSnapshots returns all snapshots newest first.
func (d *DB) ListSnapshots(ctx context.Context) ([]Snapshot, error) {
	rows, err := d.sql.QueryContext(ctx,
		"SELECT id, name, ts, payload FROM snapshots ORDER BY ts DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var s Snapshot
		var ts int64
		if err := rows.Scan(&s.ID, &s.Name, &ts, &s.Payload); err != nil {
			return nil, err
		}
		s.TS = time.Unix(ts, 0)
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteSnapshot removes a snapshot by name.
func (d *DB) DeleteSnapshot(ctx context.Context, name string) error {
	_, err := d.sql.ExecContext(ctx, "DELETE FROM snapshots WHERE name=?", name)
	return err
}
