package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	errs "github.com/mark1708/tmh/internal/errors"
)

// sep is an ASCII Unit Separator — safe inside tmux format strings and not
// present in paths, session names, or command lines. Used for the persisted
// canonical ServerEpoch.Value.
const sep = "\x1f"

// epochTransportSep is a printable colon used for tmux transport format.
// tmux 3.2a replaces literal control separators with printable characters.
const epochTransportSep = ":"

// CLIRunner shells out to `tmux` for every operation. It's the production
// Runner implementation.
type CLIRunner struct {
	// Bin overrides the binary name (default: "tmux"). Useful if the user
	// installed tmux under a custom path.
	Bin string
}

// NewCLIRunner returns a CLIRunner with default settings.
func NewCLIRunner() *CLIRunner { return &CLIRunner{Bin: "tmux"} }

func (r *CLIRunner) bin() string {
	if r.Bin != "" {
		return r.Bin
	}
	return "tmux"
}

// run executes a tmux command and returns stdout. Stderr is included in the
// error for easier diagnostics.
func (r *CLIRunner) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.bin(), args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return out.Bytes(), classifyError(err, errBuf.Bytes())
	}
	return out.Bytes(), nil
}

// runInteractive runs tmux attaching stdin/stdout/stderr directly. Used by
// AttachSession so the terminal is handed over to tmux.
func (r *CLIRunner) runInteractive(args ...string) error {
	cmd := exec.Command(r.bin(), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func classifyError(err error, stderr []byte) error {
	msg := strings.ToLower(string(stderr))
	switch {
	case strings.Contains(msg, "no server running"):
		return fmt.Errorf("%w: %s", errs.ErrServerNotRunning, strings.TrimSpace(string(stderr)))
	case strings.Contains(msg, "duplicate session"):
		return fmt.Errorf("%w: %s", errs.ErrSessionExists, strings.TrimSpace(string(stderr)))
	case strings.Contains(msg, "can't find session"), strings.Contains(msg, "session not found"):
		return fmt.Errorf("%w: %s", errs.ErrSessionNotFound, strings.TrimSpace(string(stderr)))
	case strings.Contains(msg, "can't find window"), strings.Contains(msg, "window not found"):
		return fmt.Errorf("%w: %s", errs.ErrWindowNotFound, strings.TrimSpace(string(stderr)))
	case strings.Contains(msg, "permission denied"):
		return fmt.Errorf("%w: %s", errs.ErrPermission, strings.TrimSpace(string(stderr)))
	}
	return fmt.Errorf("tmux: %v: %s", err, strings.TrimSpace(string(stderr)))
}

// --- server lifecycle ---

func (r *CLIRunner) InTmux() bool { return os.Getenv("TMUX") != "" }

func (r *CLIRunner) ServerRunning(ctx context.Context) (bool, error) {
	_, err := r.run(ctx, "list-sessions", "-F", "#{session_name}")
	if err == nil {
		return true, nil
	}
	// "no server running" is expected here
	return false, nil
}

func (r *CLIRunner) StartServer(ctx context.Context) error {
	_, err := r.run(ctx, "start-server")
	return err
}

// --- sessions ---

func (r *CLIRunner) ListSessions(ctx context.Context) ([]Session, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{session_windows}",
		"#{session_attached}",
	}, sep)
	out, err := r.run(ctx, "list-sessions", "-F", format)
	if err != nil {
		// tmux exits non-zero with "no server running" when nothing exists.
		if strings.Contains(err.Error(), errs.ErrServerNotRunning.Error()) {
			return nil, nil
		}
		return nil, err
	}
	var sessions []Session
	for _, line := range splitLines(out) {
		parts := strings.Split(line, sep)
		if len(parts) < 3 {
			continue
		}
		n, _ := strconv.Atoi(parts[1])
		attached, _ := strconv.Atoi(parts[2])
		sessions = append(sessions, Session{
			Name:     parts[0],
			Windows:  n,
			Attached: attached > 0,
		})
	}
	return sessions, nil
}

func (r *CLIRunner) HasSession(ctx context.Context, name string) (bool, error) {
	_, err := r.run(ctx, "has-session", "-t", name)
	if err == nil {
		return true, nil
	}
	// has-session returns non-zero when missing; map to false without surfacing
	return false, nil
}

func (r *CLIRunner) NewSession(ctx context.Context, opts NewSessionOpts) error {
	args, err := newSessionArgs(opts)
	if err != nil {
		return err
	}
	_, err = r.run(ctx, args...)
	return err
}

func newSessionArgs(opts NewSessionOpts) ([]string, error) {
	if err := validateSession(opts.Name); err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}
	args := []string{"new-session"}
	if opts.Detached {
		args = append(args, "-d")
	}
	args = append(args, "-s", opts.Name)
	if opts.WindowName != "" {
		args = append(args, "-n", opts.WindowName)
	}
	if opts.Dir != "" {
		args = append(args, "-c", opts.Dir)
	}
	for _, k := range sortedMapKeys(opts.Env) {
		v := opts.Env[k]
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for _, name := range sortedMapKeys(opts.SessionOptions) {
		if err := validateSessionOptionName(name); err != nil {
			return nil, err
		}
		args = append(args, ";", "set-option", "-t", opts.Name, name, opts.SessionOptions[name])
	}
	return args, nil
}

func (r *CLIRunner) AttachSession(ctx context.Context, name string) error {
	return r.runInteractive("attach-session", "-t", name)
}

func (r *CLIRunner) SwitchClient(ctx context.Context, target string) error {
	_, err := r.run(ctx, "switch-client", "-t", target)
	return err
}

func (r *CLIRunner) KillSession(ctx context.Context, name string) error {
	_, err := r.run(ctx, "kill-session", "-t", name)
	return err
}

func (r *CLIRunner) RenameSession(ctx context.Context, from, to string) error {
	_, err := r.run(ctx, "rename-session", "-t", from, to)
	return err
}

// --- windows ---

func (r *CLIRunner) ListWindows(ctx context.Context, session string) ([]Window, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{window_index}",
		"#{window_name}",
		"#{window_panes}",
		"#{window_layout}",
		"#{window_active}",
	}, sep)
	args := []string{"list-windows", "-F", format}
	if session != "" {
		args = append(args, "-t", session)
	} else {
		args = append(args, "-a")
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		if strings.Contains(err.Error(), errs.ErrServerNotRunning.Error()) {
			return nil, nil
		}
		return nil, err
	}
	var wins []Window
	for _, line := range splitLines(out) {
		parts := strings.Split(line, sep)
		if len(parts) < 6 {
			continue
		}
		idx, _ := strconv.Atoi(parts[1])
		panes, _ := strconv.Atoi(parts[3])
		active, _ := strconv.Atoi(parts[5])
		wins = append(wins, Window{
			Session: parts[0],
			Index:   idx,
			Name:    parts[2],
			Panes:   panes,
			Layout:  parts[4],
			Active:  active > 0,
		})
	}
	return wins, nil
}

func (r *CLIRunner) NewWindow(ctx context.Context, opts NewWindowOpts) (Window, error) {
	args := []string{"new-window", "-P", "-F", "#{session_name}" + sep + "#{window_index}" + sep + "#{window_name}"}
	if opts.SessionTarget != "" {
		args = append(args, "-t", opts.SessionTarget)
	}
	if opts.Name != "" {
		args = append(args, "-n", opts.Name)
	}
	if opts.Dir != "" {
		args = append(args, "-c", opts.Dir)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		return Window{}, err
	}
	line := strings.TrimSpace(string(out))
	parts := strings.Split(line, sep)
	if len(parts) < 3 {
		return Window{}, fmt.Errorf("tmux: unexpected new-window output %q", line)
	}
	idx, _ := strconv.Atoi(parts[1])
	return Window{Session: parts[0], Index: idx, Name: parts[2]}, nil
}

func (r *CLIRunner) KillWindow(ctx context.Context, target string) error {
	_, err := r.run(ctx, "kill-window", "-t", target)
	return err
}

func (r *CLIRunner) RenameWindow(ctx context.Context, target, name string) error {
	_, err := r.run(ctx, "rename-window", "-t", target, name)
	return err
}

func (r *CLIRunner) SelectWindow(ctx context.Context, target string) error {
	_, err := r.run(ctx, "select-window", "-t", target)
	return err
}

// --- panes ---

func (r *CLIRunner) ListPanes(ctx context.Context, target string) ([]Pane, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{window_index}",
		"#{pane_index}",
		"#{pane_id}",
		"#{pane_current_command}",
		"#{pane_current_path}",
		"#{pane_active}",
		"#{pane_pid}",
	}, sep)
	args := []string{"list-panes", "-F", format}
	if target != "" {
		args = append(args, "-t", target)
	} else {
		args = append(args, "-a")
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		if strings.Contains(err.Error(), errs.ErrServerNotRunning.Error()) {
			return nil, nil
		}
		return nil, err
	}
	var panes []Pane
	for _, line := range splitLines(out) {
		parts := strings.Split(line, sep)
		if len(parts) < 7 {
			continue
		}
		winIdx, _ := strconv.Atoi(parts[1])
		paneIdx, _ := strconv.Atoi(parts[2])
		active, _ := strconv.Atoi(parts[6])
		var pid int
		if len(parts) >= 8 {
			pid, _ = strconv.Atoi(parts[7])
		}
		panes = append(panes, Pane{
			Session: parts[0],
			Window:  winIdx,
			Index:   paneIdx,
			ID:      parts[3],
			Command: parts[4],
			Path:    parts[5],
			Active:  active > 0,
			PID:     pid,
		})
	}
	return panes, nil
}

func (r *CLIRunner) SplitWindow(ctx context.Context, opts SplitOpts) error {
	args := []string{"split-window"}
	if opts.Horizontal {
		args = append(args, "-h")
	} else {
		args = append(args, "-v")
	}
	if opts.Target != "" {
		args = append(args, "-t", opts.Target)
	}
	if opts.Dir != "" {
		args = append(args, "-c", opts.Dir)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	_, err := r.run(ctx, args...)
	return err
}

func (r *CLIRunner) SelectLayout(ctx context.Context, target, layout string) error {
	args := []string{"select-layout"}
	if target != "" {
		args = append(args, "-t", target)
	}
	args = append(args, layout)
	_, err := r.run(ctx, args...)
	return err
}

func (r *CLIRunner) CapturePane(ctx context.Context, target string, lines int) ([]byte, error) {
	args := []string{"capture-pane", "-p", "-e"}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	if target != "" {
		args = append(args, "-t", target)
	}
	return r.run(ctx, args...)
}

func (r *CLIRunner) SendKeys(ctx context.Context, target string, keys ...string) error {
	args := []string{"send-keys"}
	if target != "" {
		args = append(args, "-t", target)
	}
	args = append(args, keys...)
	_, err := r.run(ctx, args...)
	return err
}

func (r *CLIRunner) KillPane(ctx context.Context, target string) error {
	_, err := r.run(ctx, "kill-pane", "-t", target)
	return err
}

func (r *CLIRunner) SetAutomaticRename(ctx context.Context, target string, on bool) error {
	val := "off"
	if on {
		val = "on"
	}
	_, err := r.run(ctx, "set-window-option", "-t", target, "automatic-rename", val)
	return err
}

// --- misc ---

func (r *CLIRunner) SourceFile(ctx context.Context, path string) error {
	_, err := r.run(ctx, "source-file", path)
	return err
}

func (r *CLIRunner) DisplayPopup(ctx context.Context, opts PopupOpts) error {
	args := []string{"display-popup"}
	if opts.Close {
		args = append(args, "-E")
	}
	if opts.Width != "" {
		args = append(args, "-w", opts.Width)
	}
	if opts.Height != "" {
		args = append(args, "-h", opts.Height)
	}
	if opts.Dir != "" {
		args = append(args, "-d", opts.Dir)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	if opts.Command != "" {
		args = append(args, opts.Command)
	}
	_, err := r.run(ctx, args...)
	return err
}

// --- options + hooks ---

// ShowOption returns the value of a global option via `tmux show-options -gv`.
// Empty string when the option is at its compiled default (tmux exits 1
// with "unknown option" — we treat that as "not set" rather than an error).
func (r *CLIRunner) ShowOption(ctx context.Context, name string) (string, error) {
	out, err := r.run(ctx, "show-options", "-gv", name)
	if err != nil {
		// tmux returns non-zero when the option is not explicitly set.
		// Distinguish from a real error by checking stderr text.
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unknown option") || strings.Contains(msg, "not set") || strings.Contains(msg, "not found") {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SetOption sets a global option. When window is true uses set-window-option.
func (r *CLIRunner) SetOption(ctx context.Context, name, value string, window bool) error {
	cmd := "set-option"
	if window {
		cmd = "set-window-option"
	}
	_, err := r.run(ctx, cmd, "-g", name, value)
	return err
}

// ShowHook returns the command bound to a hook, empty if unset.
//
// `tmux show-hooks -g` prints one line per hook; bound hooks have
//
//	after-new-window[0] rename-window "…"
//
// while unbound ones print just the name. We match by prefix and strip the
// `[idx]` position index to return the bound command.
func (r *CLIRunner) ShowHook(ctx context.Context, name string) (string, error) {
	out, err := r.run(ctx, "show-hooks", "-g")
	if err != nil {
		return "", err
	}
	for _, line := range splitLines(out) {
		if command, ok := hookCommandFromLine(name, line); ok {
			return command, nil
		}
	}
	return "", nil
}

func hookCommandFromLine(name string, line string) (string, bool) {
	l := strings.TrimSpace(line)
	if l == name {
		return "", true
	}
	if strings.HasPrefix(l, name+" ") {
		return strings.TrimSpace(l[len(name):]), true
	}
	if !strings.HasPrefix(l, name) {
		return "", false
	}
	rest := strings.TrimPrefix(l, name)
	if !strings.HasPrefix(rest, "[") {
		return "", false // same prefix but different hook (e.g. after-new-window-foo)
	}
	closeIdx := strings.IndexByte(rest, ']')
	if closeIdx < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[closeIdx+1:]), true
}

// UnsetHook removes a global hook binding (`tmux set-hook -gu NAME`).
func (r *CLIRunner) UnsetHook(ctx context.Context, name string) error {
	_, err := r.run(ctx, "set-hook", "-gu", name)
	return err
}

// WindowID returns the stable @N window_id for a target window.
func (r *CLIRunner) WindowID(ctx context.Context, target string) (string, error) {
	out, err := r.run(ctx, "display-message", "-p", "-t", target, "#{window_id}")
	if err != nil {
		return "", err
	}
	return parseWindowID(out)
}

// ListWindowLinks returns all linked windows across all sessions.
func (r *CLIRunner) ListWindowLinks(ctx context.Context) ([]WindowLink, error) {
	format := strings.Join([]string{
		"#{session_id}",
		"#{session_name}",
		"#{window_id}",
		"#{window_index}",
		"#{window_name}",
		"#{window_active}",
	}, sep)
	out, err := r.run(ctx, "list-windows", "-a", "-F", format)
	if err != nil {
		if strings.Contains(err.Error(), errs.ErrServerNotRunning.Error()) {
			return nil, nil
		}
		return nil, err
	}
	return parseWindowLinks(out)
}

// LinkWindow links a window by its ID into a destination session.
func (r *CLIRunner) LinkWindow(ctx context.Context, sourceWindowID, destination string) error {
	if err := validateWindowID(sourceWindowID); err != nil {
		return err
	}
	if err := validateDestination(destination); err != nil {
		return err
	}
	_, err := r.run(ctx, "link-window", "-d", "-s", sourceWindowID, "-t", destination)
	return err
}

// UnlinkWindow removes a window link by exact session:@ID target.
func (r *CLIRunner) UnlinkWindow(ctx context.Context, session, windowID string) error {
	if err := validateSession(session); err != nil {
		return err
	}
	if err := validateWindowID(windowID); err != nil {
		return err
	}
	_, err := r.run(ctx, "unlink-window", "-t", fmt.Sprintf("%s:%s", session, windowID))
	return err
}

// ServerEpoch returns the server identity: socket_path, start_time, pid.
// Uses printable ':' as the transport separator (tmux 3.2a replaces control chars).
func (r *CLIRunner) ServerEpoch(ctx context.Context) (ServerEpoch, error) {
	format := strings.Join([]string{
		"#{socket_path}",
		"#{start_time}",
		"#{pid}",
	}, epochTransportSep)
	out, err := r.run(ctx, "display-message", "-p", format)
	if err != nil {
		return ServerEpoch{}, err
	}
	return parseServerEpoch(out)
}

// ShowSessionOption returns the value of a session-scoped option.
func (r *CLIRunner) ShowSessionOption(ctx context.Context, session, name string) (string, error) {
	out, err := r.run(ctx, "show-options", "-t", session, "-v", name)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unknown option") || strings.Contains(msg, "not set") || strings.Contains(msg, "not found") {
			return "", nil
		}
		return "", err
	}
	return parseSessionOption(out)
}

// SetSessionOption sets a session-scoped option.
func (r *CLIRunner) SetSessionOption(ctx context.Context, session, name, value string) error {
	_, err := r.run(ctx, "set-option", "-t", session, name, value)
	return err
}

var windowIDRegexp = regexp.MustCompile(`^@[0-9]+$`)
var sessionOptionRegexp = regexp.MustCompile(`^@[A-Za-z0-9][A-Za-z0-9_-]*$`)

func validateWindowID(windowID string) error {
	if windowID == "" {
		return fmt.Errorf("window_id cannot be empty")
	}
	if !windowIDRegexp.MatchString(windowID) {
		return fmt.Errorf("invalid window_id %q: must match ^@[0-9]+$", windowID)
	}
	return nil
}

func validateSession(session string) error {
	if session == "" {
		return fmt.Errorf("session cannot be empty")
	}
	if strings.Contains(session, ":") {
		return fmt.Errorf("invalid session name %q: contains colon (ambiguous target)", session)
	}
	return nil
}

func validateSessionOptionName(name string) error {
	if !sessionOptionRegexp.MatchString(name) {
		return fmt.Errorf("invalid session user option %q", name)
	}
	return nil
}

func validateDestination(destination string) error {
	if destination == "" {
		return fmt.Errorf("destination cannot be empty")
	}
	return nil
}

func parseWindowID(output []byte) (string, error) {
	s := strings.TrimSpace(string(output))
	if s == "" {
		return "", fmt.Errorf("empty window_id from tmux")
	}
	if !windowIDRegexp.MatchString(s) {
		return "", fmt.Errorf("malformed window_id %q from tmux", s)
	}
	return s, nil
}

func parseWindowLinks(output []byte) ([]WindowLink, error) {
	var links []WindowLink
	for _, line := range splitLines(output) {
		parts := strings.Split(line, sep)
		if len(parts) < 6 {
			continue
		}
		idx, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid window_index %q: %w", parts[3], err)
		}
		active, err := strconv.Atoi(parts[5])
		if err != nil {
			return nil, fmt.Errorf("invalid window_active %q: %w", parts[5], err)
		}
		links = append(links, WindowLink{
			SessionID:   parts[0],
			SessionName: parts[1],
			WindowID:    parts[2],
			WindowIndex: idx,
			WindowName:  parts[4],
			Active:      active > 0,
		})
	}
	return links, nil
}

func parseServerEpoch(output []byte) (ServerEpoch, error) {
	s := strings.TrimSpace(string(output))
	if s == "" {
		return ServerEpoch{}, fmt.Errorf("empty epoch output from tmux")
	}

	// Parse from right-to-left to handle socket paths that may contain ':'.
	// Expected format: socket_path:start_time:pid
	lastColon := strings.LastIndexByte(s, ':')
	if lastColon == -1 {
		return ServerEpoch{}, fmt.Errorf("malformed epoch output: %q (want 3 ':'-separated fields)", s)
	}

	pid := strings.TrimSpace(s[lastColon+1:])
	if pid == "" {
		return ServerEpoch{}, fmt.Errorf("epoch pid is empty")
	}

	secondLastColon := strings.LastIndexByte(s[:lastColon], ':')
	if secondLastColon == -1 {
		return ServerEpoch{}, fmt.Errorf("malformed epoch output: %q (want 3 ':'-separated fields)", s)
	}

	startTime := strings.TrimSpace(s[secondLastColon+1 : lastColon])
	if startTime == "" {
		return ServerEpoch{}, fmt.Errorf("epoch start_time is empty")
	}

	socketPath := strings.TrimSpace(s[:secondLastColon])
	if socketPath == "" {
		return ServerEpoch{}, fmt.Errorf("epoch socket_path is empty")
	}

	// Validate that startTime and pid are parseable as integers and non-negative.
	startTimeInt, err := strconv.ParseInt(startTime, 10, 64)
	if err != nil {
		return ServerEpoch{}, fmt.Errorf("epoch start_time %q is not a valid integer: %w", startTime, err)
	}
	if startTimeInt < 0 {
		return ServerEpoch{}, fmt.Errorf("epoch start_time %q must be non-negative", startTime)
	}

	pidInt, err := strconv.Atoi(pid)
	if err != nil {
		return ServerEpoch{}, fmt.Errorf("epoch pid %q is not a valid integer: %w", pid, err)
	}
	if pidInt < 0 {
		return ServerEpoch{}, fmt.Errorf("epoch pid %q must be non-negative", pid)
	}

	// Canonical persisted value uses sep ('\x1f'), not the transport separator.
	value := fmt.Sprintf("%s%s%s%s%s", socketPath, sep, startTime, sep, pid)
	return ServerEpoch{ServerKey: socketPath, Value: value}, nil
}

func parseSessionOption(output []byte) (string, error) {
	s := strings.TrimSpace(string(output))
	return s, nil
}

// --- helpers ---

func splitLines(b []byte) []string {
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
