// Package store owns durable control-plane state.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/mark1708/tmh-next/internal/domain"
	_ "modernc.org/sqlite"
)

var ErrConflict = errors.New("store revision conflict")

type SQLite struct {
	db *sql.DB
}

func Open(path string) (*SQLite, error) {
	if path == "" {
		return nil, fmt.Errorf("store path is empty")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create state directory: %w", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create state file: %w", err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close state file: %w", err)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("protect state file: %w", err)
		}
	}

	dsn := path
	if path != ":memory:" {
		u := &url.URL{Scheme: "file", Path: path}
		q := u.Query()
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "synchronous(NORMAL)")
		q.Add("_pragma", "foreign_keys(ON)")
		u.RawQuery = q.Encode()
		dsn = u.String()
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &SQLite{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS catalog_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    revision INTEGER NOT NULL,
    catalog_json BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS command_results (
    idempotency_key TEXT PRIMARY KEY,
    result_json BLOB NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO schema_migrations(version) VALUES (1);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate state database: %w", err)
	}
	return nil
}

func (s *SQLite) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLite) LoadCatalog(ctx context.Context) (*domain.Catalog, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT catalog_json FROM catalog_state WHERE singleton = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load catalog: %w", err)
	}
	var catalog domain.Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	if err := domain.ValidateCatalog(&catalog); err != nil {
		return nil, fmt.Errorf("stored catalog: %w", err)
	}
	return &catalog, nil
}

func (s *SQLite) SaveCatalog(ctx context.Context, expectedRevision uint64, catalog *domain.Catalog) error {
	if catalog == nil {
		return fmt.Errorf("save catalog: catalog is nil")
	}
	if catalog.Revision != expectedRevision+1 {
		return fmt.Errorf("save catalog: revision %d does not follow %d", catalog.Revision, expectedRevision)
	}
	if err := domain.ValidateCatalog(catalog); err != nil {
		return fmt.Errorf("save catalog: %w", err)
	}
	raw, err := json.Marshal(catalog)
	if err != nil {
		return fmt.Errorf("encode catalog: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin catalog transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current uint64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM catalog_state WHERE singleton = 1`).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		current = 0
	} else if err != nil {
		return fmt.Errorf("read catalog revision: %w", err)
	}
	if current != expectedRevision {
		return fmt.Errorf("%w: expected %d, current %d", ErrConflict, expectedRevision, current)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO catalog_state(singleton, revision, catalog_json, updated_at)
VALUES (1, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(singleton) DO UPDATE SET
    revision = excluded.revision,
    catalog_json = excluded.catalog_json,
    updated_at = CURRENT_TIMESTAMP`, catalog.Revision, raw); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit catalog: %w", err)
	}
	return nil
}

func (s *SQLite) SaveCommandResult(ctx context.Context, key string, result domain.MutationResult) error {
	if key == "" {
		return fmt.Errorf("idempotency key is empty")
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode command result: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO command_results(idempotency_key, result_json) VALUES (?, ?)`, key, raw)
	if err != nil {
		return fmt.Errorf("save command result: %w", err)
	}
	return nil
}

// SaveMutationResult commits the revised catalog and its idempotency result in
// one transaction, so a successful database commit can never expose only one.
func (s *SQLite) SaveMutationResult(ctx context.Context, expectedRevision uint64, catalog *domain.Catalog, key string, result domain.MutationResult) error {
	if catalog == nil {
		return fmt.Errorf("save mutation: catalog is nil")
	}
	if key == "" {
		return fmt.Errorf("save mutation: idempotency key is empty")
	}
	if catalog.Revision != expectedRevision+1 {
		return fmt.Errorf("save mutation: revision %d does not follow %d", catalog.Revision, expectedRevision)
	}
	if err := domain.ValidateCatalog(catalog); err != nil {
		return fmt.Errorf("save mutation: %w", err)
	}
	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		return fmt.Errorf("encode mutation catalog: %w", err)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode mutation result: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mutation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var current uint64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM catalog_state WHERE singleton = 1`).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		current = 0
	} else if err != nil {
		return fmt.Errorf("read mutation revision: %w", err)
	}
	if current != expectedRevision {
		return fmt.Errorf("%w: expected %d, current %d", ErrConflict, expectedRevision, current)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO catalog_state(singleton, revision, catalog_json, updated_at)
VALUES (1, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(singleton) DO UPDATE SET
    revision = excluded.revision,
    catalog_json = excluded.catalog_json,
    updated_at = CURRENT_TIMESTAMP`, catalog.Revision, catalogJSON); err != nil {
		return fmt.Errorf("write mutation catalog: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO command_results(idempotency_key, result_json) VALUES (?, ?)`, key, resultJSON); err != nil {
		return fmt.Errorf("write mutation result: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mutation: %w", err)
	}
	return nil
}

func (s *SQLite) CommandResult(ctx context.Context, key string) (domain.MutationResult, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT result_json FROM command_results WHERE idempotency_key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.MutationResult{}, false, nil
	}
	if err != nil {
		return domain.MutationResult{}, false, fmt.Errorf("load command result: %w", err)
	}
	var result domain.MutationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return domain.MutationResult{}, false, fmt.Errorf("decode command result: %w", err)
	}
	return result, true, nil
}
