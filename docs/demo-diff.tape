# VHS tape — drift detection with `tmh diff` + TUI dashboard.
# Render locally with `vhs docs/demo-diff.tape` → docs/demo-diff.gif.

Output docs/demo-diff.gif

Set FontSize 14
Set Width 1100
Set Height 620
Set Theme "Catppuccin Mocha"

# Seed a tiny config + live session that drifts from it.
Hide
Type "mkdir -p ~/.config/tmh ~/work/api ~/work/api-renamed"
Enter
Type "cat > ~/.config/tmh/config.yml <<'EOF'"
Enter
Type "version: 1"
Enter
Type "roots: {work: ~/work}"
Enter
Type "sessions:"
Enter
Type "  api:"
Enter
Type "    root: work"
Enter
Type "    windows: {server: api}"
Enter
Type "EOF"
Enter
Type "tmux new-session -d -s api -c ~/work/api-renamed"
Enter
Type "clear"
Enter
Show

# See drift
Type "tmh diff"
Enter
Sleep 2500ms

# Open dashboard to see drift inline
Type "tmh --dashboard"
Enter
Sleep 2500ms

# Quit TUI
Type "q"
Sleep 400ms

Hide
Type "tmux kill-server 2>/dev/null; clear"
Enter
Show
