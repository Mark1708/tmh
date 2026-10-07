// Package ui defines the shared TUI contracts: routes, locations, input
// modes, the Page interface and page Commands. It has no dependency on the
// app or mock packages, so pages and the root shell can share it freely.
package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// Route identifies one full-screen page of the cockpit.
type Route string

const (
	RouteDashboard   Route = "dashboard"
	RouteWorkspaces  Route = "workspaces"
	RouteWorkspace   Route = "workspace"
	RouteActive      Route = "active"
	RouteTerminals   Route = "terminals"
	RouteAgents      Route = "agents"
	RouteHistory     Route = "history"
	RouteSearch      Route = "search"
	RouteSnapshots   Route = "snapshots"
	RouteReconcile   Route = "reconcile"
	RouteBackends    Route = "backends"
	RouteMachines    Route = "machines"
	RoutePerformance Route = "performance"
	RouteEvents      Route = "events"
	RouteConfig      Route = "config"
)

// AllRoutes returns every route in canonical navigation order.
func AllRoutes() []Route {
	return []Route{
		RouteDashboard, RouteWorkspaces, RouteWorkspace, RouteActive, RouteTerminals,
		RouteAgents, RouteHistory, RouteSearch, RouteSnapshots, RouteReconcile,
		RouteBackends, RouteMachines, RoutePerformance, RouteEvents, RouteConfig,
	}
}

// RouteTitle returns the human title of a route.
func RouteTitle(r Route) string {
	switch r {
	case RouteDashboard:
		return "Dashboard"
	case RouteWorkspaces:
		return "Workspaces"
	case RouteWorkspace:
		return "Workspace"
	case RouteActive:
		return "Active Views"
	case RouteTerminals:
		return "Terminals"
	case RouteAgents:
		return "Agents"
	case RouteHistory:
		return "History"
	case RouteSearch:
		return "Search"
	case RouteSnapshots:
		return "Snapshots"
	case RouteReconcile:
		return "Reconcile"
	case RouteBackends:
		return "Backends"
	case RouteMachines:
		return "Machines"
	case RoutePerformance:
		return "Performance"
	case RouteEvents:
		return "Events"
	case RouteConfig:
		return "Settings"
	default:
		return string(r)
	}
}

// routePolicy captures the route contract table: allowed primary/context
// resource kinds and whether CLI may pass an explicit --resource.
type routePolicy struct {
	PrimaryKinds      []domain.ResourceKind
	ContextKinds      []domain.ResourceKind
	AllowsCLIResource bool
}

var routePolicies = map[Route]routePolicy{
	RouteDashboard:   {PrimaryKinds: nil, ContextKinds: nil, AllowsCLIResource: false},
	RouteWorkspaces:  {PrimaryKinds: nil, ContextKinds: []domain.ResourceKind{domain.KindMachine}},
	RouteWorkspace:   {PrimaryKinds: []domain.ResourceKind{domain.KindWorkspace}, AllowsCLIResource: true},
	RouteActive:      {PrimaryKinds: []domain.ResourceKind{domain.KindActive}, AllowsCLIResource: true},
	RouteTerminals:   {PrimaryKinds: []domain.ResourceKind{domain.KindTerminal}, AllowsCLIResource: true},
	RouteAgents:      {PrimaryKinds: []domain.ResourceKind{domain.KindAgent}, AllowsCLIResource: true},
	RouteHistory:     {ContextKinds: []domain.ResourceKind{domain.KindTerminal, domain.KindAgent}, AllowsCLIResource: true},
	RouteSearch:      {AllowsCLIResource: false},
	RouteSnapshots:   {PrimaryKinds: []domain.ResourceKind{domain.KindSnapshot}, AllowsCLIResource: true},
	RouteReconcile:   {PrimaryKinds: []domain.ResourceKind{domain.KindPlan}, AllowsCLIResource: true},
	RouteBackends:    {PrimaryKinds: []domain.ResourceKind{domain.KindBackend}, AllowsCLIResource: true},
	RouteMachines:    {PrimaryKinds: []domain.ResourceKind{domain.KindMachine}, AllowsCLIResource: true},
	RoutePerformance: {AllowsCLIResource: false},
	RouteEvents:      {PrimaryKinds: []domain.ResourceKind{domain.KindEvent}, AllowsCLIResource: true},
	RouteConfig:      {AllowsCLIResource: false},
}

// RouteAcceptsPrimary reports whether route allows Primary of the kind.
func RouteAcceptsPrimary(r Route, k domain.ResourceKind) bool {
	for _, allowed := range routePolicies[r].PrimaryKinds {
		if allowed == k {
			return true
		}
	}
	return false
}

// RouteAcceptsContext reports whether route allows Context of the kind.
func RouteAcceptsContext(r Route, k domain.ResourceKind) bool {
	for _, allowed := range routePolicies[r].ContextKinds {
		if allowed == k {
			return true
		}
	}
	return false
}

// RouteAllowsCLIResource reports whether --resource may target the route:
// any route that accepts a primary or context resource kind accepts it.
func RouteAllowsCLIResource(r Route) bool {
	p := routePolicies[r]
	return len(p.PrimaryKinds) > 0 || len(p.ContextKinds) > 0
}

// ValidRoute reports whether the slug is a known route.
func ValidRoute(s string) bool {
	for _, r := range AllRoutes() {
		if string(r) == s {
			return true
		}
	}
	return false
}

// Location is one entry of the typed route stack. Exact equality is the
// struct equality of all four fields.
type Location struct {
	Route   Route
	Primary domain.ResourceRef
	Context domain.ResourceRef
	Query   string
}

// InputMode classifies page keyboard ownership.
type InputMode uint8

const (
	ModeNormal InputMode = iota
	ModeFilter
	ModeText
)

// String renders the mode name.
func (m InputMode) String() string {
	switch m {
	case ModeFilter:
		return "filter"
	case ModeText:
		return "text"
	default:
		return "normal"
	}
}

// Command is a contextual page action surfaced through the Space selector
// and the command palette. Invoking Run only emits typed messages.
type Command struct {
	ID             string
	Title          string
	Description    string
	Shortcut       string
	Disabled       bool
	DisabledReason string
	Run            func() tea.Cmd
}

// HelpLine is one help entry.
type HelpLine struct{ Key, Desc string }

// Page is the resident page contract owned by the root shell.
type Page interface {
	Route() Route
	Enter(Location) tea.Cmd
	// SetCatalog receives a read-only canonical snapshot; pages project but
	// never mutate it, reconciling selection by stable ID.
	SetCatalog(*domain.Catalog)
	SetSize(width, height int)
	SetTheme(theme.Styles)
	InputMode() InputMode
	Update(tea.Msg) tea.Cmd
	View() string
	Commands() []Command
	Help() []key.Binding
}

// SelectorOption is one row of a root-owned selector overlay.
type SelectorOption struct {
	ID       string
	Title    string
	Desc     string
	Shortcut string
	Command  func() tea.Cmd
	Disabled bool
	Reason   string
}
