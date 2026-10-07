package pages

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// historyPage shows paged stream chunks with gap markers and follow mode.
// Follow only continues while the viewport sits at the bottom; any movement
// disables it and `f` resumes.
type historyPage struct {
	base
	vp     viewport.Model
	follow bool
	hasVP  bool
}

func newHistory() *historyPage {
	return &historyPage{base: base{route: ui.RouteHistory}, follow: true}
}

func (p *historyPage) Enter(loc ui.Location) tea.Cmd {
	p.enter(loc)
	p.follow = true
	if p.hasVP {
		p.vp.GotoBottom()
	}
	return nil
}

func (p *historyPage) SetSize(w, h int) {
	p.base.SetSize(w, h)
	p.vp = viewport.New(viewport.WithWidth(w-2), viewport.WithHeight(max(1, h-4)))
	p.hasVP = true
}

func (p *historyPage) SetTheme(s theme.Styles) { p.base.SetTheme(s) }

func (p *historyPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	p.notFound = ""
	if p.loc.Context.ID != "" && c != nil {
		switch p.loc.Context.Kind {
		case domain.KindTerminal:
			if c.TerminalByID(p.loc.Context.ID) == nil {
				p.notFound = p.loc.Context.ID
			}
		case domain.KindAgent:
			if c.AgentByID(p.loc.Context.ID) == nil {
				p.notFound = p.loc.Context.ID
			}
		default:
			p.notFound = p.loc.Context.ID
		}
	}
}

func (p *historyPage) chunks() []*domain.HistoryChunk {
	if p.cat == nil {
		return nil
	}
	var out []*domain.HistoryChunk
	for _, h := range p.cat.History {
		if p.loc.Context.ID == "" || (h.Scope.Kind == p.loc.Context.Kind && h.Scope.ID == p.loc.Context.ID) {
			out = append(out, h)
		}
	}
	sortChunks(out)
	return out
}

func sortChunks(chunks []*domain.HistoryChunk) {
	for i := 1; i < len(chunks); i++ {
		for j := i; j > 0 && chunkLess(chunks[j], chunks[j-1]); j-- {
			chunks[j], chunks[j-1] = chunks[j-1], chunks[j]
		}
	}
}

func chunkLess(a, b *domain.HistoryChunk) bool {
	if a.Scope.ID != b.Scope.ID {
		return a.Scope.ID < b.Scope.ID
	}
	return a.Seq < b.Seq
}

func (p *historyPage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "f":
		p.follow = true
		p.vp.GotoBottom()
		return nil
	case "/":
		return pushRoute(ui.Location{Route: ui.RouteSearch, Query: "", Context: p.loc.Context}, false)
	case "e":
		return p.exportSelector()
	}
	// Movement disables follow.
	switch k.String() {
	case "up", "k", "pgup", "pgdown", "down", "j", "home", "end":
		if p.follow && (k.String() != "end") {
			p.follow = false
		}
	}
	p.vp.Update(k)
	if p.follow {
		p.vp.GotoBottom()
	}
	return nil
}

func (p *historyPage) exportSelector() tea.Cmd {
	chunks := p.chunks()
	if len(chunks) == 0 {
		return toast("no history rows to export", "warn")
	}
	opts := make([]ui.SelectorOption, 0, len(chunks))
	for _, h := range chunks {
		opts = append(opts, ui.SelectorOption{
			ID: h.ID, Title: h.ID + " · " + h.Scope.ID + " seq " + fmt.Sprint(h.Seq),
			Desc: fmt.Sprintf("%d lines", len(h.Lines)),
		})
	}
	return openSelector("Export history rows", opts)
}

func (p *historyPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	missing := ""
	if p.loc.Context.ID != "" {
		switch p.loc.Context.Kind {
		case domain.KindTerminal:
			if p.cat.TerminalByID(p.loc.Context.ID) == nil {
				missing = p.loc.Context.ID
			}
		case domain.KindAgent:
			if p.cat.AgentByID(p.loc.Context.ID) == nil {
				missing = p.loc.Context.ID
			}
		default:
			missing = p.loc.Context.ID
		}
	}
	if missing != "" {
		return p.notFoundView(p.loc.Context.Kind, missing) + "\n" + p.styles.Dim.Render("no rows for an unknown scope")
	}
	var head strings.Builder
	head.WriteString(p.titleBar("History"))
	if p.loc.Context.ID != "" {
		head.WriteString(" " + p.styles.Dim.Render("scope "+string(p.loc.Context.Kind)+":"+p.loc.Context.ID))
	}
	badge := p.styles.Dim.Render("follow off — f to resume")
	if p.follow {
		badge = p.status("RUNNING") + " " + p.styles.Dim.Render("follow on")
	}
	head.WriteString("  " + badge)

	chunks := p.chunks()
	if len(chunks) == 0 {
		return head.String() + "\n" + p.emptyView("history chunks")
	}
	var b strings.Builder
	for _, h := range chunks {
		if h.Gap {
			b.WriteString(p.styles.Warn.Render("⋯ gap: history for this range is missing ⋯"))
			b.WriteString("\n")
		}
		b.WriteString(p.styles.Terminal.Render(h.ID + " · " + string(h.Scope.Kind) + ":" + h.Scope.ID + " · " + h.At.UTC().Format("15:04:05")))
		b.WriteString("\n")
		for _, line := range h.Lines {
			b.WriteString("  " + line + "\n")
		}
	}
	p.vp.SetContent(b.String())
	if p.follow {
		p.vp.GotoBottom()
	}
	return head.String() + "\n" + p.vp.View()
}

func (p *historyPage) Commands() []ui.Command {
	commands := []ui.Command{
		{ID: "h.follow", Title: "Follow latest", Description: "stick to the stream bottom", Shortcut: "f",
			Run: func() tea.Cmd { return func() tea.Msg { return ui.OpenSelectorMsg{Title: "noop"} } }},
		{ID: "h.search", Title: "Scoped search", Description: "search within this scope", Shortcut: "/",
			Run: func() tea.Cmd { return pushRoute(ui.Location{Route: ui.RouteSearch, Context: p.loc.Context}, false) }},
		{ID: "h.export", Title: "Export rows", Description: "write a bounded history export", Shortcut: "e",
			Disabled: len(p.chunks()) == 0, DisabledReason: "no history rows",
			Run: func() tea.Cmd {
				ids := make([]string, 0, len(p.chunks()))
				for _, h := range p.chunks() {
					ids = append(ids, h.ID)
				}
				return execute(domain.Action{Kind: domain.ActionHistoryExport, Selection: ids})
			}},
	}
	// follow toggles through the contextual selector too
	commands[0].Run = func() tea.Cmd { return func() tea.Msg { return followToggleMsg{} } }
	return commands
}

type followToggleMsg struct{}

type rawToggleMsg struct{}

func (p *historyPage) Help() []key.Binding {
	return pageHelp([2]string{"f", "follow"}, [2]string{"/", "search"}, [2]string{"e", "export"}, [2]string{"pgup/pgdn", "page"})
}

// eventsPage is the durable audit timeline with raw JSON detail.
type eventsPage struct {
	base
	vp     viewport.Model
	hasVP  bool
	follow bool
	raw    bool
}

func newEvents() *eventsPage {
	return &eventsPage{base: base{route: ui.RouteEvents}, follow: true}
}

func (p *eventsPage) Enter(loc ui.Location) tea.Cmd {
	p.enter(loc)
	p.follow = true
	if p.hasVP {
		p.vp.GotoBottom()
	}
	return nil
}

func (p *eventsPage) SetSize(w, h int) {
	p.base.SetSize(w, h)
	p.vp = viewport.New(viewport.WithWidth(w-2), viewport.WithHeight(max(1, h-4)))
	p.hasVP = true
}

func (p *eventsPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	ids := make([]string, 0, len(c.Events))
	for _, e := range c.Events {
		ids = append(ids, e.ID)
	}
	p.setCatalog(sortedIDs(ids))
	if p.selectedID != "" && c.EventByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *eventsPage) Update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(followToggleMsg); ok {
		p.follow = !p.follow
		if p.follow {
			p.vp.GotoBottom()
		}
		return nil
	}
	if _, ok := msg.(rawToggleMsg); ok {
		p.raw = !p.raw
		return nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "f":
		p.follow = !p.follow
		if p.follow {
			p.vp.GotoBottom()
		}
		return nil
	case "/":
		return pushRoute(ui.Location{Route: ui.RouteSearch}, false)
	case "enter":
		if e := p.current(); e != nil {
			return contextJump(e)
		}
	}
	switch k.String() {
	case "up", "k", "pgup", "pgdown", "down", "j":
		if p.follow {
			p.follow = false
		}
	}
	rows := make([]idLabel, 0, len(p.cat.Events))
	for _, e := range p.cat.Events {
		rows = append(rows, idLabel{id: e.ID, label: e.Message})
	}
	if p.cursorKeys(k, rows) {
		return nil
	}
	p.vp.Update(k)
	if p.follow {
		p.vp.GotoBottom()
	}
	return nil
}

func (p *eventsPage) current() *domain.Event {
	if p.cat == nil {
		return nil
	}
	return p.cat.EventByID(p.selected())
}

func contextJump(e *domain.Event) tea.Cmd {
	switch e.Resource.Kind {
	case domain.KindWorkspace:
		return pushRoute(ui.Location{Route: ui.RouteWorkspace, Primary: e.Resource}, false)
	case domain.KindTerminal:
		return pushRoute(ui.Location{Route: ui.RouteTerminals, Primary: e.Resource}, false)
	case domain.KindAgent:
		return pushRoute(ui.Location{Route: ui.RouteAgents, Primary: e.Resource}, false)
	case domain.KindSnapshot:
		return pushRoute(ui.Location{Route: ui.RouteSnapshots, Primary: e.Resource}, false)
	case domain.KindPlan:
		return pushRoute(ui.Location{Route: ui.RouteReconcile, Primary: e.Resource}, false)
	case domain.KindBackend:
		return pushRoute(ui.Location{Route: ui.RouteBackends, Primary: e.Resource}, false)
	case domain.KindMachine:
		return pushRoute(ui.Location{Route: ui.RouteMachines, Primary: e.Resource}, false)
	}
	return nil
}

func (p *eventsPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.EventByID(p.selected()) == nil {
		return p.notFoundView(domain.KindEvent, p.selected())
	}
	var head strings.Builder
	head.WriteString(p.titleBar("Events"))
	badge := p.styles.Dim.Render("follow off — f to resume")
	if p.follow {
		badge = p.status("RUNNING") + " " + p.styles.Dim.Render("follow on")
	}
	head.WriteString("  " + badge)
	if len(p.cat.Events) == 0 {
		return head.String() + "\n" + p.emptyView("events")
	}
	var b strings.Builder
	for _, e := range p.cat.Events {
		line := fmt.Sprintf("%s %-9s %-10s %s", e.At.UTC().Format("15:04:05"),
			strings.ToUpper(string(e.Severity)), string(e.Category), e.Message)
		if e.ID == p.selected() {
			line = p.styles.Select(line)
		} else {
			switch e.Severity {
			case domain.SeverityError:
				line = p.styles.Err.Render(line)
			case domain.SeverityWarn:
				line = p.styles.Warn.Render(line)
			default:
				line = p.styles.Dim.Render(line)
			}
		}
		b.WriteString(line + "\n")
		if p.raw && e.ID == p.selected() {
			b.WriteString("  " + p.styles.Info.Render(e.Detail) + "\n")
		}
	}
	p.vp.SetContent(b.String())
	if p.follow {
		p.vp.GotoBottom()
	}
	return head.String() + "\n" + p.vp.View()
}

func (p *eventsPage) Commands() []ui.Command {
	return []ui.Command{
		{ID: "ev.follow", Title: "Toggle follow", Description: "stick to the newest event", Shortcut: "f",
			Run: func() tea.Cmd { return func() tea.Msg { return followToggleMsg{} } }},
		{ID: "ev.detail", Title: "Raw detail", Description: "toggle compact/raw JSON detail", Shortcut: "",
			Run: func() tea.Cmd { return func() tea.Msg { return rawToggleMsg{} } }},
		{ID: "ev.jump", Title: "Jump to source", Description: "open the event's resource", Shortcut: "enter",
			Disabled: p.current() == nil, DisabledReason: "no event selected",
			Run: func() tea.Cmd { return contextJump(p.current()) }},
	}
}

func (p *eventsPage) Help() []key.Binding {
	return pageHelp([2]string{"f", "follow"}, [2]string{"enter", "jump"}, [2]string{"/", "search"})
}
