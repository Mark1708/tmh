.PHONY: build test test-race lint install clean fmt docs schema demo

BINARY := tmh
CMD    := ./cmd/tmh
BIN    := $(shell go env GOPATH)/bin

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

# Regenerate docs/man, docs/completions, and schemas/tmh.schema.json.
# Run after touching the CLI flag surface or config/types.go.
docs:
	go run ./cmd/tmh-gen

# Alias for just the JSON schema (faster feedback loop during config work).
schema:
	go run ./cmd/tmh-gen

# Render animated demos used by README.md. Requires `vhs`, `ttyd`, and
# `ffmpeg` on the PATH (one-time: `brew install vhs`).
demo:
	@command -v vhs >/dev/null 2>&1 || { echo "vhs not installed — brew install vhs"; exit 1; }
	vhs docs/demo-picker.tape
	vhs docs/demo-diff.tape
	vhs docs/demo-freeze.tape
