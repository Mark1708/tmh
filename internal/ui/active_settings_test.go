package ui

import (
	"errors"
	"testing"

	"github.com/mark1708/tmh/internal/config"
	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/ui/theme"
	"gopkg.in/yaml.v3"
)

func TestActiveSettingsDefaultTTL(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse([]byte("version: 1\ndefaults:\n  tmux_integration:\n    active:\n      enabled: true\n"))
	if err != nil {
		t.Fatal(err)
	}

	s := newSettings(DefaultKeys(), theme.New(theme.Mocha), LoadStrings(), cfg, t.TempDir()+"/config.yml", nil, nil, nil)
	enabled := settingsFieldByLabel(t, s.fields[catTmux], i18n.T("tui.settings.field.active_enabled"))
	ttl := settingsFieldByLabel(t, s.fields[catTmux], i18n.T("tui.settings.field.active_ttl"))

	if enabled.kind != fieldKindToggle || !enabled.on {
		t.Fatalf("active enabled field = %+v", enabled)
	}
	if ttl.kind != fieldKindDuration || ttl.text != config.DefaultActiveTTL {
		t.Fatalf("active TTL field = %+v, want effective %q", ttl, config.DefaultActiveTTL)
	}
}

func TestActiveSettingsTTLValidation(t *testing.T) {
	tests := []struct {
		value   string
		wantErr bool
	}{
		{value: "5h"},
		{value: "720h"},
		{value: ""},
		{value: "invalid", wantErr: true},
		{value: "0s", wantErr: true},
		{value: "721h", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := validateActiveTTLField(tt.value)
			if tt.wantErr != errors.Is(err, errs.ErrInvalidTTL) {
				t.Fatalf("validateActiveTTLField(%q) error = %v", tt.value, err)
			}
		})
	}
}

func TestActiveSettingsRoundTripThroughPathSet(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse([]byte("version: 1\n# keep me\ndefaults: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := newSettings(DefaultKeys(), theme.New(theme.Mocha), LoadStrings(), cfg, "unused", nil, nil, nil)
	setSettingsField(t, s.fields[catTmux], i18n.T("tui.settings.field.active_enabled"), func(f *settingsField) { f.on = true })
	setSettingsField(t, s.fields[catTmux], i18n.T("tui.settings.field.active_ttl"), func(f *settingsField) { f.text = "9h30m" })

	if got := applyFieldsToConfig(cfg.Node, s.fields); len(got) != 0 {
		t.Fatalf("applyFieldsToConfig errors = %v", got)
	}
	raw, err := yaml.Marshal(cfg.Node)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := config.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	active := roundTrip.Defaults.TmuxIntegration.Active
	if !active.Enabled || active.TTL != "9h30m" {
		t.Fatalf("round-trip active config = %+v", active)
	}
}

func settingsFieldByLabel(t *testing.T, fields []settingsField, label string) settingsField {
	t.Helper()
	for _, field := range fields {
		if field.label == label {
			return field
		}
	}
	t.Fatalf("settings field %q not found", label)
	return settingsField{}
}

func setSettingsField(t *testing.T, fields []settingsField, label string, update func(*settingsField)) {
	t.Helper()
	for i := range fields {
		if fields[i].label == label {
			update(&fields[i])
			return
		}
	}
	t.Fatalf("settings field %q not found", label)
}
