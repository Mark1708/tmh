# VHS tape — bare-tmh fuzzy picker.
# Render via `make demo` (uses scripts/render-demos.sh which sandboxes
# HOME and TMUX_TMPDIR so your real tmux server / config.yml stay safe).

Output docs/demo-picker.gif

Set FontSize 14
Set Width 1280
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms
Set PlaybackSpeed 1

# ── Bootstrap: hidden setup + `card` helper ────────────────────────────
# `card` clears the screen inside its own invocation so the typed
# command name is wiped before the caption is printed — necessary
# because `Hide` only stops frame capture, not the shell's echo of
# typed characters. Read the implementation top-to-bottom:
#   1. `clear` wipes screen + cursor home
#   2. `printf` writes the dim caption
# The typed `card "..."` line is no longer visible when `Show` fires.
Hide
Type 'card() { clear; printf "\033[2m▸ %s\033[0m\n\n" "$*"; }'
Enter
Type "mkdir -p ~/work/api ~/work/web ~/ops && cd ~"
Enter
Type "tmux new-session -d -s api -c ~/work/api"
Enter
Type "tmux new-session -d -s web -c ~/work/web"
Enter
Type "tmux new-session -d -s infra -c ~/ops"
Enter
Type "clear"
Enter

# Chapter card — typed while Hidden, then `card` itself clears the
# screen so the viewer only sees the dim caption.
Type 'card "bare \`tmh\` → fuzzy picker (TTY + tmux running)"'
Enter
Sleep 300ms
Show
Sleep 1400ms

Type "tmh"
Enter
Sleep 1800ms

# Fuzzy filter — "a" narrows to "api".
Type "a"
Sleep 700ms

# Attach
Enter
Sleep 1500ms

# Detach politely back to the shell
Ctrl+B
Type "d"
Sleep 800ms

Hide
Type "tmux kill-server 2>/dev/null; clear"
Enter
Show
