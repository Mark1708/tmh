package config

import (
	"fmt"
	"path/filepath"
	"strings"

	errs "github.com/mark1708/tmh/internal/errors"
)

// Resolved is a flattened, applied view of the configuration. It's what
// actions consume after roots/templates/extends/env are expanded.
type Resolved struct {
	Sessions []ResolvedSession
}

// ResolvedSession is a session with all fields fully materialised.
type ResolvedSession struct {
	Name    string
	Group   []string
	Dir     string // absolute path; empty = cwd at attach time
	Env     map[string]string
	Hooks   Hooks
	Windows []ResolvedWindow
}

// ResolvedWindow is a window with inherited values applied.
type ResolvedWindow struct {
	Name    string
	Dir     string // absolute path
	Layout  string // built-in name or layouts[<key>].hash
	Command string
	Env     map[string]string
	Focus   bool
	Hooks   Hooks
	Panes   []ResolvedPane
}

// ResolvedPane mirrors Pane after inheritance.
type ResolvedPane struct {
	Dir     string
	Command string
	Env     map[string]string
	Focus   bool
	Hooks   Hooks
}

// Resolve applies the layered configuration model to produce a Resolved view.
//
// Steps:
//  1. select profile (if non-empty) to scope sessions and merge env/defaults/hooks.
//  2. for each session: compute dir from roots+path; merge env defaults→profile→session.
//  3. for each window: apply extends template, then roots+path/dir, then merge env.
//  4. for each pane: apply roots+path/dir and merge env.
func Resolve(c *Config, profileName string) (*Resolved, error) {
	if c == nil {
		return &Resolved{}, nil
	}
	profile, profileGroups, err := selectProfile(c, profileName)
	if err != nil {
		return nil, err
	}

	baseDefaults := mergeDefaults(c.Defaults, profile.Defaults)
	baseEnv := mergeEnv(c.Defaults.Env, profile.Env)

	out := &Resolved{}
	for _, sessName := range sortedKeys(c.Sessions) {
		sess := c.Sessions[sessName]
		if !groupMatches(sess.Group, profileGroups) {
			continue
		}
		sessDir, err := resolveDir(c.Roots, sess.Root, sess.Path, "")
		if err != nil {
			return nil, fmt.Errorf("session %q: %w", sessName, err)
		}
		sessEnv := mergeEnv(baseEnv, sess.Env)
		sessDefaults := mergeDefaults(baseDefaults, sess.Defaults)
		sessHooks := concatHooks(profile.Hooks, sess.Hooks)

		rs := ResolvedSession{
			Name:  sessName,
			Group: sess.Group,
			Dir:   sessDir,
			Env:   sessEnv,
			Hooks: sessHooks,
		}
		for _, wname := range sess.Windows.Order {
			w := sess.Windows.Entries[wname]
			rw, err := resolveWindow(c, sessDir, sessDefaults, sessEnv, wname, w)
			if err != nil {
				return nil, fmt.Errorf("session %q, window %q: %w", sessName, wname, err)
			}
			rs.Windows = append(rs.Windows, rw)
		}
		out.Sessions = append(out.Sessions, rs)
	}
	return out, nil
}

func selectProfile(c *Config, name string) (Profile, []string, error) {
	if name == "" {
		return Profile{}, nil, nil
	}
	p, ok := c.Profiles[name]
	if !ok {
		return Profile{}, nil, fmt.Errorf("%w: profile %q", errs.ErrSchemaViolation, name)
	}
	return p, p.Groups, nil
}

func resolveWindow(c *Config, sessDir string, sessDefaults Defaults, sessEnv map[string]string, name string, w Window) (ResolvedWindow, error) {
	// apply extends
	if w.Extends != "" {
		tmpl, ok := c.Templates[w.Extends]
		if !ok {
			return ResolvedWindow{}, fmt.Errorf("%w: template %q", errs.ErrUnknownTemplate, w.Extends)
		}
		if tmpl.Extends != "" {
			return ResolvedWindow{}, fmt.Errorf("%w: template %q extends %q", errs.ErrTemplateChain, w.Extends, tmpl.Extends)
		}
		w = applyTemplate(tmpl, w)
	}

	// resolve dir
	dir, err := windowDir(c.Roots, w, sessDir)
	if err != nil {
		return ResolvedWindow{}, err
	}

	// layout defaulting
	layout := w.Layout
	if layout == "" {
		layout = sessDefaults.Layout
	}

	// env
	env := mergeEnv(sessEnv, w.Env)

	rw := ResolvedWindow{
		Name:    name,
		Dir:     dir,
		Layout:  layout,
		Command: w.Command,
		Env:     env,
		Focus:   w.Focus,
		Hooks:   w.Hooks,
	}
	for _, p := range w.Panes {
		pdir, err := paneDir(c.Roots, p, dir)
		if err != nil {
			return ResolvedWindow{}, fmt.Errorf("pane: %w", err)
		}
		rw.Panes = append(rw.Panes, ResolvedPane{
			Dir:     pdir,
			Command: p.Command,
			Env:     mergeEnv(env, p.Env),
			Focus:   p.Focus,
			Hooks:   p.Hooks,
		})
	}
	return rw, nil
}

// applyTemplate merges a template into a window. Window fields win when set.
func applyTemplate(tmpl, w Window) Window {
	if w.Dir == "" {
		w.Dir = tmpl.Dir
	}
	if w.Root == "" {
		w.Root = tmpl.Root
	}
	if w.Path == "" {
		w.Path = tmpl.Path
	}
	if w.Layout == "" {
		w.Layout = tmpl.Layout
	}
	if w.Command == "" {
		w.Command = tmpl.Command
	}
	w.Env = mergeEnv(tmpl.Env, w.Env)
	if len(w.Panes) == 0 {
		w.Panes = tmpl.Panes
	}
	// Concatenate template hooks first, then window hooks — template is the
	// "base", so its hooks run before window-specific ones at each lifecycle
	// point. Duplicates are preserved; the user can dedupe in the template.
	w.Hooks.OnCreate = append(append([]string{}, tmpl.Hooks.OnCreate...), w.Hooks.OnCreate...)
	w.Hooks.OnAttach = append(append([]string{}, tmpl.Hooks.OnAttach...), w.Hooks.OnAttach...)
	w.Hooks.OnDestroy = append(append([]string{}, tmpl.Hooks.OnDestroy...), w.Hooks.OnDestroy...)
	return w
}

// windowDir resolves a window's final absolute directory following the rules
// in the plan §1.
func windowDir(roots map[string]string, w Window, sessDir string) (string, error) {
	if w.Dir != "" && filepath.IsAbs(w.Dir) {
		return w.Dir, nil
	}
	if w.Root != "" {
		base, ok := roots[w.Root]
		if !ok {
			return "", fmt.Errorf("%w: root %q", errs.ErrUnknownRoot, w.Root)
		}
		sub := w.Path
		if sub == "" {
			sub = w.Dir
		}
		return joinClean(base, sub), nil
	}
	if sessDir != "" {
		return joinClean(sessDir, w.Dir), nil
	}
	// relative but no session root — caller resolves against cwd
	return w.Dir, nil
}

func paneDir(roots map[string]string, p Pane, winDir string) (string, error) {
	if p.Dir != "" && filepath.IsAbs(p.Dir) {
		return p.Dir, nil
	}
	if p.Root != "" {
		base, ok := roots[p.Root]
		if !ok {
			return "", fmt.Errorf("%w: root %q", errs.ErrUnknownRoot, p.Root)
		}
		sub := p.Path
		if sub == "" {
			sub = p.Dir
		}
		return joinClean(base, sub), nil
	}
	if winDir != "" {
		return joinClean(winDir, p.Dir), nil
	}
	return p.Dir, nil
}

// resolveDir resolves a session's directory from root+path fields.
func resolveDir(roots map[string]string, rootKey, path, fallback string) (string, error) {
	if rootKey == "" {
		if path != "" {
			return path, nil
		}
		return fallback, nil
	}
	base, ok := roots[rootKey]
	if !ok {
		return "", fmt.Errorf("%w: root %q", errs.ErrUnknownRoot, rootKey)
	}
	return joinClean(base, path), nil
}

func joinClean(a, b string) string {
	if b == "" {
		return filepath.Clean(a)
	}
	if b == "." {
		return filepath.Clean(a)
	}
	return filepath.Clean(filepath.Join(a, b))
}

// groupMatches returns true if sessGroups contains at least one of filters,
// or if filters is empty (no filter applied).
func groupMatches(sessGroups, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(sessGroups))
	for _, g := range sessGroups {
		set[g] = struct{}{}
	}
	for _, f := range filters {
		if _, ok := set[f]; ok {
			return true
		}
	}
	return false
}

// mergeEnv returns a new map with base overlaid by overlay. overlay wins.
func mergeEnv(base, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

// mergeDefaults overlays profile/session defaults on top of base defaults.
// Empty strings in overlay do not override.
func mergeDefaults(base, overlay Defaults) Defaults {
	out := base
	if overlay.Layout != "" {
		out.Layout = overlay.Layout
	}
	if overlay.Shell != "" {
		out.Shell = overlay.Shell
	}
	if overlay.Popup.Width != "" {
		out.Popup.Width = overlay.Popup.Width
	}
	if overlay.Popup.Height != "" {
		out.Popup.Height = overlay.Popup.Height
	}
	if overlay.Reload.BusyTTL != "" {
		out.Reload.BusyTTL = overlay.Reload.BusyTTL
	}
	out.Env = mergeEnv(base.Env, overlay.Env)
	return out
}

// concatHooks prepends profile hooks to session hooks.
func concatHooks(profile, session Hooks) Hooks {
	return Hooks{
		OnCreate:  append(append([]string(nil), profile.OnCreate...), session.OnCreate...),
		OnAttach:  append(append([]string(nil), profile.OnAttach...), session.OnAttach...),
		OnDestroy: append(append([]string(nil), profile.OnDestroy...), session.OnDestroy...),
	}
}

// sortedKeys returns the sorted keys of a session map. Sessions themselves
// don't have a spec-mandated order (INI historically cared, YAML doesn't);
// we sort alphabetically for deterministic behaviour.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Insertion sort keeps behavior deterministic without pulling in sort.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.Compare(out[j-1], out[j]) > 0; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
