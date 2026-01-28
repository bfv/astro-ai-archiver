# Astro AI Archiver Makefile

# Binary name
BINARY_NAME=astro-ai-archiver

# Build directory
BUILD_DIR=build

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Version info
VERSION?=dev-latest
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Build flags
LDFLAGS=-ldflags "-X github.com/yourusername/astro-ai-archiver/cmd/astro-ai-archiver/mcp-server.Version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Source
MAIN_PATH=./cmd/astro-ai-archiver

.PHONY: all build build-verify build-all-verify clean test test-coverage test-scanner test-watch deps help windows linux darwin windows-arm linux-arm darwin-arm

# Default target
all: clean deps build

# Build for current platform
build:
	@echo "Building for current platform..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build with tests (recommended for releases)
build-verify: test build
	@echo "✓ Build verified with passing tests"

# Build all platforms with tests (for releases)
build-all-verify: test build-all
	@echo "✓ All builds verified with passing tests"

# Build for all platforms
build-all: windows linux darwin windows-arm linux-arm darwin-arm
	@echo "All builds complete!"

# Windows x64
windows:
	@echo "Building for Windows x64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

# Windows ARM64
windows-arm:
	@echo "Building for Windows ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(MAIN_PATH)

# Linux x64
linux:
	@echo "Building for Linux x64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

# Linux ARM64
linux-arm:
	@echo "Building for Linux ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)

# macOS x64
darwin:
	@echo "Building for macOS x64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)

# macOS ARM64 (Apple Silicon)
darwin-arm:
	@echo "Building for macOS ARM64 (Apple Silicon)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOGET) github.com/astrogo/fitsio
	$(GOGET) github.com/modelcontextprotocol/go-sdk
	$(GOGET) github.com/pressly/goose/v3
	$(GOGET) github.com/rs/zerolog
	$(GOGET) github.com/spf13/cobra
	$(GOGET) github.com/spf13/viper
	$(GOGET) modernc.org/sqlite
	$(GOMOD) tidy
	@echo "Dependencies downloaded"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run specific test
test-scanner:
	@echo "Running scanner tests..."
	$(GOTEST) -v $(MAIN_PATH) -run TestNormalizeTarget

# Run all tests in watch mode (requires entr or similar)
test-watch:
	@echo "Running tests in watch mode..."
	find . -name "*.go" | entr -c make test

# Run with example config
run:
	@echo "Running..."
	$(GOCMD) run $(MAIN_PATH) mcp-server --config config.yaml

# Install (copy to GOPATH/bin)
install: build
	@echo "Installing..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Installed to $(GOPATH)/bin/$(BINARY_NAME)"

# Show help
help:
	@echo "Astro AI Archiver - Makefile commands:"
	@echo ""
	@echo "  make build          - Build for current platform"
	@echo "  make build-verify   - Build with tests (recommended for releases)"
	@echo "  make build-all      - Build for all platforms (Windows, Linux, macOS x64/ARM64)"
	@echo "  make build-all-verify - Build all platforms with tests (for releases)"
	@echo "  make windows        - Build for Windows x64"
	@echo "  make windows-arm    - Build for Windows ARM64"
	@echo "  make linux          - Build for Linux x64"
	@echo "  make linux-arm      - Build for Linux ARM64"
	@echo "  make darwin         - Build for macOS x64"
	@echo "  make darwin-arm     - Build for macOS ARM64 (Apple Silicon)"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make deps           - Download dependencies"
	@echo "  make test           - Run all tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-scanner   - Run scanner-specific tests (e.g., normalizeTarget)"
	@echo "  make test-watch     - Run tests in watch mode (requires entr)"
	@echo "  make run            - Run with example config"
	@echo "  make install        - Install to GOPATH/bin"
	@echo "  make help           - Show this help"
	@echo ""
