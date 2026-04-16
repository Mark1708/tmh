// Package cmd wires cobra subcommands for the tmh binary.
package cmd

import (
	"context"
	"os"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/ui"
	"git.mark1708.ru/me/tmh/internal/xdg"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

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
			return launchTUI()
		},
	}

	root.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", i18n.T("cli.flag.config"))
	root.PersistentFlags().StringVar(&flags.Profile, "profile", "", i18n.T("cli.flag.profile"))
	root.PersistentFlags().StringVar(&flags.Lang, "lang", "", i18n.T("cli.flag.lang"))

	root.AddCommand(
		newAttachCmd(),
		newNewCmd(),
		newInitCmd(),
		newKillCmd(),
		newLsCmd(),
		newPsCmd(),
		newSyncCmd(),
		newDiffCmd(),
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
