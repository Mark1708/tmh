package main

import (
	"fmt"
	"os"

	"git.mark1708.ru/me/tmh/cmd/tmh/cmd"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/ui/errrender"
	"git.mark1708.ru/me/tmh/internal/xdg"
)

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	initLang()
	root := cmd.NewRoot(Version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tmh:", errrender.Render(err))
		os.Exit(1)
	}
}

// initLang resolves the UI language and installs the i18n localizer before
// cobra reads any command descriptions. We consult config.yml (if present
// and valid) for defaults.lang; env vars and CLI flags layer on top via
// DetectLang. A failed load or invalid config falls through to English —
// we never surface i18n init errors to the user at startup.
func initLang() {
	var configLang string
	if cfg, err := config.Load(xdg.ConfigPath()); err == nil {
		configLang = cfg.Defaults.Lang
	}
	_ = i18n.Init(i18n.DetectLang(configLang))
}
