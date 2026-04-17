.PHONY: build test test-race lint install clean fmt docs schema

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
