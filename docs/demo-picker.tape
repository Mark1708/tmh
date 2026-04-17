# VHS tape — bare-tmh picker flow.
# Render locally with `vhs docs/demo-picker.tape` → docs/demo-picker.gif.

Output docs/demo-picker.gif

Set FontSize 14
Set Width 1100
Set Height 560
Set Theme "Catppuccin Mocha"

# Start with a tmux server running so `tmh` (bare) routes to the picker.
Hide
Type "mkdir -p ~/work/api ~/work/web ~/ops"
Enter
Type "tmux new-session -d -s api -c ~/work/api"
Enter
Type "tmux new-session -d -s web -c ~/work/web"
Enter
Type "tmux new-session -d -s infra -c ~/ops"
Enter
Type "clear"
Enter
Show

Type "tmh"
Enter
Sleep 1200ms

# Fuzzy filter — land on "api"
Type "a"
Sleep 400ms
Type "p"
Sleep 400ms
Type "i"
Sleep 800ms

# Confirm with Enter → tmux attach
Enter
Sleep 1500ms

# Detach back to shell so the demo ends cleanly
Type "C-b"
Type "d"
Sleep 500ms

Hide
Type "tmux kill-server 2>/dev/null; clear"
Enter
Show
