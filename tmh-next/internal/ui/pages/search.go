package pages

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// searchPage is the focused global query with scope cycling and grouped hits.
type searchPage struct {
	base
	input   textinput.Model
	scope   string // all|history|events|agents
	results ui.SearchResultsMsg
}

var searchScopes = []string{"all", "history", "events", "agents"}

func newSearch() *searchPage {
	in := textinput.New()
	in.Placeholder = "search workspaces, terminals, agents, history, events…"
	in.Prompt = "› "
	in.SetWidth(48)
	in.Focus()
	return &searchPage{base: base{route: ui.RouteSearch, mode: ui.ModeText}, input: in, scope: "all"}
}

func (p *searchPage) Enter(loc ui.Location) tea.Cmd {
	p.enter(loc)
	p.mode = ui.ModeText
	p.input.Focus()
	if loc.Query != "" {
		p.input.SetValue(loc.Query)
		return func() tea.Msg { return ui.SearchRequestMsg{Text: loc.Query, Scope: p.scope} }
	}
	return nil
}

func (p *searchPage) SetCatalog(c *domain.Catalog) { p.cat = c }

func (p *searchPage) InputMode() ui.InputMode {
	if p.mode == ui.ModeNormal {
		return ui.ModeText // search owns every key while focused
	}
	return p.mode
}

func (p *searchPage) Update(msg tea.Msg) tea.Cmd {
	if res, ok := msg.(ui.SearchResultsMsg); ok {
		p.results = res
		return nil
	}
	if _, ok := msg.(scopeCycleMsg); ok {
		p.cycleScope()
		return p.requery()
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd
	}
	switch k.String() {
	case "esc":
		return func() tea.Msg { return ui.PopRouteMsg{} }
	case "tab":
		p.cycleScope()
		return p.requery()
	case "enter":
		return p.jumpSelected()
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(k)
	return tea.Batch(cmd, func() tea.Msg {
		return ui.SearchRequestMsg{Text: p.input.Value(), Scope: p.scope}
	})
}

func (p *searchPage) cycleScope() {
	for i, s := range searchScopes {
		if s == p.scope {
			if i+1 < len(searchScopes) {
				p.scope = searchScopes[i+1]
			} else {
				p.scope = searchScopes[0]
			}
			return
		}
	}
	p.scope = searchScopes[0]
}

func (p *searchPage) requery() tea.Cmd {
	return func() tea.Msg { return ui.SearchRequestMsg{Text: p.input.Value(), Scope: p.scope} }
}

func (p *searchPage) jumpSelected() tea.Cmd {
	hits := p.grouped()
	if len(hits) == 0 {
		return nil
	}
	if p.cursor >= len(hits) {
		p.cursor = len(hits) - 1
	}
	hit := hits[p.cursor]
	switch hit.Ref.Kind {
	case domain.KindWorkspace:
		return pushRoute(ui.Location{Route: ui.RouteWorkspace, Primary: hit.Ref}, false)
	case domain.KindTerminal:
		return pushRoute(ui.Location{Route: ui.RouteTerminals, Primary: hit.Ref}, false)
	case domain.KindAgent:
		return pushRoute(ui.Location{Route: ui.RouteAgents, Primary: hit.Ref}, false)
	case domain.KindHistory:
		if idx := strings.LastIndexByte(hit.Title, ' '); idx > 0 {
			rest := hit.Title[idx+1:]
			if ci := strings.IndexByte(rest, ':'); ci > 0 {
				kind := domain.ResourceKind(rest[:ci])
				if kind == domain.KindTerminal || kind == domain.KindAgent {
					return pushRoute(ui.Location{Route: ui.RouteHistory,
						Context: domain.Ref(kind, rest[ci+1:])}, false)
				}
			}
		}
	case domain.KindEvent:
		return pushRoute(ui.Location{Route: ui.RouteEvents, Primary: hit.Ref}, false)
	}
	return nil
}

func (p *searchPage) grouped() []domain.SearchHit { return p.results.Hits }

func (p *searchPage) View() string {
	var b strings.Builder
	b.WriteString(p.titleBar("Search"))
	b.WriteString("\n")
	b.WriteString(p.input.View())
	scope := theme.PaintBackground("scope: "+p.scope+" (tab)", p.styles.Palette.Crust)
	b.WriteString("  " + p.styles.Chip.Render(scope))
	b.WriteString("\n")
	if p.input.Value() == "" {
		b.WriteString(p.styles.Dim.Render("type to query the control-plane catalog — results group by scope"))
		return p.clip(b.String())
	}
	hits := p.grouped()
	if len(hits) == 0 {
		b.WriteString(p.styles.Dim.Render("no hits"))
		return p.clip(b.String())
	}
	lastScope := ""
	for i, h := range hits {
		if h.Scope != lastScope {
			b.WriteString(p.styles.Title.Render("— " + h.Scope + " —"))
			b.WriteString("\n")
			lastScope = h.Scope
		}
		text := h.Title + "  " + p.styles.Dim.Render(h.Snippet)
		b.WriteString(p.row(i, text))
		b.WriteString("\n")
	}
	return p.clip(b.String())
}

func (p *searchPage) Commands() []ui.Command {
	return []ui.Command{
		{ID: "s.scope", Title: "Cycle scope", Description: "all → history → events → agents", Shortcut: "tab",
			Run: func() tea.Cmd { return func() tea.Msg { return scopeCycleMsg{} } }},
		{ID: "s.jump", Title: "Jump to hit", Description: "open the selected hit's context", Shortcut: "enter",
			Run: func() tea.Cmd { return p.jumpSelected() }},
	}
}

type scopeCycleMsg struct{}

func (p *searchPage) Help() []key.Binding {
	return pageHelp([2]string{"type", "query"}, [2]string{"tab", "scope"}, [2]string{"enter", "jump"})
}

// snapshotsPage lists snapshots with tags and selected diff.
type snapshotsPage struct{ base }

func newSnapshots() *snapshotsPage {
	return &snapshotsPage{base: base{route: ui.RouteSnapshots}}
}

func (p *snapshotsPage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *snapshotsPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	ids := make([]string, 0, len(c.Snapshots))
	for _, s := range c.Snapshots {
		ids = append(ids, s.ID)
	}
	p.setCatalog(sortedIDs(ids))
	if p.selectedID != "" && c.SnapshotByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *snapshotsPage) rows() []idLabel {
	if p.cat == nil {
		return nil
	}
	rows := make([]idLabel, 0, len(p.cat.Snapshots))
	for _, s := range p.cat.Snapshots {
		rows = append(rows, idLabel{id: s.ID, label: s.ID + " " + s.WorkspaceID + " " + strings.Join(s.Tags, ",")})
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].id < rows[j-1].id; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

func (p *snapshotsPage) current() *domain.Snapshot {
	if p.cat == nil {
		return nil
	}
	return p.cat.SnapshotByID(p.selected())
}

func (p *snapshotsPage) Update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(tagSubmittedMsg); ok {
		return nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	rows := p.rows()
	if p.cursorKeys(k, rows) {
		return nil
	}
	s := p.current()
	if s == nil {
		switch k.String() {
		case "n":
			return openPrompt("Snapshot all workspaces", "type all to snapshot every workspace",
				func(v string) tea.Cmd { return execute(domain.Action{Kind: domain.ActionSnapshotCreate, Value: v}) })
		}
		return nil
	}
	ref := domain.Ref(domain.KindSnapshot, s.ID)
	switch k.String() {
	case "n":
		return execute(domain.Action{Kind: domain.ActionSnapshotCreate, Value: s.WorkspaceID})
	case "t":
		return openPrompt("Tag snapshot "+s.ID, "tag, e.g. pre-migration",
			func(v string) tea.Cmd {
				return execute(domain.Action{Kind: domain.ActionSnapshotTag, Target: ref, Value: v})
			})
	case "x":
		return confirm("Delete snapshot "+s.ID, "snapshot and dependent plans are removed",
			domain.Action{Kind: domain.ActionSnapshotDelete, Target: ref})
	case "r":
		return tea.Batch(
			execute(domain.Action{Kind: domain.ActionSnapshotPlan, Target: ref}),
			pushRoute(ui.Location{Route: ui.RouteReconcile}, false),
		)
	}
	return nil
}

type tagSubmittedMsg struct{}

func (p *snapshotsPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.SnapshotByID(p.selected()) == nil {
		return p.notFoundView(domain.KindSnapshot, p.selected())
	}
	var b strings.Builder
	b.WriteString(p.titleBar("Snapshots"))
	b.WriteString("\n")
	rows := p.rows()
	if len(rows) == 0 {
		b.WriteString(p.emptyView("snapshots"))
		b.WriteString(p.styles.Dim.Render("\nn — snapshot all workspaces"))
		return p.clip(b.String())
	}
	list := p.listView()
	detail := p.detailView()
	if p.isWide() {
		b.WriteString(p.panelSplitHeight("Snapshot catalog", list, "Selection", detail, 58, p.remainingHeight(b.String(), 10)))
	} else if p.region == 0 {
		b.WriteString(list)
	} else {
		b.WriteString(detail)
	}
	return p.clip(b.String())
}

func (p *snapshotsPage) listView() string {
	var b strings.Builder
	for i, r := range p.rows() {
		s := p.cat.SnapshotByID(r.id)
		if s == nil {
			continue
		}
		tags := strings.Join(s.Tags, ",")
		if tags == "" {
			tags = "—"
		}
		text := fmt.Sprintf("%-10s %-12s %-18s %s", s.ID, s.WorkspaceID, tags,
			p.styles.Dim.Render(s.CreatedAt.UTC().Format("01-02 15:04")))
		b.WriteString(p.row(i, text))
		b.WriteString("\n")
	}
	return p.clip(b.String())
}

func (p *snapshotsPage) detailView() string {
	s := p.current()
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(p.styles.Title.Render("Snapshot " + s.ID))
	b.WriteString("\n")
	b.WriteString(p.keyValue("workspace", s.WorkspaceID))
	b.WriteString(p.keyValue("backend", s.BackendID))
	b.WriteString(p.keyValue("tags", strings.Join(s.Tags, ", ")))
	b.WriteString(p.keyValue("windows", fmt.Sprint(s.WindowCount)))
	b.WriteString(p.keyValue("surfaces", fmt.Sprint(s.SurfaceCount)))
	b.WriteString(p.keyValue("created", s.CreatedAt.UTC().Format("15:04:05")))
	if ws := p.cat.WorkspaceByID(s.WorkspaceID); ws != nil {
		same := ws.Observed.Equal(s.State)
		if same {
			b.WriteString(p.status("OK") + " observed matches snapshot\n")
		} else {
			b.WriteString(p.status("DRIFT") + " observed differs from snapshot\n")
		}
	}
	return p.clip(b.String())
}

func (p *snapshotsPage) Commands() []ui.Command {
	s := p.current()
	var commands []ui.Command
	if s != nil {
		ref := domain.Ref(domain.KindSnapshot, s.ID)
		commands = append(commands,
			ui.Command{ID: "sn.tag", Title: "Tag", Description: "add a tag", Shortcut: "t",
				Run: func() tea.Cmd {
					return openPrompt("Tag snapshot "+s.ID, "tag, e.g. pre-migration",
						func(v string) tea.Cmd {
							return execute(domain.Action{Kind: domain.ActionSnapshotTag, Target: ref, Value: v})
						})
				}},
			ui.Command{ID: "sn.delete", Title: "Delete", Description: "remove snapshot (confirm)", Shortcut: "x",
				Run: func() tea.Cmd {
					return confirm("Delete snapshot "+s.ID, "snapshot and dependent plans are removed",
						domain.Action{Kind: domain.ActionSnapshotDelete, Target: ref})
				}},
			ui.Command{ID: "sn.plan", Title: "Create restore plan", Description: "plan → Reconcile", Shortcut: "r",
				Run: func() tea.Cmd {
					return tea.Batch(
						execute(domain.Action{Kind: domain.ActionSnapshotPlan, Target: ref}),
						pushRoute(ui.Location{Route: ui.RouteReconcile}, false),
					)
				}},
		)
	}
	commands = append(commands, ui.Command{
		ID: "sn.create", Title: "Snapshot workspace", Description: "create a durable snapshot", Shortcut: "n",
		Run: func() tea.Cmd {
			if s != nil {
				return execute(domain.Action{Kind: domain.ActionSnapshotCreate, Value: s.WorkspaceID})
			}
			return openPrompt("Snapshot all workspaces", "type all to snapshot every workspace",
				func(v string) tea.Cmd { return execute(domain.Action{Kind: domain.ActionSnapshotCreate, Value: v}) })
		},
	})
	return commands
}

func (p *snapshotsPage) Help() []key.Binding {
	return pageHelp([2]string{"n", "new"}, [2]string{"t", "tag"}, [2]string{"x", "delete"}, [2]string{"r", "plan"}, [2]string{"tab", "region"})
}

// reconcilePage applies restore plans with mode tabs, include/exclude, apply
// and field-level undo.
type reconcilePage struct {
	base
	mode     domain.RestoreMode
	excluded map[string]bool // op IDs excluded from the next apply (page-local)
}

func newReconcile() *reconcilePage {
	return &reconcilePage{base: base{route: ui.RouteReconcile}, mode: domain.RestorePush, excluded: map[string]bool{}}
}

func (p *reconcilePage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *reconcilePage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	ids := make([]string, 0, len(c.RestorePlans))
	for _, pl := range c.RestorePlans {
		ids = append(ids, pl.ID)
	}
	p.setCatalog(sortedIDs(ids))
	if p.selectedID != "" && c.RestorePlanByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *reconcilePage) current() *domain.RestorePlan {
	if p.cat == nil {
		return nil
	}
	return p.cat.RestorePlanByID(p.selected())
}

func (p *reconcilePage) Update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(includeToggleMsg); ok {
		p.toggleInclude()
		return nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "tab", "shift+tab":
		switch p.mode {
		case domain.RestorePush:
			p.mode = domain.RestorePull
		case domain.RestorePull:
			p.mode = domain.RestoreFreeze
		default:
			p.mode = domain.RestorePush
		}
		return nil
	}
	rows := p.rows()
	if p.cursorKeys(k, rows) {
		return nil
	}
	plan := p.current()
	if plan == nil {
		return nil
	}
	ref := domain.Ref(domain.KindPlan, plan.ID)
	switch k.String() {
	case "a":
		return confirm("Apply plan "+plan.ID+" ("+string(p.mode)+")",
			"applies the included "+string(p.mode)+" operations to workspace "+plan.WorkspaceID,
			domain.Action{Kind: domain.ActionRestoreApply, Target: ref, Value: string(p.mode), Selection: p.includedIDs()})
	case "u":
		return execute(domain.Action{Kind: domain.ActionReconcileUndo, Target: ref})
	}
	return nil
}

type includeToggleMsg struct{}

// toggleInclude flips the page-local exclusion flag of the selected op. The
// catalog stays read-only; the apply action carries the included IDs in
// Action.Selection.
func (p *reconcilePage) toggleInclude() {
	plan := p.current()
	if plan == nil {
		return
	}
	ops := p.modeOps(plan)
	if p.cursor < 0 || p.cursor >= len(ops) {
		return
	}
	id := ops[p.cursor].ID
	if p.excluded[id] {
		delete(p.excluded, id)
	} else {
		p.excluded[id] = true
	}
}

// includedIDs returns the op IDs of the current mode that would apply.
func (p *reconcilePage) includedIDs() []string {
	plan := p.current()
	if plan == nil {
		return nil
	}
	var ids []string
	for _, op := range p.modeOps(plan) {
		if !p.excluded[op.ID] {
			ids = append(ids, op.ID)
		}
	}
	return ids
}

func (p *reconcilePage) modeOps(plan *domain.RestorePlan) []domain.RestoreOp {
	var out []domain.RestoreOp
	for _, op := range plan.Ops {
		if op.Mode == p.mode {
			out = append(out, op)
		}
	}
	return out
}

type opRow struct {
	idLabel
	op domain.RestoreOp
}

func (p *reconcilePage) rows() []idLabel {
	plan := p.current()
	if plan == nil {
		return nil
	}
	var rows []idLabel
	for _, op := range p.modeOps(plan) {
		rows = append(rows, idLabel{id: op.ID, label: op.Field})
	}
	return rows
}

func (p *reconcilePage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.RestorePlanByID(p.selected()) == nil {
		return p.notFoundView(domain.KindPlan, p.selected())
	}
	var b strings.Builder
	b.WriteString(p.titleBar("Reconcile"))
	if len(p.cat.RestorePlans) == 0 {
		b.WriteString("\n" + p.emptyView("restore plan"))
		b.WriteString("\n" + p.styles.Dim.Render("create one from Snapshots → r (plan)"))
		return p.clip(b.String())
	}
	plan := p.current()
	if plan == nil {
		return p.clip(b.String()) + "\n" + p.emptyView("restore plan")
	}
	b.WriteString("  " + p.styles.Dim.Render("plan "+plan.ID+" · snapshot "+plan.SnapshotID+" · workspace "+plan.WorkspaceID))
	b.WriteString(" " + p.status(planStatus(plan.Status)))
	b.WriteString("\n")

	modes := []domain.RestoreMode{domain.RestorePush, domain.RestorePull, domain.RestoreFreeze}
	var tabs []string
	for _, m := range modes {
		label := string(m)
		if m == p.mode {
			label = p.styles.Select(strings.ToUpper(label))
		}
		tabs = append(tabs, label)
	}
	b.WriteString("  " + strings.Join(tabs, " · ") + p.styles.Dim.Render("  (tab)"))
	b.WriteString("\n")

	ops := p.modeOps(plan)
	if len(ops) == 0 {
		b.WriteString(p.emptyView(string(p.mode) + " operations"))
		return p.clip(b.String())
	}
	for i, op := range ops {
		box := "[x]"
		if p.excluded[op.ID] {
			box = "[ ]"
		}
		before := op.Before
		if op.BeforeAbsent {
			before = "absent"
		}
		after := op.After
		if after == "" {
			after = "absent"
		}
		text := fmt.Sprintf("%s %-34s %s → %s", box, op.Field,
			p.styles.Dim.Render(before), p.styles.Info.Render(after))
		b.WriteString(p.row(i, text))
		b.WriteString("\n")
	}
	b.WriteString(p.styles.Dim.Render("  a apply (confirm) · u undo · space → include/exclude"))
	return p.clip(b.String())
}

func planStatus(s domain.RestorePlanStatus) string {
	switch s {
	case domain.PlanApplied:
		return "APPLIED"
	case domain.PlanUndone:
		return "OK"
	default:
		return "PENDING"
	}
}

func (p *reconcilePage) Commands() []ui.Command {
	plan := p.current()
	if plan == nil {
		return nil
	}
	ref := domain.Ref(domain.KindPlan, plan.ID)
	return []ui.Command{
		{ID: "rc.include", Title: "Include / exclude op", Description: "toggle the selected operation", Shortcut: "",
			Run: func() tea.Cmd { return func() tea.Msg { return includeToggleMsg{} } }},
		{ID: "rc.apply", Title: "Apply plan", Description: "apply included " + string(p.mode) + " ops (confirm)", Shortcut: "a",
			Disabled: plan.Status != domain.PlanPending, DisabledReason: "plan is not pending",
			Run: func() tea.Cmd {
				return confirm("Apply plan "+plan.ID+" ("+string(p.mode)+")",
					"applies the included "+string(p.mode)+" operations to workspace "+plan.WorkspaceID,
					domain.Action{Kind: domain.ActionRestoreApply, Target: ref, Value: string(p.mode), Selection: p.includedIDs()})
			}},
		{ID: "rc.undo", Title: "Undo apply", Description: "field-level inverse of the last apply", Shortcut: "u",
			Disabled: plan.Status != domain.PlanApplied, DisabledReason: "plan was not applied",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionReconcileUndo, Target: ref}) }},
	}
}

func (p *reconcilePage) Help() []key.Binding {
	return pageHelp([2]string{"tab", "mode"}, [2]string{"a", "apply"}, [2]string{"u", "undo"})
}
