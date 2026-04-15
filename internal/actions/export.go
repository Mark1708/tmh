package actions

import (
	"bytes"
	"fmt"
	"strings"

	"git.mark1708.ru/me/tmh/internal/config"

	"gopkg.in/yaml.v3"
)

// secretEnvSuffixes match env keys whose values get scrubbed in --minimal
// exports. Conservative and additive — anything that could plausibly be a
// secret is dropped.
var secretEnvSuffixes = []string{"_TOKEN", "_KEY", "_SECRET", "_PASSWORD", "_PWD", "_API_KEY"}

// ExportOptions controls export() output.
type ExportOptions struct {
	Minimal bool // strip secrets and rewrite absolute paths via roots
	Only    string
}

// Export serialises (a subset of) the config back to YAML. Minimal mode
// scrubs likely-secret env values and replaces absolute window dirs with
// their root-relative form when a root prefix matches.
func Export(cfg *config.Config, opts ExportOptions) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("export: nil config")
	}
	clone := cloneConfigForExport(cfg, opts)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(clone); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// cloneConfigForExport applies --minimal transformations to a deep copy of
// the typed Config so the in-memory config remains untouched.
func cloneConfigForExport(cfg *config.Config, opts ExportOptions) *config.Config {
	out := &config.Config{
		Version:   cfg.Version,
		Roots:     cloneStringMap(cfg.Roots),
		Defaults:  cfg.Defaults,
		Templates: cloneTemplates(cfg.Templates),
		Layouts:   cloneLayouts(cfg.Layouts),
		Profiles:  cloneProfiles(cfg.Profiles),
		Sessions:  cloneSessions(cfg.Sessions),
	}
	if !opts.Minimal {
		return out
	}

	// scrub secrets at every env layer
	out.Defaults.Env = scrubSecrets(out.Defaults.Env)
	for k, p := range out.Profiles {
		p.Env = scrubSecrets(p.Env)
		p.Defaults.Env = scrubSecrets(p.Defaults.Env)
		out.Profiles[k] = p
	}
	for k, s := range out.Sessions {
		s.Env = scrubSecrets(s.Env)
		for wname, w := range s.Windows.Entries {
			w.Env = scrubSecrets(w.Env)
			// rewrite absolute dirs into root+path when possible
			if w.Dir != "" && strings.HasPrefix(w.Dir, "/") {
				if rootName, rel := matchRoot(out.Roots, w.Dir); rootName != "" {
					w.Root = rootName
					w.Path = rel
					w.Dir = ""
				}
			}
			for i, p := range w.Panes {
				p.Env = scrubSecrets(p.Env)
				if p.Dir != "" && strings.HasPrefix(p.Dir, "/") {
					if rootName, rel := matchRoot(out.Roots, p.Dir); rootName != "" {
						p.Root = rootName
						p.Path = rel
						p.Dir = ""
					}
				}
				w.Panes[i] = p
			}
			s.Windows.Entries[wname] = w
		}
		out.Sessions[k] = s
	}

	if opts.Only != "" {
		filtered := make(map[string]config.Session)
		if v, ok := out.Sessions[opts.Only]; ok {
			filtered[opts.Only] = v
		}
		out.Sessions = filtered
	}
	return out
}

func scrubSecrets(env map[string]string) map[string]string {
	if len(env) == 0 {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if isSecretKey(k) {
			out[k] = "<redacted>"
			continue
		}
		out[k] = v
	}
	return out
}

func isSecretKey(k string) bool {
	upper := strings.ToUpper(k)
	for _, suf := range secretEnvSuffixes {
		if strings.HasSuffix(upper, suf) {
			return true
		}
	}
	return false
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneTemplates(in map[string]config.Window) map[string]config.Window {
	if in == nil {
		return nil
	}
	out := make(map[string]config.Window, len(in))
	for k, v := range in {
		v.Env = cloneStringMap(v.Env)
		out[k] = v
	}
	return out
}

func cloneLayouts(in map[string]config.Layout) map[string]config.Layout {
	if in == nil {
		return nil
	}
	out := make(map[string]config.Layout, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneProfiles(in map[string]config.Profile) map[string]config.Profile {
	if in == nil {
		return nil
	}
	out := make(map[string]config.Profile, len(in))
	for k, v := range in {
		v.Env = cloneStringMap(v.Env)
		out[k] = v
	}
	return out
}

func cloneSessions(in map[string]config.Session) map[string]config.Session {
	if in == nil {
		return nil
	}
	out := make(map[string]config.Session, len(in))
	for k, v := range in {
		v.Env = cloneStringMap(v.Env)
		v.Windows = config.Windows{
			Order:   append([]string(nil), v.Windows.Order...),
			Entries: cloneWindowEntries(v.Windows.Entries),
		}
		out[k] = v
	}
	return out
}

func cloneWindowEntries(in map[string]config.Window) map[string]config.Window {
	if in == nil {
		return nil
	}
	out := make(map[string]config.Window, len(in))
	for k, v := range in {
		v.Env = cloneStringMap(v.Env)
		v.Panes = append([]config.Pane(nil), v.Panes...)
		out[k] = v
	}
	return out
}
