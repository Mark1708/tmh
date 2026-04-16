// Package xdg resolves filesystem paths used by tmh following the XDG Base
// Directory spec. Environment variables take precedence:
//
//	TMH_CONFIG    → full path to config.yml (overrides ConfigPath entirely)
//	TMH_STATE_DIR → base state directory (overrides StateDir entirely)
package xdg

import (
	"os"
	"path/filepath"
)

// ConfigPath returns the full path to config.yml.
func ConfigPath() string {
	if v := os.Getenv("TMH_CONFIG"); v != "" {
		return v
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "tmh", "config.yml")
}

// ConfigDir returns the directory that contains config.yml.
func ConfigDir() string {
	return filepath.Dir(ConfigPath())
}

// StateDir returns the base state directory (~/.local/state/tmh by default).
func StateDir() string {
	if v := os.Getenv("TMH_STATE_DIR"); v != "" {
		return v
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(base, "tmh")
}

// BackupsDir returns the directory for config backup rotations.
func BackupsDir() string {
	return filepath.Join(StateDir(), "backups")
}

// StateDBPath returns the SQLite db path.
func StateDBPath() string {
	return filepath.Join(StateDir(), "state.db")
}

// LogPath returns the rotating log file path.
func LogPath() string {
	return filepath.Join(StateDir(), "tmh.log")
}

// HistoryPath returns the path for the persistent action history JSONL file.
func HistoryPath() string {
	return filepath.Join(StateDir(), "history.jsonl")
}

// MarksPath returns the path for the persistent marks + last-location JSON file.
func MarksPath() string {
	return filepath.Join(StateDir(), "marks.json")
}

// TmuxConfPath returns the path for the tmh-managed tmux include-file.
// Users source this from ~/.tmux.conf:
//
//	source-file ~/.config/tmh/tmux.conf
func TmuxConfPath() string {
	return filepath.Join(ConfigDir(), "tmux.conf")
}
