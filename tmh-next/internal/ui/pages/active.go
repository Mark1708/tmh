package pages

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
)

// activePage lists shared/recent/pinned views with TTL and prune flows.
type activePage struct{ base }

func newActive() *activePage { return &activePage{base: base{route: ui.RouteActive}} }

func (p *activePage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *activePage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	ids := make([]string, 0, len(c.ActiveViews))
	for _, v := range c.ActiveViews {
		ids = append(ids, v.ID)
	}
	p.setCatalog(sortedIDs(ids))
	if p.selectedID != "" && c.ActiveViewByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *activePage) rows() []idLabel {
	if p.cat == nil {
		return nil
	}
	rows := make([]idLabel, 0, len(p.cat.ActiveViews))
	for _, v := range p.cat.ActiveViews {
		rows = append(rows, idLabel{id: v.ID, label: v.ID + " " + v.Title})
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].id < rows[j-1].id; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

func (p *activePage) current() *domain.ActiveView {
	if p.cat == nil {
		return nil
	}
	return p.cat.ActiveViewByID(p.selected())
}

func (p *activePage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	rows := p.rows()
	if p.cursorKeys(k, rows) {
		return nil
	}
	v := p.current()
	if v == nil {
		return nil
	}
	ref := domain.Ref(domain.KindActive, v.ID)
	switch k.String() {
	case "t":
		return execute(domain.Action{Kind: domain.ActionTouch, Target: ref})
	case "x":
		return confirm("Prune view "+v.Title, "remove the expired unpinned view",
			domain.Action{Kind: domain.ActionPrune, Target: ref})
	case "enter":
		return pushRoute(ui.Location{Route: ui.RouteWorkspace, Primary: domain.Ref(domain.KindWorkspace, v.WorkspaceID)}, false)
	}
	return nil
}

func (p *activePage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.ActiveViewByID(p.selected()) == nil {
		return p.notFoundView(domain.KindActive, p.selected())
	}
	var b strings.Builder
	b.WriteString(p.titleBar("Active views"))
	b.WriteString("\n")
	rows := p.rows()
	if len(rows) == 0 {
		b.WriteString(p.emptyView("active views"))
		return p.clip(b.String())
	}
	for i, r := range rows {
		v := p.cat.ActiveViewByID(r.id)
		expired := !v.Pinned && v.ExpiredAt(p.cat.Now).Before(p.cat.Now)
		ttl := fmt.Sprintf("ttl %ds", v.TTLSeconds)
		if v.Pinned {
			ttl = "pinned"
		} else if expired {
			ttl = p.status("OFFLINE") + " expired"
		}
		text := fmt.Sprintf("%-10s %-18s %-7s %-10s %s", v.ID, v.Title, string(v.Kind), v.Owner, ttl)
		b.WriteString(p.row(i, text))
		b.WriteString("\n")
	}
	return p.clip(b.String())
}

func (p *activePage) Commands() []ui.Command {
	v := p.current()
	if v == nil {
		return nil
	}
	ref := domain.Ref(domain.KindActive, v.ID)
	return []ui.Command{
		{ID: "av.touch", Title: "Touch", Description: "refresh last-used", Shortcut: "t",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionTouch, Target: ref}) }},
		{ID: "av.prune", Title: "Prune", Description: "remove expired unpinned view (confirm)", Shortcut: "x",
			Disabled: v.Pinned || !v.ExpiredAt(p.cat.Now).Before(p.cat.Now),
			DisabledReason: func() string {
				if v.Pinned {
					return "view is pinned"
				}
				return "view has not expired yet"
			}(),
			Run: func() tea.Cmd {
				return confirm("Prune view "+v.Title, "remove the expired unpinned view",
					domain.Action{Kind: domain.ActionPrune, Target: ref})
			}},
		{ID: "av.pin", Title: "Pin / Unpin", Description: "toggle pinned state", Shortcut: "",
			Run: func() tea.Cmd {
				kind := domain.ActionPin
				if v.Pinned {
					kind = domain.ActionUnpin
				}
				return execute(domain.Action{Kind: kind, Target: ref})
			}},
		{ID: "av.jump", Title: "Open workspace", Description: "jump to the owning workspace", Shortcut: "enter",
			Run: func() tea.Cmd {
				return pushRoute(ui.Location{Route: ui.RouteWorkspace, Primary: domain.Ref(domain.KindWorkspace, v.WorkspaceID)}, false)
			}},
	}
}

func (p *activePage) Help() []key.Binding {
	return pageHelp([2]string{"t", "touch"}, [2]string{"x", "prune"}, [2]string{"enter", "jump"})
}

// terminalsPage shows backend/process/CWD/capabilities with preview.
type terminalsPage struct{ base }

func newTerminals() *terminalsPage { return &terminalsPage{base: base{route: ui.RouteTerminals}} }

func (p *terminalsPage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *terminalsPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	ids := make([]string, 0, len(c.Terminals))
	for _, t := range c.Terminals {
		ids = append(ids, t.ID)
	}
	p.setCatalog(sortedIDs(ids))
	if p.selectedID != "" && c.TerminalByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *terminalsPage) rows() []idLabel {
	if p.cat == nil {
		return nil
	}
	rows := make([]idLabel, 0, len(p.cat.Terminals))
	for _, t := range p.cat.Terminals {
		rows = append(rows, idLabel{id: t.ID, label: t.ID + " " + t.Name + " " + string(t.State)})
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].id < rows[j-1].id; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

func (p *terminalsPage) current() *domain.Terminal {
	if p.cat == nil {
		return nil
	}
	return p.cat.TerminalByID(p.selected())
}

func (p *terminalsPage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if p.regionTab(k) {
		return nil
	}
	rows := p.rows()
	if p.cursorKeys(k, rows) {
		return nil
	}
	t := p.current()
	if t == nil {
		return nil
	}
	ref := domain.Ref(domain.KindTerminal, t.ID)
	switch k.String() {
	case "enter":
		if t.State != domain.TerminalLive {
			return toast("Terminal "+t.Name+" has exited; choose a live terminal to attach", "warn")
		}
		return execute(domain.Action{Kind: domain.ActionAttach, Target: ref})
	case "s":
		if t.State != domain.TerminalLive {
			return toast("Terminal "+t.Name+" has exited; input is unavailable", "warn")
		}
		return openPrompt("Send input to "+t.Name, "input, e.g. make test",
			func(v string) tea.Cmd { return execute(domain.Action{Kind: domain.ActionSend, Target: ref, Value: v}) })
	case "l":
		return openPrompt("Link "+t.Name+" to workspace", "workspace id, e.g. kb",
			func(v string) tea.Cmd { return execute(domain.Action{Kind: domain.ActionLink, Target: ref, Value: v}) })
	case "x":
		if t.State != domain.TerminalLive {
			return toast("Terminal "+t.Name+" has already exited", "warn")
		}
		return confirm("Close terminal pane "+t.Name, "the pane exits and dependent agents detach",
			domain.Action{Kind: domain.ActionKill, Target: ref})
	case "h":
		return pushRoute(ui.Location{Route: ui.RouteHistory, Context: ref}, false)
	}
	return nil
}

func (p *terminalsPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.TerminalByID(p.selected()) == nil {
		return p.notFoundView(domain.KindTerminal, p.selected())
	}
	var b strings.Builder
	b.WriteString(p.titleBar("Terminals"))
	b.WriteString("\n")
	rows := p.rows()
	if len(rows) == 0 {
		b.WriteString(p.emptyView("terminals"))
		return p.clip(b.String())
	}
	list := p.listView()
	detail := p.detailView()
	if p.isWide() {
		b.WriteString(p.panelSplitHeight("Terminal fleet", list, "Selection", detail, 60, p.remainingHeight(b.String(), 10)))
	} else if p.region == 0 {
		b.WriteString(list)
	} else {
		b.WriteString(detail)
	}
	return p.clip(b.String())
}

func (p *terminalsPage) listView() string {
	var b strings.Builder
	for i, r := range p.rows() {
		t := p.cat.TerminalByID(r.id)
		if t == nil {
			continue
		}
		state := p.status("LIVE")
		if t.State == domain.TerminalExited {
			state = p.status("EXITED")
		}
		text := fmt.Sprintf("%-10s %-10s %-8s %-14s %s", t.ID, t.Name, state, t.Process, p.styles.Dim.Render(t.CWD))
		b.WriteString(p.row(i, text))
		b.WriteString("\n")
	}
	return p.clip(b.String())
}

func (p *terminalsPage) detailView() string {
	t := p.current()
	if t == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(p.styles.Title.Render("Terminal " + t.Name))
	b.WriteString("\n")
	b.WriteString(p.keyValue("id", t.ID))
	b.WriteString(p.keyValue("workspace", t.WorkspaceID))
	b.WriteString(p.keyValue("surface", t.SurfaceID))
	b.WriteString(p.keyValue("backend", t.BackendID))
	b.WriteString(p.keyValue("process", t.Process))
	b.WriteString(p.keyValue("cwd", t.CWD))
	b.WriteString(p.keyValue("capabilities", strings.Join(t.Capabilities, ",")))
	preview := "— no stream content —"
	if chunks := p.cat.HistoryForScope(domain.KindTerminal, t.ID); len(chunks) > 0 {
		last := chunks[len(chunks)-1]
		preview = strings.Join(last.Lines, " ⏎ ")
	}
	b.WriteString(p.keyValue("preview", preview))
	b.WriteString("\n")
	if t.State == domain.TerminalLive {
		b.WriteString(p.styles.Accent.Render("ENTER  Attach to Zellij session") + "\n")
		b.WriteString(p.styles.Dim.Render("tmh-next suspends while Zellij owns the terminal; detach to return."))
	} else {
		b.WriteString(p.styles.Warn.Render("This terminal has exited · attach and input are unavailable."))
	}
	return p.clip(b.String())
}

func (p *terminalsPage) Commands() []ui.Command {
	t := p.current()
	if t == nil {
		return nil
	}
	ref := domain.Ref(domain.KindTerminal, t.ID)
	live := t.State == domain.TerminalLive
	return []ui.Command{
		{ID: "t.attach", Title: "Attach Zellij session", Description: "suspend tmh-next and enter the session containing this pane", Shortcut: "enter",
			Disabled: !live, DisabledReason: "terminal has exited",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionAttach, Target: ref}) }},
		{ID: "t.send", Title: "Send input", Description: "write text to the live pane without attaching", Shortcut: "s",
			Disabled: !live, DisabledReason: "terminal has exited",
			Run: func() tea.Cmd {
				return openPrompt("Send input to "+t.Name, "input, e.g. make test",
					func(v string) tea.Cmd { return execute(domain.Action{Kind: domain.ActionSend, Target: ref, Value: v}) })
			}},
		{ID: "t.link", Title: "Link to workspace", Description: "move the terminal to another workspace", Shortcut: "l",
			Run: func() tea.Cmd {
				return openPrompt("Link "+t.Name+" to workspace", "workspace id, e.g. kb",
					func(v string) tea.Cmd { return execute(domain.Action{Kind: domain.ActionLink, Target: ref, Value: v}) })
			}},
		{ID: "t.kill", Title: "Close terminal pane", Description: "close the pane after confirmation", Shortcut: "x",
			Disabled: !live, DisabledReason: "terminal has already exited",
			Run: func() tea.Cmd {
				return confirm("Close terminal pane "+t.Name, "the pane exits and dependent agents detach",
					domain.Action{Kind: domain.ActionKill, Target: ref})
			}},
		{ID: "t.history", Title: "History", Description: "scoped history view", Shortcut: "h",
			Run: func() tea.Cmd { return pushRoute(ui.Location{Route: ui.RouteHistory, Context: ref}, false) }},
	}
}

func (p *terminalsPage) Help() []key.Binding {
	return pageHelp([2]string{"enter", "attach session"}, [2]string{"s", "send input"}, [2]string{"l", "link"}, [2]string{"x", "close pane"}, [2]string{"h", "history"}, [2]string{"tab", "region"})
}

// agentsPage is the agent state board with prompt/wait/claim.
type agentsPage struct{ base }

func newAgents() *agentsPage { return &agentsPage{base: base{route: ui.RouteAgents}} }

func (p *agentsPage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *agentsPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	ids := make([]string, 0, len(c.Agents))
	for _, a := range c.Agents {
		ids = append(ids, a.ID)
	}
	p.setCatalog(sortedIDs(ids))
	if p.selectedID != "" && c.AgentByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *agentsPage) rows() []idLabel {
	if p.cat == nil {
		return nil
	}
	rows := make([]idLabel, 0, len(p.cat.Agents))
	for _, a := range p.cat.Agents {
		rows = append(rows, idLabel{id: a.ID, label: a.ID + " " + a.Name + " " + string(a.State)})
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].id < rows[j-1].id; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

func (p *agentsPage) current() *domain.Agent {
	if p.cat == nil {
		return nil
	}
	return p.cat.AgentByID(p.selected())
}

func agentStateStatus(s domain.AgentState) string {
	switch s {
	case domain.AgentRunning:
		return "RUNNING"
	case domain.AgentNeedsInput:
		return "NEEDS INPUT"
	case domain.AgentWaiting:
		return "WAITING"
	default:
		return "OK"
	}
}

func (p *agentsPage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	rows := p.rows()
	if p.cursorKeys(k, rows) {
		return nil
	}
	a := p.current()
	if a == nil {
		return nil
	}
	ref := domain.Ref(domain.KindAgent, a.ID)
	switch k.String() {
	case "enter":
		if a.State == domain.AgentDone {
			return toast("agent is done", "warn")
		}
		return execute(domain.Action{Kind: domain.ActionAgentFocus, Target: ref})
	case "p":
		if a.State == domain.AgentDone {
			return toast("agent is done", "warn")
		}
		return openPrompt("Prompt agent "+a.Name, "instruction, e.g. summarize drift",
			func(v string) tea.Cmd {
				return execute(domain.Action{Kind: domain.ActionAgentPrompt, Target: ref, Value: v})
			})
	case "w":
		if a.State != domain.AgentRunning {
			return toast("agent is not running", "warn")
		}
		return execute(domain.Action{Kind: domain.ActionAgentWait, Target: ref})
	case "c":
		if a.ClaimedBy != "" {
			return toast("already claimed by "+a.ClaimedBy, "warn")
		}
		if a.State == domain.AgentDone {
			return toast("agent is done", "warn")
		}
		return execute(domain.Action{Kind: domain.ActionAgentClaim, Target: ref})
	}
	return nil
}

func (p *agentsPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.AgentByID(p.selected()) == nil {
		return p.notFoundView(domain.KindAgent, p.selected())
	}
	var b strings.Builder
	b.WriteString(p.titleBar("Agents"))
	b.WriteString("\n")
	rows := p.rows()
	if len(rows) == 0 {
		b.WriteString(p.emptyView("agents"))
		return p.clip(b.String())
	}
	board := p.boardView()
	detail := p.detailView()
	if p.isWide() {
		b.WriteString(p.panelSplitHeight("Agent board", board, "Selection", detail, 60, p.remainingHeight(b.String(), 10)))
	} else {
		b.WriteString(board)
	}
	return p.clip(b.String())
}

func (p *agentsPage) boardView() string {
	var b strings.Builder
	for i, r := range p.rows() {
		a := p.cat.AgentByID(r.id)
		if a == nil {
			continue
		}
		bar := p.styles.Bar.Render(strings.Repeat("█", a.Progress/10)) + p.styles.Dim.Render(strings.Repeat("·", 10-a.Progress/10))
		claim := ""
		if a.ClaimedBy != "" {
			claim = p.styles.Dim.Render(" @" + a.ClaimedBy)
		}
		text := fmt.Sprintf("%-10s %-15s %-10s %-10s %s%s", a.ID, a.Name,
			string(a.Kind), p.status(agentStateStatus(a.State)), bar, claim)
		b.WriteString(p.row(i, text))
		b.WriteString("\n")
	}
	return p.clip(b.String())
}

func (p *agentsPage) detailView() string {
	a := p.current()
	if a == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(p.styles.Title.Render("Agent " + a.Name))
	b.WriteString("\n")
	b.WriteString(p.keyValue("state", string(a.State)))
	b.WriteString(p.keyValue("kind", string(a.Kind)))
	b.WriteString(p.keyValue("workspace", a.WorkspaceID))
	//	b.WriteString(p.keyValue("terminal", orDash(a.TerminalID)))
	b.WriteString(p.keyValue("progress", fmt.Sprintf("%d%%", a.Progress)))
	b.WriteString(p.keyValue("claimed by", orDash(a.ClaimedBy)))
	b.WriteString("\n" + p.styles.Title.Render("Timeline") + "\n")
	for _, e := range a.Timeline {
		b.WriteString(fmt.Sprintf("  %s %s: %s\n", e.At.UTC().Format("15:04:05"), e.Kind, e.Text))
	}
	if len(a.Timeline) == 0 {
		b.WriteString(p.styles.Dim.Render("  no timeline entries"))
		b.WriteString("\n")
	}
	return p.clip(b.String())
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (p *agentsPage) Commands() []ui.Command {
	a := p.current()
	if a == nil {
		return nil
	}
	ref := domain.Ref(domain.KindAgent, a.ID)
	done := a.State == domain.AgentDone
	return []ui.Command{
		{ID: "ag.focus", Title: "Focus", Description: "focus the agent", Shortcut: "enter",
			Disabled: done, DisabledReason: "agent is done",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionAgentFocus, Target: ref}) }},
		{ID: "ag.prompt", Title: "Prompt", Description: "send an instruction", Shortcut: "p",
			Disabled: done, DisabledReason: "agent is done",
			Run: func() tea.Cmd {
				return openPrompt("Prompt agent "+a.Name, "instruction, e.g. summarize drift",
					func(v string) tea.Cmd {
						return execute(domain.Action{Kind: domain.ActionAgentPrompt, Target: ref, Value: v})
					})
			}},
		{ID: "ag.wait", Title: "Wait", Description: "pause for input", Shortcut: "w",
			Disabled: a.State != domain.AgentRunning, DisabledReason: "agent is not running",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionAgentWait, Target: ref}) }},
		{ID: "ag.claim", Title: "Claim", Description: "claim as demo-operator", Shortcut: "c",
			Disabled: a.ClaimedBy != "" || done, DisabledReason: func() string {
				if a.ClaimedBy != "" {
					return "already claimed by " + a.ClaimedBy
				}
				return "agent is done"
			}(),
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionAgentClaim, Target: ref}) }},
	}
}

func (p *agentsPage) Help() []key.Binding {
	return pageHelp([2]string{"enter", "focus"}, [2]string{"p", "prompt"}, [2]string{"w", "wait"}, [2]string{"c", "claim"})
}
