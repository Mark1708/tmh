package testutil

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestKeyBuilders(t *testing.T) {
	if Rune('a').String() != "a" || Space().String() != "space" {
		t.Fatal("rune/space builders wrong")
	}
	if Special(tea.KeyEnter).String() != "enter" {
		t.Fatal("special builder wrong")
	}
	if Ctrl('p').String() != "ctrl+p" {
		t.Fatal("ctrl builder wrong")
	}
	if n := len(Type("hello")); n != 5 {
		t.Fatalf("type expands to %d", n)
	}
	if Size(80, 24).Width != 80 {
		t.Fatal("size builder wrong")
	}
}

func TestWidthHelpers(t *testing.T) {
	styled := "\x1b[38;2;1;2;3mabc\x1b[mdef"
	if StringWidth(styled) != 6 {
		t.Fatalf("string width = %d", StringWidth(styled))
	}
	if MaxLineWidth("short\n\x1b[1mlonger-line\x1b[m\nx") != 11 {
		t.Fatal("max width wrong")
	}
	if Plain(styled) != "abcdef" {
		t.Fatalf("plain = %q", Plain(styled))
	}
	if !Contains(styled, "cde") || Contains(styled, "zzz") {
		t.Fatal("contains broken")
	}
	if ColumnOf("ab│cd PANEL", "PANEL") != 6 {
		t.Fatalf("column = %d, want 6 (│ counts one cell)", ColumnOf("ab│cd PANEL", "PANEL"))
	}
	if ColumnOf("abc", "zz") != -1 {
		t.Fatal("missing column not -1")
	}
}

func TestRunCmdFast(t *testing.T) {
	if _, ok := RunCmdFast(nil, time.Second); ok {
		t.Fatal("nil cmd reported ok")
	}
	msg, ok := RunCmdFast(func() tea.Msg { return TickMsgSentinel{} }, time.Second)
	if !ok || msg == nil {
		t.Fatal("fast cmd not captured")
	}
	if _, ok := RunCmdFast(func() tea.Msg { time.Sleep(300 * time.Millisecond); return nil }, 50*time.Millisecond); ok {
		t.Fatal("slow cmd not skipped")
	}
}

type TickMsgSentinel struct{}
