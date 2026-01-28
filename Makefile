# vsp Makefile

# Binary name
BINARY_NAME=vsp

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofumpt
GOLINT=golangci-lint

# Build directories
BUILD_DIR=build
CMD_DIR=./cmd/vsp

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"

# Platforms for cross-compilation
PLATFORMS_LINUX=linux/amd64 linux/arm64 linux/386 linux/arm
PLATFORMS_DARWIN=darwin/amd64 darwin/arm64
PLATFORMS_WINDOWS=windows/amd64 windows/arm64 windows/386
PLATFORMS=$(PLATFORMS_LINUX) $(PLATFORMS_DARWIN) $(PLATFORMS_WINDOWS)

# Common platforms (fast build)
PLATFORMS_COMMON=linux/amd64 darwin/arm64 windows/amd64

# Current platform detection
CURRENT_OS=$(shell go env GOOS)
CURRENT_ARCH=$(shell go env GOARCH)

.PHONY: all build clean test test-integration lint fmt deps tidy help install run
.PHONY: build-all build-all-all build-linux build-darwin build-windows
.PHONY: deploy-windows sync-embedded

all: deps lint test build

## Build

build: ## Build the binary for current platform
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY_NAME)"

build-all: ## Build for common platforms (linux-amd64, darwin-arm64, windows-amd64) + local ./build/vsp
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS_COMMON); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY_NAME)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then output=$$output.exe; fi; \
		echo "Building $$output..."; \
		GOOS=$$os GOARCH=$$arch $(GOBUILD) $(LDFLAGS) -o $$output $(CMD_DIR) || exit 1; \
	done
	@echo "Copying current platform binary to $(BUILD_DIR)/$(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME)-$(CURRENT_OS)-$(CURRENT_ARCH) $(BUILD_DIR)/$(BINARY_NAME) 2>/dev/null || \
		$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete. Binaries in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

build-all-all: ## Build for ALL platforms (linux, darwin, windows - amd64, arm64, 386, arm)
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY_NAME)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then output=$$output.exe; fi; \
		echo "Building $$output..."; \
		GOOS=$$os GOARCH=$$arch $(GOBUILD) $(LDFLAGS) -o $$output $(CMD_DIR) || exit 1; \
	done
	@echo "Build complete. Binaries in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

build-linux: ## Build for Linux (amd64, arm64, 386, arm)
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS_LINUX); do \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY_NAME)-linux-$$arch; \
		echo "Building $$output..."; \
		GOOS=linux GOARCH=$$arch $(GOBUILD) $(LDFLAGS) -o $$output $(CMD_DIR) || exit 1; \
	done

build-darwin: ## Build for macOS (amd64, arm64)
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS_DARWIN); do \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY_NAME)-darwin-$$arch; \
		echo "Building $$output..."; \
		GOOS=darwin GOARCH=$$arch $(GOBUILD) $(LDFLAGS) -o $$output $(CMD_DIR) || exit 1; \
	done

build-windows: ## Build for Windows (amd64, arm64, 386)
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS_WINDOWS); do \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY_NAME)-windows-$$arch.exe; \
		echo "Building $$output..."; \
		GOOS=windows GOARCH=$$arch $(GOBUILD) $(LDFLAGS) -o $$output $(CMD_DIR) || exit 1; \
	done

# WSL deployment directory
WINDOWS_DEPLOY_DIR=/mnt/c/bin/vibing-steampunk

deploy-windows: ## Build Windows amd64 and deploy to /mnt/c/bin/vibing-steampunk/
	@mkdir -p $(BUILD_DIR)
	@echo "Building Windows amd64 binary..."
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)
	@mkdir -p $(WINDOWS_DEPLOY_DIR)
	@echo "Deploying to $(WINDOWS_DEPLOY_DIR)..."
	cp $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(WINDOWS_DEPLOY_DIR)/$(BINARY_NAME).exe
	@echo "Deployed: $(WINDOWS_DEPLOY_DIR)/$(BINARY_NAME).exe"
	@ls -lh $(WINDOWS_DEPLOY_DIR)/$(BINARY_NAME).exe

sync-embedded: build ## Export $ZADT_VSP from SAP to embedded/abap/ (requires SAP_* env vars)
	@echo "Exporting ZADT_VSP package from SAP..."
	@mkdir -p embedded/abap
	VSP_OUTPUT_DIR=embedded/abap $(BUILD_DIR)/$(BINARY_NAME) lua scripts/sync-embedded.lua
	@echo "Files in embedded/abap/"
	@ls -lh embedded/abap/*.abap 2>/dev/null || echo "No files exported"

install: ## Install the binary
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) $(CMD_DIR)

## Development

run: ## Run the server (requires SAP_* env vars)
	$(GOCMD) run $(CMD_DIR)

deps: ## Download dependencies
	$(GOMOD) download

tidy: ## Tidy go.mod
	$(GOMOD) tidy

fmt: ## Format code
	@if command -v $(GOFMT) >/dev/null 2>&1; then \
		$(GOFMT) -w .; \
	else \
		$(GOCMD) fmt ./...; \
	fi

lint: ## Run linter
	@if command -v $(GOLINT) >/dev/null 2>&1; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

## Testing

test: ## Run tests
	$(GOTEST) -v -race ./...

test-integration: ## Run integration tests (requires SAP_* env vars)
	$(GOTEST) -tags=integration -v ./...

test-coverage: ## Run tests with coverage
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## Cleanup

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

## Help

help: ## Display this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
