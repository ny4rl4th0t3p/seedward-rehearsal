# ──────────────────────────────────────────────────────────────────────────────
# Variables
# ──────────────────────────────────────────────────────────────────────────────
BINARY_DIR  := ./build

GO          := go
CGO_ENABLED ?= 0
GOFLAGS     := CGO_ENABLED=$(CGO_ENABLED)
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_FLAGS := -trimpath -ldflags="-s -w -X 'github.com/ny4rl4th0t3p/seedward-rehearsal/internal/version.Version=$(VERSION)'"

.DEFAULT_GOAL := help

# ──────────────────────────────────────────────────────────────────────────────
# Help
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ──────────────────────────────────────────────────────────────────────────────
# Build
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: build
build: ## Build both binaries → build/rehearse and build/rehearsald
	@mkdir -p $(BINARY_DIR)
	$(GOFLAGS) $(GO) build $(BUILD_FLAGS) -o $(BINARY_DIR)/rehearse ./cmd/rehearse
	$(GOFLAGS) $(GO) build $(BUILD_FLAGS) -o $(BINARY_DIR)/rehearsald ./cmd/rehearsald

.PHONY: install
install: ## Install both binaries to $(GOPATH)/bin (or ~/go/bin)
	$(GOFLAGS) $(GO) install $(BUILD_FLAGS) ./cmd/rehearse ./cmd/rehearsald

# ──────────────────────────────────────────────────────────────────────────────
# Test
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: test
test: ## Run unit tests
	$(GO) test -count=1 ./...

.PHONY: test-race
test-race: ## Run unit tests with the race detector
	$(GO) test -race -count=1 ./...

# ──────────────────────────────────────────────────────────────────────────────
# Code quality
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: fmt
fmt: ## Format all Go source files
	$(GO) fmt ./...

.PHONY: fmt-check
fmt-check: ## Check formatting without modifying files (exits non-zero if changes needed)
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files need formatting:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy
	$(GO) mod tidy

.PHONY: tidy-check
tidy-check: ## Check that go.mod and go.sum are tidy (exits non-zero if not)
	$(GO) mod tidy && git diff --exit-code go.mod go.sum

.PHONY: lint
lint: ## Run golangci-lint (report only — used by check/CI)
	golangci-lint cache clean
	golangci-lint run

.PHONY: lint-fix
lint-fix: ## Run golangci-lint and auto-fix what it can
	golangci-lint run --fix

.PHONY: check
check: fmt-check vet tidy-check lint test ## Run all checks (fmt + vet + tidy + lint + unit tests)

# ──────────────────────────────────────────────────────────────────────────────
# Clean
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BINARY_DIR)