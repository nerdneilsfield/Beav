GO ?= go
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%d)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test lint
build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/beav ./cmd/beav
test:
	$(GO) test ./...
lint:
	golangci-lint run ./...
