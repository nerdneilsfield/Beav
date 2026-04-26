GO ?= go
VERSION ?= dev
COVER_MIN ?= 50
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%d)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: help build test race cover lint clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build bin/beav
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/beav ./cmd/beav

test: ## Run unit and integration tests
	$(GO) test ./...

race: ## Run tests with the race detector
	$(GO) test -race ./...

cover: ## Run tests with coverage and enforce COVER_MIN percentage
	$(GO) test -coverprofile=cover.out ./...
	$(GO) tool cover -func=cover.out | tee coverage.txt
	awk -v min=$(COVER_MIN) '$$1=="total:" { gsub("%","",$$3); if ($$3+0 < min) exit 1 }' coverage.txt

lint: ## Run golangci-lint
	golangci-lint run ./...

clean: ## Remove local build and coverage artifacts
	rm -rf bin cover.out coverage.txt
