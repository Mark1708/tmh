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

# Bootstrap a small live tmux with three sessions.
Hide
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

# Chapter card — emitted while Hidden so only the dim caption reaches
# the viewer, never the `printf` itself.
Type 'printf "\033[2m▸ bare \`tmh\` → fuzzy picker (TTY + tmux running)\033[0m\n\n"'
Enter
Sleep 200ms
Show
Sleep 1400ms

Type "tmh"
Enter
Sleep 1800ms

# Type a fuzzy filter — "a" narrows to "api".
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
