.PHONY: build test lint clean run proto help

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
BINARY_DIR=bin

# Protobuf
PROTO_DIR=proto
PROTO_SRC=$(PROTO_DIR)/message.proto
PROTO_OUT=pkg/proto

# Linter
LINTER=golangci-lint

all: build

## build: Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BINARY_DIR)
	@$(GOBUILD) -o $(BINARY_DIR)/$(BINARY_NAME) ./cmd/messenger

## test: Run tests
test:
	@echo "Running tests..."
	@$(GOTEST) -v ./...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@$(GOTEST) -v -coverprofile=coverage.out ./...
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

help:
	@echo "Babylon Tower - Available commands:"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
