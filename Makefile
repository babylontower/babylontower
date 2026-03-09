.PHONY: build test test-all test-integration test-coverage lint clean run proto help test-e2e docker-build docker-run docker-stop docker-clean build-all-platforms build-ui run-ui setup-ui-devcontainer build-ui-cross build-ui-linux build-ui-windows build-ui-darwin

# Version
VERSION=0.0.1
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build -buildvcs=false -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"
GOTEST=$(GOCMD) test -buildvcs=false
GOVET=$(GOCMD) vet -buildvcs=false
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Binary name
BINARY_NAME=messenger
BINARY_UI_NAME=babylon-ui
BINARY_DIR=bin
BINARY_PLATFORM_DIR=$(BINARY_DIR)/platform
BINARY_UI_PLATFORM_DIR=$(BINARY_PLATFORM_DIR)/ui

# Protobuf
PROTO_DIR=proto
PROTO_SRC=$(PROTO_DIR)/message.proto
PROTO_OUT=pkg/proto

# Linter
LINTER=golangci-lint

# Test directories
TEST_DIR=test
TEST_SCRIPTS_DIR=scripts/test

# Platforms for cross-compilation
PLATFORMS=linux darwin windows
ARCHS=amd64 arm64

all: build

## build: Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BINARY_DIR)
	@$(GOBUILD) -o $(BINARY_DIR)/$(BINARY_NAME) ./cmd/messenger

## build-ui: Build the Gio UI application
build-ui:
	@echo "Building $(BINARY_UI_NAME)..."
	@mkdir -p $(BINARY_UI_PLATFORM_DIR)/native
ifeq ($(OS),Windows_NT)
	@$(GOBUILD) -ldflags "-H windowsgui" -o $(BINARY_UI_PLATFORM_DIR)/native/$(BINARY_UI_NAME).exe ./cmd/babylon-ui
else
	@$(GOBUILD) -o $(BINARY_UI_PLATFORM_DIR)/native/$(BINARY_UI_NAME) ./cmd/babylon-ui
endif
	@echo "Build complete: $(BINARY_UI_PLATFORM_DIR)/native/$(BINARY_UI_NAME)"
	@echo ""
	@echo "Note: Gio requires system libraries for GUI rendering."
	@echo "If build fails, rebuild the devcontainer:"
	@echo "  1. VS Code: Ctrl+Shift+P → 'Dev Containers: Rebuild Container'"
	@echo "  2. Wait for rebuild to complete"
	@echo "  3. Run 'make build-ui' again"
	@echo ""
	@echo "Or see pkg/ui/DEVCONTAINER.md for setup instructions"
	@echo ""
	@echo "Run with: $(BINARY_UI_PLATFORM_DIR)/native/$(BINARY_UI_NAME)"

## build-ui-cross: Cross-compile UI for multiple platforms
build-ui-cross:
	@echo "Cross-compiling $(BINARY_UI_NAME) for multiple platforms..."
	@./scripts/build-ui-cross.sh all

## build-ui-linux: Build UI for Linux
build-ui-linux:
	@echo "Building $(BINARY_UI_NAME) for Linux..."
	@mkdir -p $(BINARY_UI_PLATFORM_DIR)/linux
	@GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UI_PLATFORM_DIR)/linux/$(BINARY_UI_NAME) ./cmd/babylon-ui
	@echo "Linux binary: $(BINARY_UI_PLATFORM_DIR)/linux/$(BINARY_UI_NAME)"

## build-ui-windows: Build UI for Windows (requires mingw-w64)
build-ui-windows:
	@echo "Building $(BINARY_UI_NAME) for Windows..."
	@mkdir -p $(BINARY_UI_PLATFORM_DIR)/windows
	@GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 \
		$(GOBUILD) -ldflags "-H windowsgui" -o $(BINARY_UI_PLATFORM_DIR)/windows/$(BINARY_UI_NAME).exe ./cmd/babylon-ui
	@echo "Windows binary: $(BINARY_UI_PLATFORM_DIR)/windows/$(BINARY_UI_NAME).exe"

## build-ui-darwin: Build UI for macOS
build-ui-darwin:
	@echo "Building $(BINARY_UI_NAME) for macOS..."
	@mkdir -p $(BINARY_UI_PLATFORM_DIR)/darwin
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
		$(GOBUILD) -o $(BINARY_UI_PLATFORM_DIR)/darwin/$(BINARY_UI_NAME) ./cmd/babylon-ui
	@echo "macOS binary: $(BINARY_UI_PLATFORM_DIR)/darwin/$(BINARY_UI_NAME)"
	@echo ""
	@echo "Note: macOS build has limited functionality without native GUI libraries."
	@echo "For full GUI support, build natively on macOS."

## setup-ui-devcontainer: Show instructions for setting up UI devcontainer
setup-ui-devcontainer:
	@echo "Setting up devcontainer for Gio UI development..."
	@echo ""
	@./scripts/setup-ui-devcontainer.sh

## run-ui: Build and run the Gio UI application
run-ui: build-ui
	@echo "Running $(BINARY_UI_NAME)..."
	@$(BINARY_UI_PLATFORM_DIR)/native/$(BINARY_UI_NAME)

## build-all: Build for all platforms (linux, macos, windows)
build-all: build-linux build-darwin build-windows

## build-linux: Build for Linux
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BINARY_PLATFORM_DIR)/linux
	@GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_PLATFORM_DIR)/linux/$(BINARY_NAME) ./cmd/messenger
	@echo "Linux binary: $(BINARY_PLATFORM_DIR)/linux/$(BINARY_NAME)"

## build-darwin: Build for macOS
build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BINARY_PLATFORM_DIR)/darwin
	@GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_PLATFORM_DIR)/darwin/$(BINARY_NAME) ./cmd/messenger
	@echo "macOS binary: $(BINARY_PLATFORM_DIR)/darwin/$(BINARY_NAME)"

## build-windows: Build for Windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BINARY_PLATFORM_DIR)/windows
	@GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_PLATFORM_DIR)/windows/$(BINARY_NAME).exe ./cmd/messenger
	@echo "Windows binary: $(BINARY_PLATFORM_DIR)/windows/$(BINARY_NAME).exe"

## test: Run tests
test:
	@echo "Running tests..."
	@$(GOTEST) -short ./...

## test-all: Run all tests including integration tests
test-all:
	@echo "Running all tests (including integration)..."
	@$(GOTEST) ./...
	@$(GOTEST) -tags=integration ./pkg/ipfsnode/... -timeout 5m

## test-integration: Run integration tests only
test-integration:
	@echo "Running integration tests..."
	@$(GOTEST) -tags=integration ./pkg/ipfsnode/... -v -timeout 5m

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@$(GOTEST) -short -coverprofile=coverage.out ./...
	@$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run linter
lint:
	@echo "Running linter..."
	@if command -v $(LINTER) > /dev/null 2>&1; then \
		GOFLAGS="-buildvcs=false" $(LINTER) run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@$(GOFMT) -s -w .

## vet: Run go vet
vet:
	@echo "Running go vet..."
	@$(GOVET) ./...

## proto: Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@if command -v protoc > /dev/null 2>&1; then \
		export PATH=$$PATH:$(go env GOPATH)/bin; \
		protoc --go_out=. --go_opt=paths=source_relative $(PROTO_SRC); \
		mv $(PROTO_DIR)/message.pb.go $(PROTO_OUT)/message.pb.go; \
	else \
		echo "protoc not installed. Install protobuf compiler first."; \
		exit 1; \
	fi

## tidy: Tidy go modules
tidy:
	@echo "Tidying go modules..."
	@$(GOMOD) tidy

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BINARY_DIR)
	@rm -f coverage.out coverage.html

## run: Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(BINARY_DIR)/$(BINARY_NAME)

## version: Show version information
version:
	@echo "Babylon Tower v$(VERSION)"
	@echo "Build time: $(BUILD_TIME)"
	@echo "Git commit: $(GIT_COMMIT)"

## install-hooks: Install git hooks
install-hooks:
	@echo "Installing git hooks..."
	@git config core.hooksPath .githooks
	@echo "Git hooks installed successfully"

## uninstall-hooks: Remove git hooks
uninstall-hooks:
	@echo "Uninstalling git hooks..."
	@git config --unset core.hooksPath
	@echo "Git hooks uninstalled"

## install-deps: Install development dependencies
install-deps:
	@echo "Installing development dependencies..."
	@$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint/v2@latest

## test-e2e: Run end-to-end tests (deprecated, use test-integration)
test-e2e:
	@echo "E2E tests are deprecated. Use 'make test-integration' instead."

## launch-multi-node: Launch N nodes for scale testing (default: 5)
launch-multi-node: build-all
	@$(TEST_SCRIPTS_DIR)/launch-multi-node.sh $(or $(NODES),5) $(or $(MODE),background)

## stop-multi-node: Stop all nodes from multi-node test
stop-multi-node:
	@$(TEST_SCRIPTS_DIR)/stop-multi-node.sh

## verify-connections: Verify node connections in multi-node test
verify-connections:
	@$(TEST_SCRIPTS_DIR)/verify-connections.sh $(or $(NODES),5)

## test-scale: Run scale test (20 nodes, wait, verify)
test-scale: build-all
	@echo "Running scale test with 20 nodes..."
	@$(TEST_SCRIPTS_DIR)/launch-multi-node.sh 20 background
	@echo "Waiting 60 seconds for network formation..."
	@sleep 60
	@$(TEST_SCRIPTS_DIR)/verify-connections.sh 20
	@echo ""
	@echo "Scale test complete. To cleanup:"
	@echo "  make stop-multi-node"
	@echo "  make clean-test"

## launch-test: Launch two instances locally for manual testing
launch-test: build-all
	@echo "Launching two instances for manual testing..."
	@echo ""
	@echo "Open two terminals and run:"
	@echo "  Terminal 1: ./scripts/test/launch-instance1.sh"
	@echo "  Terminal 2: ./scripts/test/launch-instance2.sh"
	@echo ""
	@echo "Binaries built for all platforms in $(BINARY_PLATFORM_DIR)/"

## launch-test-docker: Launch two instances in Docker for manual testing
launch-test-docker:
	@echo "Launching two Docker instances for manual testing..."
	@$(TEST_SCRIPTS_DIR)/launch-two-instances.sh docker

## launch-instance1: Launch Instance 1 (Alice)
launch-instance1: build-all
	@$(TEST_SCRIPTS_DIR)/launch-instance1.sh

## launch-instance2: Launch Instance 2 (Bob)
launch-instance2: build-all
	@$(TEST_SCRIPTS_DIR)/launch-instance2.sh

## docker-build: Build Docker image for testing
docker-build:
	@echo "Building Docker image..."
	@docker build -t babylontower:test -f $(TEST_DIR)/Dockerfile .

## docker-run: Run two instances using docker-compose
docker-run: docker-build
	@echo "Starting two Docker instances..."
	@cd $(TEST_DIR) && docker-compose up -d
	@echo ""
	@echo "Instances started:"
	@echo "  - Alice: docker exec -it babylon-alice /app/messenger"
	@echo "  - Bob:   docker exec -it babylon-bob /app/messenger"

## docker-stop: Stop Docker containers
docker-stop:
	@echo "Stopping Docker containers..."
	@cd $(TEST_DIR) && docker-compose down

## docker-clean: Clean up Docker containers and test data
docker-clean: docker-stop
	@echo "Cleaning up Docker resources..."
	@cd $(TEST_DIR) && docker-compose rm -f
	@rm -rf test-data/instance1 test-data/instance2
	@echo "Cleanup complete"

## clean-platform: Clean platform-specific binaries
clean-platform:
	@echo "Cleaning platform-specific binaries..."
	@rm -rf $(BINARY_PLATFORM_DIR)
	@echo "Platform binaries cleaned"

## clean-test: Clean test data and artifacts
clean-test:
	@echo "Cleaning test artifacts..."
	@$(TEST_SCRIPTS_DIR)/clean-test-data.sh
	@rm -f $(TEST_DIR)/coverage.out $(TEST_DIR)/coverage.html

help:
	@echo "Babylon Tower - Available commands:"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
