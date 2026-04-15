// Package errrender turns internal sentinel errors into localized,
// user-facing strings. CLI and TUI callers both go through Render so error
// messages stay consistent across surfaces while the underlying errors.Is
// chains keep English wording for logs and tests.
package errrender

import (
	"errors"
	"strings"

	errs "git.mark1708.ru/me/tmh/internal/errors"
	"git.mark1708.ru/me/tmh/internal/i18n"
)

// Render returns a localized message for known sentinels and falls back to
// the raw error text (English) for anything else — a deliberate choice so we
// never lose signal, we just don't translate every transient wrap.
func Render(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, errs.ErrConfigNotFound):
		return i18n.Tf("error.config_not_found", map[string]any{"path": extractAfter(err, "config: not found")})
	case errors.Is(err, errs.ErrConfigInvalid),
		errors.Is(err, errs.ErrSchemaViolation):
		return i18n.Tf("error.config_invalid", map[string]any{"err": err.Error()})
	case errors.Is(err, errs.ErrSessionExists):
		return i18n.T("error.session_exists")
	case errors.Is(err, errs.ErrSessionNotFound),
		errors.Is(err, errs.ErrWindowNotFound):
		return i18n.T("error.session_not_found")
	case errors.Is(err, errs.ErrServerNotRunning):
		return i18n.T("error.server_not_running")
	case errors.Is(err, errs.ErrHookDenied):
		return i18n.T("error.hook_denied")
	}
	return err.Error()
}

// extractAfter tries to pull the contextual tail (e.g. the path) out of a
// wrapped message like `config: not found: /path/to/file`. Best-effort — we
// return empty if the marker is missing, which just renders the translated
// template without the placeholder filled in.
func extractAfter(err error, marker string) string {
	msg := err.Error()
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return ""
	}
	tail := strings.TrimPrefix(msg[idx+len(marker):], ": ")
	return strings.TrimSpace(tail)
}
