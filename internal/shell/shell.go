// Package shell provides small helpers for detecting the user's shell
// and the default rc-file path for commands like `tmh reload --shell`.
package shell

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultRCFile returns the canonical rc-file path for the user's login
// shell, based on $SHELL. Supports bash, zsh, fish. For unknown shells
// the function falls back to ~/.profile.
func DefaultRCFile() string {
	home := os.Getenv("HOME")
	sh := strings.ToLower(filepath.Base(os.Getenv("SHELL")))
	switch sh {
	case "bash":
		return filepath.Join(home, ".bashrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	case "zsh":
		return filepath.Join(home, ".zshrc")
	default:
		return filepath.Join(home, ".profile")
	}
}
