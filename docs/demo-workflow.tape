# VHS tape — the unique-to-tmh workflow: declare → live → drift → detect
# → resolve (sync|freeze). Uses a `card` helper that clears the screen
# inside its own body so the typed `card "..."` command never leaks into
# the captured frames.

Output docs/demo-workflow.gif

Set FontSize 14
Set Width 1280
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set PlaybackSpeed 1

# ── Bootstrap ──────────────────────────────────────────────────────────
Hide
Type 'card() { clear; printf "\033[2m▸ %s\033[0m\n\n" "$*"; }'
Enter
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
Type "clear"
Enter

# ── Act 1 — Declared config ────────────────────────────────────────────
Type 'card "config.yml — declared state"'
Enter
Sleep 300ms
Show
Sleep 1000ms
Type "cat ~/.config/tmh/config.yml"
Enter
Sleep 3000ms

# ── Act 2 — Bring it up ────────────────────────────────────────────────
Hide
Type 'card "tmh init — live comes up from config"'
Enter
Sleep 300ms
Show
Sleep 1000ms
Type "tmh init"
Enter
Sleep 1500ms
Type "tmh ls"
Enter
Sleep 2500ms

# ── Act 3 — Introduce drift by hand ────────────────────────────────────
Hide
Type 'card "rename server → renamed live (drift!)"'
Enter
Sleep 300ms
Show
Sleep 1000ms
Type "tmux rename-window -t api:0 renamed"
Enter
Sleep 1200ms

# ── Act 4 — Detect ─────────────────────────────────────────────────────
Hide
Type 'card "tmh diff — drift is surfaced"'
Enter
Sleep 300ms
Show
Sleep 1000ms
Type "tmh diff"
Enter
Sleep 3000ms

# ── Act 5 — Resolve by freezing live into YAML ─────────────────────────
Hide
Type 'card "tmh freeze --dry-run — preview the merge"'
Enter
Sleep 300ms
Show
Sleep 1000ms
Type "tmh freeze --dry-run"
Enter
Sleep 2800ms

Hide
Type 'card "tmh freeze — write it into config.yml"'
Enter
Sleep 300ms
Show
Sleep 1000ms
Type "tmh freeze"
Enter
Sleep 1800ms

Hide
Type 'card "cat config.yml — renamed captured, comments preserved"'
Enter
Sleep 300ms
Show
Sleep 1000ms
Type "cat ~/.config/tmh/config.yml"
Enter
Sleep 3500ms

# ── Act 6 — Prove drift is now resolved ────────────────────────────────
Hide
Type 'card "tmh diff — back to ok"'
Enter
Sleep 300ms
Show
Sleep 1000ms
Type "tmh diff"
Enter
Sleep 2500ms

Hide
Type "tmux kill-server 2>/dev/null; rm -rf ~/.config/tmh ~/.local/state/tmh; clear"
Enter
Show
