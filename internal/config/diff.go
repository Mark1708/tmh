package config

// Drift captures one tracked difference between the live tmux state and the
// resolved config. Ad-hoc sessions and ad-hoc windows in ad-hoc sessions do
// not appear here.
//
// Reason holds the English human-readable description and ships in JSON for
// existing script consumers. ReasonCode is a stable dotted key (matching
// drift.reason.* in the i18n bundle) that UI callers resolve into the
// current language at render time.
type Drift struct {
	Status        DriftStatus
	Session       string
	Window        string // empty when the drift is session-level
	ConfigDir     string // resolved dir from config
	LiveDir       string // pane_current_path of first pane when available
	ConfigEntry   string // "session" or "session/window"
	Reason        string // English description; stable for JSON
	ReasonCode    string // e.g. "session_gone"; maps to drift.reason.<code> in i18n
	ConfigCommand string // expected command from config (command_differs drift only)
	LiveCommand   string // observed command from live state (command_differs drift only)
}

// Drift reason codes. Kept in sync with drift.reason.* keys in the i18n bundle.
const (
	ReasonSessionGone    = "session_gone"
	ReasonWindowGone     = "window_gone"
	ReasonDirDiffers     = "dir_differs"
	ReasonWindowNew      = "window_new"
	ReasonCommandDiffers = "command_differs"
)

// DriftStatus is one of ok/drift/new/gone.
type DriftStatus string

const (
	StatusOK    DriftStatus = "ok"
	StatusDrift DriftStatus = "drift"
	StatusNew   DriftStatus = "new"
	StatusGone  DriftStatus = "gone"
)

// LivePane is enough info per first-pane to compute drift. Callers assemble
// this from tmux.ListPanes.
type LivePane struct {
	Session string
	Window  string
	Dir     string
}

// LiveSnapshot is the merged live-tree view Diff consumes. Windows is
// ordered by tmux index.
type LiveSnapshot struct {
	Sessions []LiveSession
}

// LiveSession represents one live tmux session with its windows + first-pane
// cwd. Ad-hoc sessions (not referenced by the config) still appear here and
// Diff filters them out.
type LiveSession struct {
	Name    string
	Windows []LiveWindow
}

// LiveWindow represents one live window with first-pane cwd and command.
type LiveWindow struct {
	Name    string
	Dir     string
	Command string // foreground command of the first non-idle pane; may be empty
}

// Diff compares a resolved config view against a live snapshot and returns
// every tracked drift entry per the plan §5.
func Diff(resolved *Resolved, live LiveSnapshot) []Drift {
	if resolved == nil {
		resolved = &Resolved{}
	}

	// Index live sessions by name for quick lookup.
	liveByName := make(map[string]LiveSession, len(live.Sessions))
	for _, s := range live.Sessions {
		liveByName[s.Name] = s
	}

	// Index config sessions for quick reverse lookup.
	configByName := make(map[string]ResolvedSession, len(resolved.Sessions))
	for _, s := range resolved.Sessions {
		configByName[s.Name] = s
	}

	var out []Drift

	for _, cs := range resolved.Sessions {
		ls, liveOK := liveByName[cs.Name]
		if !liveOK {
			// session in config, not running → one "gone" per window (if any)
			if len(cs.Windows) == 0 {
				out = append(out, Drift{
					Status:      StatusGone,
					Session:     cs.Name,
					ConfigEntry: cs.Name,
					Reason:      "session in config, not running",
					ReasonCode:  ReasonSessionGone,
				})
				continue
			}
			for _, cw := range cs.Windows {
				out = append(out, Drift{
					Status:      StatusGone,
					Session:     cs.Name,
					Window:      cw.Name,
					ConfigDir:   cw.Dir,
					ConfigEntry: cs.Name + "/" + cw.Name,
					Reason:      "window in config, not running",
					ReasonCode:  ReasonWindowGone,
				})
			}
			continue
		}

		// session is live — pair windows by name
		liveByWinName := make(map[string]LiveWindow, len(ls.Windows))
		for _, lw := range ls.Windows {
			liveByWinName[lw.Name] = lw
		}
		configWinSet := make(map[string]bool, len(cs.Windows))
		for _, cw := range cs.Windows {
			configWinSet[cw.Name] = true
			lw, ok := liveByWinName[cw.Name]
			entry := cs.Name + "/" + cw.Name
			if !ok {
				out = append(out, Drift{
					Status:      StatusGone,
					Session:     cs.Name,
					Window:      cw.Name,
					ConfigDir:   cw.Dir,
					ConfigEntry: entry,
					Reason:      "window in config, not running",
					ReasonCode:  ReasonWindowGone,
				})
				continue
			}
			if cw.Dir != "" && lw.Dir != "" && cw.Dir != lw.Dir {
				out = append(out, Drift{
					Status:      StatusDrift,
					Session:     cs.Name,
					Window:      cw.Name,
					ConfigDir:   cw.Dir,
					LiveDir:     lw.Dir,
					ConfigEntry: entry,
					Reason:      "dir differs",
					ReasonCode:  ReasonDirDiffers,
				})
				continue
			}
			// Process drift: command declared in config differs from running command.
			if cw.Command != "" && lw.Command != "" && cw.Command != lw.Command {
				out = append(out, Drift{
					Status:         StatusDrift,
					Session:        cs.Name,
					Window:         cw.Name,
					ConfigDir:      cw.Dir,
					LiveDir:        lw.Dir,
					ConfigEntry:    entry,
					Reason:         "command differs: expected " + cw.Command + ", got " + lw.Command,
					ReasonCode:     ReasonCommandDiffers,
					ConfigCommand:  cw.Command,
					LiveCommand:    lw.Command,
				})
				continue
			}
			out = append(out, Drift{
				Status:      StatusOK,
				Session:     cs.Name,
				Window:      cw.Name,
				ConfigDir:   cw.Dir,
				LiveDir:     lw.Dir,
				ConfigEntry: entry,
			})
		}

		// live-only windows in a tracked session → "new"
		for _, lw := range ls.Windows {
			if configWinSet[lw.Name] {
				continue
			}
			out = append(out, Drift{
				Status:      StatusNew,
				Session:     cs.Name,
				Window:      lw.Name,
				LiveDir:     lw.Dir,
				ConfigEntry: cs.Name + "/" + lw.Name,
				Reason:      "window in live, not in config",
				ReasonCode:  ReasonWindowNew,
			})
		}
	}

	// Ad-hoc sessions (live but not in config) are ignored by the plan.
	_ = configByName
	return out
}

// HasTracked reports whether the drift set contains anything other than "ok".
func HasTracked(entries []Drift) bool {
	for _, e := range entries {
		if e.Status != StatusOK {
			return true
		}
	}
	return false
}
