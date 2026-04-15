// Package errs defines typed sentinel errors used throughout tmh.
//
// All errors are wrapped with context via fmt.Errorf("...: %w", ErrX) at the
// call site. Consumers check with errors.Is.
package errs

import "errors"

var (
	// tmux-layer errors.
	ErrServerNotRunning = errors.New("tmux: server not running")
	ErrSessionExists    = errors.New("tmux: session already exists")
	ErrSessionNotFound  = errors.New("tmux: session not found")
	ErrWindowNotFound   = errors.New("tmux: window not found")
	ErrPermission       = errors.New("tmux: permission denied")

	// config-layer errors.
	ErrConfigInvalid   = errors.New("config: invalid")
	ErrConfigNotFound  = errors.New("config: not found")
	ErrSchemaViolation = errors.New("config: schema violation")
	ErrUnknownRoot     = errors.New("config: unknown root reference")
	ErrUnknownTemplate = errors.New("config: unknown template")
	ErrUnknownLayout   = errors.New("config: unknown layout")
	ErrTemplateChain   = errors.New("config: template extends depth > 1")
	ErrLayoutMismatch  = errors.New("config: panes count doesn't match layout")

	// hooks-layer errors.
	ErrHookDenied = errors.New("hooks: user denied trust")

	// state-layer errors.
	ErrStateCorrupted = errors.New("state: db integrity check failed")
)
