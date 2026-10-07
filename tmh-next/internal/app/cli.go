package app

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
)

// Scheduler is the production/test seam for delayed messages. Production
// wraps tea.Tick; tests record tokens and fire them manually.
type Scheduler interface {
	// TimerCmd arms the 1s mutation-lane timer for a token.
	TimerCmd(token uint64) tea.Cmd
	// ToastCmd arms the toast expiry for a sequence number.
	ToastCmd(seq uint64) tea.Cmd
}

// ProductionScheduler converts armed tokens into real tea timers.
type ProductionScheduler struct {
	Delay time.Duration
}

// TimerCmd implements Scheduler with tea.Tick.
func (p ProductionScheduler) TimerCmd(token uint64) tea.Cmd {
	d := p.Delay
	if d <= 0 {
		d = time.Second
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return TickTimerMsg{Token: token} })
}

// ToastCmd implements Scheduler with a 2.5s toast expiry.
func (p ProductionScheduler) ToastCmd(seq uint64) tea.Cmd {
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return ToastExpiredMsg{Seq: seq} })
}

// ManualScheduler records armed tokens without running anything.
type ManualScheduler struct {
	TimerTokens []uint64
	ToastSeqs   []uint64
}

// TimerCmd records the timer token and returns nil.
func (m *ManualScheduler) TimerCmd(token uint64) tea.Cmd {
	m.TimerTokens = append(m.TimerTokens, token)
	return nil
}

// ToastCmd records the toast sequence and returns nil.
func (m *ManualScheduler) ToastCmd(seq uint64) tea.Cmd {
	m.ToastSeqs = append(m.ToastSeqs, seq)
	return nil
}

// --- CLI flags ----------------------------------------------------------------

// Startup is the validated CLI startup specification.
type Startup struct {
	Scenario    domain.Scenario
	Live        bool
	Stack       []ui.Location
	QuickSwitch bool
}

// ParseStartup validates argv (without the program name) and builds the
// startup specification. Validation errors must terminate with exit code 2
// before any Bubble Tea program is created.
func ParseStartup(args []string) (Startup, error) {
	fs := flag.NewFlagSet("tmh-next", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dashboard := fs.Bool("dashboard", false, "open the Dashboard")
	page := fs.String("page", "", "startup page route slug")
	resource := fs.String("resource", "", "target resource as <kind>:<id>")
	scenario := fs.String("scenario", "default", "fixture scenario: default|empty|degraded")
	live := fs.Bool("live", true, "run periodic mock ticks (disable for deterministic capture)")
	if err := fs.Parse(args); err != nil {
		return Startup{}, fmt.Errorf("invalid flags: %w", err)
	}
	if fs.NArg() > 0 {
		return Startup{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}

	sc, ok := domain.ParseScenario(*scenario)
	if !ok {
		return Startup{}, fmt.Errorf("unknown --scenario %q (want default, empty or degraded)", *scenario)
	}

	start := Startup{Scenario: sc, Live: *live}

	switch {
	case *dashboard && *page != "":
		return Startup{}, fmt.Errorf("--dashboard and --page are mutually exclusive")
	case *dashboard:
		start.Stack = []ui.Location{{Route: ui.RouteDashboard}}
		start.QuickSwitch = false
		return start, nil
	case *page == "":
		if *resource != "" {
			return Startup{}, fmt.Errorf("--resource requires --page")
		}
		// Picker-first: no flags opens [dashboard] with the quick switch overlay.
		start.Stack = []ui.Location{{Route: ui.RouteDashboard}}
		start.QuickSwitch = true
		return start, nil
	}

	route := ui.Route(*page)
	if !ui.ValidRoute(*page) {
		return Startup{}, fmt.Errorf("unknown --page %q", *page)
	}
	if route == ui.RouteDashboard {
		if *resource != "" {
			return Startup{}, fmt.Errorf("--resource is not accepted by page %q", route)
		}
		start.Stack = []ui.Location{{Route: ui.RouteDashboard}}
		return start, nil
	}

	loc := ui.Location{Route: route}
	if *resource != "" {
		kind, id, err := parseResource(*resource)
		if err != nil {
			return Startup{}, err
		}
		if !ui.RouteAllowsCLIResource(route) {
			return Startup{}, fmt.Errorf("--resource is not accepted by page %q", route)
		}
		if ui.RouteAcceptsPrimary(route, kind) {
			loc.Primary = domain.Ref(kind, id)
		} else if ui.RouteAcceptsContext(route, kind) {
			loc.Context = domain.Ref(kind, id)
		} else {
			return Startup{}, fmt.Errorf("resource kind %q is not valid for page %q", kind, route)
		}
	}
	// Direct startup always nests under Dashboard so Esc returns home.
	start.Stack = []ui.Location{{Route: ui.RouteDashboard}, loc}
	return start, nil
}

// parseResource parses the exact `<kind>:<id>` syntax.
func parseResource(v string) (domain.ResourceKind, string, error) {
	idx := strings.IndexByte(v, ':')
	if idx <= 0 || idx == len(v)-1 {
		return "", "", fmt.Errorf("--resource must be <kind>:<id>, got %q", v)
	}
	kind, id := domain.ResourceKind(v[:idx]), v[idx+1:]
	if !domain.ValidResourceKind(kind) {
		return "", "", fmt.Errorf("unknown resource kind %q", kind)
	}
	if strings.ContainsRune(id, ':') {
		return "", "", fmt.Errorf("--resource id must not contain ':': %q", v)
	}
	return kind, id, nil
}

// RunCLI validates arguments and either returns the startup spec or exits 2
// with a usage message.
func RunCLI(args []string) Startup {
	start, err := ParseStartup(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmh-next: %v\n\nusage: tmh-next [--dashboard] [--page <route>] [--resource <kind>:<id>] [--scenario default|empty|degraded] [--live=true|false]\n", err)
		os.Exit(2)
	}
	return start
}
