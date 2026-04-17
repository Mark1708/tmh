package actions

import (
	"context"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/tmux"
)

// PopupOpts mirrors the relevant subset of tmux.PopupOpts but lets the
// caller request env/cwd inheritance from the resolved config.
type PopupOpts struct {
	Command       string
	Width, Height string
	NoEnv         bool
	NoCwd         bool
}

// Popup runs a command in a tmux popup. If a tracked session/window name is
// provided and config is non-nil, env and cwd are inherited from the
// resolved config entry.
func Popup(ctx context.Context, r tmux.Runner, cfg *config.Config, profile, sessionName, windowName string, opts PopupOpts) error {
	width, height := opts.Width, opts.Height
	dir := ""
	env := map[string]string(nil)

	if cfg != nil {
		// pick popup defaults from the config
		if width == "" {
			width = cfg.Defaults.Popup.Width
		}
		if height == "" {
			height = cfg.Defaults.Popup.Height
		}
		if !opts.NoEnv || !opts.NoCwd {
			if resolved, err := config.Resolve(cfg, profile); err == nil {
				for _, s := range resolved.Sessions {
					if s.Name != sessionName {
						continue
					}
					if !opts.NoEnv {
						env = s.Env
					}
					if !opts.NoCwd {
						for _, w := range s.Windows {
							if w.Name == windowName {
								dir = w.Dir
								break
							}
						}
						if dir == "" {
							dir = s.Dir
						}
					}
				}
			}
		}
	}
	if width == "" {
		width = "80%"
	}
	if height == "" {
		height = "60%"
	}

	return r.DisplayPopup(ctx, tmux.PopupOpts{
		Width:   width,
		Height:  height,
		Dir:     dir,
		Env:     env,
		Command: opts.Command,
		Close:   true,
	})
}
