#!/bin/sh
set -eux

# Reproducible tmux 3.2a compatibility test harness
# Builds a Linux test binary, runs in ubuntu:22.04 with exact tmux 3.2a,
# and executes only the tmux 3.2a integration tests.

# Fail fast: check for required commands
for cmd in docker mktemp; do
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "Error: required command '$cmd' not found" >&2
		exit 1
	fi
done

# Create temporary directory with trap for cleanup
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

# Build Linux test binary for internal/tmux package only
echo "Building Linux test binary..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
	go test -c -tags=tmuxintegration \
	-o "$tmp_dir/tmh-tmux32.test" ./internal/tmux

# Verify binary was created
if [ ! -f "$tmp_dir/tmh-tmux32.test" ]; then
	echo "Error: failed to build test binary" >&2
	exit 1
fi

# Run tests in pinned ubuntu:22.04 container
echo "Running tests in ubuntu:22.04 with tmux 3.2a..."
docker run --rm --platform linux/amd64 \
	-v "$tmp_dir/tmh-tmux32.test:/tmh-tmux32.test:ro" \
	ubuntu:22.04 sh -euxc '
		apt-get update
		apt-get install -y --no-install-recommends tmux

		# Verify exact tmux version
		TMUX_VERSION="$(tmux -V)"
		if [ "$TMUX_VERSION" != "tmux 3.2a" ]; then
			echo "Error: unexpected tmux version: $TMUX_VERSION (expected tmux 3.2a)" >&2
			exit 1
		fi

		# Run only TestActiveTmux32* tests
		/tmh-tmux32.test -test.v -test.run "TestActiveTmux32"
	'

echo "All tmux 3.2a compatibility tests passed!"