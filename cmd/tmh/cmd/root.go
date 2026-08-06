// Package cmd wires cobra subcommands for the tmh binary.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	errs "github.com/mark1708/tmh/internal/errors"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/ui"
	"github.com/mark1708/tmh/internal/ui/picker"
	"github.com/mark1708/tmh/internal/xdg"
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
		newActiveCmd(),
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
		if missingOK && errors.Is(err, errs.ErrConfigNotFound) {
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

func openStateForConfig(cfg *config.Config) (*state.DB, error) {
	db, err := state.Open(xdg.StateDBPath())
	if err != nil {
		if cfg != nil && cfg.Defaults.TmuxIntegration.Active.Enabled {
			return nil, fmt.Errorf("open state db: %w", err)
		}
		return nil, nil
	}
	return db, nil
}

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
	r := newRunner()
	ok, err := r.ServerRunning(context.Background())
	if err != nil || !ok {
		return launchTUI()
	}

	cfg, err := loadConfig(true)
	if err != nil {
		return err
	}
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

func attachPicked(c *cobra.Command, r tmux.Runner, res picker.Result) error {
	ctx := context.Background()

	target := res.Target
	if res.NavigateTarget != "" {
		target = res.NavigateTarget
	}

	cfg, err := loadConfig(true)
	if err != nil {
		return err
	}
	db, err := openStateForConfig(cfg)
	if err != nil {
		return err
	}
	var store actions.ActiveWindowStore
	if db != nil {
		store = db
		defer db.Close()
	}

	exists, err := r.HasSession(ctx, res.Target)
	if err != nil {
		return err
	}
	if !exists && res.Dir != "" {
		if err := r.NewSession(ctx, tmux.NewSessionOpts{
			Name: res.Target, Dir: res.Dir, Detached: true,
		}); err != nil {
			return fmt.Errorf("create %q: %w", res.Target, err)
		}
	}

	now := time.Now()
	return actions.NavigateWithActive(ctx, r, cfg, store, target, now)
}

// launchTUI runs the bubbletea dashboard. Missing config remains pass-through;
// malformed config or unavailable state with active enabled fails before model startup.
func launchTUI() error {
	cfg, err := loadConfig(true)
	if err != nil {
		return err
	}
	db, err := openStateForConfig(cfg)
	if err != nil {
		return err
	}
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
	_, err = prog.Run()
	return err
}
