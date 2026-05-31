.PHONY: build test test-race lint install clean fmt docs schema demo release check-tag goreleaser-local goreleaser-snapshot

BINARY := tmh
CMD    := ./cmd/tmh
BIN    := $(shell go env GOPATH)/bin
TAG    ?=

build:
	go build -o $(BINARY) $(CMD)

install:
	go install $(CMD)

test:
	go test ./... -cover

test-race:
	go test ./... -race -cover

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

clean:
	rm -f $(BINARY)
	rm -rf dist/

# Regenerate docs/generated/man, docs/generated/completions, and schemas/tmh.schema.json.
# Run after touching the CLI flag surface or config/types.go.
docs:
	go run ./cmd/tmh-gen

# Alias for just the JSON schema (faster feedback loop during config work).
schema:
	go run ./cmd/tmh-gen

# Render animated demos used by README.md.
# The render script sandboxes HOME + the tmux socket so it doesn't touch
# your real config or live sessions. Requires `vhs` (brew install vhs).
demo:
	./scripts/render-demos.sh

release: check-tag
	git tag -a "$(TAG)" -m "tmh $(TAG)"
	git push origin "$(TAG)"
	git push github "$(TAG)"

check-tag:
	@test -n "$(TAG)" || (echo "usage: make release TAG=v1.0.0" >&2; exit 1)
	@printf '%s\n' "$(TAG)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$$' || \
		(echo "Invalid TAG: $(TAG). Expected vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-prerelease" >&2; exit 1)

goreleaser-local:
	goreleaser build --snapshot --clean

goreleaser-snapshot:
	goreleaser release --snapshot --clean
