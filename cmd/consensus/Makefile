# Lux Consensus CLI Makefile

BINARY_NAME=consensus
BUILD_DIR=./bin
INSTALL_DIR=/usr/local/bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-s -w"

.PHONY: all build clean test install uninstall deps

all: deps build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) -v

clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)

test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_DIR)..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/
	@echo "Installed successfully!"

uninstall:
	@echo "Removing $(BINARY_NAME) from $(INSTALL_DIR)..."
	@sudo rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Uninstalled successfully!"

deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Cross compilation
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 -v

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 -v
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 -v

build-all: build-linux build-darwin

# Development helpers
run-check:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) && $(BUILD_DIR)/$(BINARY_NAME) check

run-sim:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) && $(BUILD_DIR)/$(BINARY_NAME) sim --nodes 10 --rounds 20

run-bench-local:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) && $(BUILD_DIR)/$(BINARY_NAME) bench :5555 100

help:
	@echo "Available targets:"
	@echo "  make build       - Build the consensus binary"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make test        - Run tests"
	@echo "  make install     - Install to system"
	@echo "  make deps        - Download dependencies"
	@echo "  make build-all   - Cross-compile for all platforms"
	@echo "  make run-check   - Build and run parameter check"
	@echo "  make run-sim     - Build and run simulation"
	@echo "  make run-bench   - Build and run local benchmark"