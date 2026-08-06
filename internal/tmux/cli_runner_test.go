package tmux

import (
	"reflect"
	"testing"
)

// Compile-time interface compliance check.
var _ Runner = (*CLIRunner)(nil)

func TestNewSessionArgsSortsAtomicSessionOptions(t *testing.T) {
	args, err := newSessionArgs(NewSessionOpts{
		Name:       "active",
		WindowName: "__tmh_active_placeholder__",
		Detached:   true,
		SessionOptions: map[string]string{
			"@tmh-z":            "last",
			"@tmh-active-owner": "tmh/v1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"new-session", "-d", "-s", "active", "-n", "__tmh_active_placeholder__",
		";", "set-option", "-t", "active", "@tmh-active-owner", "tmh/v1",
		";", "set-option", "-t", "active", "@tmh-z", "last",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("newSessionArgs = %#v, want %#v", args, want)
	}
}

func TestNewSessionArgsRejectsNonUserOption(t *testing.T) {
	_, err := newSessionArgs(NewSessionOpts{
		Name: "active",
		SessionOptions: map[string]string{
			"destroy-unattached": "on",
		},
	})
	if err == nil {
		t.Fatal("non-user session option accepted")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "a\n", []string{"a"}},
		{"trailing newline", "a\nb\n", []string{"a", "b"}},
		{"no trailing", "a\nb", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines([]byte(tt.in))
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestValidateWindowID(t *testing.T) {
	tests := []struct {
		name     string
		windowID string
		wantErr  bool
	}{
		{"valid @1", "@1", false},
		{"valid @42", "@42", false},
		{"valid @999", "@999", false},
		{"missing @", "1", true},
		{"missing number", "@", true},
		{"empty", "", true},
		{"spaces", "@ 1", true},
		{"extra text", "@1extra", true},
		{"session prefix", "session:@1", true},
		{"only letters", "abc", true},
		{"mixed", "@a1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWindowID(tt.windowID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateWindowID(%q) error = %v, wantErr %v", tt.windowID, err, tt.wantErr)
			}
		})
	}
}

func TestParseWindowID(t *testing.T) {
	tests := []struct {
		name    string
		output  []byte
		wantID  string
		wantErr bool
	}{
		{"valid @1", []byte("@1"), "@1", false},
		{"valid with newline", []byte("@1\n"), "@1", false},
		{"valid with spaces", []byte(" @1 "), "@1", false},
		{"empty", []byte(""), "", true},
		{"missing @", []byte("1"), "", true},
		{"malformed", []byte("invalid"), "", true},
		{"mixed", []byte("@a1"), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseWindowID(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWindowID(%q) error = %v, wantErr %v", tt.output, err, tt.wantErr)
			}
			if !tt.wantErr && id != tt.wantID {
				t.Fatalf("parseWindowID(%q) = %q, want %q", tt.output, id, tt.wantID)
			}
		})
	}
}

func TestParseWindowLinks(t *testing.T) {
	tests := []struct {
		name    string
		output  []byte
		want    int
		wantErr bool
	}{
		{"valid single", []byte("sess0\x1fsess1\x1f@1\x1f1\x1fmain\x1f0\n"), 1, false},
		{"valid multiple", []byte("s0\x1fs1\x1f@1\x1f1\x1fw1\x1f0\ns0\x1fs2\x1f@1\x1f2\x1fw1\x1f0\n"), 2, false},
		{"empty", []byte(""), 0, false},
		{"malformed - invalid index", []byte("sess0\x1fsess1\x1f@1\x1fnotanumber\x1fmain\x1f0\n"), 0, true},
		{"malformed - invalid active", []byte("sess0\x1fsess1\x1f@1\x1f1\x1fmain\x1fnotanumber\n"), 0, true},
		{"missing fields", []byte("sess0\x1fsess1\x1f@1\x1f1\n"), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, err := parseWindowLinks(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWindowLinks error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(links) != tt.want {
				t.Fatalf("parseWindowLinks returned %d links, want %d", len(links), tt.want)
			}
		})
	}
}

func TestHookCommandFromLine(t *testing.T) {
	tests := []struct {
		name      string
		hook      string
		line      string
		want      string
		wantMatch bool
	}{
		{
			name:      "full indexed slot",
			hook:      "session-window-changed[1708]",
			line:      `session-window-changed[1708] run-shell "tmh active touch #{window_id}"`,
			want:      `run-shell "tmh active touch #{window_id}"`,
			wantMatch: true,
		},
		{
			name:      "full indexed slot unset",
			hook:      "session-window-changed[1708]",
			line:      "session-window-changed[1708]",
			wantMatch: true,
		},
		{
			name:      "base hook strips tmux slot",
			hook:      "after-new-window",
			line:      `after-new-window[0] rename-window "main"`,
			want:      `rename-window "main"`,
			wantMatch: true,
		},
		{
			name:      "prefix mismatch",
			hook:      "after-new-window",
			line:      `after-new-window-extra[0] run-shell "noop"`,
			wantMatch: false,
		},
		{
			name:      "different indexed slot",
			hook:      "session-window-changed[1708]",
			line:      `session-window-changed[1709] run-shell "noop"`,
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matched := hookCommandFromLine(tt.hook, tt.line)
			if matched != tt.wantMatch {
				t.Fatalf("matched = %v, want %v", matched, tt.wantMatch)
			}
			if got != tt.want {
				t.Fatalf("command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseServerEpoch(t *testing.T) {
	tests := []struct {
		name       string
		output     []byte
		wantServer string
		wantValue  string
		wantErr    bool
	}{
		{
			name:       "valid",
			output:     []byte("/tmp/tmux-1000-default:1234567890:12345"),
			wantServer: "/tmp/tmux-1000-default",
			wantValue:  "/tmp/tmux-1000-default\x1f1234567890\x1f12345",
			wantErr:    false,
		},
		{
			name:       "valid with newline",
			output:     []byte("/tmp/tmux-1000-default:1234567890:12345\n"),
			wantServer: "/tmp/tmux-1000-default",
			wantValue:  "/tmp/tmux-1000-default\x1f1234567890\x1f12345",
			wantErr:    false,
		},
		{
			name:       "valid socket with colon",
			output:     []byte("/tmp/tmux:1000:default:1234567890:12345"),
			wantServer: "/tmp/tmux:1000:default",
			wantValue:  "/tmp/tmux:1000:default\x1f1234567890\x1f12345",
			wantErr:    false,
		},
		{
			name:    "empty socket",
			output:  []byte(":1234567890:12345"),
			wantErr: true,
		},
		{
			name:    "empty start_time",
			output:  []byte("/tmp/tmux-1000-default::12345"),
			wantErr: true,
		},
		{
			name:    "empty pid",
			output:  []byte("/tmp/tmux-1000-default:1234567890:"),
			wantErr: true,
		},
		{
			name:    "missing fields",
			output:  []byte("/tmp/tmux-1000-default:1234567890"),
			wantErr: true,
		},
		{
			name:    "malformed",
			output:  []byte("invalid"),
			wantErr: true,
		},
		{
			name:    "non-numeric start_time",
			output:  []byte("/tmp/tmux-1000-default:notanumber:12345"),
			wantErr: true,
		},
		{
			name:    "non-numeric pid",
			output:  []byte("/tmp/tmux-1000-default:1234567890:notanumber"),
			wantErr: true,
		},
		{
			name:    "negative start_time",
			output:  []byte("/tmp/tmux-1000-default:-1234567890:12345"),
			wantErr: true,
		},
		{
			name:       "zero pid",
			output:     []byte("/tmp/tmux-1000-default:1234567890:0"),
			wantServer: "/tmp/tmux-1000-default",
			wantValue:  "/tmp/tmux-1000-default\x1f1234567890\x1f0",
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epoch, err := parseServerEpoch(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseServerEpoch error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if epoch.ServerKey != tt.wantServer {
					t.Errorf("parseServerEpoch ServerKey = %q, want %q", epoch.ServerKey, tt.wantServer)
				}
				if epoch.Value != tt.wantValue {
					t.Errorf("parseServerEpoch Value = %q, want %q", epoch.Value, tt.wantValue)
				}
			}
		})
	}
}

func TestParseSessionOption(t *testing.T) {
	tests := []struct {
		name    string
		output  []byte
		want    string
		wantErr bool
	}{
		{"valid", []byte("tmh/v1"), "tmh/v1", false},
		{"valid with newline", []byte("tmh/v1\n"), "tmh/v1", false},
		{"valid with spaces", []byte(" tmh/v1 "), "tmh/v1", false},
		{"empty", []byte(""), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := parseSessionOption(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseSessionOption error = %v, wantErr %v", err, tt.wantErr)
			}
			if value != tt.want {
				t.Fatalf("parseSessionOption = %q, want %q", value, tt.want)
			}
		})
	}
}
