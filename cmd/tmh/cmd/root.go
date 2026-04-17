// Package cmd wires cobra subcommands for the tmh binary.
package cmd

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"

	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/ui"
	"github.com/mark1708/tmh/internal/ui/picker"
	"github.com/mark1708/tmh/internal/xdg"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// dashboardFlag is set by the top-level --dashboard flag to force the
// full TUI regardless of TTY/tmux conditions that would otherwise route
// a bare `tmh` invocation through the quick picker (A3).
var dashboardFlag bool

// RootFlags holds global flags accessible to any subcommand.
type RootFlags struct {
	ConfigPath string
	Profile    string
	Lang       string
}

var flags RootFlags

// NewRoot builds the full cobra command tree.
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "tmh",
		Short:         i18n.T("cli.root.short"),
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
		// --lang applies only to runtime output (prints, errors, TUI); cobra
		// help text was already bound at NewRoot time. See main.initLang for
		// the startup-time resolution chain.
		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			if flags.Lang != "" {
				_ = i18n.Init(flags.Lang)
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			return bareTmh(c)
		},
	}

	root.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", i18n.T("cli.flag.config"))
	root.PersistentFlags().StringVar(&flags.Profile, "profile", "", i18n.T("cli.flag.profile"))
	root.PersistentFlags().StringVar(&flags.Lang, "lang", "", i18n.T("cli.flag.lang"))
	root.Flags().BoolVar(&dashboardFlag, "dashboard", false, i18n.T("cli.flag.dashboard"))

	root.AddCommand(
		newAttachCmd(),
		newNewCmd(),
		newInitCmd(),
		newKillCmd(),
		newLsCmd(),
		newPsCmd(),
		newSyncCmd(),
		newDiffCmd(),
		newFreezeCmd(),
		newReloadCmd(),
		newWatchCmd(),
		newStatusCmd(),
		newScratchCmd(),
		newPopupCmd(),
		newWindowCmd(),
		newLayoutCmd(),
		newSnapshotCmd(),
		newUndoCmd(),
		newExportCmd(),
		newImportCmd(),
		newTmuxCmd(),
		newVersionCmd(version),
		newDoctorCmd(),
	)
	return root
}

// resolveConfigPath returns the effective config path taking --config into account.
func resolveConfigPath() string {
	if flags.ConfigPath != "" {
		return flags.ConfigPath
	}
	return xdg.ConfigPath()
}

// loadConfig loads and validates the config. Returns an empty parseable
// config if the file is missing and missingOK is true (pass-through mode).
func loadConfig(missingOK bool) (*config.Config, error) {
	path := resolveConfigPath()
	c, err := config.Load(path)
	if err != nil {
		if missingOK {
			return config.Parse([]byte("version: 1\n"))
		}
		return nil, err
	}
	if err := config.Validate(c); err != nil {
		return nil, err
	}
	return c, nil
}

// newRunner returns the production Runner.
func newRunner() tmux.Runner { return tmux.NewCLIRunner() }

// ctxFromCmd returns the cobra command's context (unused placeholder today).
func ctxFromCmd(c *cobra.Command) context.Context { return c.Context() }

// bareTmh is the default RunE for the top-level `tmh` command. With
// --dashboard it always launches the full TUI. Otherwise it runs the
// quick picker when both stdin and stdout are TTYs and a tmux server is
// reachable; falls through to the dashboard if the picker reports
// itself unusable (empty state, explicit fall-through key, error).
func bareTmh(c *cobra.Command) error {
	if dashboardFlag {
		return launchTUI()
	}
	if !isatty.IsTerminal(os.Stdout.Fd()) || !isatty.IsTerminal(os.Stdin.Fd()) {
		return launchTUI()
	}
	// If tmux isn't running we can't attach to anything anyway; let the
	// dashboard walk the user through init/bootstrap.
	r := newRunner()
	ok, err := r.ServerRunning(context.Background())
	if err != nil || !ok {
		return launchTUI()
	}

	cfg, _ := loadConfig(true)
	res, err := picker.Run(context.Background(), r, cfg, flags.Profile)
	if err != nil {
		return fmt.Errorf("picker: %w", err)
	}
	if res.FallThroughToDashboard {
		return launchTUI()
	}
	if res.IsEmpty() {
		return nil
	}
	return attachPicked(c, r, res)
}

// attachPicked hands the controlling TTY over to tmux for the target
// chosen in the picker. The picker can also return a brand-new
// discovered-directory path; in that case we create the session first.
func attachPicked(c *cobra.Command, r tmux.Runner, res picker.Result) error {
	ctx := context.Background()
	exists, err := r.HasSession(ctx, res.Target)
	if err != nil {
		return err
	}
	if !exists && res.Dir != "" {
		// Discovered candidate — materialise it before attaching.
		if err := r.NewSession(ctx, tmux.NewSessionOpts{
			Name: res.Target, Dir: res.Dir, Detached: true,
		}); err != nil {
			return fmt.Errorf("create %q: %w", res.Target, err)
		}
	}
	if inside := os.Getenv("TMUX") != ""; inside {
		return r.SwitchClient(ctx, res.Target)
	}
	// Outside tmux: hand the TTY over to `tmux attach-session`.
	cmd := osexec.Command("tmux", "attach-session", "-t", res.Target)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// launchTUI runs the bubbletea dashboard. Reads config lazily via deps so a
// missing or invalid config still lets the user reach the empty/error state
// inside the TUI rather than failing at startup.
func launchTUI() error {
	db, _ := state.Open(xdg.StateDBPath())
	if db != nil {
		defer db.Close()
	}
	deps := ui.Deps{
		Runner:     newRunner(),
		State:      db,
		ConfigPath: resolveConfigPath(),
		Profile:    flags.Profile,
		LoadConfig: func() (*config.Config, error) { return loadConfig(true) },
	}
	model := ui.New(deps)
	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithOutput(os.Stderr))
	_, err := prog.Run()
	return err
}
