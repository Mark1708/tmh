// Package cmd wires cobra subcommands for the tmh binary.
package cmd

import (
	"context"
	"fmt"

	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/tmux"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

// RootFlags holds global flags accessible to any subcommand.
type RootFlags struct {
	ConfigPath string
	Profile    string
}

var flags RootFlags

// NewRoot builds the full cobra command tree.
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "tmh",
		Short:         "tmux hub — declarative sessions, reload, drift sync",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(c *cobra.Command, args []string) error {
			// Default action: will launch TUI dashboard once implemented.
			return fmt.Errorf("tui not implemented yet (use `tmh --help` for available commands)")
		},
	}

	root.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", "path to config.yml (overrides TMH_CONFIG and defaults)")
	root.PersistentFlags().StringVar(&flags.Profile, "profile", "", "profile name from config.yml")

	root.AddCommand(
		newAttachCmd(),
		newNewCmd(),
		newInitCmd(),
		newKillCmd(),
		newLsCmd(),
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

// loadConfig loads and validates the config. Returns nil if the file is
// missing and missingOK is true (pass-through mode).
func loadConfig(missingOK bool) (*config.Config, error) {
	path := resolveConfigPath()
	c, err := config.Load(path)
	if err != nil {
		if missingOK {
			return &config.Config{Version: 1}, nil
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
