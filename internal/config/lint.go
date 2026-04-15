package config

import (
	"strings"
)

// ExpandShorthand resolves the `$root/path` shorthand for dir fields. Only
// applied at the beginning of the string; `$$` escapes to a literal `$`.
//
// Returns (root, path, ok) where ok is true if the string started with a
// known shorthand. If ok is false, the caller should treat the input as a
// plain dir value.
func ExpandShorthand(s string, roots map[string]string) (root, path string, ok bool) {
	if s == "" || s[0] != '$' {
		return "", "", false
	}
	if strings.HasPrefix(s, "$$") {
		// Escape — caller should replace $$ with $ in the final value.
		return "", "", false
	}
	rest := s[1:]
	// split on first '/'
	slash := strings.IndexByte(rest, '/')
	var key string
	if slash < 0 {
		key = rest
		rest = ""
	} else {
		key = rest[:slash]
		rest = rest[slash+1:]
	}
	if _, exists := roots[key]; !exists {
		return "", "", false
	}
	return key, rest, true
}

// UnescapeDollar replaces `$$` with `$`. Call after ExpandShorthand returned
// ok=false on inputs that may contain escaped dollars.
func UnescapeDollar(s string) string {
	return strings.ReplaceAll(s, "$$", "$")
}

// LintWindow rewrites a Window in place to the canonical form: if Dir starts
// with `$key/...`, populates Root+Path and clears Dir.
func LintWindow(w *Window, roots map[string]string) {
	if w.Root != "" {
		return
	}
	if root, path, ok := ExpandShorthand(w.Dir, roots); ok {
		w.Root = root
		w.Path = path
		w.Dir = ""
	} else if strings.Contains(w.Dir, "$$") {
		w.Dir = UnescapeDollar(w.Dir)
	}
}

// LintConfig normalises all shorthand dir fields across a Config.
func LintConfig(c *Config) {
	if c == nil {
		return
	}
	for name, sess := range c.Sessions {
		for wname, win := range sess.Windows.Entries {
			LintWindow(&win, c.Roots)
			for i := range win.Panes {
				lintPane(&win.Panes[i], c.Roots)
			}
			sess.Windows.Entries[wname] = win
		}
		c.Sessions[name] = sess
	}
	for name, tmpl := range c.Templates {
		LintWindow(&tmpl, c.Roots)
		c.Templates[name] = tmpl
	}
}

func lintPane(p *Pane, roots map[string]string) {
	if p.Root != "" {
		return
	}
	if root, path, ok := ExpandShorthand(p.Dir, roots); ok {
		p.Root = root
		p.Path = path
		p.Dir = ""
	} else if strings.Contains(p.Dir, "$$") {
		p.Dir = UnescapeDollar(p.Dir)
	}
}
