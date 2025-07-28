# Copyright (C) 2025, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

.PHONY: all test build clean lint format check tools help benchmark benchmark-node benchmark-zmq test-parallel test-cluster

# Default target
all: build test

# Build all tools and commands
build: ## Build all tools and commands
	@echo "Building all tools..."
	@echo "Building params..."
	@go build -o bin/params ./cmd/params
	@echo "Building benchmark..."
	@go build -o bin/benchmark ./cmd/benchmark
	@echo "Building checker..."
	@go build -o bin/checker ./cmd/checker
	@echo "Building consensus CLI plugin..."
	@go build -o bin/consensus ./cmd/consensus
	@echo "Building simulator..."
	@go build -o bin/sim ./cmd/sim
	@echo "✅ All tools built successfully!"

# Run all tests
test: ## Run all tests
	@echo "Running tests..."
	@go test -race -timeout 30m ./...

# Run tests with coverage
test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -race -timeout 30m -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
bench: ## Run performance benchmarks
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./snowball ./poll ./confidence ./testing ./crypto

# Run pure consensus benchmarks
benchmark: ## Run pure algorithm benchmarks without networking
	@echo "🚀 Running pure consensus benchmarks..."
	@go test -bench=. -benchmem ./snowball ./poll ./confidence ./testing
	@./benchmark_all.sh

# Build and run benchmark node (requires ZMQ)
benchmark-node: check-zmq ## Start a benchmark node with ZMQ transport (PORT=30000)
	@echo "🔧 Building benchmark node..."
	@go build -tags zmq -o bin/benchmark-node ./cmd/benchmark-node
	@echo "✅ Built: bin/benchmark-node"
	@echo ""
	@echo "📡 Starting benchmark node on port $(PORT)..."
	@echo "   Connect peers with: -peers tcp://host:port"
	@echo "   View stats at: http://localhost:$(HTTP_PORT)/stats"
	@echo ""
	./bin/benchmark-node -port $(PORT) -http $(HTTP_PORT) $(ARGS)

# Port configuration for benchmark node
PORT ?= 30000
HTTP_PORT ?= 8080
ARGS ?=

# Run massively parallel ZMQ benchmarks with Ginkgo
benchmark-zmq: check-zmq check-ginkgo ## Run ZeroMQ transport benchmarks
	@echo "🌐 Running ZeroMQ transport benchmarks..."
	go test -tags zmq -v ./benchmark -ginkgo.v

# Run Ginkgo tests in parallel
test-parallel: check-ginkgo ## Run tests in parallel with Ginkgo
	@echo "⚡ Running tests in parallel..."
	ginkgo -p ./...

# Start multi-node local test cluster
test-cluster: check-zmq ## Start 5-node local test cluster
	@echo "🌟 Starting 5-node local cluster..."
	@echo "   Node 1: port 30000 (http: 8080)"
	@echo "   Node 2: port 30010 (http: 8081)"
	@echo "   Node 3: port 30020 (http: 8082)"
	@echo "   Node 4: port 30030 (http: 8083)"
	@echo "   Node 5: port 30040 (http: 8084)"
	@echo ""
	@echo "Building benchmark node..."
	@go build -tags zmq -o bin/benchmark-node ./cmd/benchmark-node
	@echo ""
	@echo "Starting nodes..."
	@for i in 0 1 2 3 4; do \
		PORT=$$((30000 + i * 10)) HTTP_PORT=$$((8080 + i)) ./bin/benchmark-node -port $$PORT -http $$HTTP_PORT & \
		sleep 1; \
	done
	@echo ""
	@echo "✅ Cluster started. View stats:"
	@for i in 0 1 2 3 4; do \
		echo "   Node $$((i + 1)): curl http://localhost:$$((8080 + i))/stats"; \
	done
	@echo ""
	@echo "Press Ctrl+C to stop all nodes"
	@wait

# Check if ZMQ is available
check-zmq:
	@which pkg-config > /dev/null || (echo "❌ pkg-config not found. Please install pkg-config"; exit 1)
	@pkg-config --exists libzmq || (echo "❌ ZeroMQ not found. Please install:"; \
		echo "   macOS:  brew install zeromq"; \
		echo "   Ubuntu: apt-get install libzmq3-dev"; \
		echo "   Alpine: apk add zeromq-dev"; exit 1)

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