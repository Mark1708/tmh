package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"git.mark1708.ru/me/tmh/internal/config"
	errs "git.mark1708.ru/me/tmh/internal/errors"
	"git.mark1708.ru/me/tmh/internal/state"
)

// HookContext carries enough info to render meaningful trust prompts and to
// pass env/cwd to spawned hook commands.
type HookContext struct {
	ConfigPath string
	SessionDir string            // working directory for hooks
	Env        map[string]string // env passed to commands
}

// TrustPrompter is the user-interaction surface for hook trust. The CLI
// implementation reads y/N from stdin; the TUI implementation surfaces a
// modal dialog. Returning true means "trust this config".
type TrustPrompter func(commands []string) (bool, error)

// HookOptions tunes hook execution.
type HookOptions struct {
	NoHooks  bool          // skip running anything (tmh --no-hooks)
	Prompter TrustPrompter // if nil, hooks fail with ErrHookDenied when untrusted
}

// hashConfig returns a content hash for the on-disk config so trust survives
// no-op edits but invalidates on real changes.
func hashConfig(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// EnsureTrusted verifies the (config_path, content_hash) pair is trusted in
// state. If not, it lists every hook command and asks the prompter to
// approve. Approval is persisted.
//
// Returns ErrHookDenied when the user (or the missing prompter) refuses.
func EnsureTrusted(ctx context.Context, db *state.DB, hc HookContext, hooksList []string, prompter TrustPrompter) error {
	if len(hooksList) == 0 || db == nil {
		return nil
	}
	hash, err := hashConfig(hc.ConfigPath)
	if err != nil {
		return fmt.Errorf("hooks: hash config: %w", err)
	}
	ok, err := db.IsTrusted(ctx, hc.ConfigPath, hash)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if prompter == nil {
		return fmt.Errorf("%w (no prompter available; pass --no-hooks or run interactively)", errs.ErrHookDenied)
	}
	approved, err := prompter(hooksList)
	if err != nil {
		return err
	}
	if !approved {
		return fmt.Errorf("%w", errs.ErrHookDenied)
	}
	return db.MarkTrusted(ctx, hc.ConfigPath, hash)
}

// RunHooks executes a list of shell commands sequentially. Each runs in `sh
// -c` with the supplied env and working directory. Output is forwarded to
// the supplied writers.
//
// Trust enforcement is the caller's responsibility (use EnsureTrusted first).
func RunHooks(ctx context.Context, hc HookContext, opts HookOptions, hooks []string, stdout, stderr io.Writer) error {
	if opts.NoHooks || len(hooks) == 0 {
		return nil
	}
	for _, h := range hooks {
		if strings.TrimSpace(h) == "" {
			continue
		}
		cmd := exec.CommandContext(ctx, "sh", "-c", h)
		cmd.Dir = hc.SessionDir
		cmd.Env = mergeOSEnv(hc.Env)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook %q: %w", h, err)
		}
	}
	return nil
}

// CollectHookCommands returns every hook command across the resolved config
// (defaults, profile, sessions). Used to render the trust prompt's full
// inventory.
func CollectHookCommands(cfg *config.Config, profile string) []string {
	if cfg == nil {
		return nil
	}
	resolved, err := config.Resolve(cfg, profile)
	if err != nil {
		return nil
	}
	var out []string
	for _, s := range resolved.Sessions {
		out = append(out, s.Hooks.OnCreate...)
		out = append(out, s.Hooks.OnAttach...)
		out = append(out, s.Hooks.OnDestroy...)
	}
	return out
}

// mergeOSEnv overlays a key=value map on top of os.Environ() preserving
// inheritance. Empty input returns os.Environ() unchanged.
func mergeOSEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}
	base := os.Environ()
	override := make(map[string]string, len(extra))
	for k, v := range extra {
		override[k] = v
	}
	out := make([]string, 0, len(base)+len(extra))
	for _, kv := range base {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:eq]
		if v, ok := override[k]; ok {
			out = append(out, k+"="+v)
			delete(override, k)
			continue
		}
		out = append(out, kv)
	}
	for k, v := range override {
		out = append(out, k+"="+v)
	}
	return out
}
