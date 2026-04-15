package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	errs "git.mark1708.ru/me/tmh/internal/errors"
	_ "modernc.org/sqlite"
)

// IntegrityCheck runs PRAGMA integrity_check. Returns ErrStateCorrupted wrapped
// with the sqlite message when the check reports anything other than "ok".
func (d *DB) IntegrityCheck(ctx context.Context) error {
	rows, err := d.sql.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("%w: %v", errs.ErrStateCorrupted, err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("%w: scan: %v", errs.ErrStateCorrupted, err)
		}
		if result != "ok" {
			return fmt.Errorf("%w: %s", errs.ErrStateCorrupted, result)
		}
	}
	return nil
}

// FixState moves the existing state file aside and creates a fresh database
// at the same path. Returns the renamed-to path so callers can surface it.
// Snapshots, undo events, trust, and reload queue are lost by design; this
// is the "lost history is acceptable" recovery path.
func FixState(path string) (brokenPath string, err error) {
	if path == ":memory:" {
		return "", fmt.Errorf("in-memory database has no state to fix")
	}
	ts := time.Now().Format("20060102-150405")
	brokenPath = fmt.Sprintf("%s.broken.%s", path, ts)
	if _, statErr := os.Stat(path); statErr == nil {
		if err := os.Rename(path, brokenPath); err != nil {
			return "", fmt.Errorf("rename corrupt db: %w", err)
		}
	} else {
		brokenPath = ""
	}
	// Ensure the parent directory exists before reopening.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return brokenPath, err
	}
	raw, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return brokenPath, fmt.Errorf("reopen fresh db: %w", err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(context.Background(), schema); err != nil {
		return brokenPath, fmt.Errorf("apply schema: %w", err)
	}
	return brokenPath, nil
}
