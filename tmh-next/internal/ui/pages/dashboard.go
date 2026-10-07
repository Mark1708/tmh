package pages

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
)

// dashboardPage is the attention-oriented cockpit home.
type dashboardPage struct {
	base
	cursor int
}

func newDashboard() *dashboardPage { return &dashboardPage{base: base{route: ui.RouteDashboard}} }

type dashCard struct {
	id     string
	title  string
	detail string
	loc    ui.Location
}

func (p *dashboardPage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); p.cursor = 0; return nil }

func (p *dashboardPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	p.setCatalog(nil)
}

func (p *dashboardPage) cards() []dashCard {
	var cards []dashCard
	if p.cat == nil {
		return cards
	}
	for _, w := range p.cat.Workspaces {
		if w.Status == domain.WorkspaceDrift {
			cards = append(cards, dashCard{
				id: "drift:" + w.ID, title: "DRIFT workspace " + w.Name,
				detail: fmt.Sprintf("%d drifted field(s)", len(w.Drift)),
				loc:    ui.Location{Route: ui.RouteWorkspace, Primary: domain.Ref(domain.KindWorkspace, w.ID)},
			})
		}
	}
	for _, a := range p.cat.Agents {
		if a.State == domain.AgentNeedsInput {
			cards = append(cards, dashCard{
				id: "agent:" + a.ID, title: "NEEDS INPUT agent " + a.Name,
				detail: fmt.Sprintf("%s on %s · progress %d%%", a.Kind, a.WorkspaceID, a.Progress),
				loc:    ui.Location{Route: ui.RouteAgents, Primary: domain.Ref(domain.KindAgent, a.ID)},
			})
		}
	}
	for _, m := range p.cat.Machines {
		if m.Status == domain.MachineOffline {
			cards = append(cards, dashCard{
				id: "machine:" + m.ID, title: "OFFLINE machine " + m.Name,
				detail: "unreachable since " + m.Heartbeat.UTC().Format("15:04"),
				loc:    ui.Location{Route: ui.RouteMachines, Primary: domain.Ref(domain.KindMachine, m.ID)},
			})
		}
	}
	for _, b := range p.cat.Backends {
		if b.Status == domain.BackendUnavailable {
			cards = append(cards, dashCard{
				id: "backend:" + b.ID, title: "UNAVAILABLE backend " + b.Name,
				detail: strings.Join(b.UnsupportedReasons, "; "),
				loc:    ui.Location{Route: ui.RouteBackends, Primary: domain.Ref(domain.KindBackend, b.ID)},
			})
		}
	}
	for _, v := range p.cat.ActiveViews {
		if !v.Pinned && v.ExpiredAt(p.cat.Now).Before(p.cat.Now) {
			cards = append(cards, dashCard{
				id: "expired:" + v.ID, title: "EXPIRED view " + v.Title,
				detail: "unpinned and past TTL — prune it",
				loc:    ui.Location{Route: ui.RouteActive, Primary: domain.Ref(domain.KindActive, v.ID)},
			})
		}
	}
	return cards
}

func (p *dashboardPage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	cards := p.cards()
	switch k.String() {
	case "down", "j":
		if p.cursor < len(cards)-1 {
			p.cursor++
		}
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter":
		if p.cursor < len(cards) {
			return pushRoute(cards[p.cursor].loc, false)
		}
	case "/":
		return pushRoute(ui.Location{Route: ui.RouteSearch}, false)
	}
	return nil
}

func (p *dashboardPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	var b strings.Builder
	b.WriteString(p.titleBar("Dashboard"))
	b.WriteString("\n")

	cardWidth := max(12, (p.width-pageGap*3)/4)
	running, needsInput := 0, 0
	for _, agent := range p.cat.Agents {
		switch agent.State {
		case domain.AgentRunning:
			running++
		case domain.AgentNeedsInput:
			needsInput++
		}
	}
	healthy := 0
	for _, backend := range p.cat.Backends {
		if backend.Status == domain.BackendHealthy {
			healthy++
		}
	}
	b.WriteString(horizontalCards([]string{
		p.metricCard("attention", fmt.Sprint(len(p.cards())), "items requiring action", cardWidth),
		p.metricCard("workspaces", fmt.Sprint(len(p.cat.Workspaces)), fmt.Sprintf("%d drifted", countDrift(p.cat)), cardWidth),
		p.metricCard("agents", fmt.Sprint(len(p.cat.Agents)), fmt.Sprintf("%d running · %d waiting", running, needsInput), cardWidth),
		p.metricCard("backends", fmt.Sprintf("%d/%d", healthy, len(p.cat.Backends)), "healthy and available", cardWidth),
	}))
	b.WriteString("\n")

	if p.isWide() {
		b.WriteString(p.panelSplit("Attention queue", p.attentionView(), "System health", p.healthView(), 60))
		b.WriteString("\n")
		b.WriteString(p.panelSplit("Performance", p.perfView(), "Recent activity", p.eventsView(), 50))
		if p.height >= 34 {
			b.WriteString("\n")
			remaining := p.remainingHeight(b.String(), 10)
			b.WriteString(p.panelSplitHeight("Workspace pulse", p.workspacePulse(), "Agent activity", p.agentPulse(), 50, remaining))
		}
	} else {
		b.WriteString(p.panel("Attention queue", p.attentionView(), p.width))
		b.WriteString("\n")
		b.WriteString(p.panel("System health", p.healthView(), p.width))
	}
	return p.clip(b.String())
}

func (p *dashboardPage) attentionView() string {
	cards := p.cards()
	var b strings.Builder
	if len(cards) == 0 {
		b.WriteString(p.styles.OK.Render("✓ nothing needs attention"))
		return b.String()
	}
	for i, c := range cards {
		title := c.title
		for _, prefix := range []string{"DRIFT ", "NEEDS INPUT ", "OFFLINE ", "UNAVAILABLE ", "EXPIRED "} {
			title = strings.TrimPrefix(title, prefix)
		}
		if i == p.cursor {
			title = p.styles.Select(title)
		}
		b.WriteString(fmt.Sprintf("%-12s %-38s %s",
			p.status(cardStatus(c.id)),
			title,
			p.styles.Dim.Render(c.detail),
		))
		if i < len(cards)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func cardStatus(id string) string {
	switch {
	case strings.HasPrefix(id, "drift:"):
		return "DRIFT"
	case strings.HasPrefix(id, "agent:"):
		return "NEEDS INPUT"
	case strings.HasPrefix(id, "machine:"):
		return "OFFLINE"
	case strings.HasPrefix(id, "backend:"):
		return "UNAVAILABLE"
	default:
		return "WARN"
	}
}

func (p *dashboardPage) healthView() string {
	var b strings.Builder
	for i, be := range p.cat.Backends {
		status := map[domain.BackendStatus]string{
			domain.BackendHealthy: "OK", domain.BackendPreview: "WARN", domain.BackendUnavailable: "OFFLINE",
		}[be.Status]
		b.WriteString(fmt.Sprintf("%-9s %-10s %s",
			p.status(status),
			be.Name,
			p.styles.Dim.Render(fmt.Sprintf("%d instances · %dms probe", be.Instances, be.LastProbe.LatencyMS)),
		))
		if i < len(p.cat.Backends)-1 {
			b.WriteString("\n")
		}
	}
	agents := map[domain.AgentState]int{}
	for _, a := range p.cat.Agents {
		agents[a.State]++
	}
	b.WriteString("\n\n")
	b.WriteString(p.styles.Info.Render("AGENTS") + "  " + p.styles.Dim.Render(
		fmt.Sprintf("%d running  ·  %d need input  ·  %d waiting",
			agents[domain.AgentRunning], agents[domain.AgentNeedsInput], agents[domain.AgentWaiting])))
	return b.String()
}

func countDrift(c *domain.Catalog) int {
	n := 0
	for _, w := range c.Workspaces {
		if w.Status == domain.WorkspaceDrift {
			n++
		}
	}
	return n
}

func (p *dashboardPage) perfView() string {
	samples := p.cat.Performance.Samples
	if len(samples) == 0 {
		return p.styles.Dim.Render("No performance samples")
	}
	last := samples[len(samples)-1]
	spark := sparkline(samples, max(24, p.width/3), func(s domain.PerfSample) int { return s.InputP99 })
	return p.styles.Bar.Render(spark) + "\n" +
		fmt.Sprintf("%s input  %s redraw  %s action",
			p.styles.Header.Render(fmt.Sprintf("%dms", last.InputP99)),
			p.styles.Header.Render(fmt.Sprintf("%dms", last.RedrawP99)),
			p.styles.Header.Render(fmt.Sprintf("%dms", last.ActionP99))) + "\n" +
		p.styles.Dim.Render(fmt.Sprintf("%d samples · queue depth %d", len(samples), last.QueueDepth))
}

func (p *dashboardPage) eventsView() string {
	var b strings.Builder
	start := 0
	events := p.cat.Events
	if len(events) > 4 {
		start = len(events) - 4
	}
	for i, e := range events[start:] {
		b.WriteString(fmt.Sprintf("%s %-9s %s",
			p.status(sevStatus(e.Severity)),
			e.At.UTC().Format("15:04:05"),
			e.Message,
		))
		if i < len(events[start:])-1 {
			b.WriteString("\n")
		}
	}
	if len(events) == 0 {
		b.WriteString(p.styles.Dim.Render("No events yet"))
	}
	return b.String()
}

func (p *dashboardPage) workspacePulse() string {
	var b strings.Builder
	for i, ws := range p.cat.Workspaces {
		status := "OK"
		if ws.Status == domain.WorkspaceDrift {
			status = "DRIFT"
		} else if ws.Status == domain.WorkspaceFrozen {
			status = "WARN"
		}
		b.WriteString(fmt.Sprintf("%-12s %-10s %d terminals  %d agents  %s",
			ws.Name, p.status(status), len(ws.TerminalIDs), len(ws.AgentIDs),
			p.styles.Dim.Render(ws.MachineID)))
		if i < len(p.cat.Workspaces)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (p *dashboardPage) agentPulse() string {
	var b strings.Builder
	for i, agent := range p.cat.Agents {
		b.WriteString(fmt.Sprintf("%-15s %-12s %3d%%  %s",
			agent.Name,
			p.status(agentStateStatus(agent.State)),
			agent.Progress,
			p.styles.Dim.Render(agent.WorkspaceID+" · "+string(agent.Kind)),
		))
		if i < len(p.cat.Agents)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func sevStatus(s domain.EventSeverity) string {
	switch s {
	case domain.SeverityError:
		return "ERROR"
	case domain.SeverityWarn:
		return "WARN"
	default:
		return "OK"
	}
}

// sparkline renders a bounded bar chart of the series.
func sparkline[T any](series []T, bound int, val func(T) int) string {
	if len(series) == 0 {
		return ""
	}
	if len(series) > bound {
		series = series[len(series)-bound:]
	}
	max := 1
	for _, s := range series {
		if v := val(s); v > max {
			max = v
		}
	}
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var b strings.Builder
	for _, s := range series {
		idx := val(s) * (len(blocks) - 1) / max
		b.WriteRune(blocks[idx])
	}
	return b.String()
}

func (p *dashboardPage) Commands() []ui.Command {
	commands := []ui.Command{{
		ID: "dash.search", Title: "Open search", Description: "jump to global search", Shortcut: "/",
		Run: func() tea.Cmd { return pushRoute(ui.Location{Route: ui.RouteSearch}, false) },
	}}
	if p.cursor < len(p.cards()) {
		card := p.cards()[p.cursor]
		commands = append(commands, ui.Command{
			ID: "dash.open", Title: "Open " + card.title, Description: "navigate to the attention card",
			Shortcut: "enter", Run: func() tea.Cmd { return pushRoute(card.loc, false) },
		})
	}
	return commands
}

func (p *dashboardPage) Help() []key.Binding {
	return pageHelp([2]string{"j/k", "cards"}, [2]string{"enter", "open"}, [2]string{"/", "search"})
}
