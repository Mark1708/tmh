package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/ui"
	"github.com/mark1708/tmh-next/internal/ui/theme"
)

type walkthroughStep struct {
	key    string
	title  string
	detail string
}

type helpWalkthrough struct {
	title     string
	goal      string
	openLabel string
	location  ui.Location
	steps     []walkthroughStep
}

var helpWalkthroughs = []helpWalkthrough{
	{
		title: "Attach to a session", goal: "Enter a live Zellij session and return to tmh-next after detach.",
		openLabel: "Open Workspaces", location: ui.Location{Route: ui.RouteWorkspaces},
		steps: []walkthroughStep{
			{key: "ctrl+k", title: "Quick switch", detail: "Find a workspace or terminal without leaving the keyboard."},
			{key: "enter", title: "Open the resource", detail: "Inspect its live session, terminal panes and availability."},
			{key: "enter", title: "Attach Zellij", detail: "tmh-next releases the terminal while Zellij owns the interactive session."},
			{key: "detach", title: "Return safely", detail: "Detach or exit the Zellij client; tmh-next resumes and refreshes."},
		},
	},
	{
		title: "Create a terminal", goal: "Create another shell pane in an existing managed workspace.",
		openLabel: "Open Workspaces", location: ui.Location{Route: ui.RouteWorkspaces},
		steps: []walkthroughStep{
			{key: "enter", title: "Open a workspace", detail: "Choose a workspace with a live Zellij session."},
			{key: "n", title: "New terminal pane", detail: "Create a right-hand shell pane in the current Zellij tab."},
			{key: "enter", title: "Attach the session", detail: "Enter Zellij and work in the new pane interactively."},
			{key: "esc", title: "Stay in control", detail: "Return to the workspace screen; history and topology refresh automatically."},
		},
	},
	{
		title: "Recover workspace drift", goal: "Preview a snapshot plan, choose direction, apply it, and retain undo.",
		openLabel: "Open Snapshots", location: ui.Location{Route: ui.RouteSnapshots},
		steps: []walkthroughStep{
			{key: "r", title: "Plan restore", detail: "Select a snapshot and build a field-level reconciliation plan."},
			{key: "space", title: "Review operations", detail: "Include or exclude individual plan operations before applying."},
			{key: "tab", title: "Choose direction", detail: "Push snapshot state to runtime or pull runtime state into desired config."},
			{key: "a", title: "Apply with confirmation", detail: "Destructive changes require an explicit production confirmation."},
			{key: "u", title: "Undo", detail: "Use the retained pre-apply snapshot to reverse the last plan."},
		},
	},
	{
		title: "Change settings", goal: "Edit one category, validate cross-field rules, and save only after success.",
		openLabel: "Open Settings", location: ui.Location{Route: ui.RouteConfig},
		steps: []walkthroughStep{
			{key: "j/k", title: "Choose a category", detail: "General, Runtime, History and Security stay separate and scannable."},
			{key: "enter", title: "Edit one section", detail: "A focused Huh form opens with descriptions and current values."},
			{key: "enter", title: "Save or discard", detail: "Validation blocks invalid combinations; Esc always discards the draft."},
			{key: "v", title: "Validate active settings", detail: "Check the complete current configuration without changing it."},
		},
	},
}

func (r *Root) openHelpWalkthrough() tea.Cmd {
	r.helpTokenSeq++
	r.ov = &overlay{kind: ovHelp, helpPlaying: true, helpToken: r.helpTokenSeq}
	return r.sched.WalkthroughCmd(r.helpTokenSeq)
}

func (r *Root) advanceWalkthrough(message WalkthroughTickMsg) tea.Cmd {
	if r.ov == nil || r.ov.kind != ovHelp || !r.ov.helpPlaying || message.Token != r.ov.helpToken {
		return nil
	}
	guide := helpWalkthroughs[r.ov.helpCase]
	r.ov.helpStep = (r.ov.helpStep + 1) % len(guide.steps)
	return r.sched.WalkthroughCmd(r.ov.helpToken)
}

func (r *Root) updateHelpOverlayKey(key tea.KeyPressMsg) tea.Cmd {
	if r.ov == nil || r.ov.kind != ovHelp {
		return nil
	}
	switch key.String() {
	case "esc", "q", "?":
		return r.closeOverlay()
	case "left", "h":
		r.selectWalkthrough(-1)
		return r.sched.WalkthroughCmd(r.ov.helpToken)
	case "right", "l":
		r.selectWalkthrough(1)
		return r.sched.WalkthroughCmd(r.ov.helpToken)
	case "j", "down":
		guide := helpWalkthroughs[r.ov.helpCase]
		r.ov.helpStep = (r.ov.helpStep + 1) % len(guide.steps)
		return nil
	case "k", "up":
		guide := helpWalkthroughs[r.ov.helpCase]
		r.ov.helpStep = (r.ov.helpStep - 1 + len(guide.steps)) % len(guide.steps)
		return nil
	case "space":
		r.ov.helpPlaying = !r.ov.helpPlaying
		r.helpTokenSeq++
		r.ov.helpToken = r.helpTokenSeq
		if r.ov.helpPlaying {
			return r.sched.WalkthroughCmd(r.ov.helpToken)
		}
		return nil
	case "r":
		r.ov.helpStep = 0
		r.ov.helpPlaying = true
		r.helpTokenSeq++
		r.ov.helpToken = r.helpTokenSeq
		return r.sched.WalkthroughCmd(r.ov.helpToken)
	case "enter":
		location := helpWalkthroughs[r.ov.helpCase].location
		r.closeOverlay()
		return r.navigate(location, false)
	}
	return nil
}

func (r *Root) selectWalkthrough(delta int) {
	if r.ov == nil {
		return
	}
	r.ov.helpCase = (r.ov.helpCase + delta + len(helpWalkthroughs)) % len(helpWalkthroughs)
	r.ov.helpStep = 0
	r.ov.helpPlaying = true
	r.helpTokenSeq++
	r.ov.helpToken = r.helpTokenSeq
}

func (r *Root) walkthroughView() string {
	if r.ov == nil || r.ov.kind != ovHelp {
		return ""
	}
	guide := helpWalkthroughs[r.ov.helpCase]
	var b strings.Builder
	b.WriteString(r.styles.Title.Render("tmh-next — guided help"))
	b.WriteString("  " + r.runtimeBadge())
	b.WriteString("\n")
	for index, item := range helpWalkthroughs {
		label := fmt.Sprintf(" %d %s ", index+1, item.title)
		if index == r.ov.helpCase {
			b.WriteString(r.styles.Select(label))
		} else {
			b.WriteString(r.styles.Dim.Render(label))
		}
		if index < len(helpWalkthroughs)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(r.styles.Header.Render("Walkthrough · " + guide.title))
	b.WriteString("\n" + r.styles.Dim.Render(guide.goal) + "\n\n")
	for index, step := range guide.steps {
		marker := fmt.Sprintf("  %d", index+1)
		key := r.styles.Chip.Render(fmt.Sprintf(" %-7s ", step.key))
		title := r.styles.Header.Render(step.title)
		if index == r.ov.helpStep {
			marker = r.styles.Accent.Render("▶ " + fmt.Sprint(index+1))
			title = r.styles.Accent.Render(step.title)
		}
		b.WriteString(fmt.Sprintf("%s  %s  %s\n", marker, key, title))
		b.WriteString("     " + r.styles.Dim.Render(step.detail) + "\n")
		if index < len(guide.steps)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	for index := range guide.steps {
		if index == r.ov.helpStep {
			b.WriteString(r.styles.Accent.Render("●"))
		} else {
			b.WriteString(r.styles.Dim.Render("○"))
		}
		b.WriteString(" ")
	}
	state := "PAUSED"
	if r.ov.helpPlaying {
		state = "PLAYING · 1.4s per step"
	}
	b.WriteString("  " + r.styles.Info.Render(state))
	b.WriteString("\n\n" + r.styles.Dim.Render("←/→ use case · j/k step · space play/pause · r restart · enter "+guide.openLabel+" · esc close"))
	b.WriteString("\n" + r.styles.Dim.Render("global keys: ctrl+p commands · ctrl+k quick switch · ? help · q quit"))
	return r.styles.Overlay.Render(theme.PaintBackground(b.String(), r.styles.Palette.Mantle))
}
