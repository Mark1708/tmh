package actions

import (
	"context"
	"strings"
	"testing"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
)

// TestSetupWithActiveConfig_EnabledIncludesHook verifies that when active is enabled,
// the hook is included if slot is empty or OK.
func TestSetupWithActiveConfig_EnabledIncludesHook(t *testing.T) {
	ctx := context.Background()
	mock := tmuxtest.New()
	mock.Hooks()["session-window-changed[1708]"] = "(unset)"

	cfg := &config.Config{
		Defaults: config.Defaults{
			TmuxIntegration: config.TmuxIntegrationConfig{
				Active: config.ActiveSessionConfig{
					Enabled: true,
					TTL:     "24h",
				},
			},
		},
	}

	snippets := SetupWithActiveConfig(ctx, mock, cfg)

	// Should include the active hook since enabled and slot is unset
	var foundHook bool
	for _, s := range snippets {
		if strings.Contains(s.Line, ActiveHookCommand) {
			foundHook = true
			break
		}
	}
	if !foundHook {
		t.Error("active hook should be included when enabled and slot is unset")
	}
}

// TestSetupWithActiveConfig_DisabledExcludesHook verifies that when active is disabled,
// the hook is never included.
func TestSetupWithActiveConfig_DisabledExcludesHook(t *testing.T) {
	ctx := context.Background()
	mock := tmuxtest.New()
	mock.Hooks()["session-window-changed[1708]"] = "(unset)"

	cfg := &config.Config{
		Defaults: config.Defaults{
			TmuxIntegration: config.TmuxIntegrationConfig{
				Active: config.ActiveSessionConfig{
					Enabled: false,
				},
			},
		},
	}

	snippets := SetupWithActiveConfig(ctx, mock, cfg)

	// Should not include the active hook when disabled
	for _, s := range snippets {
		if s.Line == ActiveHookCommand {
			t.Error("active hook should not be included when disabled")
		}
	}
}

// TestSetupWithActiveConfig_ForeignHookConflict verifies that when foreign hook exists,
// it is not replaced (disabled) or marked OK (enabled).
func TestSetupWithActiveConfig_ForeignHookConflict(t *testing.T) {
	ctx := context.Background()
	foreignCommand := `run-shell "echo foreign"`
	mock := tmuxtest.New()
	mock.Hooks()["session-window-changed[1708]"] = foreignCommand

	tests := []struct {
		name    string
		enabled bool
		expect  bool // expect hook in snippets?
	}{
		{"enabled foreign conflict", true, false},
		{"disabled foreign conflict", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Defaults: config.Defaults{
					TmuxIntegration: config.TmuxIntegrationConfig{
						Active: config.ActiveSessionConfig{
							Enabled: tt.enabled,
						},
					},
				},
			}

			snippets := SetupWithActiveConfig(ctx, mock, cfg)

			// Should not include hook when foreign command exists
			var foundHook bool
			for _, s := range snippets {
				if s.Line == ActiveHookCommand {
					foundHook = true
					break
				}
			}
			if foundHook != tt.expect {
				t.Errorf("foundHook=%v, want=%v", foundHook, tt.expect)
			}
		})
	}
}

// TestAuditTmuxConfigWithActiveConfig_DisabledDoesNotReportMissing verifies that when
// active is disabled, missing active hook is not reported as a problem.
func TestAuditTmuxConfigWithActiveConfig_DisabledDoesNotReportMissing(t *testing.T) {
	ctx := context.Background()
	mock := tmuxtest.New()
	mock.Hooks()["session-window-changed[1708]"] = "(unset)"

	cfg := &config.Config{
		Defaults: config.Defaults{
			TmuxIntegration: config.TmuxIntegrationConfig{
				Active: config.ActiveSessionConfig{
					Enabled: false,
				},
			},
		},
	}

	findings := AuditTmuxConfigWithActiveConfig(ctx, mock, cfg)

	// Should not report any issue with active hook when disabled
	for _, f := range findings {
		if f.Check == "active session hook" && f.Level != AuditOK {
			t.Errorf("active hook should not be reported when disabled, got level=%s", f.Level)
		}
	}
}

// TestAuditTmuxConfigWithActiveConfig_EnabledReportsMissing verifies that when
// active is enabled and slot is unset, it reports missing hook.
func TestAuditTmuxConfigWithActiveConfig_EnabledReportsMissing(t *testing.T) {
	ctx := context.Background()
	mock := tmuxtest.New()
	mock.Hooks()["session-window-changed[1708]"] = "(unset)"

	cfg := &config.Config{
		Defaults: config.Defaults{
			TmuxIntegration: config.TmuxIntegrationConfig{
				Active: config.ActiveSessionConfig{
					Enabled: true,
					TTL:     "24h",
				},
			},
		},
	}

	findings := AuditTmuxConfigWithActiveConfig(ctx, mock, cfg)

	// Should report missing active hook when enabled
	var foundIssue bool
	for _, f := range findings {
		if f.Check == "active session hook" && f.Level != AuditOK {
			foundIssue = true
			break
		}
	}
	if !foundIssue {
		t.Error("missing active hook should be reported when enabled")
	}
}

// TestSetup_PreservesLegacyBehavior verifies that legacy Setup without config
// preserves existing behavior (no config awareness - hook not included).
func TestSetup_PreservesLegacyBehavior(t *testing.T) {
	ctx := context.Background()
	mock := tmuxtest.New()
	mock.Hooks()["session-window-changed[1708]"] = "(unset)"

	// Legacy Setup should work without config parameter
	snippets := Setup(ctx, mock)

	// Should NOT include hook since no config is provided (legacy behavior)
	var foundHook bool
	for _, s := range snippets {
		if strings.Contains(s.Line, ActiveHookCommand) {
			foundHook = true
			break
		}
	}
	if foundHook {
		t.Error("legacy Setup should not include hook when no config is provided")
	}
}
