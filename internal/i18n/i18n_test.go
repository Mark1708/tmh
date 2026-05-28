package i18n

import (
	"strings"
	"testing"
)

func TestInit_DefaultsToEnglish(t *testing.T) {
	if err := Init(""); err != nil {
		t.Fatal(err)
	}
	if Active() != "en" {
		t.Fatalf("Active() = %q, want en", Active())
	}
	if got := T("cli.root.short"); !strings.Contains(strings.ToLower(got), "tmux hub") {
		t.Fatalf("English translation missing: %q", got)
	}
}

func TestInit_Russian(t *testing.T) {
	if err := Init("ru"); err != nil {
		t.Fatal(err)
	}
	if Active() != "ru" {
		t.Fatalf("Active() = %q, want ru", Active())
	}
	// Use a stable Russian-only key for the smoke check: cli.root.short
	// doubles as a marketing slogan and may legitimately vary. The keymap
	// title is a short, content-stable phrase that's always localized.
	if got := T("tui.keymap.title"); got != "клавиши" {
		t.Fatalf("Russian translation missing: got %q for tui.keymap.title", got)
	}
}

func TestInit_UnsupportedFallsBackToEnglish(t *testing.T) {
	if err := Init("de"); err != nil {
		t.Fatal(err)
	}
	if Active() != "en" {
		t.Fatalf("Active() = %q for unsupported, want en fallback", Active())
	}
}

func TestT_MissingKeyReturnsKey(t *testing.T) {
	_ = Init("en")
	got := T("does.not.exist")
	if got != "does.not.exist" {
		t.Fatalf("missing key should round-trip, got %q", got)
	}
}

func TestTf_SubstitutesTemplateData(t *testing.T) {
	_ = Init("en")
	got := Tf("tui.toast.session_killed", map[string]any{"name": "atlas"})
	if got != "killed atlas" {
		t.Fatalf("Tf = %q", got)
	}
}

func TestDetectLang_ConfigWins(t *testing.T) {
	got := DetectLangWithEnv("ru", func(string) string { return "" })
	if got != "ru" {
		t.Fatalf("got %q", got)
	}
}

func TestDetectLang_EnvFallbackChain(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"TMH_LANG wins", map[string]string{"TMH_LANG": "ru", "LANG": "en_US"}, "ru"},
		{"LC_ALL", map[string]string{"LC_ALL": "ru_RU.UTF-8"}, "ru"},
		{"LANG", map[string]string{"LANG": "ru_RU"}, "ru"},
		{"unsupported → en", map[string]string{"LANG": "de_DE.UTF-8"}, "en"},
		{"nothing set", map[string]string{}, "en"},
		{"empty strings ignored", map[string]string{"TMH_LANG": "", "LC_ALL": "", "LANG": "ru_RU.UTF-8"}, "ru"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLangWithEnv("", func(k string) string { return tt.env[k] })
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseLocale(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"ru_RU.UTF-8", "ru"},
		{"en_US.UTF-8", "en"},
		{"ru-RU", "ru"},
		{"C", "c"},
		{"POSIX", "posix"},
		{"de", "de"},
	}
	for _, tt := range tests {
		if got := parseLocale(tt.in); got != tt.want {
			t.Errorf("parseLocale(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAvailable(t *testing.T) {
	got := Available()
	if len(got) < 2 {
		t.Fatalf("Available should list ≥2 langs, got %v", got)
	}
	hasEn, hasRu := false, false
	for _, c := range got {
		switch c {
		case "en":
			hasEn = true
		case "ru":
			hasRu = true
		}
	}
	if !hasEn || !hasRu {
		t.Fatalf("expected en + ru, got %v", got)
	}
}
