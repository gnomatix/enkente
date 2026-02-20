APP_NAME := enkente
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

GO       := go
DIST_DIR := dist
RELEASE_DIR := release

# Default: build for current platform into dist/
.PHONY: build
build:
	@mkdir -p $(DIST_DIR)
	$(GO) build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)$(shell go env GOEXE) ./cmd/enkente

# Run the serve command directly
.PHONY: run
run: build
	$(DIST_DIR)/$(APP_NAME)$(shell go env GOEXE) serve -p 8080

# Run all tests
.PHONY: test
test:
	$(GO) test ./... -v

# Lint
.PHONY: lint
lint:
	$(GO) vet ./...

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf $(DIST_DIR) $(RELEASE_DIR) bin/

# Cross-platform release builds
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: release
release: clean
	@mkdir -p $(RELEASE_DIR)
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		output="$(RELEASE_DIR)/$(APP_NAME)-$(VERSION)-$$os-$$arch$$ext"; \
		echo "Building $$output..."; \
		GOOS=$$os GOARCH=$$arch $(GO) build $(LDFLAGS) -o $$output ./cmd/enkente; \
	done
	@echo "Release builds complete in $(RELEASE_DIR)/"

# Build for a specific platform: make build-for OS=linux ARCH=arm64
.PHONY: build-for
build-for:
	@mkdir -p $(DIST_DIR)
	GOOS=$(OS) GOARCH=$(ARCH) $(GO) build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-$(OS)-$(ARCH)$(if $(filter windows,$(OS)),.exe,) ./cmd/enkente

# Install locally
.PHONY: install
install:
	$(GO) install $(LDFLAGS) ./cmd/enkente

# Compile the VS Code extension
.PHONY: extension
extension:
	cd vscode-extension && npm install && npx tsc -p ./

# Package the VS Code extension as a .vsix
.PHONY: vsix
vsix: extension
	@mkdir -p $(DIST_DIR)
	cd vscode-extension && npx -y @vscode/vsce package --allow-missing-repository -o ../$(DIST_DIR)/enkente-chat-bridge.vsix

.PHONY: all
all: build extension

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build       - Build for current platform (output: dist/)"
	@echo "  run         - Build and run the serve command"
	@echo "  test        - Run all tests"
	@echo "  lint        - Run go vet"
	@echo "  clean       - Remove dist/ and release/ dirs"
	@echo "  release     - Cross-compile for all platforms (output: release/)"
	@echo "  build-for   - Build for specific OS/ARCH (e.g. make build-for OS=linux ARCH=arm64)"
	@echo "  install     - Install to GOPATH/bin"
	@echo "  extension   - Build the VS Code extension"
	@echo "  all         - Build Go binary + VS Code extension"
	@echo "  help        - Show this help"
