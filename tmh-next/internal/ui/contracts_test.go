package ui

import (
	"testing"

	"github.com/mark1708/tmh-next/internal/domain"
)

func TestRoutePolicies(t *testing.T) {
	if !RouteAcceptsPrimary(RouteAgents, "agent") || RouteAcceptsPrimary(RouteAgents, "workspace") {
		t.Fatal("agents primary policy wrong")
	}
	if !RouteAcceptsContext(RouteHistory, "terminal") || !RouteAcceptsContext(RouteHistory, "agent") {
		t.Fatal("history context policy wrong")
	}
	if !RouteAcceptsContext(RouteWorkspaces, "machine") {
		t.Fatal("workspaces machine context rejected")
	}
	for _, r := range []Route{RouteDashboard, RouteSearch, RoutePerformance, RouteConfig} {
		if RouteAllowsCLIResource(r) {
			t.Errorf("%s must reject --resource", r)
		}
	}
	for _, r := range []Route{RouteWorkspace, RouteAgents, RouteHistory, RouteReconcile} {
		if !RouteAllowsCLIResource(r) {
			t.Errorf("%s must accept --resource", r)
		}
	}
	if !ValidRoute("reconcile") || ValidRoute("bogus") {
		t.Fatal("route validation broken")
	}
	if n := len(AllRoutes()); n != 15 {
		t.Fatalf("routes = %d, want 15", n)
	}
	if RouteTitle(RoutePerformance) != "Performance" {
		t.Fatal("route title wrong")
	}
}

func TestLocationEquality(t *testing.T) {
	a := Location{Route: RouteAgents, Primary: domain.Ref(domain.KindAgent, "a1")}
	b := a
	if a != b {
		t.Fatal("equal locations differ")
	}
	b.Query = "x"
	if a == b {
		t.Fatal("query difference ignored")
	}
	for _, m := range []InputMode{ModeNormal, ModeFilter, ModeText} {
		if m.String() == "" {
			t.Errorf("mode %d renders empty", m)
		}
	}
}
