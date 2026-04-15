package errrender

import (
	"fmt"
	"strings"
	"testing"

	errs "git.mark1708.ru/me/tmh/internal/errors"
	"git.mark1708.ru/me/tmh/internal/i18n"
)

func TestRender_SentinelsLocalized(t *testing.T) {
	if err := i18n.Init("ru"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		err        error
		wantSubstr string
	}{
		{"session exists", errs.ErrSessionExists, "существует"},
		{"session not found", errs.ErrSessionNotFound, "не найдена"},
		{"server not running", errs.ErrServerNotRunning, "не запущен"},
		{"hook denied", errs.ErrHookDenied, "hooks"},
		{"wrapped config invalid", fmt.Errorf("load: %w", errs.ErrConfigInvalid), "некорректен"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.err)
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tt.wantSubstr)) {
				t.Fatalf("Render(%v) = %q, want substring %q", tt.err, got, tt.wantSubstr)
			}
		})
	}
}

func TestRender_UnknownErrorReturnsRawMessage(t *testing.T) {
	_ = i18n.Init("en")
	err := fmt.Errorf("disk: no space left on device")
	if got := Render(err); got != err.Error() {
		t.Fatalf("Render = %q, want raw %q", got, err.Error())
	}
}

func TestRender_ConfigNotFoundExtractsPath(t *testing.T) {
	_ = i18n.Init("en")
	err := fmt.Errorf("%w: /etc/tmh/config.yml", errs.ErrConfigNotFound)
	got := Render(err)
	if !strings.Contains(got, "/etc/tmh/config.yml") {
		t.Fatalf("Render = %q, want path in message", got)
	}
}

func TestRender_Nil(t *testing.T) {
	if got := Render(nil); got != "" {
		t.Fatalf("Render(nil) = %q, want empty", got)
	}
}
