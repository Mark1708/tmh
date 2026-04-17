# VHS tape — authoring flow: build live, then `tmh freeze` into YAML.
# Render locally with `vhs docs/demo-freeze.tape` → docs/demo-freeze.gif.

Output docs/demo-freeze.gif

Set FontSize 14
Set Width 1100
Set Height 620
Set Theme "Catppuccin Mocha"

# Start from an empty config and build a few live sessions by hand.
Hide
Type "mkdir -p ~/.config/tmh ~/work/orgA/api ~/work/orgA/web ~/work/personal/notes"
Enter
Type "echo 'version: 1' > ~/.config/tmh/config.yml"
Enter
Type "tmux new-session -d -s api -c ~/work/orgA/api"
Enter
Type "tmux new-session -d -s web -c ~/work/orgA/web"
Enter
Type "tmux new-session -d -s notes -c ~/work/personal/notes"
Enter
Type "clear"
Enter
Show

# Dry-run first
Type "tmh freeze --dry-run"
Enter
Sleep 2500ms

# Then actually write — the config now has roots + three sessions
Type "tmh freeze"
Enter
Sleep 1500ms

Type "cat ~/.config/tmh/config.yml"
Enter
Sleep 2500ms

Hide
Type "tmux kill-server 2>/dev/null; rm -rf ~/.config/tmh; clear"
Enter
Show
