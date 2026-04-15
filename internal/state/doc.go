// Package state owns the SQLite store for snapshots, events, trust, and the
// reload queue. WAL + busy_timeout=5000 enables concurrent TUI+CLI access.
package state
