// Package tmuxtest provides an in-memory tmux.Runner for testing. It records
// every call so tests can assert on the sequence of operations.
//
// Production code must not import this package.
package tmuxtest

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	errs "git.mark1708.ru/me/tmh/internal/errors"
	"git.mark1708.ru/me/tmh/internal/tmux"
)

// Call captures one Runner method invocation for assertions.
type Call struct {
	Method string
	Args   map[string]any
}

// MockRunner is a Runner backed by in-memory maps. Safe for concurrent
// access within a single test goroutine tree (protected by a mutex).
type MockRunner struct {
	mu         sync.Mutex
	server     bool
	nested     bool
	calls      []Call
	sessions   map[string]*mockSession
	order      []string // session creation order, for stable listing
	paneSerial int
	options    map[string]string // server option table used by ShowOption/SetOption
	hooks      map[string]string // hook name → bound command
}

type mockSession struct {
	name     string
	attached bool
	windows  []*mockWindow
}

type mockWindow struct {
	index   int
	name    string
	layout  string
	panes   []*mockPane
	active  bool
	autoRen bool
}

type mockPane struct {
	id      string
	command string
	path    string
	active  bool
}

// New returns an empty MockRunner with the tmux server already running.
func New() *MockRunner {
	return &MockRunner{server: true, sessions: map[string]*mockSession{}}
}

// SetInTmux toggles whether InTmux() returns true.
func (m *MockRunner) SetInTmux(v bool) { m.nested = v }

// Calls returns a snapshot of recorded calls since the last Reset.
func (m *MockRunner) Calls() []Call {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Call, len(m.calls))
	copy(out, m.calls)
	return out
}

// Reset clears recorded calls but preserves in-memory state.
func (m *MockRunner) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
}

// MethodNames returns the ordered sequence of method names called.
func (m *MockRunner) MethodNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	for i, c := range m.calls {
		out[i] = c.Method
	}
	return out
}

func (m *MockRunner) record(method string, args map[string]any) {
	m.calls = append(m.calls, Call{Method: method, Args: args})
}

// Compile-time interface assertion.
var _ tmux.Runner = (*MockRunner)(nil)

// --- server lifecycle ---

func (m *MockRunner) InTmux() bool { return m.nested }

func (m *MockRunner) ServerRunning(_ context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("ServerRunning", nil)
	return m.server, nil
}

func (m *MockRunner) StartServer(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("StartServer", nil)
	m.server = true
	return nil
}

// --- sessions ---

func (m *MockRunner) ListSessions(_ context.Context) ([]tmux.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("ListSessions", nil)
	if !m.server {
		return nil, nil
	}
	out := make([]tmux.Session, 0, len(m.order))
	for _, name := range m.order {
		s := m.sessions[name]
		out = append(out, tmux.Session{Name: name, Windows: len(s.windows), Attached: s.attached})
	}
	return out, nil
}

func (m *MockRunner) HasSession(_ context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("HasSession", map[string]any{"name": name})
	_, ok := m.sessions[name]
	return ok, nil
}

func (m *MockRunner) NewSession(_ context.Context, opts tmux.NewSessionOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("NewSession", map[string]any{
		"name": opts.Name, "dir": opts.Dir, "windowName": opts.WindowName,
	})
	if !m.server {
		return fmt.Errorf("%w", errs.ErrServerNotRunning)
	}
	if _, exists := m.sessions[opts.Name]; exists {
		return fmt.Errorf("%w: %s", errs.ErrSessionExists, opts.Name)
	}
	winName := opts.WindowName
	if winName == "" {
		winName = opts.Name
	}
	pane := &mockPane{id: m.nextPaneID(), path: opts.Dir, active: true}
	win := &mockWindow{index: 1, name: winName, panes: []*mockPane{pane}, active: true}
	m.sessions[opts.Name] = &mockSession{name: opts.Name, windows: []*mockWindow{win}}
	m.order = append(m.order, opts.Name)
	return nil
}

func (m *MockRunner) AttachSession(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("AttachSession", map[string]any{"name": name})
	s, ok := m.sessions[name]
	if !ok {
		return fmt.Errorf("%w: %s", errs.ErrSessionNotFound, name)
	}
	s.attached = true
	return nil
}

func (m *MockRunner) SwitchClient(_ context.Context, target string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("SwitchClient", map[string]any{"target": target})
	return nil
}

func (m *MockRunner) KillSession(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("KillSession", map[string]any{"name": name})
	if _, ok := m.sessions[name]; !ok {
		return fmt.Errorf("%w: %s", errs.ErrSessionNotFound, name)
	}
	delete(m.sessions, name)
	for i, n := range m.order {
		if n == name {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	return nil
}

func (m *MockRunner) RenameSession(_ context.Context, from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("RenameSession", map[string]any{"from": from, "to": to})
	s, ok := m.sessions[from]
	if !ok {
		return fmt.Errorf("%w: %s", errs.ErrSessionNotFound, from)
	}
	if _, exists := m.sessions[to]; exists {
		return fmt.Errorf("%w: %s", errs.ErrSessionExists, to)
	}
	delete(m.sessions, from)
	s.name = to
	m.sessions[to] = s
	for i, n := range m.order {
		if n == from {
			m.order[i] = to
			break
		}
	}
	return nil
}

// --- windows ---

func (m *MockRunner) ListWindows(_ context.Context, session string) ([]tmux.Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("ListWindows", map[string]any{"session": session})
	var out []tmux.Window
	if session == "" {
		for _, name := range m.order {
			out = append(out, m.windowsFor(name)...)
		}
	} else {
		out = m.windowsFor(session)
	}
	return out, nil
}

func (m *MockRunner) windowsFor(name string) []tmux.Window {
	s, ok := m.sessions[name]
	if !ok {
		return nil
	}
	out := make([]tmux.Window, 0, len(s.windows))
	for _, w := range s.windows {
		out = append(out, tmux.Window{
			Session: name, Index: w.index, Name: w.name,
			Panes: len(w.panes), Layout: w.layout, Active: w.active,
		})
	}
	return out
}

func (m *MockRunner) NewWindow(_ context.Context, opts tmux.NewWindowOpts) (tmux.Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("NewWindow", map[string]any{
		"session": opts.SessionTarget, "name": opts.Name, "dir": opts.Dir,
	})
	sessName := strings.TrimSuffix(opts.SessionTarget, ":")
	s, ok := m.sessions[sessName]
	if !ok {
		return tmux.Window{}, fmt.Errorf("%w: %s", errs.ErrSessionNotFound, sessName)
	}
	idx := 1
	for _, w := range s.windows {
		if w.index >= idx {
			idx = w.index + 1
		}
	}
	for _, w := range s.windows {
		w.active = false
	}
	pane := &mockPane{id: m.nextPaneID(), path: opts.Dir, active: true}
	win := &mockWindow{index: idx, name: opts.Name, panes: []*mockPane{pane}, active: true}
	s.windows = append(s.windows, win)
	return tmux.Window{Session: sessName, Index: idx, Name: opts.Name}, nil
}

func (m *MockRunner) KillWindow(_ context.Context, target string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("KillWindow", map[string]any{"target": target})
	sess, idx, err := parseWindowTarget(target)
	if err != nil {
		return err
	}
	s, ok := m.sessions[sess]
	if !ok {
		return fmt.Errorf("%w: %s", errs.ErrSessionNotFound, sess)
	}
	for i, w := range s.windows {
		if w.index == idx || w.name == fmt.Sprintf("%d", idx) {
			s.windows = append(s.windows[:i], s.windows[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", errs.ErrWindowNotFound, target)
}

func (m *MockRunner) RenameWindow(_ context.Context, target, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("RenameWindow", map[string]any{"target": target, "name": name})
	w, err := m.findWindow(target)
	if err != nil {
		return err
	}
	w.name = name
	return nil
}

func (m *MockRunner) SelectWindow(_ context.Context, target string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("SelectWindow", map[string]any{"target": target})
	w, err := m.findWindow(target)
	if err != nil {
		return err
	}
	w.active = true
	return nil
}

// --- panes ---

func (m *MockRunner) ListPanes(_ context.Context, target string) ([]tmux.Pane, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("ListPanes", map[string]any{"target": target})
	var out []tmux.Pane
	if target == "" {
		keys := make([]string, 0, len(m.sessions))
		for k := range m.sessions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, name := range keys {
			out = append(out, m.panesFor(name, 0)...)
		}
		return out, nil
	}
	sess, idx, err := parseWindowTarget(target)
	if err == nil {
		return m.panesFor(sess, idx), nil
	}
	// session-only target
	return m.panesFor(target, 0), nil
}

func (m *MockRunner) panesFor(session string, windowIdx int) []tmux.Pane {
	s, ok := m.sessions[session]
	if !ok {
		return nil
	}
	var out []tmux.Pane
	for _, w := range s.windows {
		if windowIdx > 0 && w.index != windowIdx {
			continue
		}
		for i, p := range w.panes {
			out = append(out, tmux.Pane{
				Session: session, Window: w.index, Index: i + 1,
				ID: p.id, Command: p.command, Path: p.path, Active: p.active,
			})
		}
	}
	return out
}

func (m *MockRunner) SplitWindow(_ context.Context, opts tmux.SplitOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("SplitWindow", map[string]any{
		"target": opts.Target, "horizontal": opts.Horizontal, "dir": opts.Dir,
	})
	w, err := m.findWindow(opts.Target)
	if err != nil {
		return err
	}
	path := opts.Dir
	if path == "" && len(w.panes) > 0 {
		path = w.panes[0].path
	}
	for _, p := range w.panes {
		p.active = false
	}
	w.panes = append(w.panes, &mockPane{id: m.nextPaneID(), path: path, active: true})
	return nil
}

func (m *MockRunner) SelectLayout(_ context.Context, target, layout string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("SelectLayout", map[string]any{"target": target, "layout": layout})
	w, err := m.findWindow(target)
	if err != nil {
		return err
	}
	w.layout = layout
	return nil
}

func (m *MockRunner) CapturePane(_ context.Context, target string, lines int) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("CapturePane", map[string]any{"target": target, "lines": lines})
	return []byte(fmt.Sprintf("mock capture of %s", target)), nil
}

func (m *MockRunner) SendKeys(_ context.Context, target string, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("SendKeys", map[string]any{"target": target, "keys": keys})
	return nil
}

func (m *MockRunner) KillPane(_ context.Context, target string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("KillPane", map[string]any{"target": target})
	return nil
}

func (m *MockRunner) SetAutomaticRename(_ context.Context, target string, on bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("SetAutomaticRename", map[string]any{"target": target, "on": on})
	if w, err := m.findWindow(target); err == nil {
		w.autoRen = on
	}
	return nil
}

// --- misc ---

func (m *MockRunner) SourceFile(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("SourceFile", map[string]any{"path": path})
	return nil
}

func (m *MockRunner) DisplayPopup(_ context.Context, opts tmux.PopupOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("DisplayPopup", map[string]any{
		"command": opts.Command, "dir": opts.Dir, "width": opts.Width, "height": opts.Height,
	})
	return nil
}

// --- options + hooks ---

// Options lets tests pre-populate the mock option table so AuditTmuxConfig
// sees a predictable server state.
func (m *MockRunner) Options() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.options == nil {
		m.options = map[string]string{}
	}
	return m.options
}

// Hooks lets tests pre-populate the mock hook table.
func (m *MockRunner) Hooks() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hooks == nil {
		m.hooks = map[string]string{}
	}
	return m.hooks
}

func (m *MockRunner) ShowOption(_ context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("ShowOption", map[string]any{"name": name})
	if m.options == nil {
		return "", nil
	}
	return m.options[name], nil
}

func (m *MockRunner) SetOption(_ context.Context, name, value string, window bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("SetOption", map[string]any{"name": name, "value": value, "window": window})
	if m.options == nil {
		m.options = map[string]string{}
	}
	m.options[name] = value
	return nil
}

func (m *MockRunner) ShowHook(_ context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("ShowHook", map[string]any{"name": name})
	if m.hooks == nil {
		return "", nil
	}
	return m.hooks[name], nil
}

func (m *MockRunner) UnsetHook(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("UnsetHook", map[string]any{"name": name})
	if m.hooks != nil {
		delete(m.hooks, name)
	}
	return nil
}

// --- helpers ---

func (m *MockRunner) nextPaneID() string {
	m.paneSerial++
	return fmt.Sprintf("%%%d", m.paneSerial)
}

// findWindow locates a window by "session:index" or "session:name" target.
func (m *MockRunner) findWindow(target string) (*mockWindow, error) {
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 || parts[0] == "" {
		return nil, fmt.Errorf("%w: %s", errs.ErrWindowNotFound, target)
	}
	s, ok := m.sessions[parts[0]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errs.ErrSessionNotFound, parts[0])
	}
	suffix := parts[1]
	// suffix may be "index[.pane]" or "name"
	if dot := strings.IndexByte(suffix, '.'); dot >= 0 {
		suffix = suffix[:dot]
	}
	if idx, err := strconv.Atoi(suffix); err == nil {
		for _, w := range s.windows {
			if w.index == idx {
				return w, nil
			}
		}
	}
	for _, w := range s.windows {
		if w.name == suffix {
			return w, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", errs.ErrWindowNotFound, target)
}

func parseWindowTarget(target string) (session string, index int, err error) {
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid target %q", target)
	}
	suffix := parts[1]
	if dot := strings.IndexByte(suffix, '.'); dot >= 0 {
		suffix = suffix[:dot]
	}
	idx, err := strconv.Atoi(suffix)
	if err != nil {
		return parts[0], 0, err
	}
	return parts[0], idx, nil
}
