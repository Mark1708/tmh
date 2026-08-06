package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux/tmuxtest"
	"github.com/mark1708/tmh/internal/ui/picker"
)

func TestLoadConfigMissingOKDoesNotHideMalformedConfig(t *testing.T) {
	resetRootFlags(t)
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("version: [\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	flags.ConfigPath = path

	_, err := loadConfig(true)
	if err == nil {
		t.Fatal("expected malformed config error")
	}
	if !strings.Contains(err.Error(), "config: invalid") {
		t.Fatalf("error = %v, want config invalid", err)
	}
}

func TestLoadConfigMissingOKAllowsAbsentConfig(t *testing.T) {
	resetRootFlags(t)
	flags.ConfigPath = filepath.Join(t.TempDir(), "missing.yml")

	cfg, err := loadConfig(true)
	if err != nil {
		t.Fatalf("load missing config: %v", err)
	}
	if cfg.Defaults.TmuxIntegration.Active.Enabled {
		t.Fatal("missing config should keep active windows disabled")
	}
}

func TestOpenStateForConfigFailsClosedWhenActiveEnabled(t *testing.T) {
	stateDirFile := filepath.Join(t.TempDir(), "state-is-file")
	if err := os.WriteFile(stateDirFile, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write state dir file: %v", err)
	}
	t.Setenv("TMH_STATE_DIR", stateDirFile)

	cfg := &config.Config{}
	cfg.Defaults.TmuxIntegration.Active = config.ActiveSessionConfig{Enabled: true, TTL: "5h"}

	db, err := openStateForConfig(cfg)
	if err == nil {
		if db != nil {
			_ = db.Close()
		}
		t.Fatal("expected state open error")
	}
	if !strings.Contains(err.Error(), "open state db") {
		t.Fatalf("error = %v, want state db context", err)
	}
}

func TestAttachPickedOpensStateBeforeCreatingDiscoveredSession(t *testing.T) {
	resetRootFlags(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(configPath, []byte("version: 1\ndefaults:\n  tmux_integration:\n    active:\n      enabled: true\n      ttl: 5h\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	flags.ConfigPath = configPath
	stateDirFile := filepath.Join(dir, "state-is-file")
	if err := os.WriteFile(stateDirFile, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write state dir file: %v", err)
	}
	t.Setenv("TMH_STATE_DIR", stateDirFile)
	runner := tmuxtest.New()

	err := attachPicked(nil, runner, picker.Result{Target: "discovered", Dir: dir})
	if err == nil {
		t.Fatal("expected state open error")
	}
	if strings.Contains(methodNames(runner.Calls()), "NewSession") {
		t.Fatalf("created discovered session before state open failed: %+v", runner.Calls())
	}
}

func methodNames(calls []tmuxtest.Call) string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Method)
	}
	return strings.Join(names, ",")
}

func TestOpenStateForConfigAllowsUnavailableStateWhenActiveDisabled(t *testing.T) {
	stateDirFile := filepath.Join(t.TempDir(), "state-is-file")
	if err := os.WriteFile(stateDirFile, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write state dir file: %v", err)
	}
	t.Setenv("TMH_STATE_DIR", stateDirFile)

	db, err := openStateForConfig(&config.Config{})
	if err != nil {
		t.Fatalf("disabled active should not require state: %v", err)
	}
	if db != nil {
		t.Fatalf("disabled active with unavailable state returned db: %#v", db)
	}
}

func resetRootFlags(t *testing.T) {
	t.Helper()
	oldFlags := flags
	oldDashboard := dashboardFlag
	flags = RootFlags{}
	dashboardFlag = false
	t.Cleanup(func() {
		flags = oldFlags
		dashboardFlag = oldDashboard
	})
}
