package cmd

import (
	"fmt"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/config"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var (
		minimal bool
		only    string
	)
	c := &cobra.Command{
		Use:   "export",
		Short: i18n.T("cli.export.short"),
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
	c.Flags().BoolVar(&minimal, "minimal", false, i18n.T("cli.flag.export.minimal"))
	c.Flags().StringVar(&only, "only", "", i18n.T("cli.flag.export.only"))
	return c
}

func newImportCmd() *cobra.Command {
	var (
		merge, replace bool
	)
	c := &cobra.Command{
		Use:   "import <path>",
		Short: i18n.T("cli.import.short"),
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
			fmt.Fprintln(c.OutOrStdout(), i18n.Tf("cli.print.imported_into", map[string]any{"path": path}))
			return nil
		},
	}
	c.Flags().BoolVar(&merge, "merge", false, i18n.T("cli.flag.import.merge"))
	c.Flags().BoolVar(&replace, "replace", false, i18n.T("cli.flag.import.replace"))
	return c
}
