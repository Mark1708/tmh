package actions

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
)

// scratchSessionPrefix — все эфемерные сессии создаются с этим префиксом,
// чтобы lazy-sweep знал что трогать.
const scratchSessionPrefix = "scratch-"

// scratchTTLKind — kind для events таблицы.
const scratchTTLKind = "scratch_ttl"

// ScratchOptions parameterises a scratch session.
type ScratchOptions struct {
	Dir string
	TTL time.Duration // zero = no TTL (manual kill)
}

// scratchPayload is what we serialise into events.payload for sweeps.
type scratchPayload struct {
	TTLSeconds int64 `json:"ttl_seconds"`
	CreatedAt  int64 `json:"created_at"`
}

// CreateScratch creates an ephemeral session and, if TTL > 0, records its
// expiry so future tmh invocations can sweep it.
func CreateScratch(ctx context.Context, r tmux.Runner, db *state.DB, opts ScratchOptions) (string, error) {
	name := scratchSessionPrefix + time.Now().Format("1504")
	if exists, _ := r.HasSession(ctx, name); exists {
		// add seconds to disambiguate within the same minute
		name = scratchSessionPrefix + time.Now().Format("150405")
	}
	if err := r.NewSession(ctx, tmux.NewSessionOpts{
		Name: name, Dir: opts.Dir, Detached: true,
	}); err != nil {
		return "", err
	}
	if opts.TTL > 0 && db != nil {
		payload, _ := json.Marshal(scratchPayload{
			TTLSeconds: int64(opts.TTL.Seconds()),
			CreatedAt:  time.Now().Unix(),
		})
		if _, err := db.InsertEvent(ctx, scratchTTLKind, name, string(payload)); err != nil {
			return name, err
		}
	}
	return name, nil
}

// SweepExpiredScratch enumerates scratch_ttl events and kills any session
// whose TTL has passed. Safe to call from any tmh entrypoint — it's a no-op
// if no scratch sessions are queued.
func SweepExpiredScratch(ctx context.Context, r tmux.Runner, db *state.DB) (killed []string, err error) {
	if db == nil {
		return nil, nil
	}
	events, err := db.EventsByKind(ctx, scratchTTLKind)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	for _, e := range events {
		var p scratchPayload
		if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
			// malformed entry — drop it
			_ = db.DeleteEvent(ctx, e.ID)
			continue
		}
		if now-p.CreatedAt < p.TTLSeconds {
			continue
		}
		exists, _ := r.HasSession(ctx, e.Target)
		if exists {
			if err := r.KillSession(ctx, e.Target); err != nil {
				continue
			}
		}
		killed = append(killed, e.Target)
		_ = db.DeleteEvent(ctx, e.ID)
	}
	return killed, nil
}

// IsScratchName reports whether a session name follows the scratch prefix.
func IsScratchName(name string) bool {
	return strings.HasPrefix(name, scratchSessionPrefix)
}
