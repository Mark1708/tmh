package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Discovered represents one auto-generated session candidate. It's the
// distilled output of a DiscoverRule after glob expansion + optional
// zoxide cross-reference.
type Discovered struct {
	// Name is the candidate session name (filepath.Base of Path).
	Name string
	// Path is the absolute directory the session would attach into.
	Path string
	// Template is the template name seeded from the rule (may be empty).
	Template string
	// FromZoxide is true when the entry was pulled from zoxide (as
	// opposed to a filesystem glob).
	FromZoxide bool
}

// ZoxideRunner is the shell-out seam used by ExpandDiscoverRules; tests
// replace it to avoid depending on the zoxide binary in CI.
type ZoxideRunner interface {
	Run(ctx context.Context, limit int) ([]string, error)
}

// DefaultZoxideRunner invokes `zoxide query --list --limit N` and returns
// the resulting absolute paths. Absent binary / non-zero exit → empty
// slice + nil error (graceful fallback per the plan).
type DefaultZoxideRunner struct{}

func (DefaultZoxideRunner) Run(ctx context.Context, limit int) ([]string, error) {
	if _, err := exec.LookPath("zoxide"); err != nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, "zoxide", "query", "--list").Output()
	if err != nil {
		return nil, nil
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > limit {
		lines = lines[:limit]
	}
	return lines, nil
}

// ExpandDiscoverRules walks every rule in cfg.Discover and returns the
// deduplicated candidate list, ordered (glob matches first, then zoxide).
// Sessions already declared in cfg.Sessions are excluded — declared wins.
//
// Pass a DefaultZoxideRunner{} in production; tests can stub ZoxideRunner.
func ExpandDiscoverRules(ctx context.Context, cfg *Config, zox ZoxideRunner) ([]Discovered, error) {
	if cfg == nil || len(cfg.Discover) == 0 {
		return nil, nil
	}
	home, _ := os.UserHomeDir()
	seen := make(map[string]bool)
	for name := range cfg.Sessions {
		seen[name] = true
	}

	var out []Discovered
	for _, rule := range cfg.Discover {
		for _, p := range expandGlob(rule.Path, home) {
			name := filepath.Base(p)
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, Discovered{
				Name: name, Path: p, Template: rule.Template,
			})
		}
		if rule.Zoxide && zox != nil {
			paths, _ := zox.Run(ctx, rule.ZoxideLimit)
			for _, p := range paths {
				if !filepath.IsAbs(p) {
					continue
				}
				name := filepath.Base(p)
				if seen[name] {
					continue
				}
				seen[name] = true
				out = append(out, Discovered{
					Name: name, Path: p, Template: rule.Template, FromZoxide: true,
				})
			}
		}
	}
	return out, nil
}

// expandGlob resolves ~ and runs filepath.Glob. Directories only.
func expandGlob(pattern, home string) []string {
	if pattern == "" {
		return nil
	}
	if strings.HasPrefix(pattern, "~/") && home != "" {
		pattern = filepath.Join(home, pattern[2:])
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil || !fi.IsDir() {
			continue
		}
		dirs = append(dirs, m)
	}
	return dirs
}
