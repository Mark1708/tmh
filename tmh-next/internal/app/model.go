// Package app implements the tmh-next root shell: the typed route stack,
// resident pages, root-owned overlays, the serialized mutation lane and the
// search lane over a consumer-owned Backend.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/components"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// PageFactory builds resident pages per route at startup.
type PageFactory func(ui.Route) ui.Page

// Options configures the root shell.
type Options struct {
	Client      control.Client
	Watcher     control.Watcher
	DemoTicker  control.DemoTicker
	Scheduler   Scheduler
	Live        bool
	Stack       []ui.Location
	QuickSwitch bool
	Factory     PageFactory
}

type mutKind uint8

const (
	mutNone mutKind = iota
	mutAction
	mutTick
	mutReload
)

// mutator identifies the single in-flight mutation (action, tick or reload).
type mutator struct {
	kind     mutKind
	token    uint64
	identity string
}

type overlayKind uint8

const (
	ovNone overlayKind = iota
	ovPalette
	ovQuickSwitch
	ovSelector
	ovPrompt
	ovConfirm
	ovHelp
	ovConfig
)

type overlay struct {
	kind    overlayKind
	title   string
	sel     *components.Selector
	prompt  *components.Prompt
	confirm *components.Confirm

	helpCase    int
	helpStep    int
	helpPlaying bool
	helpToken   uint64

	confirmAction domain.Action
	confirmAccept func(domain.Action) tea.Cmd

	form        huh.Model
	formDone    bool
	formHarvest func() (domain.Config, bool)

	promptSubmit func(string) tea.Cmd
}

// Root is the Bubble Tea v2 root model.
type Root struct {
	client     control.Client
	watcher    control.Watcher
	demoTicker control.DemoTicker
	sched      Scheduler
	live       bool
	factory    PageFactory
	cat        *domain.Catalog
	pages      map[ui.Route]ui.Page
	stack      []ui.Location
	entered    map[ui.Route]bool

	width, height int
	tooSmall      bool
	ready         bool
	fatal         string
	eventSeq      uint64

	timerArmed  bool
	armedToken  uint64
	tickPending bool
	tokenSeq    uint64
	mut         mutator

	searchSeq     int
	searchText    string
	searchScope   string
	searchRetried map[int]bool

	ov           *overlay
	toasts       []components.Toast
	toastSeq     uint64
	helpTokenSeq uint64
	quickSwitch  bool

	styles theme.Styles
	dark   bool
	help   help.Model
}

// NewRoot builds the resident page set and the initial stack.
func NewRoot(opts Options) *Root {
	if opts.Scheduler == nil {
		opts.Scheduler = ProductionScheduler{Delay: 0}
	}
	r := &Root{
		client:        opts.Client,
		watcher:       opts.Watcher,
		demoTicker:    opts.DemoTicker,
		sched:         opts.Scheduler,
		live:          opts.Live,
		factory:       opts.Factory,
		pages:         map[ui.Route]ui.Page{},
		entered:       map[ui.Route]bool{},
		stack:         opts.Stack,
		styles:        theme.New(true),
		dark:          true,
		help:          help.New(),
		searchRetried: map[int]bool{},
	}
	r.quickSwitch = opts.QuickSwitch
	if len(r.stack) == 0 {
		r.stack = []ui.Location{{Route: ui.RouteDashboard}}
	}
	for _, route := range ui.AllRoutes() {
		if opts.Factory != nil {
			r.pages[route] = opts.Factory(route)
		}
	}
	return r
}

// Init loads the initial control-plane snapshot and queries terminal color.
func (r *Root) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			if r.client == nil {
				return LoadFailedMsg{Err: fmt.Errorf("runtime client is nil")}
			}
			snapshot, err := r.client.Snapshot(context.Background())
			if err != nil {
				return LoadFailedMsg{Err: err}
			}
			if err := snapshot.Validate(); err != nil {
				return LoadFailedMsg{Err: err}
			}
			return LoadCatalogMsg{Catalog: snapshot.Catalog, EventSeq: snapshot.EventSeq}
		},
		tea.RequestBackgroundColor,
	)
}

// activeLocation returns the top of the route stack.
func (r *Root) activeLocation() ui.Location {
	if len(r.stack) == 0 {
		return ui.Location{Route: ui.RouteDashboard}
	}
	return r.stack[len(r.stack)-1]
}

// activePage returns the page for the active location.
func (r *Root) activePage() ui.Page {
	return r.pages[r.activeLocation().Route]
}

// broadcastCatalog pushes a read-only snapshot to every resident page.
func (r *Root) broadcastCatalog() {
	if r.cat == nil {
		return
	}
	for _, route := range ui.AllRoutes() {
		if p := r.pages[route]; p != nil {
			p.SetCatalog(r.cat)
		}
	}
}

// propagateSize pushes the inset body size to every page. Root chrome owns a
// two-cell horizontal gutter and one body row above/below the page.
func (r *Root) propagateSize() {
	pageWidth := max(1, r.width-4)
	pageHeight := max(1, r.height-4)
	for _, route := range ui.AllRoutes() {
		if p := r.pages[route]; p != nil {
			p.SetSize(pageWidth, pageHeight)
		}
	}
}

// propagateTheme pushes styles to every page and the open overlay.
func (r *Root) propagateTheme() {
	for _, route := range ui.AllRoutes() {
		if p := r.pages[route]; p != nil {
			p.SetTheme(r.styles)
		}
	}
	if r.ov != nil && r.ov.sel != nil {
		r.ov.sel.SetTheme(r.styles)
	}
}

// armTimer arms the deterministic demo tick. Production clients receive
// state changes through the control-plane watch stream instead.
func (r *Root) armTimer() tea.Cmd {
	if !r.live || r.demoTicker == nil || r.timerArmed {
		return nil
	}
	r.timerArmed = true
	r.tokenSeq++
	r.armedToken = r.tokenSeq
	return r.sched.TimerCmd(r.armedToken)
}

// launchTick advances only the deterministic demo provider.
func (r *Root) launchTick() tea.Cmd {
	if r.demoTicker == nil {
		return nil
	}
	if r.mut.kind != mutNone {
		r.tickPending = true
		return nil
	}
	r.mut = mutator{kind: mutTick, token: r.tokenSeq}
	rev := r.currentRevision()
	return func() tea.Msg {
		res, err := r.demoTicker.Tick(context.Background(), rev)
		if err != nil {
			return MutationFailedMsg{IsTick: true, Identity: tickIdentity(rev), Err: err}
		}
		return MutationResultMsg{Result: res}
	}
}

func tickIdentity(rev uint64) string { return fmt.Sprintf("tick@%d", rev) }

func (r *Root) currentRevision() uint64 {
	if r.cat == nil {
		return 0
	}
	return r.cat.Revision
}
func (r *Root) isProduction() bool {
	return r.cat != nil && r.cat.Scenario == domain.Scenario("production")
}

// busyReason explains why a user mutation is rejected right now.
func (r *Root) busyReason() string {
	switch r.mut.kind {
	case mutAction:
		return "an action is in flight"
	case mutTick:
		return "a demo refresh is in flight"
	case mutReload:
		return "recovering from a revision conflict"
	default:
		return ""
	}
}

// resolveDefaults fills omitted primary refs per the route contract table.
// Explicit IDs are preserved verbatim — pages render Not found for them.
func (r *Root) resolveDefaults(loc ui.Location) ui.Location {
	if r.cat == nil || loc.Primary.ID != "" {
		return loc
	}
	firstID := func(ids []string) string {
		if len(ids) == 0 {
			return ""
		}
		sorted := append([]string(nil), ids...)
		sort.Strings(sorted)
		return sorted[0]
	}
	switch loc.Route {
	case ui.RouteWorkspace:
		if id := firstID(workspaceIDs(r.cat)); id != "" {
			loc.Primary = domain.Ref(domain.KindWorkspace, id)
		}
	case ui.RouteActive:
		ids := make([]string, 0, len(r.cat.ActiveViews))
		for _, v := range r.cat.ActiveViews {
			ids = append(ids, v.ID)
		}
		if id := firstID(ids); id != "" {
			loc.Primary = domain.Ref(domain.KindActive, id)
		}
	case ui.RouteTerminals:
		ids := make([]string, 0, len(r.cat.Terminals))
		for _, t := range r.cat.Terminals {
			ids = append(ids, t.ID)
		}
		if id := firstID(ids); id != "" {
			loc.Primary = domain.Ref(domain.KindTerminal, id)
		}
	case ui.RouteAgents:
		ids := make([]string, 0, len(r.cat.Agents))
		for _, a := range r.cat.Agents {
			ids = append(ids, a.ID)
		}
		if id := firstID(ids); id != "" {
			loc.Primary = domain.Ref(domain.KindAgent, id)
		}
	case ui.RouteSnapshots:
		ids := make([]string, 0, len(r.cat.Snapshots))
		for _, s := range r.cat.Snapshots {
			ids = append(ids, s.ID)
		}
		if id := firstID(ids); id != "" {
			loc.Primary = domain.Ref(domain.KindSnapshot, id)
		}
	case ui.RouteReconcile:
		ids := make([]string, 0, len(r.cat.RestorePlans))
		for _, p := range r.cat.RestorePlans {
			ids = append(ids, p.ID)
		}
		if id := firstID(ids); id != "" {
			loc.Primary = domain.Ref(domain.KindPlan, id)
		}
	case ui.RouteBackends:
		ids := make([]string, 0, len(r.cat.Backends))
		for _, b := range r.cat.Backends {
			ids = append(ids, b.ID)
		}
		if id := firstID(ids); id != "" {
			loc.Primary = domain.Ref(domain.KindBackend, id)
		}
	case ui.RouteMachines:
		ids := make([]string, 0, len(r.cat.Machines))
		for _, m := range r.cat.Machines {
			ids = append(ids, m.ID)
		}
		if id := firstID(ids); id != "" {
			loc.Primary = domain.Ref(domain.KindMachine, id)
		}
	case ui.RouteEvents:
		ids := make([]string, 0, len(r.cat.Events))
		for _, e := range r.cat.Events {
			ids = append(ids, e.ID)
		}
		if id := firstID(ids); id != "" {
			loc.Primary = domain.Ref(domain.KindEvent, id)
		}
	}
	return loc
}

func workspaceIDs(c *domain.Catalog) []string {
	ids := make([]string, 0, len(c.Workspaces))
	for _, w := range c.Workspaces {
		ids = append(ids, w.ID)
	}
	return ids
}

// openOverlay installs a root-owned overlay.
func (r *Root) openOverlay(o *overlay) tea.Cmd {
	r.ov = o
	return nil
}

// closeOverlay dismisses the open overlay without side effects.
func (r *Root) closeOverlay() tea.Cmd {
	r.ov = nil
	return nil
}

// pushToast appends a notification and arms its expiry. Repeated connection
// failures refresh the existing notification instead of flooding the stack.
func (r *Root) pushToast(text, kind string) tea.Cmd {
	r.toastSeq++
	seq := r.toastSeq
	for i := len(r.toasts) - 1; i >= 0; i-- {
		if r.toasts[i].Text == text && r.toasts[i].Kind == kind {
			r.toasts[i].Seq = seq
			return r.sched.ToastCmd(seq)
		}
	}
	r.toasts = append(r.toasts, components.Toast{Seq: seq, Text: text, Kind: kind})
	return r.sched.ToastCmd(seq)
}

// navigate pushes or replaces the top location and enters the page. Going
// home to the Dashboard collapses the stack — Dashboard stays the lower
// bound, never a mid-stack repeat.
func (r *Root) navigate(loc ui.Location, replace bool) tea.Cmd {
	loc = r.resolveDefaults(loc)
	if loc.Route == ui.RouteDashboard && len(r.stack) > 1 {
		r.stack = []ui.Location{{Route: ui.RouteDashboard}}
		return r.enterTop()
	}
	if replace && len(r.stack) > 0 {
		if r.stack[len(r.stack)-1] == loc {
			return nil
		}
		r.stack[len(r.stack)-1] = loc
	} else {
		if r.stack[len(r.stack)-1] == loc {
			return nil
		}
		r.stack = append(r.stack, loc)
	}
	return r.enterTop()
}

// pop pops the route stack; Dashboard is the lower bound.
func (r *Root) pop() tea.Cmd {
	if len(r.stack) <= 1 {
		return nil
	}
	r.stack = r.stack[:len(r.stack)-1]
	return r.enterTop()
}

// enterTop calls Enter on the freshly activated page.
func (r *Root) enterTop() tea.Cmd {
	p := r.activePage()
	if p == nil {
		return nil
	}
	return p.Enter(r.activeLocation())
}

// buildConfigForm preserves the complete form for compatibility tests and
// callers. The Settings page uses buildConfigSectionForm to keep each edit
// short and contextual.
func (r *Root) buildConfigForm(draft domain.Config) (*huh.Form, func() (domain.Config, bool)) {
	return r.buildConfigSectionForm(draft, "")
}

// buildConfigSectionForm constructs a root-owned Huh draft editor. Every
// field binds to local draft state; only a completed Save reaches the backend.
func (r *Root) buildConfigSectionForm(draft domain.Config, section string) (*huh.Form, func() (domain.Config, bool)) {
	page := draft.DefaultPage
	leader := string(draft.LeaderDisplay)
	backend := draft.DefaultBackend
	historyMode := string(draft.HistoryMode)
	retention := strconv.Itoa(draft.RetentionHours)
	overflow := string(draft.Overflow)
	redact := draft.RedactSecrets
	rawInput := draft.AllowRawInput
	trust := string(draft.RemoteTrust)
	allowlist := "none"
	if len(draft.AllowlistedMachines) > 0 {
		allowlist = draft.AllowlistedMachines[0]
	}

	backendIDs := []string{backend}
	machineIDs := []string{"none"}
	if r.cat != nil {
		for _, item := range r.cat.Backends {
			if !slices.Contains(backendIDs, item.ID) {
				backendIDs = append(backendIDs, item.ID)
			}
		}
		for _, machine := range r.cat.Machines {
			machineIDs = append(machineIDs, machine.ID)
		}
	}
	backendOpts := make([]huh.Option[string], 0, len(backendIDs))
	for _, id := range backendIDs {
		backendOpts = append(backendOpts, huh.NewOption(id, id))
	}
	machineOpts := make([]huh.Option[string], 0, len(machineIDs))
	for _, id := range machineIDs {
		machineOpts = append(machineOpts, huh.NewOption(id, id))
	}

	var save bool
	harvestCfg := func() domain.Config {
		cfg := draft
		cfg.DefaultPage = page
		cfg.LeaderDisplay = domain.LeaderDisplay(leader)
		cfg.DefaultBackend = backend
		cfg.HistoryMode = domain.HistoryMode(historyMode)
		cfg.RetentionHours, _ = strconv.Atoi(strings.TrimSpace(retention))
		cfg.Overflow = domain.OverflowPolicy(overflow)
		cfg.RedactSecrets = redact
		cfg.AllowRawInput = rawInput
		cfg.RemoteTrust = domain.RemoteTrust(trust)
		cfg.AllowlistedMachines = nil
		if cfg.RemoteTrust == domain.TrustAllowlisted && allowlist != "none" {
			cfg.AllowlistedMachines = []string{allowlist}
		}
		return cfg
	}
	harvest := func() (domain.Config, bool) { return harvestCfg(), save }

	general := []huh.Field{
		huh.NewSelect[string]().Title("Default page").
			Description("Page opened by tmh-next --dashboard and new clients.").
			Options(optionList(routeSlugs())...).Value(&page),
		huh.NewSelect[string]().Title("Leader display").
			Description("How command prefixes are shown in help and status bars.").
			Options(huh.NewOption("icon", "icon"), huh.NewOption("path", "path"), huh.NewOption("none", "none")).
			Value(&leader),
	}
	runtimeFields := []huh.Field{
		huh.NewSelect[string]().Title("Default backend").
			Description("Provider used when a route does not specify one.").
			Options(backendOpts...).Value(&backend),
	}
	history := []huh.Field{
		huh.NewSelect[string]().Title("History mode").
			Description("Persistent stores redacted pane captures in SQLite.").
			Options(huh.NewOption("off", "off"), huh.NewOption("memory", "memory"), huh.NewOption("persistent", "persistent")).
			Value(&historyMode),
		huh.NewInput().Title("Retention hours").
			Description("persistent 1–8760 · memory 1–24 · off 0").
			Value(&retention).
			Validate(func(value string) error {
				if _, err := strconv.Atoi(strings.TrimSpace(value)); err != nil {
					return fmt.Errorf("retention must be a whole number of hours")
				}
				return nil
			}),
		huh.NewSelect[string]().Title("Overflow policy").
			Description("Reject new captures or archive the oldest records.").
			Options(huh.NewOption("reject", "reject"), huh.NewOption("archive-oldest", "archive-oldest")).
			Value(&overflow),
	}
	security := []huh.Field{
		huh.NewConfirm().Title("Redact secrets in history").
			Description("Masks token, password, API key and secret assignments.").Value(&redact),
		huh.NewConfirm().Title("Allow raw terminal input").
			Description("Enables direct pane input; keep disabled unless required.").Value(&rawInput),
		huh.NewSelect[string]().Title("Remote trust").
			Options(huh.NewOption("local-only", "local-only"), huh.NewOption("prompt", "prompt"), huh.NewOption("allowlisted", "allowlisted")).
			Value(&trust),
		huh.NewSelect[string]().Title("Allowlisted machine").
			Description("Required only when remote trust is allowlisted.").
			Options(machineOpts...).Value(&allowlist),
	}

	saveTitle := "Save settings to demo memory"
	if r.isProduction() {
		saveTitle = "Save settings to ~/.config/tmh/config.yml"
	}
	saveField := huh.NewConfirm().Title(saveTitle).Value(&save).Validate(func(accepted bool) error {
		if !accepted {
			return nil
		}
		return harvestCfg().Validate()
	})

	var groups []*huh.Group
	if section == "" {
		groups = []*huh.Group{
			huh.NewGroup(append(general, runtimeFields...)...),
			huh.NewGroup(history...),
			huh.NewGroup(append(security, saveField)...),
		}
	} else {
		fields := map[string][]huh.Field{
			"general": general, "runtime": runtimeFields, "history": history, "security": security,
		}[section]
		if len(fields) == 0 {
			fields = general
		}
		fields = append(fields, saveField)
		groups = []*huh.Group{huh.NewGroup(fields...).Title(strings.ToUpper(section[:1]) + section[1:])}
	}
	form := huh.NewForm(groups...).
		WithWidth(72).
		WithShowHelp(true).
		WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin))
	return form, harvest
}

// encodeConfig serializes a draft for the config-save action value.
func encodeConfig(c domain.Config) string {
	b, _ := json.Marshal(c)
	return string(b)
}

func routeSlugs() []string {
	slugs := make([]string, 0, 15)
	for _, rt := range ui.AllRoutes() {
		slugs = append(slugs, string(rt))
	}
	return slugs
}

func optionList(values []string) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(values))
	for _, v := range values {
		opts = append(opts, huh.NewOption(v, v))
	}
	return opts
}
