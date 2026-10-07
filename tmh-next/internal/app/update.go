package app

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/components"
	"github.com/mark1708/tmh-next/internal/ui/layout"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// Update implements tea.Model with the fixed input precedence:
//
//  1. ctrl+c always quits
//  2. too-small mode accepts only q/ctrl+c
//  3. a root-owned overlay receives every key (Esc closes/discards)
//  4. page Filter/Text mode receives every key including Esc
//  5. Normal mode runs global keys (ctrl+p, ctrl+k, ?, space, esc, q)
//  6. everything else goes to the active page
func (r *Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyPressMsg:
		return r.updateKey(m)

	case tea.WindowSizeMsg:
		r.width, r.height = m.Width, m.Height
		r.tooSmall = layout.Classify(m.Width, m.Height) == layout.ModeTooSmall
		r.propagateSize()
		r.help.SetWidth(m.Width)
		if r.ov != nil && r.ov.form != nil {
			var cmd tea.Cmd
			r.ov.form, cmd = r.ov.form.Update(msg)
			return r, cmd
		}
		return r, nil

	case tea.BackgroundColorMsg:
		r.dark = m.IsDark()
		r.styles = theme.New(r.dark)
		r.propagateTheme()
		return r, nil

	case LoadCatalogMsg:
		r.cat = m.Catalog
		r.broadcastCatalog()
		for i := range r.stack {
			r.stack[i] = r.resolveDefaults(r.stack[i])
		}
		r.propagateSize()
		r.propagateTheme()
		r.ready = true
		cmd := r.enterTop()
		var cmds []tea.Cmd
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if timer := r.armTimer(); timer != nil {
			cmds = append(cmds, timer)
		}
		if r.quickSwitch {
			r.quickSwitch = false
			cmds = append(cmds, func() tea.Msg { return OpenQuickSwitchMsg{} })
		}
		return r, tea.Batch(cmds...)

	case LoadFailedMsg:
		r.fatal = fmt.Sprintf("control-plane snapshot failed: %v", m.Err)
		return r, nil

	case ReloadedMsg:
		if r.mut.kind != mutReload {
			return r, nil // stale reload
		}
		r.cat = m.Catalog
		r.broadcastCatalog()
		r.mut = mutator{kind: mutNone}
		next := r.pushToast("conflict recovered: catalog reloaded; retry your action", "warn")
		var cmds []tea.Cmd
		cmds = append(cmds, next)
		if r.tickPending {
			r.tickPending = false
			cmds = append(cmds, r.launchTick())
		} else if t := r.armTimer(); t != nil {
			cmds = append(cmds, t)
		}
		return r, tea.Batch(cmds...)

	case TickTimerMsg:
		if !r.timerArmed || m.Token != r.armedToken {
			return r, nil // stale timer token
		}
		r.timerArmed = false
		if r.mut.kind != mutNone {
			r.tickPending = true
			return r, nil
		}
		return r, r.launchTick()

	case MutationResultMsg:
		return r, r.acceptMutation(m.Result)

	case MutationFailedMsg:
		return r, r.rejectMutation(m)

	case SearchRequestMsg:
		r.searchSeq++
		r.searchText, r.searchScope = m.Text, m.Scope
		seq := r.searchSeq
		return r, func() tea.Msg {
			res, err := r.client.Search(context.Background(), domain.SearchQuery{Text: m.Text, Scope: m.Scope, Limit: 40})
			if err != nil {
				return ShowToastMsg{Text: "search failed: " + err.Error(), Kind: "err"}
			}
			return SearchResultsMsg{Seq: seq, Revision: res.Revision, Hits: res.Hits}
		}

	case SearchResultsMsg:
		if m.Seq != r.searchSeq {
			return r, nil // stale lane result
		}
		if m.Revision != r.currentRevision() && !r.searchRetried[m.Seq] {
			r.searchRetried[m.Seq] = true
			text, scope := r.searchText, r.searchScope
			return r, func() tea.Msg { return SearchRequestMsg{Text: text, Scope: scope} }
		}
		if p := r.pages[ui.RouteSearch]; p != nil {
			return r, p.Update(msg)
		}
		return r, nil

	case ExecuteActionMsg:
		return r, r.requestAction(m.Action)

	case PushRouteMsg:
		return r, r.navigate(m.Loc, m.Replace)

	case PopRouteMsg:
		return r, r.pop()

	case OpenPaletteMsg:
		return r, r.openPalette()

	case OpenQuickSwitchMsg:
		return r, r.openQuickSwitch()

	case OpenHelpMsg:
		r.openOverlay(&overlay{kind: ovHelp})
		return r, nil

	case OpenSelectorMsg:
		return r, r.openContextSelector(m.Title, m.Options)

	case OpenPromptMsg:
		p := components.NewPrompt(m.Title, m.Placeholder, m.Initial, r.styles, m.Validate)
		r.ov = &overlay{kind: ovPrompt, prompt: p, title: m.Title}
		// Stash the submit callback through the prompt envelope.
		r.ov.promptSubmit = m.Submit
		return r, nil

	case ConfirmActionMsg:
		c := components.NewConfirm(m.Title, m.Detail, r.styles)
		r.ov = &overlay{kind: ovConfirm, confirm: c, confirmAction: m.Action, confirmAccept: m.OnAccept, title: m.Title}
		return r, nil

	case OpenConfigFormMsg:
		form, harvest := r.buildConfigForm(m.Draft)
		r.ov = &overlay{kind: ovConfig, form: form, formHarvest: harvest}
		return r, form.Init()

	case ShowToastMsg:
		return r, r.pushToast(m.Text, m.Kind)

	case ToastExpiredMsg:
		kept := r.toasts[:0]
		for _, t := range r.toasts {
			if t.Seq != m.Seq {
				kept = append(kept, t)
			}
		}
		r.toasts = kept
		return r, nil

	case QuitNowMsg:
		return r, tea.Quit
	}

	// An open root-owned config form receives every non-key message (group
	// transitions, blink, resize echoes) before the active page sees it.
	if r.ov != nil && r.ov.kind == ovConfig && r.ov.form != nil {
		var cmd tea.Cmd
		r.ov.form, cmd = r.ov.form.Update(msg)
		return r, r.checkFormCompletion(cmd)
	}
	// Selector overlays also own their non-key messages: the Bubbles list
	// applies async filter results (FilterMatchesMsg) out of band.
	if r.ov != nil && r.ov.sel != nil {
		_ = r.ov.sel.Update(msg)
	}
	// Non-key lifecycle messages still reach the active page.
	if p := r.activePage(); p != nil {
		return r, p.Update(msg)
	}
	return r, nil
}

// checkFormCompletion emits exactly one config-save when the form completes.
func (r *Root) checkFormCompletion(cmd tea.Cmd) tea.Cmd {
	if r.ov == nil || r.ov.form == nil || r.ov.formDone {
		return cmd
	}
	if form, ok := r.ov.form.(*huh.Form); ok && form.State == huh.StateCompleted {
		r.ov.formDone = true
		draft, save := r.ov.formHarvest()
		r.closeOverlay()
		if !save {
			return tea.Batch(cmd, r.pushToast("config draft discarded (save=No)", "info"))
		}
		return tea.Batch(cmd,
			r.requestAction(domain.Action{Kind: domain.ActionConfigSave, Value: encodeConfig(draft)}),
			r.pushToast("config draft submitted [MOCK]", "mock"))
	}
	return cmd
}

// updateKey implements the key precedence chain.
func (r *Root) updateKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	// 1. ctrl+c always quits.
	if s == "ctrl+c" {
		return r, tea.Quit
	}
	// 2. too-small accepts only q.
	if r.tooSmall {
		if s == "q" {
			return r, tea.Quit
		}
		return r, nil
	}
	// 3. root-owned overlays own every key.
	if r.ov != nil {
		return r, r.updateOverlayKey(k)
	}
	// 4. page filter/text modes own every key (including Esc and q).
	if p := r.activePage(); p != nil && p.InputMode() != ui.ModeNormal {
		return r, p.Update(k)
	}
	// 5. global normal-mode keys.
	switch s {
	case "ctrl+p":
		return r, r.openPalette()
	case "ctrl+k":
		return r, r.openQuickSwitch()
	case "?":
		r.openOverlay(&overlay{kind: ovHelp})
		return r, nil
	case "space":
		return r, r.openPageCommands()
	case "esc":
		return r, r.pop()
	case "q":
		return r, tea.Quit
	}
	// 6. active page.
	if p := r.activePage(); p != nil {
		return r, p.Update(k)
	}
	return r, nil
}

// updateOverlayKey routes keys into the open overlay and resolves it.
func (r *Root) updateOverlayKey(k tea.KeyPressMsg) tea.Cmd {
	ov := r.ov
	switch ov.kind {
	case ovConfig:
		// Root-owned Esc discards the draft without contacting the backend.
		if k.String() == "esc" {
			r.closeOverlay()
			return r.pushToast("config draft discarded", "info")
		}
		var cmd tea.Cmd
		ov.form, cmd = ov.form.Update(k)
		return r.checkFormCompletion(cmd)

	case ovPrompt:
		cmd := ov.prompt.Update(k)
		switch {
		case ov.prompt.Cancelled():
			r.closeOverlay()
			return r.pushToast("input discarded", "info")
		case ov.prompt.Submitted():
			value := ov.prompt.Value()
			submit := ov.promptSubmit
			r.closeOverlay()
			if submit != nil {
				return submit(value)
			}
		}
		return cmd

	case ovConfirm:
		cmd := ov.confirm.Update(k)
		switch {
		case ov.confirm.Accepted():
			action := ov.confirmAction
			accept := ov.confirmAccept
			r.closeOverlay()
			if accept != nil {
				return accept(action)
			}
			action.Confirmed = true
			return r.requestAction(action)
		case ov.confirm.Rejected():
			r.closeOverlay()
			return r.pushToast("action cancelled", "info")
		}
		return cmd

	case ovHelp:
		switch k.String() {
		case "esc", "q", "?", "enter", "space":
			return r.closeOverlay()
		}
		return nil

	default: // selector-based overlays: palette, quick switch, context actions
		if ov.sel == nil {
			return r.closeOverlay()
		}
		cmd := ov.sel.Update(k)
		if choice, ok := ov.sel.Choice(); ok {
			r.closeOverlay()
			if choice.Command != nil {
				return choice.Command()
			}
			return nil
		}
		if ov.sel.Cancelled() {
			r.closeOverlay()
		}
		return cmd
	}
}

// requestAction runs the serialized mutation lane for a user action.
func (r *Root) requestAction(action domain.Action) tea.Cmd {
	if r.mut.kind != mutNone {
		return r.pushToast("busy: "+r.busyReason()+" — action rejected", "info")
	}
	action.ExpectedRevision = r.currentRevision()
	r.tokenSeq++
	r.mut = mutator{kind: mutAction, token: r.tokenSeq, identity: action.Identity()}
	identity := action.Identity()
	a := action
	return func() tea.Msg {
		res, err := r.client.Execute(context.Background(), a)
		if err != nil {
			return MutationFailedMsg{Kind: a.Kind, Identity: identity, Err: err}
		}
		return MutationResultMsg{Result: res}
	}
}

// acceptMutation validates and applies a mutation result. Stale or malformed
// results never change root state.
func (r *Root) acceptMutation(res domain.MutationResult) tea.Cmd {
	validChain := res.BaseRevision == r.currentRevision() &&
		res.NewRevision == res.BaseRevision+1 &&
		res.Catalog != nil && res.Catalog.Revision == res.NewRevision

	switch {
	case r.mut.kind == mutTick && res.IsTick:
		if !validChain {
			r.mut = mutator{kind: mutNone}
			return r.startReload("tick result malformed")
		}
	case r.mut.kind == mutAction && !res.IsTick:
		if res.Action.Identity() != r.mut.identity || !validChain {
			return nil // stale/malformed: ignored
		}
	default:
		return nil // identity mismatch: ignored
	}

	r.cat = res.Catalog
	r.broadcastCatalog()
	r.mut = mutator{kind: mutNone}

	var cmds []tea.Cmd
	if !res.IsTick {
		cmds = append(cmds, r.pushToast(res.Message, "ok"))
	}
	if r.tickPending {
		r.tickPending = false
		cmds = append(cmds, r.launchTick())
	} else if t := r.armTimer(); t != nil {
		cmds = append(cmds, t)
	}
	return tea.Batch(cmds...)
}

// rejectMutation handles failed mutations: conflict triggers a reload (no
// auto-retry), other failures surface a recoverable error toast.
func (r *Root) rejectMutation(m MutationFailedMsg) tea.Cmd {
	if r.mut.kind == mutNone {
		return nil
	}
	if m.IsTick && r.mut.kind != mutTick {
		return nil
	}
	if !m.IsTick && (r.mut.kind != mutAction || m.Identity != r.mut.identity) {
		return nil
	}

	if domain.IsRevisionConflict(m.Err) {
		return r.startReload("revision conflict")
	}

	r.mut = mutator{kind: mutNone}
	var cmds []tea.Cmd
	cmds = append(cmds, r.pushToast(fmt.Sprintf("%s failed [%s]: %v", label(m), domain.ErrCodeOf(m.Err), m.Err), "err"))
	if r.tickPending {
		r.tickPending = false
		cmds = append(cmds, r.launchTick())
	} else if t := r.armTimer(); t != nil {
		cmds = append(cmds, t)
	}
	return tea.Batch(cmds...)
}

// startReload recovers from a conflict by reloading the live backend state.
func (r *Root) startReload(reason string) tea.Cmd {
	if r.mut.kind != mutNone && r.mut.kind != mutReload {
		// lane stays busy until the reload completes
	}
	r.mut = mutator{kind: mutReload}
	return func() tea.Msg {
		snapshot, err := r.client.Snapshot(context.Background())
		if err != nil {
			return ShowToastMsg{Text: "reload failed: " + err.Error(), Kind: "err"}
		}
		if err := snapshot.Validate(); err != nil {
			return ShowToastMsg{Text: "reload failed: " + err.Error(), Kind: "err"}
		}
		return ReloadedMsg{Catalog: snapshot.Catalog}
	}
}

func label(m MutationFailedMsg) string {
	if m.IsTick {
		return "tick"
	}
	return string(m.Kind)
}

// --- overlay constructors ------------------------------------------------------

// openPalette builds the command palette: navigation + enabled page commands.
func (r *Root) openPalette() tea.Cmd {
	var opts []ui.SelectorOption
	for _, route := range ui.AllRoutes() {
		rt := route
		opts = append(opts, ui.SelectorOption{
			ID:    "nav:" + string(rt),
			Title: ui.RouteTitle(rt),
			Desc:  "navigate",
			Command: func() tea.Cmd {
				return func() tea.Msg { return PushRouteMsg{Loc: ui.Location{Route: rt}} }
			},
		})
	}
	if p := r.activePage(); p != nil {
		for _, c := range p.Commands() {
			cmd := c
			opts = append(opts, ui.SelectorOption{
				ID: c.ID, Title: c.Title, Desc: c.Description, Shortcut: c.Shortcut,
				Disabled: c.Disabled, Reason: c.DisabledReason, Command: cmd.Run,
			})
		}
	}
	return r.openSelectorOverlay(ovPalette, "Command palette", opts)
}

// openQuickSwitch builds the shared quick switch overlay over workspaces,
// active views, terminals and agents; a choice focuses (mock) and navigates.
func (r *Root) openQuickSwitch() tea.Cmd {
	var opts []ui.SelectorOption
	if r.cat == nil {
		return r.openSelectorOverlay(ovQuickSwitch, "Quick switch", nil)
	}
	for _, w := range r.cat.Workspaces {
		ws := w
		opts = append(opts, ui.SelectorOption{
			ID: "ws:" + ws.ID, Title: "workspace · " + ws.Name, Desc: string(ws.Status),
			Command: func() tea.Cmd {
				return tea.Batch(
					func() tea.Msg {
						return PushRouteMsg{Loc: ui.Location{Route: ui.RouteWorkspace, Primary: domain.Ref(domain.KindWorkspace, ws.ID)}}
					},
					func() tea.Msg {
						return ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionAttach, Target: domain.Ref(domain.KindWorkspace, ws.ID)}}
					},
				)
			},
		})
	}
	for _, v := range r.cat.ActiveViews {
		view := v
		opts = append(opts, ui.SelectorOption{
			ID: "view:" + view.ID, Title: "active · " + view.Title, Desc: string(view.Kind),
			Command: func() tea.Cmd {
				return tea.Batch(
					func() tea.Msg {
						return PushRouteMsg{Loc: ui.Location{Route: ui.RouteActive, Primary: domain.Ref(domain.KindActive, view.ID)}}
					},
					func() tea.Msg {
						return ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionTouch, Target: domain.Ref(domain.KindActive, view.ID)}}
					},
				)
			},
		})
	}
	for _, t := range r.cat.Terminals {
		tm := t
		opts = append(opts, ui.SelectorOption{
			ID: "term:" + tm.ID, Title: "terminal · " + tm.Name, Desc: string(tm.State),
			Command: func() tea.Cmd {
				return tea.Batch(
					func() tea.Msg {
						return PushRouteMsg{Loc: ui.Location{Route: ui.RouteTerminals, Primary: domain.Ref(domain.KindTerminal, tm.ID)}}
					},
					func() tea.Msg {
						return ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionAttach, Target: domain.Ref(domain.KindTerminal, tm.ID)}}
					},
				)
			},
		})
	}
	for _, a := range r.cat.Agents {
		ag := a
		opts = append(opts, ui.SelectorOption{
			ID: "agent:" + ag.ID, Title: "agent · " + ag.Name, Desc: string(ag.State),
			Command: func() tea.Cmd {
				return tea.Batch(
					func() tea.Msg {
						return PushRouteMsg{Loc: ui.Location{Route: ui.RouteAgents, Primary: domain.Ref(domain.KindAgent, ag.ID)}}
					},
					func() tea.Msg {
						return ExecuteActionMsg{Action: domain.Action{Kind: domain.ActionAgentFocus, Target: domain.Ref(domain.KindAgent, ag.ID)}}
					},
				)
			},
		})
	}
	return r.openSelectorOverlay(ovQuickSwitch, "Quick switch (mock focus + jump)", opts)
}

// openPageCommands opens the Space context selector with the page commands.
func (r *Root) openPageCommands() tea.Cmd {
	p := r.activePage()
	if p == nil {
		return nil
	}
	var opts []ui.SelectorOption
	for _, c := range p.Commands() {
		cmd := c
		opts = append(opts, ui.SelectorOption{
			ID: cmd.ID, Title: cmd.Title, Desc: cmd.Description, Shortcut: cmd.Shortcut,
			Disabled: cmd.Disabled, Reason: cmd.DisabledReason, Command: cmd.Run,
		})
	}
	return r.openSelectorOverlay(ovSelector, ui.RouteTitle(p.Route())+" actions", opts)
}

// openContextSelector opens a caller-provided selector.
func (r *Root) openContextSelector(title string, options []ui.SelectorOption) tea.Cmd {
	return r.openSelectorOverlay(ovSelector, title, options)
}

func (r *Root) openSelectorOverlay(kind overlayKind, title string, options []ui.SelectorOption) tea.Cmd {
	sel := components.NewSelector(title, options, min(r.width-8, 64), r.height-4, r.styles)
	r.openOverlay(&overlay{kind: kind, sel: sel, title: title})
	return nil
}
