package components

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mark1708/tmh-next/internal/ui/theme"
)

// Prompt is a root-owned single-line text input overlay. Empty submit is
// disabled; Esc discards.
type Prompt struct {
	input      textinput.Model
	title      string
	styles     theme.Styles
	submitted  bool
	cancelled  bool
	validateFn func(string) error
	errText    string
}

// NewPrompt builds a prompt overlay.
func NewPrompt(title, placeholder, initial string, styles theme.Styles, validate func(string) error) *Prompt {
	in := textinput.New()
	in.Placeholder = placeholder
	in.SetValue(initial)
	in.SetWidth(48)
	in.Prompt = "› "
	in.Focus()
	return &Prompt{input: in, title: title, styles: styles, validateFn: validate}
}

// Update drives the prompt.
func (p *Prompt) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			p.cancelled = true
			return nil
		case "enter":
			v := strings.TrimSpace(p.input.Value())
			if v == "" {
				p.errText = "empty input is disabled"
				return nil
			}
			if p.validateFn != nil {
				if err := p.validateFn(v); err != nil {
					p.errText = err.Error()
					return nil
				}
			}
			p.errText = ""
			p.submitted = true
			return nil
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return cmd
}

// Value returns the submitted value.
func (p *Prompt) Value() string { return strings.TrimSpace(p.input.Value()) }

// Submitted reports a confirmed submit.
func (p *Prompt) Submitted() bool { return p.submitted }

// Cancelled reports Esc.
func (p *Prompt) Cancelled() bool { return p.cancelled }

// Done reports conclusion.
func (p *Prompt) Done() bool { return p.submitted || p.cancelled }

// View renders the prompt panel.
func (p *Prompt) View() string {
	var b strings.Builder
	b.WriteString(p.styles.Title.Render(p.title))
	b.WriteString("  ")
	b.WriteString(p.styles.Dim.Render("(enter to accept, esc to discard)"))
	b.WriteString("\n")
	b.WriteString(p.input.View())
	if p.errText != "" {
		b.WriteString("\n")
		b.WriteString(p.styles.Err.Render("✗ " + p.errText))
	}
	return p.styles.Overlay.Render(theme.PaintBackground(b.String(), p.styles.Palette.Mantle))
}

// Confirm is the destructive-action confirmation overlay. Every confirm
// surface carries a MOCK ONLY marker.
type Confirm struct {
	title    string
	detail   string
	styles   theme.Styles
	accepted bool
	rejected bool
}

// NewConfirm builds a confirmation overlay.
func NewConfirm(title, detail string, styles theme.Styles) *Confirm {
	return &Confirm{title: title, detail: detail, styles: styles}
}

// Update drives the confirm dialog: y/enter accepts, n/esc rejects.
func (c *Confirm) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "y", "Y", "enter":
			c.accepted = true
		case "n", "N", "esc":
			c.rejected = true
		}
	}
	return nil
}

// Accepted reports y/enter.
func (c *Confirm) Accepted() bool { return c.accepted }

// Rejected reports n/esc.
func (c *Confirm) Rejected() bool { return c.rejected }

// Done reports conclusion.
func (c *Confirm) Done() bool { return c.accepted || c.rejected }

// View renders the confirmation panel.
func (c *Confirm) View() string {
	var b strings.Builder
	b.WriteString(c.styles.Title.Render(c.title))
	b.WriteString("\n")
	b.WriteString(c.styles.Base.Render(c.detail))
	b.WriteString("\n\n")
	b.WriteString(c.styles.Mock.Render("MOCK ONLY"))
	b.WriteString("  ")
	b.WriteString(c.styles.Dim.Render("y/enter accept · n/esc reject — nothing leaves the demo"))
	return c.styles.Overlay.Render(theme.PaintBackground(b.String(), c.styles.Palette.Mantle))
}

// Toast is one transient notification with a monotonic sequence number that
// prevents an old expiry from clearing a newer toast.
type Toast struct {
	Seq  uint64
	Text string
	Kind string // info|ok|warn|err
}

// View renders the toast line.
func (t Toast) View(styles theme.Styles) string {
	prefix := styles.Info.Render("ℹ")
	switch t.Kind {
	case "ok":
		prefix = styles.OK.Render("✓")
	case "warn":
		prefix = styles.Warn.Render("⚠")
	case "err":
		prefix = styles.Err.Render("✗")
	case "mock":
		prefix = styles.Mock.Render("MOCK")
	}
	content := prefix + " " + t.Text
	return styles.Chip.Render(theme.PaintBackground(content, styles.Palette.Crust))
}

// Toasts renders the visible toast stack right-aligned.
func Toasts(toasts []Toast, styles theme.Styles, width int) string {
	if len(toasts) == 0 {
		return ""
	}
	var lines []string
	for _, t := range toasts {
		lines = append(lines, lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(t.View(styles)))
	}
	return lipgloss.JoinVertical(lipgloss.Right, lines...)
}
