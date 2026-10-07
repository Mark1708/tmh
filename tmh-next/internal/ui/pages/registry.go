package pages

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/ui"
)

// New builds the resident page for a route.
func New(route ui.Route) ui.Page {
	switch route {
	case ui.RouteDashboard:
		return newDashboard()
	case ui.RouteWorkspaces:
		return newWorkspaces()
	case ui.RouteWorkspace:
		return newWorkspace()
	case ui.RouteActive:
		return newActive()
	case ui.RouteTerminals:
		return newTerminals()
	case ui.RouteAgents:
		return newAgents()
	case ui.RouteHistory:
		return newHistory()
	case ui.RouteSearch:
		return newSearch()
	case ui.RouteSnapshots:
		return newSnapshots()
	case ui.RouteReconcile:
		return newReconcile()
	case ui.RouteBackends:
		return newBackends()
	case ui.RouteMachines:
		return newMachines()
	case ui.RoutePerformance:
		return newPerformance()
	case ui.RouteEvents:
		return newEvents()
	case ui.RouteConfig:
		return newConfigPage()
	default:
		return nil
	}
}

// Factory adapts New to the app.PageFactory signature.
func Factory(route ui.Route) ui.Page { return New(route) }

var _ = tea.KeyPressMsg{}
