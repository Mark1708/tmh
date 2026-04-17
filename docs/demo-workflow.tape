# VHS tape — the unique-to-tmh workflow: declare → live → drift → detect
# → resolve (sync|freeze). This is the one that explains why tmh exists
# vs tmuxinator/tmuxp.

Output docs/demo-workflow.gif

Set FontSize 14
Set Width 1280
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set PlaybackSpeed 1

# ── Bootstrap: clean state ──────────────────────────────────────────────
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
Type "clear"
Enter
Show
Sleep 400ms

# ── Act 1 — Declared config ─────────────────────────────────────────────
Type 'printf "\033[2m▸ config.yml — declared state\033[0m\n"'
Enter
Sleep 800ms
Type "cat ~/.config/tmh/config.yml"
Enter
Sleep 2500ms

# ── Act 2 — Bring it up ─────────────────────────────────────────────────
Type 'printf "\n\033[2m▸ tmh init — live comes up from config\033[0m\n"'
Enter
Sleep 600ms
Type "tmh init"
Enter
Sleep 1500ms
Type "tmh ls"
Enter
Sleep 2000ms

# ── Act 3 — Introduce drift by hand (simulates a real workflow change) ─
Type 'printf "\n\033[2m▸ rename server → renamed live (drift!)\033[0m\n"'
Enter
Sleep 600ms
Type "tmux rename-window -t api:0 renamed"
Enter
Sleep 800ms

# ── Act 4 — Detect ──────────────────────────────────────────────────────
Type 'printf "\n\033[2m▸ tmh diff — drift is surfaced\033[0m\n"'
Enter
Sleep 600ms
Type "tmh diff"
Enter
Sleep 2500ms

# ── Act 5 — Resolve by freezing live into YAML ──────────────────────────
Type 'printf "\n\033[2m▸ tmh freeze --dry-run — preview the merge\033[0m\n"'
Enter
Sleep 600ms
Type "tmh freeze --dry-run"
Enter
Sleep 2500ms

Type 'printf "\n\033[2m▸ tmh freeze — write it into config.yml\033[0m\n"'
Enter
Sleep 600ms
Type "tmh freeze"
Enter
Sleep 1500ms

Type 'printf "\n\033[2m▸ cat config.yml — renamed captured, comments preserved\033[0m\n"'
Enter
Sleep 600ms
Type "cat ~/.config/tmh/config.yml"
Enter
Sleep 3000ms

# ── Act 6 — Prove drift is now resolved ─────────────────────────────────
Type 'printf "\n\033[2m▸ tmh diff — back to ok\033[0m\n"'
Enter
Sleep 600ms
Type "tmh diff"
Enter
Sleep 2500ms

Hide
Type "tmux kill-server 2>/dev/null; rm -rf ~/.config/tmh ~/.local/state/tmh; clear"
Enter
Show
