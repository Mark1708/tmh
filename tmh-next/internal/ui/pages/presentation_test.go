package pages

import (
	"strings"
	"testing"

	"github.com/mark1708/tmh-next/internal/testutil"
	"github.com/mark1708/tmh-next/internal/ui"
)

func TestWidePagesUseHorizontalCanvas(t *testing.T) {
	t.Parallel()
	cases := []struct {
		route       ui.Route
		left, right string
		minWidth    int
		minHeight   int
	}{
		{ui.RouteDashboard, "Attention queue", "System health", 150, 24},
		{ui.RouteWorkspaces, "Workspace inventory", "Selection", 150, 14},
		{ui.RouteWorkspace, "Observed topology", "Workspace detail", 150, 14},
		{ui.RoutePerformance, "Latency timeline", "Recent operations", 150, 14},
		{ui.RouteTerminals, "Terminal fleet", "Selection", 150, 8},
		{ui.RouteAgents, "Agent board", "Selection", 150, 8},
		{ui.RouteSnapshots, "Snapshot catalog", "Selection", 150, 8},
	}
	for _, tc := range cases {
		t.Run(string(tc.route), func(t *testing.T) {
			p, _ := pageFor(t, tc.route, "default")
			p.Enter(ui.Location{Route: tc.route})
			p.SetSize(192, 54)
			view := testutil.Plain(p.View())
			if !sameLine(view, tc.left, tc.right) {
				t.Fatalf("wide layout is not horizontal: %q and %q never share a row\n%s", tc.left, tc.right, view)
			}
			if width := testutil.MaxLineWidth(view); width < tc.minWidth {
				t.Fatalf("page only uses %d columns, want at least %d", width, tc.minWidth)
			}
			if height := len(strings.Split(view, "\n")); height < tc.minHeight {
				t.Fatalf("page only uses %d rows, want at least %d", height, tc.minHeight)
			}
		})
	}
}

func sameLine(view, left, right string) bool {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, left) && strings.Contains(line, right) {
			return true
		}
	}
	return false
}
