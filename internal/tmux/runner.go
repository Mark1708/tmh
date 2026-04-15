package tmux

import "context"

// Runner is the abstraction over the tmux CLI used by every side-effect
// action. Production code uses CLIRunner; tests use tmuxtest.MockRunner.
type Runner interface {
	// server lifecycle
	ServerRunning(ctx context.Context) (bool, error)
	StartServer(ctx context.Context) error
	InTmux() bool

	// sessions
	ListSessions(ctx context.Context) ([]Session, error)
	HasSession(ctx context.Context, name string) (bool, error)
	NewSession(ctx context.Context, opts NewSessionOpts) error
	AttachSession(ctx context.Context, name string) error
	SwitchClient(ctx context.Context, target string) error
	KillSession(ctx context.Context, name string) error
	RenameSession(ctx context.Context, from, to string) error

	// windows
	ListWindows(ctx context.Context, session string) ([]Window, error)
	NewWindow(ctx context.Context, opts NewWindowOpts) (Window, error)
	KillWindow(ctx context.Context, target string) error
	RenameWindow(ctx context.Context, target, name string) error
	SelectWindow(ctx context.Context, target string) error

	// panes
	ListPanes(ctx context.Context, target string) ([]Pane, error)
	SplitWindow(ctx context.Context, opts SplitOpts) error
	SelectLayout(ctx context.Context, target, layout string) error
	CapturePane(ctx context.Context, target string, lines int) ([]byte, error)
	SendKeys(ctx context.Context, target string, keys ...string) error
	KillPane(ctx context.Context, target string) error
	SetAutomaticRename(ctx context.Context, target string, on bool) error

	// misc
	SourceFile(ctx context.Context, path string) error
	DisplayPopup(ctx context.Context, opts PopupOpts) error

	// server-level introspection (used by tmh tmux audit and settings UI).
	// ShowOption returns the raw value for a global option, empty string
	// when the option is at its compiled default.
	ShowOption(ctx context.Context, name string) (string, error)
	// SetOption sets a global option (equivalent of `tmux set -g NAME VALUE`
	// or `tmux setw -g ...` when window is true).
	SetOption(ctx context.Context, name, value string, window bool) error
	// ShowHook returns the command bound to a hook (`after-new-window` etc.)
	// or empty string when nothing is set.
	ShowHook(ctx context.Context, name string) (string, error)
	// UnsetHook removes a global hook binding.
	UnsetHook(ctx context.Context, name string) error
}
