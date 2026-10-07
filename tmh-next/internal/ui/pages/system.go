package pages

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
)

// backendsPage is the capability matrix with probe/doctor/toggle.
type backendsPage struct{ base }

func newBackends() *backendsPage {
	return &backendsPage{base: base{route: ui.RouteBackends}}
}

func (p *backendsPage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *backendsPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	ids := make([]string, 0, len(c.Backends))
	for _, b := range c.Backends {
		ids = append(ids, b.ID)
	}
	p.setCatalog(sortedIDs(ids))
	if p.selectedID != "" && c.BackendByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *backendsPage) rows() []idLabel {
	if p.cat == nil {
		return nil
	}
	rows := make([]idLabel, 0, len(p.cat.Backends))
	for _, b := range p.cat.Backends {
		rows = append(rows, idLabel{id: b.ID, label: b.ID + " " + b.Name + " " + string(b.Status)})
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].id < rows[j-1].id; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

func (p *backendsPage) current() *domain.Backend {
	if p.cat == nil {
		return nil
	}
	return p.cat.BackendByID(p.selected())
}

func (p *backendsPage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	rows := p.rows()
	if p.cursorKeys(k, rows) {
		return nil
	}
	b := p.current()
	if b == nil {
		return nil
	}
	ref := domain.Ref(domain.KindBackend, b.ID)
	switch k.String() {
	case "p":
		return execute(domain.Action{Kind: domain.ActionBackendProbe, Target: ref})
	case "d":
		return execute(domain.Action{Kind: domain.ActionBackendDoctor, Target: ref})
	case "e":
		if b.Default {
			return toast("default backend cannot be disabled", "warn")
		}
		return confirm("Disable backend "+b.Name, "enabled flag flips off",
			domain.Action{Kind: domain.ActionBackendToggle, Target: ref})
	}
	return nil
}

func (p *backendsPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.BackendByID(p.selected()) == nil {
		return p.notFoundView(domain.KindBackend, p.selected())
	}
	var b strings.Builder
	b.WriteString(p.titleBar("Backends"))
	b.WriteString("\n")
	rows := p.rows()
	if len(rows) == 0 {
		b.WriteString(p.emptyView("backends"))
		return p.clip(b.String())
	}
	for i, r := range rows {
		be := p.cat.BackendByID(r.id)
		if be == nil {
			continue
		}
		status := map[domain.BackendStatus]string{
			domain.BackendHealthy: "OK", domain.BackendPreview: "WARN", domain.BackendUnavailable: "OFFLINE",
		}[be.Status]
		enabled := "on"
		if !be.Enabled {
			enabled = "off"
		}
		caps := strings.Join(be.Capabilities, ",")
		if caps == "" {
			caps = "—"
		}
		text := fmt.Sprintf("%-8s %-8s %-11s %-4s inst=%d %s", be.ID, be.Name, p.status(status),
			enabled, be.Instances, p.styles.Dim.Render(caps))
		b.WriteString(p.row(i, text))
		b.WriteString("\n")
	}
	cur := p.current()
	if cur != nil {
		b.WriteString("\n" + p.styles.Title.Render("Capabilities · "+cur.ID))
		b.WriteString("\n")
		for _, cap := range cur.Capabilities {
			b.WriteString(p.status("OK") + " " + cap + "\n")
		}
		for _, r := range cur.UnsupportedReasons {
			b.WriteString(p.status("DRIFT") + " " + r + "\n")
		}
		if cur.LastProbe.At.IsZero() {
			b.WriteString(p.styles.Dim.Render("never probed"))
		} else {
			probe := p.status("OK") + " probe ok " + fmt.Sprint(cur.LastProbe.LatencyMS) + "ms"
			if !cur.LastProbe.OK {
				probe = p.status("FAILED") + " probe failed"
			}
			b.WriteString(probe + p.styles.Dim.Render(" · "+cur.LastProbe.At.UTC().Format("15:04:05")))
			b.WriteString("\n")
		}
	}
	return p.clip(b.String())
}

func (p *backendsPage) Commands() []ui.Command {
	b := p.current()
	if b == nil {
		return nil
	}
	ref := domain.Ref(domain.KindBackend, b.ID)
	return []ui.Command{
		{ID: "b.probe", Title: "Probe", Description: "measure backend latency", Shortcut: "p",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionBackendProbe, Target: ref}) }},
		{ID: "b.doctor", Title: "Doctor", Description: "capability findings", Shortcut: "d",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionBackendDoctor, Target: ref}) }},
		{ID: "b.toggle", Title: "Enable / disable", Description: "flip the enabled flag (confirm to disable)", Shortcut: "e",
			Disabled: b.Default, DisabledReason: "default backend is protected",
			Run: func() tea.Cmd {
				if b.Enabled {
					return confirm("Disable backend "+b.Name, "enabled flag flips off",
						domain.Action{Kind: domain.ActionBackendToggle, Target: ref})
				}
				return execute(domain.Action{Kind: domain.ActionBackendToggle, Target: ref})
			}},
	}
}

func (p *backendsPage) Help() []key.Binding {
	return pageHelp([2]string{"p", "probe"}, [2]string{"d", "doctor"}, [2]string{"e", "toggle"})
}

// machinesPage manages local/remote nodes.
type machinesPage struct{ base }

func newMachines() *machinesPage {
	return &machinesPage{base: base{route: ui.RouteMachines}}
}

func (p *machinesPage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *machinesPage) SetCatalog(c *domain.Catalog) {
	p.cat = c
	ids := make([]string, 0, len(c.Machines))
	for _, m := range c.Machines {
		ids = append(ids, m.ID)
	}
	p.setCatalog(sortedIDs(ids))
	if p.selectedID != "" && c.MachineByID(p.selectedID) == nil {
		p.notFound = p.selectedID
	} else {
		p.notFound = ""
	}
}

func (p *machinesPage) rows() []idLabel {
	if p.cat == nil {
		return nil
	}
	rows := make([]idLabel, 0, len(p.cat.Machines))
	for _, m := range p.cat.Machines {
		rows = append(rows, idLabel{id: m.ID, label: m.ID + " " + m.Name + " " + string(m.Status)})
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].id < rows[j-1].id; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

func (p *machinesPage) current() *domain.Machine {
	if p.cat == nil {
		return nil
	}
	return p.cat.MachineByID(p.selected())
}

func (p *machinesPage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	rows := p.rows()
	if p.cursorKeys(k, rows) {
		return nil
	}
	m := p.current()
	if m == nil {
		return nil
	}
	ref := domain.Ref(domain.KindMachine, m.ID)
	switch k.String() {
	case "c":
		return execute(domain.Action{Kind: domain.ActionMachineConnect, Target: ref})
	case "p":
		return execute(domain.Action{Kind: domain.ActionMachinePing, Target: ref})
	case "d":
		return execute(domain.Action{Kind: domain.ActionMachineDoctor, Target: ref})
	case "enter":
		return pushRoute(ui.Location{Route: ui.RouteWorkspaces, Context: ref}, false)
	}
	return nil
}

func (p *machinesPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	if p.selected() != "" && p.cat.MachineByID(p.selected()) == nil {
		return p.notFoundView(domain.KindMachine, p.selected())
	}
	var b strings.Builder
	b.WriteString(p.titleBar("Machines"))
	b.WriteString("\n")
	rows := p.rows()
	if len(rows) == 0 {
		b.WriteString(p.emptyView("machines"))
		return p.clip(b.String())
	}
	for i, r := range rows {
		m := p.cat.MachineByID(r.id)
		if m == nil {
			continue
		}
		status := "ONLINE"
		latency := fmt.Sprintf("%dms", m.LatencyMS)
		if m.Status == domain.MachineOffline {
			status = "OFFLINE"
			latency = "—"
		}
		text := fmt.Sprintf("%-10s %-16s %-7s inst=%d hb=%s %s", m.ID, m.Host, p.status(status),
			m.Instances, m.Heartbeat.UTC().Format("15:04:05"), p.styles.Dim.Render(latency))
		b.WriteString(p.row(i, text))
		b.WriteString("\n")
	}
	cur := p.current()
	if cur != nil {
		b.WriteString("\n" + p.styles.Title.Render("Diagnostics · "+cur.ID))
		b.WriteString("\n")
		if len(cur.LastResult.Findings) == 0 {
			b.WriteString(p.styles.Dim.Render("no diagnostics recorded — run c/p/d"))
			b.WriteString("\n")
		}
		for _, f := range cur.LastResult.Findings {
			b.WriteString("  " + f + "\n")
		}
	}
	return p.clip(b.String())
}

func (p *machinesPage) Commands() []ui.Command {
	m := p.current()
	if m == nil {
		return nil
	}
	ref := domain.Ref(domain.KindMachine, m.ID)
	offline := m.Status == domain.MachineOffline
	reason := "machine is offline"
	return []ui.Command{
		{ID: "m.connect", Title: "Connect", Description: "open a connection", Shortcut: "c",
			Disabled: offline, DisabledReason: reason,
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionMachineConnect, Target: ref}) }},
		{ID: "m.ping", Title: "Ping", Description: "measure latency", Shortcut: "p",
			Disabled: offline, DisabledReason: reason,
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionMachinePing, Target: ref}) }},
		{ID: "m.doctor", Title: "Doctor", Description: "run diagnostics", Shortcut: "d",
			Disabled: offline, DisabledReason: reason,
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionMachineDoctor, Target: ref}) }},
		{ID: "m.workspaces", Title: "Machine workspaces", Description: "workspaces filtered by this machine", Shortcut: "enter",
			Run: func() tea.Cmd { return pushRoute(ui.Location{Route: ui.RouteWorkspaces, Context: ref}, false) }},
	}
}

func (p *machinesPage) Help() []key.Binding {
	return pageHelp([2]string{"c", "connect"}, [2]string{"p", "ping"}, [2]string{"d", "doctor"}, [2]string{"enter", "workspaces"})
}

// performancePage shows bounded series with record/benchmark and a local
// chart-projection pause.
type performancePage struct {
	base
	paused bool
}

func newPerformance() *performancePage {
	return &performancePage{base: base{route: ui.RoutePerformance}}
}

func (p *performancePage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *performancePage) SetCatalog(c *domain.Catalog) { p.cat = c }

func (p *performancePage) Update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(pauseToggleMsg); ok {
		p.paused = !p.paused
		return nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "r":
		return execute(domain.Action{Kind: domain.ActionPerfRecord})
	case "b":
		return execute(domain.Action{Kind: domain.ActionPerfBenchmark})
	}
	return nil
}

type pauseToggleMsg struct{}

func (p *performancePage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	perf := p.cat.Performance
	samples := perf.Samples
	if p.paused && len(samples) > 0 {
		samples = samples[:len(samples)-1]
	}

	var b strings.Builder
	b.WriteString(p.titleBar("Performance"))
	if p.paused {
		b.WriteString("  " + p.styles.Warn.Render("● PAUSED") + " " + p.styles.Dim.Render("local chart projection paused"))
	}
	b.WriteString("\n")
	if !perf.Available {
		b.WriteString(p.styles.Err.Render("Performance service unavailable") + "\n")
	}

	last := domain.PerfSample{}
	if len(samples) > 0 {
		last = samples[len(samples)-1]
	}
	cardWidth := max(16, (p.width-pageGap*2)/3)
	b.WriteString(horizontalCards([]string{
		p.metricCard("input p99", fmt.Sprintf("%d ms", last.InputP99), fmt.Sprintf("target ≤ %d ms · p50 %d", perf.Targets.InputP99MS, last.InputP50), cardWidth),
		p.metricCard("redraw p99", fmt.Sprintf("%d ms", last.RedrawP99), fmt.Sprintf("target ≤ %d ms", perf.Targets.RedrawP99MS), cardWidth),
		p.metricCard("action p99", fmt.Sprintf("%d ms", last.ActionP99), fmt.Sprintf("target ≤ %d ms · queue %d", perf.Targets.ActionP99MS, last.QueueDepth), cardWidth),
	}))
	b.WriteString("\n")

	chartWidth := max(30, min(84, p.width-34))
	chart := p.performanceChart(samples, chartWidth)

	if p.isWide() {
		b.WriteString(p.panelSplitHeight("Latency timeline", chart, "Recent operations", p.performanceOps(perf), 72, p.remainingHeight(b.String(), 10)))
	} else {
		b.WriteString(p.panel("Latency timeline", chart, p.width))
	}
	return p.clip(b.String())
}

func (p *performancePage) performanceSeries(label string, samples []domain.PerfSample, width int, value func(domain.PerfSample) int) string {
	latest := 0
	if len(samples) > 0 {
		latest = value(samples[len(samples)-1])
	}
	return p.styles.Info.Render(label) + "  " +
		p.styles.Bar.Render(sparkline(samples, width, value)) + "  " +
		p.styles.Header.Render(fmt.Sprintf("%3dms", latest))
}

func (p *performancePage) performanceChart(samples []domain.PerfSample, width int) string {
	var b strings.Builder
	b.WriteString(p.styles.Dim.Render("input p99 · redraw p99 · action p99") + "\n\n")
	b.WriteString(p.performanceSeries("INPUT ", samples, width, func(s domain.PerfSample) int { return s.InputP99 }) + "\n")
	b.WriteString(p.performanceSeries("REDRAW", samples, width, func(s domain.PerfSample) int { return s.RedrawP99 }) + "\n")
	b.WriteString(p.performanceSeries("ACTION", samples, width, func(s domain.PerfSample) int { return s.ActionP99 }) + "\n")
	if len(samples) == 0 {
		return strings.TrimSuffix(b.String(), "\n")
	}
	last := samples[len(samples)-1]
	b.WriteString("\n" + p.styles.Dim.Render(fmt.Sprintf("%d bounded samples · latest %s", len(samples), last.At.UTC().Format("15:04:05"))) + "\n\n")
	b.WriteString(p.styles.Dim.Render("SAMPLE TRACE       INPUT    REDRAW   ACTION   QUEUE") + "\n")
	start := max(0, len(samples)-14)
	for i := start; i < len(samples); i++ {
		sample := samples[i]
		b.WriteString(fmt.Sprintf("%s      %3dms     %3dms     %3dms      %d",
			sample.At.UTC().Format("15:04:05"),
			sample.InputP99,
			sample.RedrawP99,
			sample.ActionP99,
			sample.QueueDepth))
		if i < len(samples)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (p *performancePage) performanceOps(perf domain.Performance) string {
	var b strings.Builder
	last := domain.PerfSample{}
	if len(perf.Samples) > 0 {
		last = perf.Samples[len(perf.Samples)-1]
	}
	b.WriteString(p.styles.Dim.Render("SLO STATUS") + "\n")
	b.WriteString(p.keyValue("input", p.sloStatus(last.InputP99, perf.Targets.InputP99MS)))
	b.WriteString(p.keyValue("redraw", p.sloStatus(last.RedrawP99, perf.Targets.RedrawP99MS)))
	b.WriteString(p.keyValue("action", p.sloStatus(last.ActionP99, perf.Targets.ActionP99MS)))
	b.WriteString(p.keyValue("queue", fmt.Sprint(last.QueueDepth)))
	b.WriteString("\n" + p.styles.Dim.Render("RECORDER") + "\n")
	if len(perf.Benchmarks) == 0 && len(perf.Traces) == 0 {
		b.WriteString("No explicit traces recorded\n")
		b.WriteString(p.styles.Dim.Render("r  capture trace\nb  run benchmark"))
		return b.String()
	}
	if n := len(perf.Benchmarks); n > 0 {
		last := perf.Benchmarks[n-1]
		b.WriteString(p.styles.Accent.Render(fmt.Sprintf("BENCHMARK #%d", n)) + "\n")
		b.WriteString(p.keyValue("operations", fmt.Sprint(last.Ops)))
		b.WriteString(p.keyValue("average", fmt.Sprintf("%d ms", last.AvgMS)))
		b.WriteString(p.keyValue("p99", fmt.Sprintf("%d ms", last.P99MS)))
		b.WriteString(p.keyValue("recorded", last.At.UTC().Format("15:04:05")))
	}
	if n := len(perf.Traces); n > 0 {
		b.WriteString("\n")
		last := perf.Traces[n-1]
		b.WriteString(p.styles.Accent.Render(fmt.Sprintf("TRACE #%d", n)) + "\n")
		b.WriteString(p.keyValue("kind", last.Kind))
		b.WriteString(p.keyValue("p99", fmt.Sprintf("%d ms", last.P99MS)))
		b.WriteString(p.styles.Dim.Render(last.Detail))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (p *performancePage) sloStatus(actual, target int) string {
	if actual <= target {
		return p.status("OK") + fmt.Sprintf("  %d / %d ms", actual, target)
	}
	return p.status("WARN") + fmt.Sprintf("  %d / %d ms", actual, target)
}

func (p *performancePage) Commands() []ui.Command {
	return []ui.Command{
		{ID: "pf.record", Title: "Record trace", Description: "append a mock trace", Shortcut: "r",
			Disabled: !p.cat.Performance.Available, DisabledReason: "performance service unavailable",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionPerfRecord}) }},
		{ID: "pf.benchmark", Title: "Run benchmark", Description: "append a mock benchmark", Shortcut: "b",
			Disabled: !p.cat.Performance.Available, DisabledReason: "performance service unavailable",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionPerfBenchmark}) }},
		{ID: "pf.pause", Title: "Pause / resume chart", Description: "local projection freeze (not a backend mutation)", Shortcut: "",
			Run: func() tea.Cmd { return func() tea.Msg { return pauseToggleMsg{} } }},
	}
}

func (p *performancePage) Help() []key.Binding {
	return pageHelp([2]string{"r", "record"}, [2]string{"b", "benchmark"})
}

// configPage renders configuration categories and opens the root-owned draft.
type configPage struct {
	base
	category int
}

var configCategories = []string{"General", "Backends", "History", "Security", "Tmux"}

func newConfigPage() *configPage {
	return &configPage{base: base{route: ui.RouteConfig}}
}

func (p *configPage) Enter(loc ui.Location) tea.Cmd { p.enter(loc); return nil }

func (p *configPage) SetCatalog(c *domain.Catalog) { p.cat = c }

func (p *configPage) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "tab", "shift+tab":
		p.category = (p.category + 1) % len(configCategories)
		return nil
	case "e", "enter":
		if p.cat == nil {
			return nil
		}
		return func() tea.Msg { return ui.OpenConfigFormMsg{Draft: p.cat.Config} }
	case "v":
		if p.cat == nil {
			return nil
		}
		return execute(domain.Action{Kind: domain.ActionConfigValidate, Value: encodeDraft(p.cat.Config)})
	case "r":
		return execute(domain.Action{Kind: domain.ActionConfigReload})
	}
	return nil
}

func encodeDraft(c domain.Config) string {
	b, _ := json.Marshal(c)
	return string(b)
}

func (p *configPage) View() string {
	if p.cat == nil {
		return p.emptyView("catalog")
	}
	cfg := p.cat.Config
	var b strings.Builder
	b.WriteString(p.titleBar("Config"))
	b.WriteString("\n")
	for i, cat := range configCategories {
		if i == p.category {
			b.WriteString(p.styles.Select("[" + cat + "]"))
		} else {
			b.WriteString(" " + cat + " ")
		}
		b.WriteString(p.styles.Dim.Render("·"))
	}
	b.WriteString("\n\n")
	switch p.category {
	case 0: // General
		b.WriteString(p.keyValue("default page", cfg.DefaultPage))
		b.WriteString(p.keyValue("leader display", string(cfg.LeaderDisplay)))
		b.WriteString(p.keyValue("default backend", cfg.DefaultBackend))
	case 1: // Backends
		for _, be := range p.cat.Backends {
			enabled := "enabled"
			if !be.Enabled {
				enabled = "disabled"
			}
			b.WriteString(fmt.Sprintf("%-8s %-9s default=%v\n", be.ID, enabled, be.Default))
		}
	case 2: // History
		b.WriteString(p.keyValue("mode", string(cfg.HistoryMode)))
		b.WriteString(p.keyValue("retention", fmt.Sprintf("%dh", cfg.RetentionHours)))
		b.WriteString(p.keyValue("overflow", string(cfg.Overflow)))
	case 3: // Security
		b.WriteString(p.keyValue("redact secrets", fmt.Sprint(cfg.RedactSecrets)))
		b.WriteString(p.keyValue("raw input", fmt.Sprint(cfg.AllowRawInput)))
		b.WriteString(p.keyValue("remote trust", string(cfg.RemoteTrust)))
		b.WriteString(p.keyValue("allowlisted", strings.Join(cfg.AllowlistedMachines, ", ")))
	case 4: // Tmux compatibility
		b.WriteString(p.keyValue("default-server-socket", "$TMUX_TMPDIR/tmux-0/default"))
		b.WriteString(p.keyValue("copy-mode", "vi"))
		b.WriteString(p.styles.Dim.Render("compatibility knobs are display-only in the demo"))
		b.WriteString("\n")
	}
	b.WriteString("\n" + p.styles.Dim.Render("e edit draft (Huh form) · v validate · r reload baseline"))
	return p.clip(b.String())
}

func (p *configPage) Commands() []ui.Command {
	commands := []ui.Command{
		{ID: "cf.edit", Title: "Edit draft", Description: "root-owned Huh form", Shortcut: "e",
			Run: func() tea.Cmd { return func() tea.Msg { return ui.OpenConfigFormMsg{Draft: p.cat.Config} } }},
		{ID: "cf.reload", Title: "Reload baseline", Description: "reset to the scenario config", Shortcut: "r",
			Run: func() tea.Cmd { return execute(domain.Action{Kind: domain.ActionConfigReload}) }},
	}
	if p.cat != nil {
		commands = append(commands, ui.Command{
			ID: "cf.validate", Title: "Validate current", Description: "validation badge only", Shortcut: "v",
			Run: func() tea.Cmd {
				return execute(domain.Action{Kind: domain.ActionConfigValidate, Value: encodeDraft(p.cat.Config)})
			},
		})
	}
	return commands
}

func (p *configPage) Help() []key.Binding {
	return pageHelp([2]string{"e", "edit"}, [2]string{"v", "validate"}, [2]string{"r", "reload"}, [2]string{"tab", "category"})
}
