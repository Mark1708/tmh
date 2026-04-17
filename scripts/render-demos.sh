#!/bin/sh
# scripts/render-demos.sh — render docs/demo-*.tape into GIFs without
# touching the host's tmux server, ~/.config/tmh, or ~/work.
#
# Every demo tape mutates HOME-relative state (~/.config/tmh, ~/work/*)
# and runs `tmux kill-server` during teardown. Running them on a real
# workstation would clobber live sessions and real configs, so we
# render them inside a throwaway HOME + a private tmux socket.

set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

command -v vhs >/dev/null 2>&1 || {
  echo "vhs not installed — brew install vhs"
  exit 1
}

# Locate the tmh binary we'll expose inside the sandbox. Prefer an
# already-installed copy so the demos exercise exactly what users see.
REAL_HOME="$HOME"
TMH_BIN="$(command -v tmh 2>/dev/null || true)"
[ -z "$TMH_BIN" ] && TMH_BIN="$REAL_HOME/.local/bin/tmh"
if [ ! -x "$TMH_BIN" ]; then
  echo "tmh binary not found at $TMH_BIN"
  echo "run: go build -o $REAL_HOME/.local/bin/tmh ./cmd/tmh"
  exit 1
fi

SANDBOX="$(mktemp -d)"
trap 'rm -rf "$SANDBOX"' EXIT INT TERM

# Symlink the tmh binary into the sandbox PATH so `tmh` inside the
# tape resolves regardless of how the real HOME/PATH look.
mkdir -p "$SANDBOX/.local/bin"
ln -s "$TMH_BIN" "$SANDBOX/.local/bin/tmh"

export HOME="$SANDBOX"
export PATH="$SANDBOX/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
# TMUX_TMPDIR redirects the tmux socket into the sandbox so
# `tmux kill-server` in tape teardown only destroys the demo server.
export TMUX_TMPDIR="$SANDBOX"

for tape in docs/demo-picker.tape docs/demo-diff.tape docs/demo-freeze.tape; do
  echo "→ rendering $tape"
  vhs "$REPO_ROOT/$tape"
done

echo "✓ done — GIFs written to docs/demo-*.gif"
