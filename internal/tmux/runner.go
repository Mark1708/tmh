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
}
