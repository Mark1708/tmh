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

SANDBOX="$(mktemp -d)"
# `go install` drops read-only files into GOMODCACHE; chmod before rm so
# the trap can clean up without permission errors.
cleanup() { chmod -R u+w "$SANDBOX" 2>/dev/null || true; rm -rf "$SANDBOX"; }
trap cleanup EXIT INT TERM
mkdir -p "$SANDBOX/.local/bin"

# Reuse the real user's Go caches so `go install` doesn't re-download
# every module on each run — and cleanup is about the sandbox HOME only,
# not a fresh multi-gig module cache.
REAL_GOMODCACHE="$(go env GOMODCACHE)"
REAL_GOCACHE="$(go env GOCACHE)"
export GOMODCACHE="$REAL_GOMODCACHE"
export GOCACHE="$REAL_GOCACHE"

# Preserve the original PATH for `go` + `brew`-installed tools (vhs,
# ttyd, ffmpeg) which the tapes need; but put the sandbox binary dir
# first so `tmh` inside the tape resolves to our freshly built copy.
ORIGINAL_PATH="$PATH"
export HOME="$SANDBOX"
export PATH="$SANDBOX/.local/bin:$ORIGINAL_PATH"
# TMUX_TMPDIR redirects the tmux socket into the sandbox so
# `tmux kill-server` in tape teardown only destroys the demo server.
export TMUX_TMPDIR="$SANDBOX"

# Build a fresh tmh binary into the sandbox PATH so the demos exercise
# exactly what the current working tree produces — not whatever stale
# version lives in the user's $HOME/.local/bin from an earlier session.
echo "→ building tmh into sandbox"
GOBIN="$SANDBOX/.local/bin" go install ./cmd/tmh

for tape in docs/demo-picker.tape docs/demo-tour.tape docs/demo-workflow.tape; do
  echo "→ rendering $tape"
  vhs "$REPO_ROOT/$tape"
done

echo "✓ done — GIFs written to docs/demo-*.gif"
