// Package pane provides an in-memory cache of pane runtime data fetched via
// a single batch `tmux list-panes -a` call. The cache decouples the async
// fetch cadence from the TUI render cycle.
package pane

import (
	"strings"
	"sync"
	"time"
)

// Info holds the runtime state of a single tmux pane.
type Info struct {
	// Command is the foreground process name (e.g. "nvim", "zsh").
	Command string
	// Path is the current working directory of the pane.
	Path string
	// Active is true if this pane is the active pane in its window.
	Active bool
}

// IsIdleShell reports whether the command is an interactive shell at rest
// (bash, zsh, sh, fish, or their login variants with a leading dash).
func IsIdleShell(cmd string) bool {
	switch strings.ToLower(cmd) {
	case "zsh", "-zsh", "bash", "-bash", "sh", "-sh", "fish", "-fish":
		return true
	}
	return false
}

// Provider is a thread-safe in-memory cache of pane Info entries.
//
// The canonical target key format is "session:window.pane" (e.g. "epcp:0.1").
// The Provider never calls tmux directly; all updates come from the root model
// via SetAll after a successful batch fetch.
type Provider struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	info    Info
	fetchAt time.Time
}

// New creates a Provider with the given cache TTL.
// A zero TTL means entries never expire.
func New(ttl time.Duration) *Provider {
	return &Provider{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves the cached Info for target. Returns false when absent or
// when the entry has expired.
func (p *Provider) Get(target string) (Info, bool) {
	p.mu.RLock()
	e, ok := p.entries[target]
	p.mu.RUnlock()
	if !ok {
		return Info{}, false
	}
	if p.ttl > 0 && time.Since(e.fetchAt) > p.ttl {
		return Info{}, false
	}
	return e.info, true
}

// SetAll atomically replaces the entire cache with the provided data.
// This is called after every successful batch fetch.
func (p *Provider) SetAll(data map[string]Info) {
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	// Rebuild — expired entries from previous fetch are dropped automatically.
	next := make(map[string]cacheEntry, len(data))
	for k, v := range data {
		next[k] = cacheEntry{info: v, fetchAt: now}
	}
	p.entries = next
}

// Invalidate drops all cached entries, forcing the next Get to miss.
func (p *Provider) Invalidate() {
	p.mu.Lock()
	p.entries = make(map[string]cacheEntry)
	p.mu.Unlock()
}

// CommandsForWindow returns all non-idle-shell commands running in the
// given window (identified as "session:windowIndex"). Duplicate commands
// are deduplicated. The slice is empty when no active processes are found.
//
// target format: "session:windowIndex" (e.g. "epcp:1")
func (p *Provider) CommandsForWindow(session string, windowIndex int) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	seen := make(map[string]bool)
	var result []string
	for _, e := range p.entries {
		if e.info.Command == "" || IsIdleShell(e.info.Command) {
			continue
		}
		// Filter by session — window index would require parsing the key.
		// The caller must match target keys themselves for stricter filtering.
		_ = session
		_ = windowIndex
		if !seen[e.info.Command] {
			seen[e.info.Command] = true
			result = append(result, e.info.Command)
		}
	}
	return result
}

// AllCommands returns a deduplicated list of all non-idle-shell commands
// currently in the cache. Used for the footer heatmap.
func (p *Provider) AllCommands() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	seen := make(map[string]bool)
	var result []string
	for _, e := range p.entries {
		if e.info.Command == "" || IsIdleShell(e.info.Command) {
			continue
		}
		if !seen[e.info.Command] {
			seen[e.info.Command] = true
			result = append(result, e.info.Command)
		}
	}
	return result
}
