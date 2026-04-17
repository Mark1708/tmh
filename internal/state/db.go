package state

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"os"

	_ "modernc.org/sqlite"
)

// DB is the tmh state store. It wraps a *sql.DB with WAL + busy_timeout
// tuned for concurrent TUI + CLI access to the same file.
type DB struct {
	sql *sql.DB
}

// Open opens or creates the state database at path. ":memory:" is accepted
// for tests. The schema is applied on every Open (CREATE TABLE IF NOT EXISTS).
func Open(path string) (*DB, error) {
	if path != ":memory:" && filepath.Dir(path) != "." {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("state: mkdir: %w", err)
		}
	}
	// modernc/sqlite accepts URI-style pragmas via _pragma.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("state: open: %w", err)
	}
	if err := raw.Ping(); err != nil {
		raw.Close()
		return nil, fmt.Errorf("state: ping: %w", err)
	}
	// Tighten permissions on the on-disk DB file (contains trust-hashes,
	// history, snapshots). Ignore :memory: and any failure to chmod — it's
	// a best-effort hardening, not a correctness guarantee.
	if path != ":memory:" {
		_ = os.Chmod(path, 0o600)
	}
	db := &DB{sql: raw}
	if err := db.migrate(); err != nil {
		raw.Close()
		return nil, err
	}
	return db, nil
}

// Close releases the database connection.
func (d *DB) Close() error { return d.sql.Close() }

// SQL returns the underlying *sql.DB for subpackages that need direct access.
// Not exported in the public API.
func (d *DB) handle() *sql.DB { return d.sql }

const schema = `
CREATE TABLE IF NOT EXISTS events (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  ts       INTEGER NOT NULL,
  kind     TEXT NOT NULL,
  target   TEXT NOT NULL,
  payload  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts DESC);

CREATE TABLE IF NOT EXISTS snapshots (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  name     TEXT NOT NULL UNIQUE,
  ts       INTEGER NOT NULL,
  payload  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS trust (
  config_path TEXT NOT NULL,
  config_hash TEXT NOT NULL,
  trusted_at  INTEGER NOT NULL,
  PRIMARY KEY (config_path, config_hash)
);

CREATE TABLE IF NOT EXISTS reload_queue (
  pane_id     TEXT PRIMARY KEY,
  pane_target TEXT NOT NULL,
  queued_at   INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL,
  action      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_reload_expires ON reload_queue(expires_at);
`

func (d *DB) migrate() error {
	_, err := d.sql.ExecContext(context.Background(), schema)
	if err != nil {
		return fmt.Errorf("state: migrate: %w", err)
	}
	return nil
}
