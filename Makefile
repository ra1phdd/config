GO ?= go
APP ?= ./
BIN ?= bin/main
GOFMT ?= gofmt
GOFUMPT ?= gofumpt
GOIMPORTS ?= goimports
GOLANGCI_LINT ?= golangci-lint
DEADCODE ?= deadcode
GO_FILES := $(shell find . -type f -name '*.go' -not -path './.git/*' -not -path './vendor/*')

.PHONY: run build test test-integration test-e2e test-manual lint fmt check-go-files check-format-tools check-lint-tools
.PHONY: help

help:
	@echo "Makefile commands"
	@echo ""
	@echo "Development:"
	@echo "  make run              - Run the go project (default: ./cmd/main)"
	@echo "  make build            - Build the go binary (output: bin/main)"
	@echo "  make test             - Run unit tests (go vet + go test)"
	@echo "  make test-integration - Run integration tests"
	@echo "  make test-e2e         - Run end-to-end tests"
	@echo "  make test-manual      - Run manual tests (requires ALLOW_MANUAL_TESTS=1)"
	@echo "  make lint             - Run linters (gofmt, gofumpt, goimports, golangci-lint, deadcode)"
	@echo "  make fmt              - Format Go code"
	@echo ""
	@echo "Variables (override with make VAR=value):"
	@echo "  APP=$(APP)"
	@echo "  BIN=$(BIN)"
	@echo "  GHCR_REGISTRY=$(GHCR_REGISTRY)"


run:
	$(GO) run $(APP)

build:
	mkdir -p $(dir $(BIN))
	$(GO) build -o $(BIN) $(APP)

test:
	$(GO) vet ./...
	$(GO) test ./...

test-integration:
	$(GO) test -tags=integration ./...

test-e2e:
	$(GO) test -tags=e2e ./...

test-manual:
	@if [ -n "$${CI:-}" ]; then echo 'manual tests must not run in CI' >&2; exit 1; fi
	@test "$(ALLOW_MANUAL_TESTS)" = "1" || { echo 'manual tests require explicit permission: set ALLOW_MANUAL_TESTS=1' >&2; exit 1; }
	$(GO) test -tags=manual ./...

lint: check-go-files check-lint-tools
	@test -z "$$($(GOFMT) -l $(GO_FILES))" || { $(GOFMT) -l $(GO_FILES); echo 'gofmt required; run make fmt' >&2; exit 1; }
	@test -z "$$($(GOFUMPT) -l $(GO_FILES))" || { $(GOFUMPT) -l $(GO_FILES); echo 'gofumpt required; run make fmt' >&2; exit 1; }
	@test -z "$$($(GOIMPORTS) -l $(GO_FILES))" || { $(GOIMPORTS) -l $(GO_FILES); echo 'goimports required; run make fmt' >&2; exit 1; }
	$(GOLANGCI_LINT) run --fix
	@test -z "$$($(DEADCODE) -test $(APP))" || { $(DEADCODE) -test $(APP); echo 'deadcode required; run make fmt' >&2; exit 1; }

fmt: check-go-files check-format-tools
	$(GOFMT) -w $(GO_FILES)
	$(GOIMPORTS) -w $(GO_FILES)
	$(GOFUMPT) -w $(GO_FILES)

check-go-files:
	@test -n "$(GO_FILES)" || { echo 'no Go files found' >&2; exit 1; }

check-format-tools:
	@for tool in "$(GOFMT)" "$(GOFUMPT)" "$(GOIMPORTS)"; do command -v "$$tool" >/dev/null || { echo "required tool not found in PATH: $$tool" >&2; exit 1; }; done

check-lint-tools: check-format-tools
	@command -v "$(GOLANGCI_LINT)" >/dev/null || { echo "required tool not found in PATH: $(GOLANGCI_LINT)" >&2; exit 1; }
