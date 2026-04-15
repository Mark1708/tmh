package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/config"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	var (
		name   string
		dir    string
		layout string
		group  string
		save   bool
		attach bool
	)
	c := &cobra.Command{
		Use:   "new",
		Short: "Create a session (interactive wizard if flags are omitted)",
		RunE: func(c *cobra.Command, args []string) error {
			// When called without --name/--dir, run the interactive wizard.
			if name == "" || dir == "" {
				cwd, _ := os.Getwd()
				defaults := wizardDefaults{Name: filepath.Base(cwd), Dir: cwd}
				res, err := runNewWizard(defaults)
				if err != nil {
					return err
				}
				if res == nil {
					return nil // user cancelled
				}
				name, dir, layout, group, save, attach = res.Name, res.Dir, res.Layout, res.Group, res.Save, res.Attach
			}
			if layout == "" {
				layout = "3-pane"
			}
			r := newRunner()
			sess := config.ResolvedSession{
				Name: name, Dir: dir,
				Windows: []config.ResolvedWindow{{Name: name, Dir: dir, Layout: layout}},
			}
			if err := actions.CreateSession(context.Background(), r, sess, nil); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "created:", name)

			if save {
				if err := saveNewToConfig(name, dir, layout, group); err != nil {
					return fmt.Errorf("save to config: %w", err)
				}
				fmt.Fprintln(c.OutOrStdout(), "saved to", resolveConfigPath())
			}
			if attach {
				return actions.Attach(context.Background(), r, name)
			}
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "session name")
	c.Flags().StringVar(&dir, "dir", "", "working directory")
	c.Flags().StringVar(&layout, "layout", "", "1-pane | 2-pane | 3-pane (default 3-pane)")
	c.Flags().StringVar(&group, "group", "", "group tag (only used with --save)")
	c.Flags().BoolVar(&save, "save", false, "also write this session to config.yml")
	c.Flags().BoolVar(&attach, "attach", false, "attach after creating")
	return c
}

// wizardDefaults pre-fills the form so common cases need only Enter presses.
type wizardDefaults struct {
	Name string
	Dir  string
}

// wizardResult collects the user's choices; nil means "cancelled".
type wizardResult struct {
	Name   string
	Dir    string
	Layout string
	Group  string
	Save   bool
	Attach bool
}

func runNewWizard(defaults wizardDefaults) (*wizardResult, error) {
	out := wizardResult{
		Name:   defaults.Name,
		Dir:    defaults.Dir,
		Layout: "3-pane",
		Attach: true,
	}
	groupOptions, _ := knownGroups()
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("имя сессии").
				Description("уникальное имя, без пробелов и слэшей").
				Value(&out.Name).
				Validate(validateSessionName),
			huh.NewInput().
				Title("рабочий каталог").
				Description("абсолютный путь; ~/ автоматически раскроется").
				Value(&out.Dir).
				Validate(validateDir),
			huh.NewSelect[string]().
				Title("layout").
				Options(
					huh.NewOption("3-pane  (main + 2 side)", "3-pane"),
					huh.NewOption("2-pane  (half / half)", "2-pane"),
					huh.NewOption("1-pane  (single pane)", "1-pane"),
				).
				Value(&out.Layout),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("group").
				Description("тег для tmh init --group; (none) — не группировать").
				Options(groupOrEmpty(groupOptions)...).
				Value(&out.Group),
			huh.NewConfirm().
				Title("сохранить в config.yml?").
				Affirmative("yes").
				Negative("no").
				Value(&out.Save),
			huh.NewConfirm().
				Title("attach после создания?").
				Affirmative("yes").
				Negative("no").
				Value(&out.Attach),
		),
	)
	if err := form.Run(); err != nil {
		if err.Error() == "user aborted" {
			return nil, nil
		}
		return nil, err
	}
	// expand ~/ in dir before returning
	if strings.HasPrefix(out.Dir, "~/") {
		out.Dir = filepath.Join(os.Getenv("HOME"), out.Dir[2:])
	}
	return &out, nil
}

func validateSessionName(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("required")
	}
	if strings.ContainsAny(s, " /\t") {
		return fmt.Errorf("no spaces or slashes")
	}
	return nil
}

func validateDir(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("required")
	}
	if strings.HasPrefix(s, "~/") {
		s = filepath.Join(os.Getenv("HOME"), s[2:])
	}
	info, err := os.Stat(s)
	if err != nil {
		return fmt.Errorf("not found: %s", s)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", s)
	}
	return nil
}

// knownGroups scans the current config for declared group tags to offer as
// picker options. Returns empty on any load/parse error.
func knownGroups() ([]string, error) {
	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, s := range cfg.Sessions {
		for _, g := range s.Group {
			seen[g] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for g := range seen {
		out = append(out, g)
	}
	return out, nil
}

func groupOrEmpty(groups []string) []huh.Option[string] {
	opts := []huh.Option[string]{huh.NewOption("(none)", "")}
	for _, g := range groups {
		opts = append(opts, huh.NewOption(g, g))
	}
	return opts
}

// saveNewToConfig persists the new session into config.yml using the
// canonical shorthand (bare string = dir) when layout is default.
func saveNewToConfig(name, dir, layout, group string) error {
	path := resolveConfigPath()
	cfg, err := config.Load(path)
	if err != nil {
		cfg, err = config.Parse([]byte("version: 1\n"))
		if err != nil {
			return err
		}
	}
	base := fmt.Sprintf("sessions.%s", name)
	if group != "" {
		if err := config.PathSet(cfg.Node, base+".group", group); err != nil {
			return err
		}
	}
	winBase := base + ".windows." + name
	if layout != "" && layout != "3-pane" {
		if err := config.PathSet(cfg.Node, winBase+".layout", layout); err != nil {
			return err
		}
		if err := config.PathSet(cfg.Node, winBase+".dir", dir); err != nil {
			return err
		}
	} else {
		if err := config.PathSet(cfg.Node, winBase, dir); err != nil {
			return err
		}
	}
	return config.Write(cfg, path, config.WriteOptions{
		BackupDir:      xdg.BackupsDir(),
		MaxBackups:     20,
		PreserveBlanks: true,
	})
}
