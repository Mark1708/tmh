package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark1708/tmh/internal/config"
)

// TestMigration_SmokeParse verifies that every fixture in testdata/configs/
// loads without error and produces a non-nil Config with Version ≥ 1.
// This guards against regressions where adding a new mandatory field breaks
// existing configs that don't declare it.
func TestMigration_SmokeParse(t *testing.T) {
	fixtures, err := filepath.Glob("../../testdata/configs/*.yml")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixture configs found in testdata/configs/")
	}
	for _, path := range fixtures {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			cfg, err := config.Parse(data)
			if err != nil {
				t.Fatalf("Parse(%s): %v", path, err)
			}
			if cfg == nil {
				t.Fatal("Parse returned nil config")
			}
			if cfg.Version < 1 {
				t.Fatalf("expected Version >= 1, got %d", cfg.Version)
			}
			// New optional fields must have sane zero-value defaults — they must
			// never cause a nil-deref or panic during normal operation.
			_ = cfg.Defaults.History.MaxEntries
			_ = cfg.Defaults.Display.ShowFooterHeatmap
			_ = cfg.Defaults.Behaviour.DryRunDefault
			_ = cfg.Defaults.Marks.PersistAcrossSessions
			_ = cfg.Defaults.TmuxIntegration.MouseMode
		})
	}
}

// TestMigration_NewFieldsHaveSaneDefaults verifies the zero-value of each new
// config field introduced during the refactoring is well-defined and non-breaking.
func TestMigration_NewFieldsHaveSaneDefaults(t *testing.T) {
	cfg, err := config.Parse([]byte("version: 1\n"))
	if err != nil {
		t.Fatalf("parse minimal: %v", err)
	}

	// History: MaxEntries 0 means "use built-in default" — not "zero entries".
	if cfg.Defaults.History.MaxEntries < 0 {
		t.Errorf("History.MaxEntries should not be negative, got %d", cfg.Defaults.History.MaxEntries)
	}
	// Retention empty string means "use built-in default (30d)".
	if cfg.Defaults.History.Retention != "" {
		t.Errorf("History.Retention should be empty for minimal config, got %q", cfg.Defaults.History.Retention)
	}
	// Display booleans default to false (disabled).
	if cfg.Defaults.Display.ShowFooterHeatmap {
		t.Error("ShowFooterHeatmap should default to false")
	}
	// Behaviour: DryRunDefault false means y is the default confirm key.
	if cfg.Defaults.Behaviour.DryRunDefault {
		t.Error("DryRunDefault should default to false")
	}
}

// TestMigration_LegacyConfigFullResolve verifies that a legacy config
// (no new fields) fully resolves through Resolve without panicking.
func TestMigration_LegacyConfigFullResolve(t *testing.T) {
	data, err := os.ReadFile("../../testdata/configs/legacy_no_new_fields.yml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Resolve must not panic or error on a well-formed legacy config.
	resolved, err := config.Resolve(cfg, "")
	if err != nil {
		t.Fatalf("resolve legacy config: %v", err)
	}
	if resolved == nil {
		t.Fatal("Resolve returned nil")
	}
	// The legacy session must survive resolution.
	if len(resolved.Sessions) == 0 {
		t.Error("expected at least one resolved session")
	}
}
