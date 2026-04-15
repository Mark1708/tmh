.PHONY: build test test-race lint install clean fmt

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
