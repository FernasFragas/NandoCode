.PHONY: build test test-race test-integration test-e2e lint fmt fmt-check vet security verify-phase-0 tidy install clean vendor check help docker-build docker-run docker-run-daemon docker-stop docker-clean docker-shell docker-compose-up docker-compose-down docker-compose-logs regression-fast regression-full load-suite regression-report

# Variables
BINARY_NAME=nandocodego
BUILD_DIR=./bin
CMD_DIR=./cmd/nandocodego
INSTALL_PATH=/usr/local/bin

# Build metadata.
# MODULE_PATH is read from go.mod so ldflags do not hardcode a GitHub URL.
MODULE_PATH := $(shell go list -m 2>/dev/null)
VERSION ?= 0.0.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
           -X '$(MODULE_PATH)/internal/version.Version=$(VERSION)' \
           -X '$(MODULE_PATH)/internal/version.Commit=$(COMMIT)' \
           -X '$(MODULE_PATH)/internal/version.BuildTime=$(BUILD_TIME)'

# Docker variables
DOCKER_IMAGE_NAME=nandocodego
DOCKER_TAG ?= latest
DOCKER_FULL_NAME=$(DOCKER_IMAGE_NAME):$(DOCKER_TAG)
DOCKER_CONTAINER_NAME=nandocodego-container
DOCKER_HOST_PORT ?= 8080
DOCKER_CONTAINER_PORT ?= 8080
DOCKER_WEB_HOST ?= 0.0.0.0

# Default target
all: build

## help: Display this help message
help:
	@echo "nandocodego Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build            Build the binary"
	@echo "  test             Run unit tests"
	@echo "  test-race        Run tests with race detector"
	@echo "  test-integration Run integration tests"
	@echo "  test-e2e         Run end-to-end tests"
	@echo "  lint             Run golangci-lint"
	@echo "  fmt              Format code with gofumpt"
	@echo "  fmt-check        Check formatting without modifying files"
	@echo "  vet              Run go vet"
	@echo "  security         Run security checks"
	@echo "  verify-phase-0   Run Phase 0 verification"
	@echo "  tidy             Tidy go.mod and go.sum"
	@echo "  install          Install binary to $(INSTALL_PATH)"
	@echo "  clean            Remove build artifacts"
	@echo "  vendor           Vendor dependencies"
	@echo "  check            Run all checks (build, vet, test, lint)"
	@echo "  regression-fast  Run fast regression gate from docs plan"
	@echo "  regression-full  Run full regression gate from docs plan"
	@echo "  load-suite       Run load/perf test suite from docs plan"
	@echo "  regression-report Run all automated gates and generate markdown report"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build     Build Docker image"
	@echo "  docker-run       Run application in Docker container (interactive, publishes $(DOCKER_HOST_PORT):$(DOCKER_CONTAINER_PORT))"
	@echo "  docker-run-daemon Run application in Docker container (background, publishes $(DOCKER_HOST_PORT):$(DOCKER_CONTAINER_PORT))"
	@echo "  docker-stop      Stop running Docker containers"
	@echo "  docker-clean     Remove Docker images and containers"
	@echo "  docker-shell     Get a shell inside the Docker container"
	@echo "  docker-compose-up Start the compose stack"
	@echo "  docker-compose-down Stop the compose stack"
	@echo "  docker-compose-logs Follow compose logs"
	@echo ""
	@echo "  help             Display this help message"

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)"

## test: Run unit tests
test:
	@echo "Running unit tests..."
	go test -v -timeout=30s ./...

## test-race: Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -race -timeout=120s ./...

## test-integration: Run integration tests
test-integration:
	@echo "Running integration tests..."
	go test -v -timeout=120s -tags=integration ./...

## test-e2e: Run end-to-end tests
test-e2e:
	@echo "Running e2e tests..."
	go test -v -timeout=300s -tags=e2e ./e2e/...

## lint: Run golangci-lint
lint:
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint not found. Install: https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

## fmt: Format code with gofumpt
fmt:
	@echo "Formatting code..."
	@if command -v gofumpt >/dev/null 2>&1; then \
		gofumpt -l -w .; \
	else \
		echo "⚠️  gofumpt not found, using gofmt instead"; \
		gofmt -l -w .; \
	fi
	@echo "✓ Code formatted"

## fmt-check: Check formatting without modifying files
fmt-check:
	@echo "Checking formatting..."
	@if command -v gofumpt >/dev/null 2>&1; then \
		files="$$(gofumpt -l .)"; \
	else \
		echo "gofumpt not found, using gofmt instead"; \
		files="$$(gofmt -l .)"; \
	fi; \
	if [ -n "$$files" ]; then \
		echo "$$files"; \
		echo "Formatting check failed"; \
		exit 1; \
	fi
	@echo "✓ Formatting clean"

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## security: Run security checks
security:
	@echo "Running security checks..."
	@./tools/check-allowed-deps.sh
	@./tools/check-network-policy.sh
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not found; install github.com/securego/gosec/v2/cmd/gosec"; \
		exit 1; \
	fi
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not found; install golang.org/x/vuln/cmd/govulncheck"; \
		exit 1; \
	fi

## verify-phase-0: Run Phase 0 verification
verify-phase-0:
	@./tools/verify-phase-0.sh

## tidy: Tidy go.mod and go.sum
tidy:
	@echo "Tidying Go modules..."
	go mod tidy

## install: Install binary to /usr/local/bin
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PATH)
	@echo "✓ Installed: $(INSTALL_PATH)/$(BINARY_NAME)"

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@go clean
	@echo "✓ Cleaned"

## vendor: Vendor dependencies
vendor:
	@echo "Vendoring dependencies..."
	go mod vendor
	@echo "✓ Dependencies vendored"

## check: Run all checks (like CI)
check:
	@echo "Running all checks..."
	@echo ""
	@echo "==> Building..."
	@$(MAKE) build
	@echo ""
	@echo "==> Running go vet..."
	@$(MAKE) vet
	@echo ""
	@echo "==> Running tests with race detector..."
	@$(MAKE) test-race
	@echo ""
	@echo "==> Running linter..."
	@$(MAKE) lint
	@echo ""
	@echo "==> Running security checks..."
	@$(MAKE) verify-phase-0
	@echo ""
	@echo "✓ All checks passed!"

## regression-fast: Run fast regression gate from docs/REGRESSION-AND-LOAD-TEST-PLAN.md
regression-fast:
	@tools/run-regression-fast.sh

## regression-full: Run full regression gate from docs/REGRESSION-AND-LOAD-TEST-PLAN.md
regression-full:
	@tools/run-regression-full.sh

## load-suite: Run load/perf suite from docs/REGRESSION-AND-LOAD-TEST-PLAN.md
load-suite:
	@tools/run-load-suite.sh

## regression-report: Run all automated gates and generate markdown report artifact
regression-report:
	@tools/run-regression-report.sh

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image $(DOCKER_FULL_NAME)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t $(DOCKER_FULL_NAME) .
	@echo "✓ Docker image built: $(DOCKER_FULL_NAME)"

## docker-run: Run application in Docker container (interactive)
docker-run:
	@echo "Running $(DOCKER_FULL_NAME) interactively..."
	docker run --rm -it \
		--name $(DOCKER_CONTAINER_NAME) \
		-p $(DOCKER_HOST_PORT):$(DOCKER_CONTAINER_PORT) \
		-e XDG_CONFIG_HOME=/home/nandocodego/.config \
		-e XDG_DATA_HOME=/home/nandocodego/.local/share \
		-e XDG_CACHE_HOME=/home/nandocodego/.cache \
		-e XDG_STATE_HOME=/home/nandocodego/.local/state \
		-e NANDOCODEGO_HOST=$(DOCKER_WEB_HOST) \
		-e NANDOCODEGO_PORT=$(DOCKER_CONTAINER_PORT) \
		-v $(HOME)/.config/nandocodego:/home/nandocodego/.config/nandocodego \
		-v $(HOME)/.local/share/nandocodego:/home/nandocodego/.local/share/nandocodego \
		-v $(HOME)/.cache/nandocodego:/home/nandocodego/.cache/nandocodego \
		-v $(HOME)/.local/state/nandocodego:/home/nandocodego/.local/state/nandocodego \
		$(DOCKER_FULL_NAME) $(ARGS)

## docker-run-daemon: Run application in Docker container (background)
docker-run-daemon:
	@echo "Running $(DOCKER_FULL_NAME) in background..."
	docker run -d \
		--name $(DOCKER_CONTAINER_NAME) \
		-p $(DOCKER_HOST_PORT):$(DOCKER_CONTAINER_PORT) \
		-e XDG_CONFIG_HOME=/home/nandocodego/.config \
		-e XDG_DATA_HOME=/home/nandocodego/.local/share \
		-e XDG_CACHE_HOME=/home/nandocodego/.cache \
		-e XDG_STATE_HOME=/home/nandocodego/.local/state \
		-e NANDOCODEGO_HOST=$(DOCKER_WEB_HOST) \
		-e NANDOCODEGO_PORT=$(DOCKER_CONTAINER_PORT) \
		-v $(HOME)/.config/nandocodego:/home/nandocodego/.config/nandocodego \
		-v $(HOME)/.local/share/nandocodego:/home/nandocodego/.local/share/nandocodego \
		-v $(HOME)/.cache/nandocodego:/home/nandocodego/.cache/nandocodego \
		-v $(HOME)/.local/state/nandocodego:/home/nandocodego/.local/state/nandocodego \
		$(DOCKER_FULL_NAME) $(ARGS)
	@echo "✓ Container started: $(DOCKER_CONTAINER_NAME)"

## docker-stop: Stop running Docker containers
docker-stop:
	@echo "Stopping Docker container..."
	@docker stop $(DOCKER_CONTAINER_NAME) 2>/dev/null || true
	@docker rm $(DOCKER_CONTAINER_NAME) 2>/dev/null || true
	@echo "✓ Container stopped"

## docker-clean: Remove Docker images and containers
docker-clean: docker-stop
	@echo "Removing Docker images..."
	@docker rmi $(DOCKER_FULL_NAME) 2>/dev/null || true
	@docker rmi $(DOCKER_IMAGE_NAME):latest 2>/dev/null || true
	@echo "✓ Docker artifacts cleaned"

## docker-shell: Get a shell inside the Docker container
docker-shell:
	@echo "Opening shell in $(DOCKER_FULL_NAME)..."
	docker run --rm -it \
		--name $(DOCKER_CONTAINER_NAME)-shell \
		-e XDG_CONFIG_HOME=/home/nandocodego/.config \
		-e XDG_DATA_HOME=/home/nandocodego/.local/share \
		-e XDG_CACHE_HOME=/home/nandocodego/.cache \
		-e XDG_STATE_HOME=/home/nandocodego/.local/state \
		-v $(HOME)/.config/nandocodego:/home/nandocodego/.config/nandocodego \
		-v $(HOME)/.local/share/nandocodego:/home/nandocodego/.local/share/nandocodego \
		-v $(HOME)/.cache/nandocodego:/home/nandocodego/.cache/nandocodego \
		-v $(HOME)/.local/state/nandocodego:/home/nandocodego/.local/state/nandocodego \
		--entrypoint /bin/sh \
		$(DOCKER_FULL_NAME)


## docker-compose-up: Start the compose stack
docker-compose-up:
	@echo "Starting docker compose stack..."
	docker compose up --build

## docker-compose-down: Stop the compose stack
docker-compose-down:
	@echo "Stopping docker compose stack..."
	docker compose down

## docker-compose-logs: Follow compose logs
docker-compose-logs:
	docker compose logs -f
