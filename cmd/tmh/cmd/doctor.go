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
				fmt.Fprintln(c.OutOrStdout(), "\ntmux integration:")
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
					fmt.Fprintf(c.OutOrStdout(), "%s%-38s %s\n", marker, f.Check, f.Message)
					if f.Level != actions.AuditOK && f.FixHint != "" {
						fmt.Fprintf(c.OutOrStdout(), "      → %s\n", f.FixHint)
					}
				}
			}
			if hasError {
				return cmdErr("one or more checks failed")
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
		out = append(out, doctorResult{"ERROR", "tmux not installed (apt/brew install tmux)"})
	} else {
		if v, ok := tmuxVersionAtLeast(3, 2); ok {
			out = append(out, doctorResult{"INFO", "tmux " + v})
		} else {
			out = append(out, doctorResult{"ERROR", "tmux version < 3.2 (display-popup unavailable)"})
		}
	}

	// shell
	sh := os.Getenv("SHELL")
	if sh == "" {
		out = append(out, doctorResult{"WARN", "$SHELL not set"})
	} else {
		out = append(out, doctorResult{"INFO", "shell: " + sh})
	}

	// config.yml
	cfg, err := config.Load(resolveConfigPath())
	switch {
	case err != nil && strings.Contains(err.Error(), "not found"):
		out = append(out, doctorResult{"WARN", "config.yml not found at " + resolveConfigPath()})
	case err != nil:
		out = append(out, doctorResult{"ERROR", "config.yml invalid: " + err.Error()})
	default:
		if verr := config.Validate(cfg); verr != nil {
			out = append(out, doctorResult{"ERROR", "config.yml schema: " + verr.Error()})
		} else {
			out = append(out, doctorResult{"INFO", "config.yml valid"})
		}
	}

	// tmux server reachable (not a hard failure)
	r := tmux.NewCLIRunner()
	ok, _ := r.ServerRunning(context.Background())
	if ok {
		out = append(out, doctorResult{"INFO", "tmux server running"})
	} else {
		out = append(out, doctorResult{"INFO", "tmux server not running (will start on first action)"})
	}

	// fd
	if _, err := exec.LookPath("fd"); err != nil {
		out = append(out, doctorResult{"INFO", "fd not installed (optional, used for file-picker)"})
	}
	// terminal-notifier
	if _, err := exec.LookPath("terminal-notifier"); err != nil {
		out = append(out, doctorResult{"INFO", "terminal-notifier not installed (optional, used for tmh watch)"})
	}
	// GOPRIVATE
	if v := os.Getenv("GOPRIVATE"); strings.Contains(v, "git.mark1708.ru") {
		out = append(out, doctorResult{"INFO", "GOPRIVATE includes git.mark1708.ru"})
	} else {
		out = append(out, doctorResult{"INFO", "GOPRIVATE does not include git.mark1708.ru (needed only for `go install` from self-hosted source)"})
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
