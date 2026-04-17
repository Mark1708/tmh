package shell

import (
	"path/filepath"
	"testing"
)

func TestDefaultRCFile(t *testing.T) {
	cases := []struct {
		shell string
		want  string // relative to $HOME
	}{
		{"/bin/bash", ".bashrc"},
		{"/usr/local/bin/bash", ".bashrc"},
		{"/usr/bin/zsh", ".zshrc"},
		{"/bin/zsh", ".zshrc"},
		{"/opt/homebrew/bin/fish", filepath.Join(".config", "fish", "config.fish")},
		{"/usr/bin/fish", filepath.Join(".config", "fish", "config.fish")},
		{"", ".profile"},
		{"/bin/sh", ".profile"},
	}
	home := "/tmp/home"
	t.Setenv("HOME", home)
	for _, tc := range cases {
		t.Run(tc.shell, func(t *testing.T) {
			t.Setenv("SHELL", tc.shell)
			got := DefaultRCFile()
			want := filepath.Join(home, tc.want)
			if got != want {
				t.Fatalf("DefaultRCFile() = %q; want %q", got, want)
			}
		})
	}
}
