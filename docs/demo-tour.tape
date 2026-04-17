# VHS tape — full TUI tour: dashboard navigation, help overlay, palette,
# settings, create/kill/undo. Narrated via a dim `card` helper that
# clears the screen inside its own invocation, so the typed `card`
# command never surfaces when `Show` fires.

Output docs/demo-tour.gif

Set FontSize 14
Set Width 1280
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 55ms
Set PlaybackSpeed 1

# ── Bootstrap ───────────────────────────────────────────────────────────
Hide
Type 'card() { clear; printf "\033[2m▸ %s\033[0m\n\n" "$*"; }'
Enter
Type "mkdir -p ~/.config/tmh ~/work/api/src ~/work/web/src ~/work/kb"
Enter
Type "cat > ~/.config/tmh/config.yml <<'EOF'"
Enter
Type "version: 1"
Enter
Type "roots: {work: ~/work}"
Enter
Type "sessions:"
Enter
Type "  api:  {root: work, windows: {server: api/src, logs: api/src}}"
Enter
Type "  web:  {root: work, windows: {dev: web/src}}"
Enter
Type "  kb:   {root: work, windows: {notes: kb}}"
Enter
Type "EOF"
Enter
Type "tmh init >/dev/null"
Enter
Type "clear"
Enter

# ── Act 1 — Launch the dashboard ────────────────────────────────────────
Type 'card "tmh --dashboard — full TUI"'
Enter
Sleep 300ms
Show
Sleep 1200ms

Type "tmh --dashboard"
Enter
Sleep 1800ms

# Peek at the help overlay so every key in this demo is visible up-front.
Type "?"
Sleep 2500ms
Type "?"
Sleep 600ms

# ── Act 2 — Navigate the tree ───────────────────────────────────────────
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms
Type "l"
Sleep 600ms
Type "j"
Sleep 400ms
Tab
Sleep 900ms

# ── Act 3 — Command palette ─────────────────────────────────────────────
Type ":"
Sleep 900ms
Type "diff"
Sleep 700ms
Enter
Sleep 1800ms
Escape
Sleep 500ms

# ── Act 4 — Create via wizard ───────────────────────────────────────────
Type "n"
Sleep 1200ms
Type "scratch"
Sleep 400ms
Tab
Sleep 300ms
Type "~/work/scratch"
Sleep 500ms
Enter
Sleep 1500ms

# ── Act 5 — Settings ────────────────────────────────────────────────────
Type "S"
Sleep 1200ms
Tab
Sleep 400ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Escape
Sleep 200ms
Type "j"
Sleep 300ms
Tab
Sleep 400ms
Type "j"
Sleep 300ms
Escape
Sleep 300ms
Escape
Sleep 500ms

# ── Act 6 — Theme cycle ─────────────────────────────────────────────────
Ctrl+T
Sleep 800ms
Ctrl+T
Sleep 800ms

# ── Act 7 — Kill + Undo ─────────────────────────────────────────────────
Type "G"
Sleep 400ms
Type "d"
Sleep 1200ms
Enter
Sleep 1500ms
Type "u"
Sleep 1500ms

# ── Act 8 — History + quit ──────────────────────────────────────────────
Ctrl+L
Sleep 1800ms
Escape
Sleep 500ms

Type "q"
Sleep 800ms

Hide
Type "tmux kill-server 2>/dev/null; rm -rf ~/.config/tmh ~/.local/state/tmh; clear"
Enter
Show
