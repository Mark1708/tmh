// Package i18n is the localization layer for tmh. It wraps go-i18n v2 with a
// package-level localizer so callers don't thread the language through every
// API.
//
// Usage:
//
//	i18n.Init(i18n.DetectLang(cfg.Defaults.Lang))  // once at startup
//	fmt.Println(i18n.T("cli.root.short"))          // anywhere else
//	fmt.Println(i18n.Tf("toast.session_killed", map[string]any{"name": "atlas"}))
//
// Default language is English. Russian is the only other supported locale;
// any unsupported language tag (e.g. `de_DE`) falls back to English silently
// so missing translations never surface as raw keys to the user.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localesFS embed.FS

// DefaultLang is the language used when no other source resolves to something
// supported.
const DefaultLang = "en"

// supported lists languages for which we ship a locales/*.json bundle.
// Order is insignificant (DefaultLang is always the tie-breaker).
var supported = []string{"en", "ru"}

var (
	mu         sync.RWMutex
	bundle     *goi18n.Bundle
	localizer  *goi18n.Localizer
	activeLang = DefaultLang
)

// Available returns the list of supported language codes in stable order.
func Available() []string {
	out := make([]string, len(supported))
	copy(out, supported)
	return out
}

// Active returns the language code currently in effect.
func Active() string {
	mu.RLock()
	defer mu.RUnlock()
	return activeLang
}

// Init loads bundled translations and installs a package-level localizer for
// the requested language. Passing an unsupported language silently falls
// back to DefaultLang. Init is safe to call multiple times (e.g. on
// in-process language switches from the TUI settings screen).
func Init(lang string) error {
	mu.Lock()
	defer mu.Unlock()

	if !isSupported(lang) {
		lang = DefaultLang
	}

	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("json", json.Unmarshal)

	for _, code := range supported {
		data, err := localesFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			return fmt.Errorf("i18n: load %s: %w", code, err)
		}
		if _, err := b.ParseMessageFileBytes(data, code+".json"); err != nil {
			return fmt.Errorf("i18n: parse %s: %w", code, err)
		}
	}

	bundle = b
	// Localizer falls back through its argument chain, so we pass the
	// requested lang first and English second. Missing keys resolve to the
	// key itself (see T below).
	localizer = goi18n.NewLocalizer(bundle, lang, DefaultLang)
	activeLang = lang
	return nil
}

// T returns the translated message for key, or the key itself if the bundle
// has not been initialised or the key is missing.
func T(key string) string {
	mu.RLock()
	loc := localizer
	mu.RUnlock()
	if loc == nil {
		return key
	}
	msg, err := loc.Localize(&goi18n.LocalizeConfig{MessageID: key})
	if err != nil {
		return key
	}
	return msg
}

// Tf is T with template-data substitution. Placeholders in the message use
// go text/template syntax: {{.name}}. Missing data keys render empty.
func Tf(key string, data map[string]any) string {
	mu.RLock()
	loc := localizer
	mu.RUnlock()
	if loc == nil {
		return key
	}
	msg, err := loc.Localize(&goi18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: data,
	})
	if err != nil {
		return key
	}
	return msg
}

// DetectLang resolves the effective language from (in priority order):
//   - explicit lang arg (e.g. --lang flag)
//   - configLang (defaults.lang from config.yml)
//   - TMH_LANG env
//   - LC_MESSAGES / LC_ALL / LANG env
//   - DefaultLang
//
// Any unsupported language prefix falls through to DefaultLang rather than
// surfacing raw keys.
func DetectLang(configLang string) string {
	return DetectLangWithEnv(configLang, os.Getenv)
}

// DetectLangWithEnv mirrors DetectLang but lets tests inject a fake getenv.
func DetectLangWithEnv(configLang string, getenv func(string) string) string {
	// configLang wins over env.
	if lang := normalise(configLang); isSupported(lang) {
		return lang
	}
	for _, v := range []string{"TMH_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if raw := getenv(v); raw != "" {
			if lang := parseLocale(raw); isSupported(lang) {
				return lang
			}
		}
	}
	return DefaultLang
}

// parseLocale extracts the base language tag from POSIX locales like
// "ru_RU.UTF-8" or BCP-47 strings like "en-US".
func parseLocale(locale string) string {
	locale = strings.Split(locale, ".")[0] // drop ".UTF-8"
	locale = strings.ReplaceAll(locale, "-", "_")
	locale = strings.Split(locale, "_")[0] // drop region
	return normalise(locale)
}

func normalise(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func isSupported(code string) bool {
	if code == "" {
		return false
	}
	for _, s := range supported {
		if s == code {
			return true
		}
	}
	return false
}
