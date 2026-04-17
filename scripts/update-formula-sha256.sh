#!/bin/sh
# scripts/update-formula-sha256.sh — after a tagged release is published
# on GitHub, fetch checksums.txt and rewrite the SHA-256 placeholders in
# homebrew/tmh.rb (and print instructions for the tap repo).
#
# Usage: bash scripts/update-formula-sha256.sh v1.0.0

set -eu

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 <tag>    e.g. $0 v1.0.0" >&2
  exit 1
fi

# Accept either "v1.0.0" or "1.0.0".
VER_NOPREFIX="${VERSION#v}"
TAG="v$VER_NOPREFIX"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FORMULA="$REPO_ROOT/homebrew/tmh.rb"
if [ ! -f "$FORMULA" ]; then
  echo "formula not found at $FORMULA" >&2
  exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

BASE_URL="https://github.com/mark1708/tmh/releases/download/$TAG"

echo "→ fetching $BASE_URL/checksums.txt"
if ! curl -fsSL "$BASE_URL/checksums.txt" -o "$TMP/checksums.txt"; then
  echo "failed to fetch checksums.txt — is the release published?" >&2
  echo "open: https://github.com/mark1708/tmh/releases/tag/$TAG" >&2
  exit 1
fi

echo "→ fetching checksums.txt.sig and verifying signature"
if curl -fsSL "$BASE_URL/checksums.txt.sig" -o "$TMP/checksums.txt.sig"; then
  if command -v gpg >/dev/null 2>&1; then
    if ! gpg --verify "$TMP/checksums.txt.sig" "$TMP/checksums.txt" 2>&1 | tail -3; then
      echo "⚠ GPG signature did NOT verify — refusing to update formula" >&2
      echo "  (imported the maintainer's key? see docs/verify.md)" >&2
      exit 1
    fi
  else
    echo "⚠ gpg not installed — skipping signature verification"
  fi
else
  echo "⚠ no checksums.txt.sig — unsigned release, update anyway?"
fi

# Map goreleaser archive names → formula placeholders.
DARWIN_ARM64_LINE="tmh_${VER_NOPREFIX}_darwin_arm64.tar.gz"
DARWIN_AMD64_LINE="tmh_${VER_NOPREFIX}_darwin_amd64.tar.gz"
LINUX_ARM64_LINE="tmh_${VER_NOPREFIX}_linux_arm64.tar.gz"
LINUX_AMD64_LINE="tmh_${VER_NOPREFIX}_linux_amd64.tar.gz"

sha_for() {
  grep "  $1\$" "$TMP/checksums.txt" | awk '{print $1}'
}

DARWIN_ARM64_SHA="$(sha_for "$DARWIN_ARM64_LINE")"
DARWIN_AMD64_SHA="$(sha_for "$DARWIN_AMD64_LINE")"
LINUX_ARM64_SHA="$(sha_for "$LINUX_ARM64_LINE")"
LINUX_AMD64_SHA="$(sha_for "$LINUX_AMD64_LINE")"

for pair in \
  "DARWIN_ARM64:$DARWIN_ARM64_SHA" \
  "DARWIN_AMD64:$DARWIN_AMD64_SHA" \
  "LINUX_ARM64:$LINUX_ARM64_SHA" \
  "LINUX_AMD64:$LINUX_AMD64_SHA"; do
  key="${pair%%:*}"
  val="${pair#*:}"
  if [ -z "$val" ]; then
    echo "⚠ no sha256 for $key in checksums.txt — does the archive exist?" >&2
    exit 1
  fi
done

echo "→ patching $FORMULA"
# Rewrite placeholders and bump version atomically.
SED_SCRIPT="$TMP/sed.script"
cat > "$SED_SCRIPT" <<EOF
s/^  version "[^"]*"/  version "$VER_NOPREFIX"/
s/REPLACE_WITH_DARWIN_ARM64_SHA256/$DARWIN_ARM64_SHA/
s/REPLACE_WITH_DARWIN_AMD64_SHA256/$DARWIN_AMD64_SHA/
s/REPLACE_WITH_LINUX_ARM64_SHA256/$LINUX_ARM64_SHA/
s/REPLACE_WITH_LINUX_AMD64_SHA256/$LINUX_AMD64_SHA/
EOF
sed -i.bak -f "$SED_SCRIPT" "$FORMULA"
rm -f "$FORMULA.bak"

echo
echo "✓ $FORMULA updated. Diff:"
echo "----------------------------------------"
git -C "$REPO_ROOT" diff --no-color -- "$FORMULA" || true
echo "----------------------------------------"
echo
echo "Next steps:"
echo "  1. Commit and push the main repo:"
echo "     git add homebrew/tmh.rb"
echo "     git commit -m 'chore(homebrew): bump to $VERSION'"
echo "     git push"
echo
echo "  2. Mirror the same formula into your tap repo:"
echo "     cd /path/to/homebrew-tap"
echo "     cp $FORMULA Formula/tmh.rb"
echo "     git add Formula/tmh.rb"
echo "     git commit -m 'bump tmh to $VERSION'"
echo "     git push"
echo
echo "  3. Smoke-test the install:"
echo "     brew update && brew reinstall mark1708/tap/tmh"
echo "     tmh version  # should print $VER_NOPREFIX"
