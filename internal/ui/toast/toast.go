// Package toast defines toast notification kinds and helpers for the tmh TUI.
//
// The core design principle is tag-compare: every Show call increments a
// sequence counter and embeds the current sequence in the expiry Tick closure.
// The dismiss handler checks that the incoming sequence matches the current
// counter before clearing the toast, so an old Tick can never dismiss a newer
// notification.
package toast

import "time"

// Kind classifies the severity/intent of a toast notification.
type Kind int

const (
	// KindInfo is used for neutral informational messages (2 s TTL).
	KindInfo Kind = iota
	// KindSuccess is used for completed actions (3 s TTL).
	KindSuccess
	// KindError is used for failed or unexpected conditions (5 s TTL).
	KindError
)

// TTL returns the recommended display duration for a kind.
func (k Kind) TTL() time.Duration {
	switch k {
	case KindError:
		return 5 * time.Second
	case KindSuccess:
		return 3 * time.Second
	default:
		return 2 * time.Second
	}
}
