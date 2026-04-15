package state

import (
	"context"
	"database/sql"
	"time"
)

// IsTrusted reports whether a config file at path with the given hash has
// been marked trusted by the user.
func (d *DB) IsTrusted(ctx context.Context, path, hash string) (bool, error) {
	var trustedAt int64
	err := d.sql.QueryRowContext(ctx,
		"SELECT trusted_at FROM trust WHERE config_path=? AND config_hash=?",
		path, hash,
	).Scan(&trustedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkTrusted records (path, hash) as trusted as of now.
func (d *DB) MarkTrusted(ctx context.Context, path, hash string) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO trust(config_path, config_hash, trusted_at) VALUES(?, ?, ?)
		ON CONFLICT(config_path, config_hash) DO UPDATE SET trusted_at=excluded.trusted_at
	`, path, hash, time.Now().Unix())
	return err
}

// ForgetTrust removes trust records for a path (any hash).
func (d *DB) ForgetTrust(ctx context.Context, path string) error {
	_, err := d.sql.ExecContext(ctx, "DELETE FROM trust WHERE config_path=?", path)
	return err
}
