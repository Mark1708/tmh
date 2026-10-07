package pages

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
)

// workspacesPage is the workspace collection with filter and local diff.
type workspacesPage struct{ base }

func newWorkspaces() *workspacesPage {
	return &workspacesPage{base: base{route: ui.RouteWorkspaces}}
}

func (p *workspacesPage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *workspacesPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	p.setCatalog(workspaceIDsOf(c))
}

func workspaceIDsOf(c *domain.Catalog) []string {
	if c == nil {
		return nil
	}
	ids := make([]string, 0, len(c.Workspaces))
	for _, w := range c.Workspaces {
		ids = append(ids, w.ID)
	}
	return sortedIDs(ids)
}

func (p *workspacesPage) rows() []idLabel {
	if p.cat == nil {
		return nil
	}
	var rows []idLabel
	for _, w := range p.cat.Workspaces {
		if p.loc.Context.Kind == domain.KindMachine && w.MachineID != p.loc.Context.ID {
			continue
		}
		rows = append(rows, idLabel{id: w.ID, label: w.ID + " " + w.Name + " " + string(w.Status)})
	}
	// stable sort by id
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].id < rows[j-1].id; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

func (p *workspacesPage) visible() []idLabel { return p.filterIds(p.rows()) }

func (p *workspacesPage) target() domain.ResourceRef {
	return domain.Ref(domain.KindWorkspace, p.selected())
}

func (p *workspacesPage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	rows := p.rows()
	filtered, handled := p.baseFilterKeys(k, rows)
	if handled {
		return nil
	}
	rows = filtered
	if p.cursorKeys(k, rows) {
		return nil
	}
	if len(rows) == 0 || p.selected() == "" {
		return nil
	}
	ws := p.cat.WorkspaceByID(p.selected())
	if ws == nil {
		return nil
	}
	switch k.String() {
	case "enter":
		return pushRoute(ui.Location{Route: ui.RouteWorkspace, Primary: p.target()}, false)
	case "a":
		if !ws.Live(p.cat.Terminals) {
			return toast("No live Zellij session in "+ws.Name+"; open the workspace to inspect its terminals", "warn")
		}
		return execute(domain.Action{Kind: domain.ActionAttach, Target: p.target()})
	case "f":
		return confirm("Freeze workspace "+ws.ID, "desired := observed; drift cleared",
			domain.Action{Kind: domain.ActionFreeze, Target: p.target()})
	case "s":
		return execute(domain.Action{Kind: domain.ActionSnapshotCreate, Target: p.target()})
	case "d":
		return openSelector("Local diff "+ws.ID, p.diffOptions(ws))
	}
	return nil
}

func (p *workspacesPage) diffOptions(ws *domain.Workspace) []ui.SelectorOption {
	var opts []ui.SelectorOption
	for _, d := range ws.Drift {
		opts = append(opts, ui.SelectorOption{
			ID: "diff:" + d.Field, Title: d.Field,
			Desc: "desired " + d.Desired + " · observed " + d.Observed,
		})
	}
	if len(opts) == 0 {
		opts = append(opts, ui.SelectorOption{
			ID: "diff:none", Title: "No drift", Desc: "desired matches observed",
		})
	}
	return opts
}

func (p *workspacesPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.loc.Context.ID != "" && p.cat.MachineByID(p.loc.Context.ID) == nil {
		return p.notFoundView(domain.KindMachine, p.loc.Context.ID)
	}
	rows := p.visible()
	var b strings.Builder
	b.WriteString(p.titleBar("Workspaces"))
	if p.loc.Context.Kind == domain.KindMachine {
		b.WriteString("  " + p.styles.Dim.Render("filtered by machine "+p.loc.Context.ID))
	}
	b.WriteString("\n")
	if len(rows) == 0 {
		b.WriteString(p.panel("Workspace inventory", p.emptyView("workspaces"), p.width))
		return p.clip(b.String())
	}

	live, drifted, terminals, agents := 0, 0, 0, 0
	for _, row := range rows {
		ws := p.cat.WorkspaceByID(row.id)
		if ws == nil {
			continue
		}
		if ws.Live(p.cat.Terminals) {
			live++
		}
		if ws.Status == domain.WorkspaceDrift {
			drifted++
		}
		terminals += len(ws.TerminalIDs)
		agents += len(ws.AgentIDs)
	}
	cardWidth := max(12, (p.width-pageGap*3)/4)
	b.WriteString(horizontalCards([]string{
		p.metricCard("total", fmt.Sprint(len(rows)), "visible workspaces", cardWidth),
		p.metricCard("live", fmt.Sprint(live), "with live terminals", cardWidth),
		p.metricCard("drift", fmt.Sprint(drifted), "need reconciliation", cardWidth),
		p.metricCard("attached", fmt.Sprintf("%d / %d", terminals, agents), "terminals / agents", cardWidth),
	}))
	b.WriteString("\n")

	list := p.workspaceList(rows)
	if p.isWide() {
		b.WriteString(p.panelSplitHeight("Workspace inventory", list, "Selection", p.workspaceSelection(), 64, p.remainingHeight(b.String(), 10)))
	} else {
		b.WriteString(p.panel("Workspace inventory", list, p.width))
	}
	return p.clip(b.String())
}

func (p *workspacesPage) workspaceList(rows []idLabel) string {
	var b strings.Builder
	b.WriteString(p.styles.Dim.Render("  WORKSPACE      STATE       WINDOWS   TERMS   AGENTS   MACHINE"))
	b.WriteString("\n")
	for i, row := range rows {
		ws := p.cat.WorkspaceByID(row.id)
		if ws == nil {
			continue
		}
		status := "OK"
		if ws.Status == domain.WorkspaceDrift {
			status = "DRIFT"
		} else if ws.Status == domain.WorkspaceFrozen {
			status = "WARN"
		}
		text := fmt.Sprintf("%-14s %-11s %3d / %-3d %7d %8d   %s",
			ws.Name, p.status(status), len(ws.Observed.Windows), len(ws.Desired.Windows),
			len(ws.TerminalIDs), len(ws.AgentIDs), p.styles.Dim.Render(ws.MachineID))
		b.WriteString(p.row(i, text))
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (p *workspacesPage) workspaceSelection() string {
	ws := p.cat.WorkspaceByID(p.selected())
	if ws == nil {
		return p.styles.Dim.Render("No workspace selected")
	}
	var b strings.Builder
	b.WriteString(p.styles.Header.Render(ws.Name))
	b.WriteString("  " + p.status(strings.ToUpper(string(ws.Status))) + "\n\n")
	b.WriteString(p.keyValue("machine", ws.MachineID))
	b.WriteString(p.keyValue("backend", ws.BackendID))
	b.WriteString(p.keyValue("terminals", fmt.Sprint(len(ws.TerminalIDs))))
	b.WriteString(p.keyValue("agents", fmt.Sprint(len(ws.AgentIDs))))
	b.WriteString(p.keyValue("focused", ws.LastFocused.UTC().Format("15:04:05")))
	if len(ws.Drift) == 0 {
		b.WriteString("\n" + p.styles.OK.Render("✓ desired state matches observed"))
	} else {
		b.WriteString("\n" + p.styles.Warn.Render(fmt.Sprintf("%d drifted field(s)", len(ws.Drift))))
		for _, drift := range ws.Drift {
			b.WriteString("\n  " + p.styles.Dim.Render(drift.Field))
		}
	}
	return b.String()
}

func (p *workspacesPage) Commands() []ui.Command {
	var commands []ui.Command
	if sel := p.selected(); sel != "" {
		workspace := p.cat.WorkspaceByID(sel)
		live := workspace != nil && workspace.Live(p.cat.Terminals)
		commands = append(commands,
			ui.Command{ID: "ws.open", Title: "Open workspace", Description: "inspect topology and create terminal panes", Shortcut: "enter",
				Run: func() tea.Cmd { return pushRoute(ui.Location{Route: ui.RouteWorkspace, Primary: p.target()}, false) }},
			ui.Command{ID: "ws.attach", Title: "Attach session", Description: "leave tmh-next and enter this workspace's live Zellij session", Shortcut: "a",
				Disabled: !live, DisabledReason: "workspace has no live Zellij session",
				Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionAttach, Target: p.target()}) }},
			ui.Command{ID: "ws.freeze", Title: "Freeze", Description: "desired := observed", Shortcut: "f",
				Run: func() tea.Cmd {
					return confirm("Freeze workspace "+sel, "desired := observed; drift cleared",
						domain.Action{Kind: domain.ActionFreeze, Target: p.target()})
				}},
			ui.Command{ID: "ws.snapshot", Title: "Snapshot", Description: "create a durable snapshot", Shortcut: "s",
				Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionSnapshotCreate, Target: p.target()}) }},
			ui.Command{ID: "ws.diff", Title: "Local diff", Description: "desired vs observed fields", Shortcut: "d",
				Run: func() tea.Cmd {
					if ws := p.cat.WorkspaceByID(sel); ws != nil {
						return openSelector("Local diff "+sel, p.diffOptions(ws))
					}
					return nil
				}},
		)
	}
	return commands
}

func (p *workspacesPage) Help() []key.Binding {
	return pageHelp([2]string{"/", "filter"}, [2]string{"enter", "open"}, [2]string{"a", "attach"}, [2]string{"f", "freeze"}, [2]string{"s", "snapshot"}, [2]string{"d", "diff"})
}

// workspacePage is the single-workspace graph/tree view.
type workspacePage struct{ base }

func newWorkspace() *workspacePage {
	return &workspacePage{base: base{route: ui.RouteWorkspace}}
}

func (p *workspacePage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *workspacePage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	p.setCatalog(workspaceIDsOf(c))
	if p.selectedID != "" && c != nil && c.WorkspaceByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *workspacePage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if p.regionTab(k) {
		return nil
	}
	ws := p.current()
	if ws == nil {
		return nil
	}
	ref := domain.Ref(domain.KindWorkspace, ws.ID)
	var surfaces []idLabel
	for _, w := range ws.Observed.Windows {
		for _, sf := range w.Surfaces {
			surfaces = append(surfaces, idLabel{id: sf.ID, label: sf.ID + " " + sf.Title})
		}
	}
	p.cursorKeys(k, surfaces)
	switch k.String() {
	case "enter":
		if !ws.Live(p.cat.Terminals) {
			return toast("No live Zellij session to attach; create or restore the session first", "warn")
		}
		return execute(domain.Action{Kind: domain.ActionAttach, Target: ref})
	case "n":
		terminal := firstLiveTerminal(p.cat, ws)
		if terminal == nil {
			return toast("A live Zellij terminal is required before creating another pane", "warn")
		}
		return execute(domain.Action{Kind: domain.ActionSplit, Target: domain.Ref(domain.KindTerminal, terminal.ID)})
	case "l":
		if len(ws.TerminalIDs) == 0 {
			return toast("no terminal to link", "warn")
		}
		return openPrompt("Link terminal "+ws.TerminalIDs[0]+" to workspace", "workspace id, e.g. ecp",
			func(v string) tea.Cmd {
				return execute(domain.Action{Kind: domain.ActionLink,
					Target: domain.Ref(domain.KindTerminal, ws.TerminalIDs[0]), Value: v})
			})
	case "h":
		return pushRoute(ui.Location{Route: ui.RouteHistory, Context: ref}, false)
	}
	return nil
}

func (p *workspacePage) current() *domain.Workspace {
	if p.cat == nil {
		return nil
	}
	return p.cat.WorkspaceByID(p.selected())
}

func firstLiveTerminal(catalog *domain.Catalog, workspace *domain.Workspace) *domain.Terminal {
	if catalog == nil || workspace == nil {
		return nil
	}
	for _, id := range workspace.TerminalIDs {
		if terminal := catalog.TerminalByID(id); terminal != nil && terminal.State == domain.TerminalLive {
			return terminal
		}
	}
	return nil
}

func (p *workspacePage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.WorkspaceByID(p.selected()) == nil {
		return p.notFoundView(domain.KindWorkspace, p.selected())
	}
	ws := p.current()
	if ws == nil {
		return p.emptyView("workspaces")
	}

	var b strings.Builder
	b.WriteString(p.titleBar("Workspace · " + ws.Name))
	b.WriteString("  " + p.styles.Dim.Render(ws.ID))
	b.WriteString("\n")

	cardWidth := max(12, (p.width-pageGap*3)/4)
	b.WriteString(horizontalCards([]string{
		p.metricCard("state", strings.ToUpper(string(ws.Status)), "desired vs observed", cardWidth),
		p.metricCard("machine", ws.MachineID, "execution host", cardWidth),
		p.metricCard("backend", ws.BackendID, "runtime adapter", cardWidth),
		p.metricCard("activity", ws.LastFocused.UTC().Format("15:04:05"), fmt.Sprintf("%d terminal(s)", len(ws.TerminalIDs)), cardWidth),
	}))
	b.WriteString("\n")

	tree := p.treeView(ws)
	detail := p.detailView(ws)
	if p.isWide() {
		b.WriteString(p.panelSplitHeight("Observed topology", tree, "Workspace detail", detail, 64, p.remainingHeight(b.String(), 10)))
	} else if p.region == 0 {
		b.WriteString(p.panel("Observed topology", tree, p.width))
	} else {
		b.WriteString(p.panel("Workspace detail", detail, p.width))
	}
	return p.clip(b.String())
}

func (p *workspacePage) treeView(ws *domain.Workspace) string {
	var b strings.Builder
	b.WriteString(p.styles.Dim.Render("OBSERVED RUNTIME") + "\n")
	for wi, window := range ws.Observed.Windows {
		b.WriteString(p.styles.Terminal.Render("▣ "+window.ID) + p.styles.Dim.Render("  "+window.Layout))
		b.WriteString("\n")
		for _, sf := range window.Surfaces {
			marker := "├─"
			if sf.ID == ws.SelectedSurfaceID {
				marker = "└▶"
			}
			term := p.cat.TerminalBySurface(sf.ID)
			owner := p.styles.Dim.Render("unbound")
			if term != nil {
				owner = p.styles.Terminal.Render(term.ID + "  " + string(term.State))
			}
			b.WriteString(fmt.Sprintf("  %s %-10s %-22s %s", marker, sf.ID, sf.Title, owner))
			b.WriteString("\n")
		}
		if wi < len(ws.Observed.Windows)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n" + p.styles.Dim.Render("DESIRED DEFINITION") + "\n")
	for _, window := range ws.Desired.Windows {
		b.WriteString(p.styles.Accent.Render("◇ "+window.ID) + p.styles.Dim.Render("  "+window.Layout) + "\n")
		for _, sf := range window.Surfaces {
			b.WriteString(fmt.Sprintf("  └─ %-10s %s\n", sf.ID, sf.Title))
		}
	}
	if ws.Status == domain.WorkspaceDrift {
		b.WriteString("\n" + p.status("DRIFT") + "  desired topology differs\n")
		for _, drift := range ws.Drift {
			b.WriteString("  + " + p.styles.Dim.Render(drift.Field) + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (p *workspacePage) detailView(ws *domain.Workspace) string {
	var b strings.Builder
	b.WriteString(p.keyValue("status", strings.ToUpper(string(ws.Status))))
	b.WriteString(p.keyValue("machine", ws.MachineID))
	b.WriteString(p.keyValue("backend", ws.BackendID))
	b.WriteString(p.keyValue("terminals", fmt.Sprint(len(ws.TerminalIDs))))
	b.WriteString(p.keyValue("agents", fmt.Sprint(len(ws.AgentIDs))))
	b.WriteString(p.keyValue("focused", ws.LastFocused.UTC().Format("15:04:05")))
	b.WriteString("\n")
	if len(ws.Drift) == 0 {
		b.WriteString(p.styles.OK.Render("✓ Desired state matches observed"))
	} else {
		b.WriteString(p.styles.Warn.Render(fmt.Sprintf("%d drifted field(s)", len(ws.Drift))))
		for _, drift := range ws.Drift {
			b.WriteString("\n\n")
			b.WriteString(p.styles.Header.Render(drift.Field) + "\n")
			b.WriteString(p.styles.Dim.Render("desired  ") + drift.Desired + "\n")
			b.WriteString(p.styles.Dim.Render("observed ") + drift.Observed)
		}
	}

	live := firstLiveTerminal(p.cat, ws)
	b.WriteString("\n\n" + p.styles.Dim.Render("SESSION ACTIONS"))
	if live == nil {
		b.WriteString("\n" + p.styles.Warn.Render("No live Zellij session · attach and new pane unavailable"))
	} else {
		b.WriteString("\n" + p.styles.Accent.Render("ENTER  Attach to Zellij session"))
		b.WriteString("\n" + p.styles.Terminal.Render("N      New terminal pane in the current tab"))
		b.WriteString("\n" + p.styles.Dim.Render("tmh-next suspends while Zellij owns the terminal; detach to return."))
	}
	b.WriteString("\n\n" + p.styles.Dim.Render("ATTACHED RESOURCES"))
	for _, id := range ws.TerminalIDs {
		if term := p.cat.TerminalByID(id); term != nil {
			b.WriteString(fmt.Sprintf("\n%s  %-10s %s",
				p.styles.Terminal.Render("▣ "+term.Name),
				strings.ToUpper(string(term.State)),
				p.styles.Dim.Render(term.Process)))
		}
	}
	for _, id := range ws.AgentIDs {
		if agent := p.cat.AgentByID(id); agent != nil {
			b.WriteString(fmt.Sprintf("\n%s  %-12s %3d%%",
				p.styles.Agent.Render("◆ "+agent.Name),
				strings.ToUpper(string(agent.State)),
				agent.Progress))
		}
	}
	return b.String()
}

func (p *base) keyValue(k, v string) string {
	return fmt.Sprintf("%s %s\n", p.styles.KeyValue.Render(fmt.Sprintf("%-10s", k)), v)
}

func (p *workspacePage) Commands() []ui.Command {
	ws := p.current()
	if ws == nil {
		return nil
	}
	ref := domain.Ref(domain.KindWorkspace, ws.ID)
	live := firstLiveTerminal(p.cat, ws)
	return []ui.Command{
		{ID: "w.attach", Title: "Attach Zellij session", Description: "suspend tmh-next, enter the session, and return after detach", Shortcut: "enter",
			Disabled: live == nil, DisabledReason: "workspace has no live Zellij session",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionAttach, Target: ref}) }},
		{ID: "w.new-terminal", Title: "New terminal pane", Description: "create a right-hand shell pane in the current Zellij tab", Shortcut: "n",
			Disabled: live == nil, DisabledReason: "a live terminal is required to locate the Zellij tab",
			Run: func() tea.Cmd {
				return execute(domain.Action{Kind: domain.ActionSplit, Target: domain.Ref(domain.KindTerminal, live.ID)})
			}},
		{ID: "w.history", Title: "Scoped history", Description: "history for this workspace terminals", Shortcut: "h",
			Run: func() tea.Cmd { return pushRoute(ui.Location{Route: ui.RouteHistory, Context: ref}, false) }},
	}
}

func (p *workspacePage) Help() []key.Binding {
	return pageHelp([2]string{"enter", "attach session"}, [2]string{"n", "new terminal"}, [2]string{"l", "link"}, [2]string{"h", "history"}, [2]string{"tab", "region"})
}
