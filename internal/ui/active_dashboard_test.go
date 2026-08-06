package ui

import (
	"strings"
	"testing"

	"github.com/mark1708/tmh/internal/actions"
	"github.com/mark1708/tmh/internal/i18n"
	"github.com/mark1708/tmh/internal/state"
	"github.com/mark1708/tmh/internal/ui/theme"
)

func TestActiveDashboardDisabledOmitsRuntimeSection(t *testing.T) {
	d := activeDashboardFixture(t)
	d.SetActiveStatus(actions.ActiveStatusReport{Enabled: false})

	view := d.View()
	if strings.Contains(view, "ACTIVE (runtime)") {
		t.Fatalf("disabled dashboard unexpectedly contains runtime section:\n%s", view)
	}
}

func TestActiveDashboardRendersStatesSourcesAndCollision(t *testing.T) {
	d := activeDashboardFixture(t)
	index := 7
	d.SetActiveStatus(actions.ActiveStatusReport{
		Enabled: true,
		Owned:   true,
		Entries: []actions.ActiveWindowStatus{
			{
				WindowID: "@42", WindowName: "editor", State: state.ActiveWindowTracked,
				ActiveIndex: &index,
				SourceLinks: []actions.ActiveSourceLink{{SessionName: "work", WindowIndex: 2}},
			},
			{WindowID: "@77", WindowName: "shell", State: state.ActiveWindowOrphaned, Expired: true},
		},
	})

	view := d.View()
	for _, want := range []string{"ACTIVE (runtime)", "@42", "tracked", "work:2", "#7", "@77", "orphaned", "expired"} {
		if !strings.Contains(view, want) {
			t.Fatalf("active dashboard missing %q:\n%s", want, view)
		}
	}

	d.SetActiveStatus(actions.ActiveStatusReport{Enabled: true, Collision: true, CollisionReason: "ownership marker is missing"})
	d.restoreCursorByID("active-runtime")
	collision := d.View()
	if !strings.Contains(collision, "collision") || !strings.Contains(collision, "ownership marker is missing") {
		t.Fatalf("collision warning missing:\n%s", collision)
	}
}

func TestActiveDashboardCursorStableByWindowIDAfterRenumber(t *testing.T) {
	d := activeDashboardFixture(t)
	firstIndex := 3
	d.SetActiveStatus(actions.ActiveStatusReport{Enabled: true, Entries: []actions.ActiveWindowStatus{{
		WindowID: "@42", WindowName: "editor", State: state.ActiveWindowTracked, ActiveIndex: &firstIndex,
	}}})
	d.restoreCursorByID("@42")
	if got := d.currentTargetKey(); got != "@42" {
		t.Fatalf("selected active ID = %q", got)
	}

	secondIndex := 11
	d.SetActiveStatus(actions.ActiveStatusReport{Enabled: true, Entries: []actions.ActiveWindowStatus{{
		WindowID: "@42", WindowName: "renamed", State: state.ActiveWindowTracked, ActiveIndex: &secondIndex,
	}}})
	if got := d.currentTargetKey(); got != "@42" {
		t.Fatalf("selection moved after renumber: %q", got)
	}
	row := d.currentRow()
	if row == nil || row.Active == nil || row.Active.ActiveIndex == nil || *row.Active.ActiveIndex != 11 {
		t.Fatalf("selected row was not refreshed: %+v", row)
	}
}

func TestActiveDashboardAttachTargetUsesWindowIDWithoutChangingDestructiveTarget(t *testing.T) {
	d := activeDashboardFixture(t)
	index := 1
	d.SetActiveStatus(actions.ActiveStatusReport{Enabled: true, Owned: true, Entries: []actions.ActiveWindowStatus{{
		WindowID: "@13", WindowName: "homelab", State: state.ActiveWindowTracked, ActiveIndex: &index,
		SourceLinks: []actions.ActiveSourceLink{{SessionName: "infra", WindowIndex: 1}},
	}}})
	d.restoreCursorByID("@13")

	if got := d.SelectedAttachTarget(); got != "@13" {
		t.Fatalf("active attach target = %q, want @13", got)
	}
	if got := d.SelectedTarget(); got != "" {
		t.Fatalf("active destructive target = %q, want empty", got)
	}
}

func activeDashboardFixture(t *testing.T) *dashboardModel {
	t.Helper()
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	d := newDashboard(DefaultKeys(), theme.New(theme.Mocha), LoadStrings())
	d.Resize(120, 30)
	d.SetData(&actions.Listing{Sessions: []actions.ListedSession{{Name: "work", Live: true}}}, nil)
	return d
}
