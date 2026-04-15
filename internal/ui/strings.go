package ui

import "git.mark1708.ru/me/tmh/internal/i18n"

// UIStrings holds all translated UI-level strings. Built once per language
// via LoadStrings() and stored on Model; sub-models receive a copy at
// construction time. When the active language changes, Model.strings is
// rebuilt and propagated to long-lived children (dashboard) via SetStrings.
//
// Parametric messages (with {{.name}} placeholders) live under the Tf* method
// helpers, which defer to i18n.Tf so call-sites can pass template data
// directly.
type UIStrings struct {
	Loading         string
	NoSessions      string
	NothingSelected string
	AttachHint      string

	Footer   UIFooter
	Modal    UIModal
	Keymap   UIKeymap
	Settings UISettings
	Diff     UIDiff
	Palette  UIPalette
	Toast    UIToast
}

type UIFooter struct {
	Attach, Dotfiles, Sync, Settings, Palette, Help, Quit string
}

type UIModal struct {
	ErrorTitle   string
	ErrorDismiss string
	EmptyTitle   string
	EmptyHint    string
	ConfirmYes   string
	ConfirmNo    string
	KillBody     string
}

type UIKeymap struct {
	Title string

	SectionNav, SectionActions, SectionSync, SectionOther string

	NavUpdown, NavCollapse, NavTopBottom, NavPage string
	ActionAttach, ActionNew, ActionKill, ActionUndo string
	SyncRefresh, SyncReload, SyncPush, SyncDiff     string
	OtherPalette, OtherSettings, OtherHelp, OtherQuit string
}

type UISettings struct {
	Title    string
	Language string
	Theme    string
	Tmux     string
	Hint     string
}

type UIDiff struct {
	TitleEmpty  string
	BackHint    string
	ConfigLabel string
	LiveLabel   string
	EscReturn   string
}

type UIPalette struct {
	Placeholder string
	NoMatches   string
}

type UIToast struct {
	ReloadTriggered string
	UndoUnavailable string
}

// LoadStrings pulls every plain translation from i18n.T into a typed bundle.
// Call after i18n.Init() (re)sets the active language.
func LoadStrings() UIStrings {
	s := UIStrings{
		Loading:         i18n.T("tui.loading"),
		NoSessions:      i18n.T("tui.no_sessions"),
		NothingSelected: i18n.T("tui.nothing_selected"),
		AttachHint:      i18n.T("tui.attach_hint"),
	}

	s.Footer = UIFooter{
		Attach:   i18n.T("tui.footer.attach"),
		Dotfiles: i18n.T("tui.footer.dotfiles"),
		Sync:     i18n.T("tui.footer.sync"),
		Settings: i18n.T("tui.footer.settings"),
		Palette:  i18n.T("tui.footer.palette"),
		Help:     i18n.T("tui.footer.help"),
		Quit:     i18n.T("tui.footer.quit"),
	}

	s.Modal = UIModal{
		ErrorTitle:   i18n.T("tui.modal.error.title"),
		ErrorDismiss: i18n.T("tui.modal.error.dismiss"),
		EmptyTitle:   i18n.T("tui.modal.empty.title"),
		EmptyHint:    i18n.T("tui.modal.empty.hint"),
		ConfirmYes:   i18n.T("tui.modal.confirm.yes"),
		ConfirmNo:    i18n.T("tui.modal.confirm.no"),
		KillBody:     i18n.T("tui.modal.kill.body"),
	}

	s.Keymap = UIKeymap{
		Title:          i18n.T("tui.keymap.title"),
		SectionNav:     i18n.T("tui.keymap.section.nav"),
		SectionActions: i18n.T("tui.keymap.section.actions"),
		SectionSync:    i18n.T("tui.keymap.section.sync"),
		SectionOther:   i18n.T("tui.keymap.section.other"),
		NavUpdown:      i18n.T("tui.keymap.nav.updown"),
		NavCollapse:    i18n.T("tui.keymap.nav.collapse"),
		NavTopBottom:   i18n.T("tui.keymap.nav.topbottom"),
		NavPage:        i18n.T("tui.keymap.nav.page"),
		ActionAttach:   i18n.T("tui.keymap.action.attach"),
		ActionNew:      i18n.T("tui.keymap.action.new"),
		ActionKill:     i18n.T("tui.keymap.action.kill"),
		ActionUndo:     i18n.T("tui.keymap.action.undo"),
		SyncRefresh:    i18n.T("tui.keymap.sync.refresh"),
		SyncReload:     i18n.T("tui.keymap.sync.reload"),
		SyncPush:       i18n.T("tui.keymap.sync.push"),
		SyncDiff:       i18n.T("tui.keymap.sync.diff"),
		OtherPalette:   i18n.T("tui.keymap.other.palette"),
		OtherSettings:  i18n.T("tui.keymap.other.settings"),
		OtherHelp:      i18n.T("tui.keymap.other.help"),
		OtherQuit:      i18n.T("tui.keymap.other.quit"),
	}

	s.Settings = UISettings{
		Title:    i18n.T("tui.settings.title"),
		Language: i18n.T("tui.settings.language"),
		Theme:    i18n.T("tui.settings.theme"),
		Tmux:     i18n.T("tui.settings.tmux"),
		Hint:     i18n.T("tui.settings.hint"),
	}

	s.Diff = UIDiff{
		TitleEmpty:  i18n.T("tui.diff.title.empty"),
		BackHint:    i18n.T("tui.diff.back_hint"),
		ConfigLabel: i18n.T("tui.diff.config_label"),
		LiveLabel:   i18n.T("tui.diff.live_label"),
		EscReturn:   i18n.T("tui.diff.esc_return"),
	}

	s.Palette = UIPalette{
		Placeholder: i18n.T("tui.palette.placeholder"),
		NoMatches:   i18n.T("tui.palette.no_matches"),
	}

	s.Toast = UIToast{
		ReloadTriggered: i18n.T("tui.toast.reload_triggered"),
		UndoUnavailable: i18n.T("tui.toast.undo_unavailable"),
	}

	return s
}
