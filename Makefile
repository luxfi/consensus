# Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

.PHONY: all test build clean lint format check tools help benchmark benchmark-node benchmark-zmq test-parallel test-cluster

# Default target
all: build test

# Build all tools and commands
build: ## Build all tools and commands
	@echo "Building all tools..."
	@echo "Building params..."
	@go build -o bin/params ./cmd/params || echo "ERROR: Failed to build params"
	@echo "Building checker..."
	@go build -o bin/checker ./cmd/checker || echo "ERROR: Failed to build checker"
	@echo "Building simulator..."
	@go build -o bin/sim ./cmd/sim || echo "ERROR: Failed to build sim"
	@echo "Building zmq-bench..."
	@go build -tags zmq -o bin/zmq-bench ./cmd/zmq-bench || echo "WARNING: Skipping zmq-bench - requires ZMQ"
	@echo "Building consensus CLI..."
	@go build -o bin/consensus ./cmd/consensus || echo "ERROR: Failed to build consensus CLI"
	@echo "Build complete! Successfully built tools:"
	@ls -1 bin/ 2>/dev/null | grep -v '^$$' || echo "No tools built"

# Build tools that depend on node package (currently broken due to unused imports)
build-full: build ## Build all tools including those with node dependencies
	@echo "Building benchmark (requires fixing node package imports)..."
	@go build -tags zmq -o bin/benchmark ./cmd/benchmark 2>&1 || echo "WARNING: Skipping - fix needed in node/network/peer/test_peer.go"
	@echo "Building benchmark tool..."
	@go build -o bin/benchmark ./cmd/benchmark 2>&1 || echo "WARNING: Skipping - fix needed in node package"

# Run all tests
test: ## Run all tests
	@echo "Running tests..."
	@go test -race -timeout 5m -tags="!integration" ./... 2>&1 | grep -v "warning.*LD_DYSYMTAB" | grep -v "has malformed LC_DYSYMTAB"

# Run tests (verbose, showing warnings)
test-verbose: ## Run tests with all output including warnings
	@echo "Running tests (verbose)..."
	@go test -race -timeout 5m -tags="!integration" ./...

# Run tests with coverage
test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -race -timeout 5m -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
bench: ## Run performance benchmarks
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./benchmark ./validator ./config ./protocol/... ./engine/...

# Run pure consensus benchmarks
benchmark: ## Run pure algorithm benchmarks without networking
	@echo "🚀 Running pure consensus benchmarks..."
	@go test -bench=. -benchmem ./benchmark ./validator ./config ./protocol/... ./engine/...

# Build zmq-bench tool
zmq-bench: ## Build the ZMQ benchmark tool
	@echo "Building zmq-bench tool..."
	@go build -tags zmq -o bin/zmq-bench ./cmd/zmq-bench || echo "ERROR: zmq-bench requires ZMQ"

# Run zmq-bench tool
run-zmq-bench: zmq-bench ## Run ZMQ benchmark tool with configurable parameters
	@echo "🚀 Running benchmark..."
	@echo "   Nodes: $(NODES)"
	@echo "   Batch: $(BATCH)"
	@echo "   Interval: $(INTERVAL)"
	@echo "   Rounds: $(ROUNDS)"
	@echo ""
	./bin/zmq-bench -nodes $(NODES) -batch $(BATCH) -interval $(INTERVAL) -rounds $(ROUNDS)

# Configuration for benchmarks
NODES ?= 10
BATCH ?= 4096
INTERVAL ?= 5ms
ROUNDS ?= 100

# Run massively parallel ZMQ benchmarks with Ginkgo
benchmark-zmq: check-ginkgo ## Run ZeroMQ transport benchmarks
	@echo "🌐 Running transport benchmarks..."
	go test -tags zmq -v ./benchmark -ginkgo.v

# Run Ginkgo tests in parallel
test-parallel: check-ginkgo ## Run tests in parallel with Ginkgo
	@echo "⚡ Running tests in parallel..."
	ginkgo -p ./...

# Run CI benchmark suite (10, 100, 1000 nodes)
ci-cluster: zmq-bench ## Run CI multi-node consensus benchmarks (10, 100, 1000 nodes)
	@echo "🌟 Running CI benchmark suite..."
	@echo ""
	@echo "### 10 nodes (default)"
	@./bin/zmq-bench -nodes 10 -batch 8192 -interval 1ms -rounds 500 -quiet
	@echo ""
	@echo "### 100 nodes"
	@./bin/zmq-bench -nodes 100 -batch 8192 -interval 1ms -rounds 100 -quiet
	@echo ""
	@echo "### 500 nodes"
	@./bin/zmq-bench -nodes 1000 -batch 8192 -interval 1ms -rounds 50 -quiet

# Run quick benchmark
benchmark-quick: zmq-bench ## Quick benchmark with 10 nodes
	@./bin/zmq-bench -nodes 10 -batch 8192 -interval 1ms -rounds 20

# Run maximum TPS benchmark
benchmark-max-tps: zmq-bench ## Run benchmark optimized for maximum TPS
	@echo "🚀 Running maximum TPS benchmark..."
	@CPU_COUNT=$$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4); \
	echo "   CPU cores: $$CPU_COUNT"; \
	echo "   Nodes: $$((CPU_COUNT * 2))"; \
	echo "   Batch: 16384"; \
	echo "   Interval: 1ms"; \
	echo ""; \
	./bin/zmq-bench -nodes $$((CPU_COUNT * 2)) -batch 16384 -interval 1ms -rounds 100


# Check if Ginkgo is installed
check-ginkgo:
	@which ginkgo > /dev/null || (echo "📦 Installing Ginkgo..."; go install github.com/onsi/ginkgo/v2/ginkgo@latest)

# Run linters
lint: ## Run linters
	@echo "Running linters..."
	@golangci-lint run ./...

# Format code
format: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@goimports -w .

# Check if code is properly formatted
check-format: ## Check if code is properly formatted
	@echo "Checking code format..."
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "The following files need formatting:"; \
		gofmt -l .; \
		exit 1; \
	fi

# Run static analysis
static-analysis: ## Run static analysis
	@echo "Running static analysis..."
	@go vet ./...
	@staticcheck ./...

# Generate mocks
generate-mocks: ## Generate mock files
	@echo "Generating mocks..."
	@go generate ./...

# Clean build artifacts
clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/ coverage.out coverage.html benchmark_report.txt *.prof

# Install development tools
tools: ## Install development tools
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/golang/mock/mockgen@latest

# Run a specific test
test-specific: ## Run a specific test (use TEST=TestName)
	@if [ -z "$(TEST)" ]; then \
		echo "Usage: make test-specific TEST=TestName"; \
		exit 1; \
	fi
	@echo "Running test: $(TEST)"
	@go test -race -v -run $(TEST) ./...

# Run tests for a specific package
test-package: ## Run tests for a specific package (use PKG=./confidence)
	@if [ -z "$(PKG)" ]; then \
		echo "Usage: make test-package PKG=./confidence"; \
		exit 1; \
	fi
	@echo "Testing package: $(PKG)"
	@go test -race -v $(PKG)

# Check for security vulnerabilities
security: ## Check for security vulnerabilities
	@echo "Checking for vulnerabilities..."
	@govulncheck ./...

# Update dependencies
update-deps: ## Update dependencies
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy

# Verify dependencies
verify-deps: ## Verify dependencies
	@echo "Verifying dependencies..."
	@go mod verify

# Run pre-commit checks
pre-commit: check-format lint test ## Run pre-commit checks

# Build and run the params tool
run-params: build ## Build and run params tool
	@./bin/params

# Show help
help: ## Show this help
	@echo "Lux Consensus Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "Benchmark Examples:"
	@echo "  # Run pure algorithm benchmarks"
	@echo "  make benchmark"
	@echo ""
	@echo "  # Start single benchmark node"
	@echo "  make benchmark-node PORT=40000"
	@echo ""
	@echo "  # Start node and connect to peers"
	@echo "  make benchmark-node ARGS='-peers tcp://192.168.1.10:30000'"
	@echo ""
	@echo "  # Start 5-node test cluster"
	@echo "  make test-cluster"
	@echo ""
	@echo "  # Run on remote machine"
	@echo "  ssh user@remote 'cd consensus && make benchmark-node PORT=30001'"
