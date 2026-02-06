# cli-replay Makefile
# Build targets for development and release

.PHONY: build test lint fmt clean install help

# Binary name
BINARY := cli-replay
# Output directory
BIN_DIR := bin
# Build flags for static binary
LDFLAGS := -s -w
# Go build flags
BUILD_FLAGS := -ldflags="$(LDFLAGS)"

# Default target
all: lint test build

## Build targets

# Build the binary
build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY) ./cmd/cli-replay

# Build for all platforms (release)
build-all: build-linux build-darwin build-windows

build-linux:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY)-linux-amd64 ./cmd/cli-replay
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY)-linux-arm64 ./cmd/cli-replay

build-darwin:
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY)-darwin-amd64 ./cmd/cli-replay
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY)-darwin-arm64 ./cmd/cli-replay

build-windows:
	@mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY)-windows-amd64.exe ./cmd/cli-replay

## Test targets

# Run all tests
test:
	go test -race -cover ./...

# Run tests with verbose output
test-verbose:
	go test -race -cover -v ./...

# Run tests and generate coverage report
test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Quality targets

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	gofmt -s -w .
	goimports -w .

# Check formatting (CI)
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Code not formatted. Run 'make fmt'" && exit 1)

## Utility targets

# Install binary to GOPATH/bin
install: build
	go install ./cmd/cli-replay

# Clean build artifacts
clean:
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

# Download dependencies
deps:
	go mod download
	go mod tidy

# Verify dependencies
verify:
	go mod verify

# Show help
help:
	@echo "cli-replay Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build targets:"
	@echo "  build        Build the binary (default)"
	@echo "  build-all    Build for all platforms"
	@echo "  install      Install to GOPATH/bin"
	@echo ""
	@echo "Test targets:"
	@echo "  test         Run tests with race detection"
	@echo "  test-verbose Run tests with verbose output"
	@echo "  test-coverage Generate coverage report"
	@echo ""
	@echo "Quality targets:"
	@echo "  lint         Run golangci-lint"
	@echo "  fmt          Format code"
	@echo "  fmt-check    Check code formatting"
	@echo ""
	@echo "Utility targets:"
	@echo "  clean        Remove build artifacts"
	@echo "  deps         Download and tidy dependencies"
	@echo "  verify       Verify dependencies"
