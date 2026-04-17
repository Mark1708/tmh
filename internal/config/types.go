package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Config is the top-level tmh configuration document.
//
// Pointers are used for optional maps to distinguish "absent" from "empty".
type Config struct {
	Version   int                `yaml:"version"`
	Roots     map[string]string  `yaml:"roots,omitempty"`
	Defaults  Defaults           `yaml:"defaults,omitempty"`
	Templates map[string]Window  `yaml:"templates,omitempty"`
	Layouts   map[string]Layout  `yaml:"layouts,omitempty"`
	Profiles  map[string]Profile `yaml:"profiles,omitempty"`
	Sessions  map[string]Session `yaml:"sessions,omitempty"`

	// Node holds the raw yaml.v3 document root for comment-preserving writes.
	// Populated by the parser; not marshalled.
	Node *yaml.Node `yaml:"-"`
}

// Defaults captures global fallbacks that apply when a field is unset deeper
// in the tree.
type Defaults struct {
	Layout string            `yaml:"layout,omitempty"`
	Shell  string            `yaml:"shell,omitempty"`
	Lang   string            `yaml:"lang,omitempty"` // en | ru; empty → auto-detect (see i18n.DetectLang)
	Popup  Popup             `yaml:"popup,omitempty"`
	Env    map[string]string `yaml:"env,omitempty"`
	Reload ReloadDefaults    `yaml:"reload,omitempty"`
	// History controls persistent action history behaviour.
	// All fields are optional; zero values fall back to the built-in defaults.
	History HistoryConfig `yaml:"history,omitempty"`
	// Display tunes what the TUI renders for each row.
	Display DisplayConfig `yaml:"display,omitempty"`
	// Behaviour tunes runtime TUI behaviour.
	Behaviour BehaviourConfig `yaml:"behaviour,omitempty"`
	// Marks controls the marks (bookmark) feature.
	Marks MarksConfig `yaml:"marks,omitempty"`
	// TmuxIntegration holds settings written to the tmh-managed include-file.
	TmuxIntegration TmuxIntegrationConfig `yaml:"tmux_integration,omitempty"`
}

// DisplayConfig controls visual representation in the TUI.
type DisplayConfig struct {
	// ShowProcessesInTree shows running process names alongside session/window rows.
	ShowProcessesInTree bool `yaml:"show_processes_in_tree,omitempty"`
	// ShowFooterHeatmap shows a live/idle pane counter in the footer.
	ShowFooterHeatmap bool `yaml:"show_footer_heatmap,omitempty"`
	// PreviewDefaultPane is the 0-based pane index used for the detail preview.
	// Special values: 0=first (default), -1=active.
	PreviewDefaultPane int `yaml:"preview_default_pane,omitempty"`
	// TreeDensity controls row spacing. "compact" or "normal" (default).
	TreeDensity string `yaml:"tree_density,omitempty"`
}

// BehaviourConfig controls runtime behaviour of the TUI.
type BehaviourConfig struct {
	// AutoRefreshInterval is the cadence for pane-command polling.
	// Valid: "1s", "2s", "5s", "10s", "off". Default: "2s".
	AutoRefreshInterval string `yaml:"auto_refresh_interval,omitempty"`
	// DryRunDefault makes the confirm dialog default to dry-run mode.
	DryRunDefault bool `yaml:"dry_run_default,omitempty"`
	// ConfirmOnKill requires confirmation before killing sessions/windows.
	// Defaults to true when nil.
	ConfirmOnKill *bool `yaml:"confirm_on_kill,omitempty"`
	// OptimisticRendering applies UI changes before the tmux round-trip confirms them.
	OptimisticRendering bool `yaml:"optimistic_rendering,omitempty"`
}

// MarksConfig controls the marks (bookmark) feature.
type MarksConfig struct {
	// PersistAcrossSessions writes marks to disk so they survive restarts.
	// Defaults to true when nil.
	PersistAcrossSessions *bool `yaml:"persist_across_sessions,omitempty"`
}

// TmuxIntegrationConfig holds settings written to the tmh-managed tmux
// include-file (~/.config/tmh/tmux.conf). These take effect only after the
// user adds `source-file ~/.config/tmh/tmux.conf` to their ~/.tmux.conf.
type TmuxIntegrationConfig struct {
	DefaultTerminal        string `yaml:"default_terminal,omitempty"`
	EscapeTimeMs           *int   `yaml:"escape_time_ms,omitempty"`
	MouseMode              *bool  `yaml:"mouse_mode,omitempty"`
	StatusRightIntegration *bool  `yaml:"status_right_integration,omitempty"`
	BaseIndex              *int   `yaml:"base_index,omitempty"`
	PaneBaseIndex          *int   `yaml:"pane_base_index,omitempty"`
}

// HistoryConfig controls the append-only JSONL action history.
type HistoryConfig struct {
	// MaxEntries is the maximum number of history records to keep.
	// Amortized rewrite triggers at MaxEntries*1.2. Default: 1000.
	MaxEntries int `yaml:"max_entries,omitempty"`
	// Retention is a duration string (e.g. "30d", "7d", "forever").
	// Records older than Retention are pruned on startup. Default: "30d".
	Retention string `yaml:"retention,omitempty"`
	// AutoClearOnStartup removes entries older than Retention when the TUI
	// starts. Default: true.
	AutoClearOnStartup *bool `yaml:"auto_clear_on_startup,omitempty"`
	// ArchiveOnClear controls whether a manual "clear history" renames the
	// current file to history.jsonl.archived-<ts> before truncating.
	// Default: true.
	ArchiveOnClear *bool `yaml:"archive_on_clear,omitempty"`
}

// ReloadDefaults tunes the deferred reload queue.
type ReloadDefaults struct {
	BusyTTL string `yaml:"busy_ttl,omitempty"` // duration string, e.g. "10m"
}

// Popup holds default dimensions for `tmh popup`.
type Popup struct {
	Width  string `yaml:"width,omitempty"`
	Height string `yaml:"height,omitempty"`
}

// Layout names a reusable tmux layout hash.
type Layout struct {
	Hash        string `yaml:"hash"`
	Description string `yaml:"description,omitempty"`
}

// Profile bundles a group filter plus optional overrides applied at runtime.
type Profile struct {
	Groups   []string          `yaml:"groups,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Defaults Defaults          `yaml:"defaults,omitempty"`
	Hooks    Hooks             `yaml:"hooks,omitempty"`
}

// Session describes one tmux session.
type Session struct {
	Group    []string          `yaml:"group,omitempty"`
	Root     string            `yaml:"root,omitempty"`
	Path     string            `yaml:"path,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Defaults Defaults          `yaml:"defaults,omitempty"`
	Hooks    Hooks             `yaml:"hooks,omitempty"`
	Windows  Windows           `yaml:"windows,omitempty"`
}

// Windows preserves declaration order by wrapping a MapSlice-like structure.
// YAML maps are unordered per spec, but tmux cares about window order; we
// capture it from the yaml.Node during UnmarshalYAML.
type Windows struct {
	Order   []string
	Entries map[string]Window
}

// Window describes one tmux window. Short YAML form (plain string) becomes
// Window{Dir: s} via UnmarshalYAML.
type Window struct {
	Dir     string            `yaml:"dir,omitempty"`
	Root    string            `yaml:"root,omitempty"`
	Path    string            `yaml:"path,omitempty"`
	Layout  string            `yaml:"layout,omitempty"`
	Command string            `yaml:"command,omitempty"`
	Extends string            `yaml:"extends,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Focus   bool              `yaml:"focus,omitempty"`
	Panes   []Pane            `yaml:"panes,omitempty"`
	Hooks   Hooks             `yaml:"hooks,omitempty"`
}

// Pane describes an explicit per-pane entry inside a Window.panes[] list.
type Pane struct {
	Dir     string            `yaml:"dir,omitempty"`
	Root    string            `yaml:"root,omitempty"`
	Path    string            `yaml:"path,omitempty"`
	Command string            `yaml:"command,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Focus   bool              `yaml:"focus,omitempty"`
	Hooks   Hooks             `yaml:"hooks,omitempty"`
}

// Hooks lists commands to run at lifecycle points. A YAML scalar is coerced
// into a single-element slice.
type Hooks struct {
	OnCreate  []string `yaml:"on_create,omitempty"`
	OnAttach  []string `yaml:"on_attach,omitempty"`
	OnDestroy []string `yaml:"on_destroy,omitempty"`
}

// --- UnmarshalYAML shims ---

// UnmarshalYAML accepts either a scalar (shorthand dir) or a mapping for Window.
func (w *Window) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		w.Dir = n.Value
		return nil
	case yaml.MappingNode:
		// Use an auxiliary type without the method to avoid recursion.
		type raw Window
		var r raw
		if err := n.Decode(&r); err != nil {
			return err
		}
		*w = Window(r)
		return nil
	default:
		return fmt.Errorf("window: expected scalar or mapping at line %d:%d", n.Line, n.Column)
	}
}

// UnmarshalYAML for Windows preserves the declaration order of keys.
func (ws *Windows) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("windows: expected mapping at line %d:%d", n.Line, n.Column)
	}
	ws.Order = make([]string, 0, len(n.Content)/2)
	ws.Entries = make(map[string]Window, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		valNode := n.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode {
			return fmt.Errorf("windows: non-scalar key at line %d:%d", keyNode.Line, keyNode.Column)
		}
		name := keyNode.Value
		var w Window
		if err := w.UnmarshalYAML(valNode); err != nil {
			return fmt.Errorf("windows[%s]: %w", name, err)
		}
		ws.Order = append(ws.Order, name)
		ws.Entries[name] = w
	}
	return nil
}

// UnmarshalYAML for Hooks accepts either scalar or sequence per field. yaml.v3
// doesn't give us per-field hooks, so we unmarshal via an intermediate type
// whose fields are stringOrList.
func (h *Hooks) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("hooks: expected mapping at line %d:%d", n.Line, n.Column)
	}
	var raw struct {
		OnCreate  stringOrList `yaml:"on_create"`
		OnAttach  stringOrList `yaml:"on_attach"`
		OnDestroy stringOrList `yaml:"on_destroy"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	h.OnCreate = raw.OnCreate
	h.OnAttach = raw.OnAttach
	h.OnDestroy = raw.OnDestroy
	return nil
}

// stringOrList decodes `"foo"` or `[foo, bar]` into []string.
type stringOrList []string

func (sl *stringOrList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*sl = []string{n.Value}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("hooks: non-scalar item at line %d:%d", c.Line, c.Column)
			}
			out = append(out, c.Value)
		}
		*sl = out
		return nil
	default:
		return fmt.Errorf("hooks: expected scalar or sequence at line %d:%d", n.Line, n.Column)
	}
}
