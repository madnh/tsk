BINARY  := tsk
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)"

.DEFAULT_GOAL := help

.PHONY: help build run install vet clean release

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build binary to ./tsk
	go build $(LDFLAGS) -o $(BINARY) .

run: ## Run without building (pass args via ARGS="...")
	go run $(LDFLAGS) . -- $(ARGS)

install: ## Install binary to GOPATH/bin
	go install $(LDFLAGS) .

vet: ## Run go vet
	go vet ./...

clean: ## Remove built binary
	rm -f $(BINARY)

release: ## Run goreleaser release
	goreleaser release --clean
