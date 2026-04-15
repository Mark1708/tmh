package config

import (
	"fmt"

	errs "git.mark1708.ru/me/tmh/internal/errors"
)

// BuiltinLayouts is the set of layout names tmh implements natively as
// split-window macros (not stored in the layouts: section).
var BuiltinLayouts = map[string]int{
	"1-pane": 1,
	"2-pane": 2,
	"3-pane": 3,
}

// Validate runs static checks that the schema cannot express:
//   - every session/window/pane.root points at an existing roots entry
//   - every window.extends points at an existing template
//   - templates do not themselves extend other templates (depth ≤ 1)
//   - every window.layout is either a builtin or a layouts[] key
//   - panes[] length matches the layout when the layout is a known builtin
//
// Returns a joined error with each violation wrapping a typed sentinel so
// callers can errors.Is against the specific failure mode.
func Validate(c *Config) error {
	if c == nil {
		return nil
	}
	var violations []error

	// extends depth: templates must not extend anything.
	for name, t := range c.Templates {
		if t.Extends != "" {
			violations = append(violations,
				fmt.Errorf("%w: template %q extends %q", errs.ErrTemplateChain, name, t.Extends))
		}
		if t.Root != "" {
			if _, ok := c.Roots[t.Root]; !ok {
				violations = append(violations,
					fmt.Errorf("%w: template %q references root %q", errs.ErrUnknownRoot, name, t.Root))
			}
		}
	}

	for sname, sess := range c.Sessions {
		if sess.Root != "" {
			if _, ok := c.Roots[sess.Root]; !ok {
				violations = append(violations,
					fmt.Errorf("%w: session %q references root %q", errs.ErrUnknownRoot, sname, sess.Root))
			}
		}
		for _, wname := range sess.Windows.Order {
			w := sess.Windows.Entries[wname]
			violations = append(violations, validateWindow(c, sname, wname, w)...)
		}
	}

	for pname, p := range c.Profiles {
		for _, g := range p.Groups {
			_ = g // profiles may reference any group; no enforcement
		}
		for _, h := range [][]string{p.Hooks.OnCreate, p.Hooks.OnAttach, p.Hooks.OnDestroy} {
			_ = h
		}
		_ = pname
	}

	if len(violations) == 0 {
		return nil
	}
	return joinErrors(violations)
}

func validateWindow(c *Config, sname, wname string, w Window) []error {
	var errs_ []error
	if w.Root != "" {
		if _, ok := c.Roots[w.Root]; !ok {
			errs_ = append(errs_,
				fmt.Errorf("%w: session %q window %q references root %q", errs.ErrUnknownRoot, sname, wname, w.Root))
		}
	}
	if w.Extends != "" {
		if _, ok := c.Templates[w.Extends]; !ok {
			errs_ = append(errs_,
				fmt.Errorf("%w: session %q window %q extends %q", errs.ErrUnknownTemplate, sname, wname, w.Extends))
		}
	}
	expectedPanes := 0
	if w.Layout != "" {
		if n, ok := BuiltinLayouts[w.Layout]; ok {
			expectedPanes = n
		} else if _, ok := c.Layouts[w.Layout]; !ok {
			errs_ = append(errs_,
				fmt.Errorf("%w: session %q window %q layout %q", errs.ErrUnknownLayout, sname, wname, w.Layout))
		}
	}
	if len(w.Panes) > 0 && expectedPanes > 0 && len(w.Panes) != expectedPanes {
		errs_ = append(errs_,
			fmt.Errorf("%w: session %q window %q has %d panes but layout %q expects %d",
				errs.ErrLayoutMismatch, sname, wname, len(w.Panes), w.Layout, expectedPanes))
	}
	for i, p := range w.Panes {
		if p.Root != "" {
			if _, ok := c.Roots[p.Root]; !ok {
				errs_ = append(errs_,
					fmt.Errorf("%w: session %q window %q pane %d references root %q",
						errs.ErrUnknownRoot, sname, wname, i, p.Root))
			}
		}
	}
	return errs_
}

// joinErrors produces a compound error. Using Go 1.20+ errors.Join would be
// cleaner but we keep the helper local for readability.
func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return &multiError{errs: errs}
}

type multiError struct{ errs []error }

func (m *multiError) Error() string {
	var buf []byte
	for i, e := range m.errs {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, e.Error()...)
	}
	return string(buf)
}

func (m *multiError) Unwrap() []error { return m.errs }
