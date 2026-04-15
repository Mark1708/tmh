package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	errs "git.mark1708.ru/me/tmh/internal/errors"
)

// sep is an ASCII Unit Separator — safe inside tmux format strings and not
// present in paths, session names, or command lines.
const sep = "\x1f"

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
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	_, err := r.run(ctx, args...)
	return err
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
		panes = append(panes, Pane{
			Session: parts[0],
			Window:  winIdx,
			Index:   paneIdx,
			ID:      parts[3],
			Command: parts[4],
			Path:    parts[5],
			Active:  active > 0,
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

// --- helpers ---

func splitLines(b []byte) []string {
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
