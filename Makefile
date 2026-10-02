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
SSO_HELPER_DIR=./cmd/vsp-sso
SSO_HELPER=$(BUILD_DIR)/vsp-sso.exe

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

# The local build carries its platform in the name, exactly like the
# cross-compiled ones, and $(BUILD_DIR)/$(BINARY_NAME) is a link to it. Tool
# configs (.mcp.json, .vsp.json, editor settings) point at one name or the
# other; with a link they are the same file, so a single `make build` refreshes
# whatever they reference. Before this, `build` wrote only ./build/vsp while
# every MCP config pointed at ./build/vsp-<os>-<arch>, which only build-all
# refreshed — so the binary the agents ran silently fell behind the source.
EXE=$(if $(filter windows,$(CURRENT_OS)),.exe,)
LOCAL_BINARY=$(BINARY_NAME)-$(CURRENT_OS)-$(CURRENT_ARCH)$(EXE)

.PHONY: all build clean test lint lint-full metrics fmt deps tidy help install install-user link local-alias run
.PHONY: build-all build-all-all build-linux build-darwin build-windows build-win sso-helper
.PHONY: deploy-windows sync-embedded release release-dist refresh-deps fetch-deps check-deps

all: deps lint test build

## Build

build: ## Build for the current platform, as build/vsp-<os>-<arch> with build/vsp linked to it
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(LOCAL_BINARY) $(CMD_DIR)
	@$(MAKE) --no-print-directory local-alias
	@echo "Built: $(BUILD_DIR)/$(LOCAL_BINARY) (also $(BUILD_DIR)/$(BINARY_NAME))"

local-alias: ## Point build/vsp at this platform's binary
	@if [ "$(CURRENT_OS)" = "windows" ]; then \
		cp $(BUILD_DIR)/$(LOCAL_BINARY) $(BUILD_DIR)/$(BINARY_NAME)$(EXE); \
	else \
		ln -sf $(LOCAL_BINARY) $(BUILD_DIR)/$(BINARY_NAME); \
	fi

sso-helper: ## Build the Windows SSO capture helper (needed for browser SSO under WSL)
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(SSO_HELPER) $(SSO_HELPER_DIR)
	@echo "Built: $(SSO_HELPER)"

build-all: sso-helper ## Build for common platforms (linux-amd64, darwin-arm64, windows-amd64) + local ./build/vsp
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS_COMMON); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY_NAME)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then output=$$output.exe; fi; \
		echo "Building $$output..."; \
		GOOS=$$os GOARCH=$$arch $(GOBUILD) $(LDFLAGS) -o $$output $(CMD_DIR) || exit 1; \
	done
	@echo "Pointing $(BUILD_DIR)/$(BINARY_NAME) at this platform's binary..."
	@$(MAKE) --no-print-directory local-alias 2>/dev/null || \
		$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete. Binaries in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

build-all-all: sso-helper ## Build for ALL platforms (linux, darwin, windows - amd64, arm64, 386, arm)
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

# WSL deployment directory. Overridable:
#   make deploy-windows WINDOWS_DEPLOY_DIR=/mnt/c/tools
WINDOWS_DEPLOY_DIR ?= /mnt/c/bin

build-win: build-windows ## Alias for build-windows

deploy-windows: ## Build vsp.exe + vsp-sso.exe (amd64) and copy both to $(WINDOWS_DEPLOY_DIR)
	@mkdir -p $(BUILD_DIR)
	@echo "Building windows/amd64..."
	@GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)
	@GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(SSO_HELPER) $(SSO_HELPER_DIR)
	@mkdir -p $(WINDOWS_DEPLOY_DIR)
	@# Both binaries go, not just vsp. FindSSOHelper looks beside the running
	@# executable, so a vsp.exe deployed alone cannot do browser SSO at all —
	@# and the failure surfaces later, as a capture that cannot start.
	@# A running .exe cannot be overwritten on a Windows mount, so say which
	@# one is held rather than leaving a half-finished deploy.
	@for f in $(BINARY_NAME).exe vsp-sso.exe; do \
		src=$(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe; \
		[ "$$f" = "vsp-sso.exe" ] && src=$(SSO_HELPER); \
		cp "$$src" "$(WINDOWS_DEPLOY_DIR)/$$f" || { \
			echo "could not write $(WINDOWS_DEPLOY_DIR)/$$f — is it running on the Windows side?"; \
			exit 1; }; \
	done
	@# Prove what landed. A silently mis-targeted cross-compile is the one
	@# failure this target could otherwise hand on without noticing.
	@echo
	@for f in $(BINARY_NAME).exe vsp-sso.exe; do \
		printf '  %-14s ' "$$f"; \
		file -b "$(WINDOWS_DEPLOY_DIR)/$$f" | cut -d, -f1-2; \
	done
	@echo
	@ls -lh $(WINDOWS_DEPLOY_DIR)/$(BINARY_NAME).exe $(WINDOWS_DEPLOY_DIR)/vsp-sso.exe
	@echo
	@# printf, not echo: echo eats the backslash in a Windows path and prints C:in.
	@printf 'On the Windows side: %s\n' "$$(wslpath -w $(WINDOWS_DEPLOY_DIR) 2>/dev/null || echo $(WINDOWS_DEPLOY_DIR))"

sync-embedded: ## Copy the ZADT_VSP sources vsp install deploys from src/ into embedded/abap/
	$(GOCMD) generate ./embedded/abap

# SAP system for dependency refresh (override with: make release SAP_SYSTEM=prod)
SAP_SYSTEM ?= a4h

fetch-deps: ## Build the embedded abapGit archive from upstream, reproducibly
	go run ./tools/fetchdeps

check-deps: ## Report what the embedded archive holds, and refuse an empty one
	go run ./tools/fetchdeps -check

refresh-deps: ## Refresh embedded ZIPs from SAP — prefer fetch-deps, which is reproducible
	@echo "Refreshing embedded dependencies from SAP system '$(SAP_SYSTEM)'..."
	@if ./build/vsp -s $(SAP_SYSTEM) export '$$ZGIT' -o embedded/deps/abapgit-full.zip.tmp 2>/dev/null; then \
		mv embedded/deps/abapgit-full.zip.tmp embedded/deps/abapgit-full.zip; \
		echo "  abapgit-full.zip: updated"; \
	else \
		rm -f embedded/deps/abapgit-full.zip.tmp; \
		echo "  abapgit-full.zip: kept existing (SAP export failed)"; \
	fi
	@if ./build/vsp -s $(SAP_SYSTEM) export 'ZABAPGIT' -o embedded/deps/abapgit-standalone.zip.tmp 2>/dev/null; then \
		mv embedded/deps/abapgit-standalone.zip.tmp embedded/deps/abapgit-standalone.zip; \
		echo "  abapgit-standalone.zip: updated"; \
	else \
		rm -f embedded/deps/abapgit-standalone.zip.tmp; \
		echo "  abapgit-standalone.zip: kept existing (SAP export failed)"; \
	fi

release: build refresh-deps build-all ## Full release: build vsp, refresh deps from SAP, rebuild all platforms
	@echo "Release build complete."

# The release workflow builds and checks through .github/ci/release.sh; this
# runs the same script, so it is both the dry run and the manual fallback. It
# refuses a dirty tree or a HEAD that is not the tag: use a fresh worktree.
release-dist: ## Build + verify the 9 release assets for TAG into dist/ (what CI runs): make release-dist TAG=vX.Y.Z
	@test -n "$(TAG)" || { echo "usage: make release-dist TAG=vX.Y.Z"; exit 1; }
	./.github/ci/release.sh build $(TAG) dist
	./.github/ci/release.sh verify $(TAG) dist
	./.github/ci/release.sh run $(TAG) dist $(LOCAL_BINARY)

# GOPATH is a Go environment value, not a make variable: left as $(GOPATH) it
# expanded to nothing and this target installed to /bin.
GOPATH_BIN=$(shell go env GOPATH)/bin

install: ## Install the binary to GOPATH/bin
	@mkdir -p $(GOPATH_BIN)
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH_BIN)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Installed: $(GOPATH_BIN)/$(BINARY_NAME)"

install-user: build ## Copy the binary to ~/.local/bin (independent of this checkout)
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/$(BINARY_NAME)
	@echo "Installed: ~/.local/bin/$(BINARY_NAME)"
	@echo "A copy goes stale on the next build — use 'make link' while developing."

link: build ## Symlink ~/.local/bin/vsp at this checkout, so every build is picked up
	@mkdir -p ~/.local/bin
	@ln -sf $(abspath $(BUILD_DIR)/$(BINARY_NAME)) ~/.local/bin/$(BINARY_NAME)
	@echo "Linked: ~/.local/bin/$(BINARY_NAME) -> $(abspath $(BUILD_DIR)/$(BINARY_NAME))"
	@echo "'make build' now updates what is on PATH; nothing to re-install."
	@case ":$$PATH:" in *":$$HOME/.local/bin:"*) ;; \
		*) echo "NOTE: ~/.local/bin is not on your PATH — on macOS it is not there by default." ;; esac

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

lint: ## Run linter (the CI gate: correctness linters, new issues since origin/main)
	@if command -v $(GOLINT) >/dev/null 2>&1; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

lint-full: ## Run every linter over the whole tree (advisory debt count, as CI does)
	./.github/ci/lint.sh full

metrics: ## Size and complexity metrics (JSON on stdout, summary on stderr)
	cd .github/ci/metrics && go run . -root ../../.. -baseline ../complexity-baseline.json -o -

## Testing

test: ## Run tests
	$(GOTEST) -v -race ./...

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
