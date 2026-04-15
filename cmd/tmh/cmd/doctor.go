package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/tmux"

	"github.com/spf13/cobra"
)

// newDoctorCmd checks the environment. Exits non-zero if any ERROR appears.
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: i18n.T("cli.doctor.short"),
		RunE: func(c *cobra.Command, args []string) error {
			checks := runDoctor()
			hasError := false
			for _, r := range checks {
				fmt.Fprintf(c.OutOrStdout(), "%-8s %s\n", r.level, r.msg)
				if r.level == "ERROR" {
					hasError = true
				}
			}
			// tmux integration audit — separate block so the user can see
			// option-level findings distinctly from environment checks.
			findings := actions.AuditTmuxConfig(context.Background(), tmux.NewCLIRunner())
			if len(findings) > 0 {
				fmt.Fprintln(c.OutOrStdout(), "\n"+i18n.T("doctor.tmux_integration_header"))
				for _, f := range findings {
					marker := "  "
					switch f.Level {
					case actions.AuditOK:
						marker = "  ✓ "
					case actions.AuditWarn:
						marker = "  ⚠ "
					case actions.AuditError:
						marker = "  ✗ "
						hasError = true
					}
					msg := f.Message
					if f.MessageKey != "" {
						if t := i18n.T(f.MessageKey); t != f.MessageKey {
							msg = t
						}
					}
					fmt.Fprintf(c.OutOrStdout(), "%s%-38s %s\n", marker, f.Check, msg)
					if f.Level != actions.AuditOK && f.FixHint != "" {
						fix := f.FixHint
						if f.FixKey != "" {
							if t := i18n.T(f.FixKey); t != f.FixKey {
								fix = t
							}
						}
						fmt.Fprintf(c.OutOrStdout(), "      → %s\n", fix)
					}
				}
			}
			if hasError {
				return cmdErr("%s", i18n.T("doctor.failed"))
			}
			return nil
		},
	}
}

type doctorResult struct {
	level string // INFO | WARN | ERROR
	msg   string
}

func runDoctor() []doctorResult {
	var out []doctorResult

	// tmux installed + version
	if _, err := exec.LookPath("tmux"); err != nil {
		out = append(out, doctorResult{"ERROR", i18n.T("doctor.tmux.missing")})
	} else {
		if v, ok := tmuxVersionAtLeast(3, 2); ok {
			out = append(out, doctorResult{"INFO", i18n.Tf("doctor.tmux.ok", map[string]any{"version": v})})
		} else {
			out = append(out, doctorResult{"ERROR", i18n.T("doctor.tmux.old_version")})
		}
	}

	// shell
	sh := os.Getenv("SHELL")
	if sh == "" {
		out = append(out, doctorResult{"WARN", i18n.T("doctor.shell.missing")})
	} else {
		out = append(out, doctorResult{"INFO", i18n.Tf("doctor.shell.present", map[string]any{"shell": sh})})
	}

	// config.yml
	cfg, err := config.Load(resolveConfigPath())
	switch {
	case err != nil && strings.Contains(err.Error(), "not found"):
		out = append(out, doctorResult{"WARN", i18n.Tf("doctor.config.missing", map[string]any{"path": resolveConfigPath()})})
	case err != nil:
		out = append(out, doctorResult{"ERROR", i18n.Tf("doctor.config.invalid", map[string]any{"err": err.Error()})})
	default:
		if verr := config.Validate(cfg); verr != nil {
			out = append(out, doctorResult{"ERROR", i18n.Tf("doctor.config.schema", map[string]any{"err": verr.Error()})})
		} else {
			out = append(out, doctorResult{"INFO", i18n.T("doctor.config.ok")})
		}
	}

	// tmux server reachable (not a hard failure)
	r := tmux.NewCLIRunner()
	ok, _ := r.ServerRunning(context.Background())
	if ok {
		out = append(out, doctorResult{"INFO", i18n.T("doctor.server.running")})
	} else {
		out = append(out, doctorResult{"INFO", i18n.T("doctor.server.stopped")})
	}

	// fd
	if _, err := exec.LookPath("fd"); err != nil {
		out = append(out, doctorResult{"INFO", i18n.T("doctor.fd.missing")})
	}
	// terminal-notifier
	if _, err := exec.LookPath("terminal-notifier"); err != nil {
		out = append(out, doctorResult{"INFO", i18n.T("doctor.notifier.missing")})
	}
	// GOPRIVATE
	if v := os.Getenv("GOPRIVATE"); strings.Contains(v, "git.mark1708.ru") {
		out = append(out, doctorResult{"INFO", i18n.T("doctor.goprivate.set")})
	} else {
		out = append(out, doctorResult{"INFO", i18n.T("doctor.goprivate.unset")})
	}

	return out
}

// tmuxVersionAtLeast runs `tmux -V` and checks against major.minor.
func tmuxVersionAtLeast(wantMajor, wantMinor int) (string, bool) {
	out, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(out))
	// format: "tmux 3.3a" or "tmux 3.4"
	parts := strings.Fields(v)
	if len(parts) < 2 {
		return v, false
	}
	ver := parts[1]
	var maj, min int
	fmt.Sscanf(ver, "%d.%d", &maj, &min)
	if maj > wantMajor || (maj == wantMajor && min >= wantMinor) {
		return v, true
	}
	return v, false
}
