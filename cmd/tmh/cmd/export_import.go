package cmd

import (
	"fmt"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var (
		minimal bool
		only    string
	)
	c := &cobra.Command{
		Use:   "export",
		Short: "Export config to stdout",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := loadConfig(false)
			if err != nil {
				return err
			}
			out, err := actions.Export(cfg, actions.ExportOptions{Minimal: minimal, Only: only})
			if err != nil {
				return err
			}
			fmt.Fprint(c.OutOrStdout(), string(out))
			return nil
		},
	}
	c.Flags().BoolVar(&minimal, "minimal", false, "redact secrets and rewrite absolute paths via roots")
	c.Flags().StringVar(&only, "only", "", "export only this session (implies --minimal=false unless set)")
	return c
}

func newImportCmd() *cobra.Command {
	var (
		merge, replace bool
	)
	c := &cobra.Command{
		Use:   "import <path>",
		Short: "Import a config snippet (--merge or --replace)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if merge == replace {
				return cmdErr("specify exactly one of --merge or --replace")
			}
			mode := actions.ImportMerge
			if replace {
				mode = actions.ImportReplace
			}
			dst, _ := loadConfig(true)
			merged, err := actions.ImportFile(dst, args[0], mode)
			if err != nil {
				return err
			}
			path := resolveConfigPath()
			if err := config.Write(merged, path, config.WriteOptions{
				BackupDir:      xdg.BackupsDir(),
				MaxBackups:     20,
				PreserveBlanks: true,
			}); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "imported into", path)
			return nil
		},
	}
	c.Flags().BoolVar(&merge, "merge", false, "merge entries (overlay wins on conflicts)")
	c.Flags().BoolVar(&replace, "replace", false, "replace entire config")
	return c
}
