package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"git.mark1708.ru/me/tmh/internal/actions"
	"git.mark1708.ru/me/tmh/internal/i18n"
	"git.mark1708.ru/me/tmh/internal/state"
	"git.mark1708.ru/me/tmh/internal/xdg"

	"github.com/spf13/cobra"
)

func newWatchCmd() *cobra.Command {
	var auto bool
	c := &cobra.Command{
		Use:   "watch",
		Short: i18n.T("cli.watch.short"),
		RunE: func(c *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			events := make(chan actions.WatchEvent, 8)
			go func() {
				if err := actions.Watch(ctx, actions.WatchPaths(resolveConfigPath()), events, c.ErrOrStderr()); err != nil {
					fmt.Fprintln(c.ErrOrStderr(), "watch:", err)
				}
			}()

			db, _ := state.Open(xdg.StateDBPath())
			if db != nil {
				defer db.Close()
			}

			rcFile := filepath.Join(os.Getenv("HOME"), ".zshrc")

			// Optional: drain the reload queue periodically.
			drainT := time.NewTicker(2 * time.Second)
			defer drainT.Stop()

			fmt.Fprintln(c.OutOrStdout(), "watching:", actions.WatchPaths(resolveConfigPath()))
			for {
				select {
				case <-ctx.Done():
					return nil
				case ev := <-events:
					fmt.Fprintf(c.OutOrStdout(), "%s: %s changed\n", time.Now().Format("15:04:05"), ev.Kind)
					if !auto {
						continue
					}
					r := newRunner()
					switch ev.Kind {
					case "zshrc":
						_, _ = actions.Reload(context.Background(), r, db, rcFile,
							actions.ReloadOptions{Shell: true, Busy: true, RcFile: rcFile})
					case "tmuxconf":
						_, _ = actions.Reload(context.Background(), r, nil, "",
							actions.ReloadOptions{Tmux: true, TmuxConf: ev.Path})
					}
				case <-drainT.C:
					if db == nil {
						continue
					}
					_, _ = actions.DrainReloadQueue(context.Background(), newRunner(), db, rcFile)
				}
			}
		},
	}
	c.Flags().BoolVar(&auto, "auto", false, i18n.T("cli.flag.watch.auto"))
	return c
}
