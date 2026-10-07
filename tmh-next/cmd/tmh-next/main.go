// Command tmh-next runs the terminal-native tmh-next demo cockpit on the
// pinned Charm v2 stack. Everything is mock: actions mutate an in-memory
// catalog only, and every mutating surface carries a MOCK badge.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/app"
	"github.com/mark1708/tmh-next/internal/mock"
	"github.com/mark1708/tmh-next/internal/ui/pages"
)

func main() {
	start := app.RunCLI(os.Args[1:])

	client := mock.NewClient(start.Scenario)
	root := app.NewRoot(app.Options{
		Client:      client,
		DemoTicker:  client,
		Scheduler:   app.ProductionScheduler{Delay: 0},
		Live:        start.Live,
		Stack:       start.Stack,
		QuickSwitch: start.QuickSwitch,
		Factory:     pages.Factory,
	})

	if _, err := tea.NewProgram(root).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tmh-next:", err)
		os.Exit(1)
	}
}
