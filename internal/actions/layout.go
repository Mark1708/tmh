package actions

import (
	"context"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/tmux"
)

// LayoutSave reads the current window's layout hash from tmux and writes it
// to config.layouts[name]. Returns the captured hash.
func LayoutSave(ctx context.Context, r tmux.Runner, cfg *config.Config, sessionName string, windowIndex int, name, description string) (string, error) {
	wins, err := r.ListWindows(ctx, sessionName)
	if err != nil {
		return "", err
	}
	var hash string
	for _, w := range wins {
		if w.Index == windowIndex {
			hash = w.Layout
			break
		}
	}
	if hash == "" {
		return "", fmt.Errorf("layout: window %s:%d not found or has no layout", sessionName, windowIndex)
	}
	if cfg == nil {
		return hash, nil
	}
	base := "layouts." + name
	if err := config.PathSet(cfg.Node, base+".hash", hash); err != nil {
		return hash, err
	}
	if description != "" {
		if err := config.PathSet(cfg.Node, base+".description", description); err != nil {
			return hash, err
		}
	}
	return hash, nil
}
