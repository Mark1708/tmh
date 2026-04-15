package tmux

// Session is a snapshot of one tmux session.
type Session struct {
	Name     string
	Windows  int
	Attached bool
}

// Window is a snapshot of one tmux window within a session.
type Window struct {
	Session string
	Index   int
	Name    string
	Panes   int
	Layout  string
	Active  bool
}

// Pane is a snapshot of one tmux pane within a window.
type Pane struct {
	Session string
	Window  int
	Index   int
	ID      string
	Command string
	Path    string
	Active  bool
}

// NewSessionOpts parameterises a session creation call.
type NewSessionOpts struct {
	Name       string
	Dir        string
	WindowName string
	Env        map[string]string
	Detached   bool
}

// NewWindowOpts parameterises a window creation call.
type NewWindowOpts struct {
	SessionTarget string
	Name          string
	Dir           string
	Env           map[string]string
}

// SplitOpts parameterises a split-window call.
type SplitOpts struct {
	Target     string // session:window.pane
	Horizontal bool   // true = -h (left/right), false = -v (top/bottom)
	Dir        string
	Env        map[string]string
}

// PopupOpts parameterises a display-popup call.
type PopupOpts struct {
	Width   string // percent or cells, e.g. "80%" or "120"
	Height  string
	Dir     string
	Env     map[string]string
	Command string
	Close   bool // -E: close when command exits
}
