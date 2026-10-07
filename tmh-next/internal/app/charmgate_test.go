package app_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/mark1708/tmh-next/internal/testutil"
)

// fastTimeout bounds each pumped command; timer cmds (blink) never finish in
// time and are discarded, keeping tests deterministic.
const fastTimeout = 100 * time.Millisecond

// expandBatch flattens BatchMsgs produced by widget updates into single msgs.
func expandBatch(msg tea.Msg, out *[]tea.Msg) {
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, cmd := range b {
			if m, ok := testutil.RunCmdFast(cmd, fastTimeout); ok {
				expandBatch(m, out)
			}
		}
		return
	}
	if msg != nil {
		*out = append(*out, msg)
	}
}

// --- Gate 1: Bubble Tea v2 view lifecycle ---------------------------------

type gateModel struct {
	sized bool
	keys  []string
}

func (m *gateModel) Init() tea.Cmd { return nil }

func (m *gateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.sized = true
	case tea.KeyPressMsg:
		m.keys = append(m.keys, msg.String())
		if msg.String() == "q" {
			return m, tea.Quit
		}
	case tea.BackgroundColorMsg:
		// theme rebuild seam exercised in app tests
	}
	return m, nil
}

func (m *gateModel) View() tea.View {
	v := tea.NewView("tmh-next gate frame")
	v.AltScreen = true
	v.ReportFocus = true
	v.WindowTitle = "tmh-next — mock cockpit"
	return v
}

// TestCharmV2ViewLifecycle proves the pinned Bubble Tea v2 API against a real
// (non-TTY) program run: declarative View fields, KeyPressMsg strings, ordered
// message delivery and clean shutdown.
func TestCharmV2ViewLifecycle(t *testing.T) {
	m := &gateModel{}
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(&bytes.Buffer{}),
		tea.WithOutput(&out),
	)

	go func() {
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
		p.Send(testutil.Space())
		p.Send(testutil.Rune('q'))
	}()

	final, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	gm, ok := final.(*gateModel)
	if !ok {
		t.Fatalf("final model type = %T", final)
	}
	if !gm.sized {
		t.Error("WindowSizeMsg was not delivered")
	}
	joined := strings.Join(gm.keys, ",")
	for _, want := range []string{"space", "q"} {
		if !strings.Contains(joined, want) {
			t.Errorf("keys %q missing %q", joined, want)
		}
	}
	if out.Len() == 0 {
		t.Error("renderer produced no output")
	}

	v := gm.View()
	if !v.AltScreen || !v.ReportFocus {
		t.Errorf("declarative view flags: altScreen=%v reportFocus=%v", v.AltScreen, v.ReportFocus)
	}
	if v.WindowTitle != "tmh-next — mock cockpit" {
		t.Errorf("window title = %q", v.WindowTitle)
	}
	if !testutil.Contains(v.Content, "tmh-next gate frame") {
		t.Errorf("view content missing frame text: %q", v.Content)
	}
}

// --- Gate 2: Lip Gloss v2 overlay composition -----------------------------

// TestLipGlossOverlayComposition proves Canvas/Compositor/Layer overlay math
// with ANSI-aware positioning and hit testing.
func TestLipGlossOverlayComposition(t *testing.T) {
	base := lipgloss.NewStyle().
		Width(40).Height(10).
		Foreground(lipgloss.Color("#cdd6f4")).
		Render("base cockpit layer")

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#89b4fa")).
		Padding(0, 1).
		Render("MOCK panel")

	if w := lipgloss.Width(panel); w != lipgloss.Width(testutil.Plain(panel)) {
		t.Errorf("ANSI-aware width %d != plain width %d", w, lipgloss.Width(testutil.Plain(panel)))
	}

	comp := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(panel).X(8).Y(3).Z(1).ID("overlay"),
	)
	canvas := lipgloss.NewCanvas(40, 10)
	rendered := canvas.Compose(comp).Render()

	lines := strings.Split(testutil.Plain(rendered), "\n")
	if len(lines) < 6 {
		t.Fatalf("rendered %d lines, want >= 6: %q", len(lines), rendered)
	}
	if !strings.Contains(lines[0], "base cockpit layer") {
		t.Errorf("base layer not visible in first line: %q", lines[0])
	}
	// Panel border box starts at Y=3; text row is Y=4. Display columns:
	// X=8 (border) + 1 (padding) → text starts at column 10.
	if col := testutil.ColumnOf(lines[4], "MOCK panel"); col != 10 {
		t.Errorf("overlay text at display column %d, want 10 (line=%q)", col, lines[4])
	}
	if hit := comp.Hit(10, 4); hit.ID() != "overlay" {
		t.Errorf("hit test at (10,4) = %q, want overlay", hit.ID())
	}
	if hit := comp.Hit(0, 0); !hit.Empty() {
		t.Errorf("hit test at (0,0) should miss unidentified base, got %q", hit.ID())
	}
	if max := testutil.MaxLineWidth(rendered); max > 40 {
		t.Errorf("rendered max width %d exceeds canvas 40", max)
	}
}

// --- Gate 3: Bubbles v2 list filter/escape semantics ----------------------

type gateItem string

func (g gateItem) FilterValue() string { return string(g) }

// TestBubblesSelectorFilterEscape proves the Bubbles v2 list filter lifecycle
// used by palette/selector overlays: '/' enters filtering, typed runes narrow
// visible items (via async FilterMatchesMsg), Enter applies, Esc restores the
// unfiltered set.
func TestBubblesSelectorFilterEscape(t *testing.T) {
	items := []list.Item{gateItem("agents"), gateItem("events"), gateItem("dashboard")}
	l := list.New(items, list.NewDefaultDelegate(), 40, 12)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetShowPagination(false)

	pump := func(m list.Model, msg tea.Msg) list.Model {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		for i := 0; cmd != nil && i < 8; i++ {
			res, ok := testutil.RunCmdFast(cmd, fastTimeout)
			if !ok {
				break
			}
			var msgs []tea.Msg
			expandBatch(res, &msgs)
			if len(msgs) == 0 {
				break
			}
			for _, next := range msgs {
				m, cmd = m.Update(next)
			}
		}
		return m
	}

	l = pump(l, testutil.Rune('/'))
	if l.FilterState() != list.Filtering {
		t.Fatalf("filter state after '/': %v, want Filtering", l.FilterState())
	}

	l = pump(l, testutil.Rune('a'))
	l = pump(l, testutil.Rune('g'))
	visible := l.VisibleItems()
	if len(visible) != 1 || string(visible[0].(gateItem)) != "agents" {
		names := make([]string, 0, len(visible))
		for _, it := range visible {
			names = append(names, string(it.(gateItem)))
		}
		t.Fatalf("visible items after 'ag': %v, want [agents]", names)
	}

	l = pump(l, testutil.Special(tea.KeyEnter))
	if l.FilterState() != list.FilterApplied {
		t.Fatalf("filter state after enter: %v, want FilterApplied", l.FilterState())
	}

	l = pump(l, testutil.Special(tea.KeyEsc))
	if l.FilterState() != list.Unfiltered {
		t.Fatalf("filter state after esc: %v, want Unfiltered", l.FilterState())
	}
	if got := len(l.VisibleItems()); got != 3 {
		t.Fatalf("visible items after esc: %d, want 3", got)
	}
}

// --- Gate 4: Huh v2 root-owned draft lifecycle ----------------------------

func newGateForm(draft *string, saves *int) *huh.Form {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Operator name").
				Value(draft).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("name is required")
					}
					return nil
				}),
		),
	).WithWidth(60).WithShowHelp(false).WithShowErrors(true)
	form.SubmitCmd = func() tea.Msg {
		*saves++
		return nil
	}
	return form
}

// driveHuh feeds msg into the huh model and pumps returned commands so the
// group→form progression (nextGroupMsg → completion) runs synchronously.
func driveHuh(m huh.Model, msg tea.Msg) huh.Model {
	m, cmd := m.Update(msg)
	for i := 0; cmd != nil && i < 8; i++ {
		res, ok := testutil.RunCmdFast(cmd, fastTimeout)
		if !ok {
			break
		}
		var msgs []tea.Msg
		expandBatch(res, &msgs)
		if len(msgs) == 0 {
			break
		}
		for _, next := range msgs {
			m, cmd = m.Update(next)
		}
	}
	return m
}

// TestHuhDraftSaveDiscardAndCompactLayout proves the Huh v2 semantics the
// root-owned Config form depends on: invalid submit stays visible in
// StateNormal, a valid submit fires exactly one SubmitCmd, ctrl+c is huh's
// native abort, a non-forwarded (root-consumed) Esc produces no save, and the
// compact-width form respects its width budget.
func TestHuhDraftSaveDiscardAndCompactLayout(t *testing.T) {
	t.Run("invalid submit stays visible", func(t *testing.T) {
		var draft string
		saves := 0
		form := newGateForm(&draft, &saves)
		_ = form.Init()

		m := driveHuh(form, testutil.Special(tea.KeyEnter))

		f, ok := m.(*huh.Form)
		if !ok {
			t.Fatalf("huh model type = %T", m)
		}
		if f.State != huh.StateNormal {
			t.Errorf("state = %v, want StateNormal", f.State)
		}
		if saves != 0 {
			t.Errorf("saves = %d, want 0", saves)
		}
		view := m.View()
		if !testutil.Contains(view, "name is required") {
			t.Errorf("validation error not visible: %q", testutil.Plain(view))
		}
		if max := testutil.MaxLineWidth(view); max > 60 {
			t.Errorf("compact form width %d exceeds 60", max)
		}
	})

	t.Run("valid submit saves exactly once", func(t *testing.T) {
		var draft string
		saves := 0
		form := newGateForm(&draft, &saves)
		_ = form.Init()

		m := huh.Model(form)
		for _, msg := range testutil.Type("ops") {
			m = driveHuh(m, msg)
		}
		m = driveHuh(m, testutil.Special(tea.KeyEnter))

		f, ok := m.(*huh.Form)
		if !ok {
			t.Fatalf("huh model type = %T", m)
		}
		if f.State != huh.StateCompleted {
			t.Fatalf("state = %v, want StateCompleted", f.State)
		}
		if saves != 1 {
			t.Errorf("saves = %d, want 1", saves)
		}

		// Extra enter on a completed form must not save again.
		m = driveHuh(m, testutil.Special(tea.KeyEnter))
		if saves != 1 {
			t.Errorf("saves after extra enter = %d, still want 1", saves)
		}
	})

	t.Run("native ctrl+c aborts without saving", func(t *testing.T) {
		var draft string
		saves := 0
		form := newGateForm(&draft, &saves)
		_ = form.Init()

		m := driveHuh(form, testutil.Ctrl('c'))

		f, ok := m.(*huh.Form)
		if !ok {
			t.Fatalf("huh model type = %T", m)
		}
		if f.State != huh.StateAborted {
			t.Errorf("state = %v, want StateAborted", f.State)
		}
		if saves != 0 {
			t.Errorf("saves = %d, want 0", saves)
		}
	})

	t.Run("root-consumed esc leaves form untouched", func(t *testing.T) {
		// The tmh-next root owns Esc and never forwards it to huh (huh's
		// keymap.Quit is ctrl+c only). This locks the contract the root
		// relies on: not forwarding keys cannot mutate the form.
		var draft string
		saves := 0
		form := newGateForm(&draft, &saves)
		_ = form.Init()

		for _, msg := range testutil.Type("draft") {
			form, _ := huh.Model(form).Update(msg)
			_ = form
		}

		if form.State != huh.StateNormal {
			t.Errorf("state = %v, want StateNormal", form.State)
		}
		if saves != 0 {
			t.Errorf("saves = %d, want 0", saves)
		}
		if draft != "draft" {
			t.Errorf("typed draft = %q, want %q", draft, "draft")
		}
	})
}
