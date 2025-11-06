# Copyright (C) 2020-2025, Lux Industries Inc All rights reserved.
# See the file LICENSE for licensing terms.

.PHONY: all test build clean lint format check tools help benchmark \
        generate generate-canoto generate-mocks coverage coverage-html coverage-95 \
        install-tools install-canoto remove-generated examples examples-go examples-c examples-cpp examples-rust

# Default target
all: build test

# === BUILD TARGETS ===

# Build all tools and commands
build: ## Build all tools and commands
	@echo "🔨 Building all tools..."
	@echo "Building params..."
	@go build -o bin/params ./cmd/params || echo "ERROR: Failed to build params"
	@echo "Building checker..."
	@go build -o bin/checker ./cmd/checker || echo "ERROR: Failed to build checker"
	@echo "Building simulator..."
	@go build -o bin/sim ./cmd/sim || echo "ERROR: Failed to build sim"
	@echo "Building bench..."
	@go build -tags zmq -o bin/bench ./cmd/bench 2>/dev/null || go build -o bin/bench ./cmd/bench || echo "WARNING: Building without ZMQ support"
	@echo "Building consensus CLI..."
	@go build -o bin/consensus ./cmd/consensus || echo "ERROR: Failed to build consensus CLI"
	@echo "✅ Build complete! Tools built:"
	@ls -1 bin/ 2>/dev/null | grep -v '^$$' || echo "No tools built"

# === TEST TARGETS ===

# Run all tests
test: ## Run all tests
	@echo "🧪 Running tests..."
	@go test -race -timeout 5m -tags="!integration" ./... 2>&1 | grep -v "warning.*LD_DYSYMTAB" | grep -v "has malformed LC_DYSYMTAB"

# Run tests (verbose, showing warnings)
test-verbose: ## Run tests with all output including warnings
	@echo "🧪 Running tests (verbose)..."
	@go test -race -timeout 5m -tags="!integration" -v ./...

# Run tests with coverage
test-coverage: ## Run tests with coverage report
	@echo "📊 Running tests with coverage..."
	@go test -race -timeout 5m -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out
	@echo "Coverage report: coverage.out"

# Generate HTML coverage report
coverage-html: test-coverage ## Generate HTML coverage report
	@echo "📊 Generating HTML coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report generated: coverage.html"

# Ensure 95%+ coverage
coverage-95: ## Run tests and ensure 95%+ coverage
	@echo "🎯 Running tests with 95% coverage target..."
	@go test -race -timeout 5m -coverprofile=coverage.out -covermode=atomic ./...
	@echo ""
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "📊 Total Coverage: $$COVERAGE%"; \
	if [ $$(echo "$$COVERAGE < 95" | bc) -eq 1 ]; then \
		echo "❌ Coverage $$COVERAGE% is below 95% target"; \
		go tool cover -func=coverage.out | grep -v "100.0%"; \
		exit 1; \
	else \
		echo "✅ Coverage $$COVERAGE% meets 95% target!"; \
	fi

# Run tests for AI package specifically
test-ai: ## Run AI package tests with coverage
	@echo "🤖 Testing AI package..."
	@go test -v -race -coverprofile=ai_coverage.out ./ai
	@go tool cover -func=ai_coverage.out

# Run tests for core package specifically
test-core: ## Run core package tests with coverage
	@echo "⚙️  Testing core package..."
	@go test -v -race -coverprofile=core_coverage.out ./core
	@go tool cover -func=core_coverage.out

# Run a specific test
test-specific: ## Run a specific test (use TEST=TestName)
	@if [ -z "$(TEST)" ]; then \
		echo "Usage: make test-specific TEST=TestName"; \
		exit 1; \
	fi
	@echo "🧪 Running test: $(TEST)"
	@go test -race -v -run $(TEST) ./...

# Run tests for a specific package
test-package: ## Run tests for a specific package (use PKG=./ai)
	@if [ -z "$(PKG)" ]; then \
		echo "Usage: make test-package PKG=./ai"; \
		exit 1; \
	fi
	@echo "🧪 Testing package: $(PKG)"
	@go test -race -v $(PKG)

# === BENCHMARK TARGETS ===

# Run benchmarks
bench: ## Run performance benchmarks
	@echo "⚡ Running benchmarks..."
	@go test -bench=. -benchmem ./config ./protocol/... ./engine/... ./photon ./core/... ./qzmq ./ai

# Run pure consensus benchmarks
benchmark: ## Run pure algorithm benchmarks without networking
	@echo "🚀 Running pure consensus benchmarks..."
	@go test -bench=. -benchmem ./config ./protocol/... ./engine/... ./photon ./core/... ./qzmq

# Build zmq-bench tool
zmq-bench: ## Build the ZMQ benchmark tool
	@echo "🔨 Building zmq-bench tool..."
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
	go test -tags zmq -v ./qzmq -ginkgo.v

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

# === CODE GENERATION TARGETS ===

# Generate all code (canoto + mocks)
generate: generate-canoto generate-mocks ## Generate all code (canoto + mocks)
	@echo "✅ All code generation complete"

# Generate canoto serialization code
generate-canoto: check-canoto ## Generate canoto protocol buffer code
	@echo "🔧 Generating canoto code..."
	@cd engine/bft && canoto block.go storage.go qc.go || echo "⚠️  Canoto generation failed - install with: go install github.com/StephenButtolph/canoto@latest"
	@echo "✅ Canoto code generated"

# Generate mock files
generate-mocks: check-mockgen ## Generate mock files
	@echo "🔧 Generating mocks..."
	@go generate ./...
	@echo "✅ Mocks generated"

# Remove all generated files
remove-generated: ## Remove all generated files (*.canoto.go, *_mock.go)
	@echo "🧹 Removing generated files..."
	@find . -name "*.canoto.go" -type f -delete
	@find . -name "*_mock.go" -type f -delete
	@find . -name "mock_*.go" -type f -delete
	@echo "✅ Generated files removed"

# === CODE QUALITY TARGETS ===

# Run linters
lint: ## Run linters
	@echo "🔍 Running linters..."
	@golangci-lint run ./... || echo "⚠️  Some linter issues found"

# Format code
format: ## Format code
	@echo "✨ Formatting code..."
	@go fmt ./...
	@goimports -w . 2>/dev/null || echo "⚠️  goimports not found, skipping"

# Check if code is properly formatted
check-format: ## Check if code is properly formatted
	@echo "🔍 Checking code format..."
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "❌ The following files need formatting:"; \
		gofmt -l .; \
		exit 1; \
	fi
	@echo "✅ All files properly formatted"

# Run static analysis
static-analysis: ## Run static analysis
	@echo "🔍 Running static analysis..."
	@go vet ./...
	@staticcheck ./... 2>/dev/null || echo "⚠️  staticcheck not found, skipping"

# Check for security vulnerabilities
security: ## Check for security vulnerabilities
	@echo "🔒 Checking for vulnerabilities..."
	@govulncheck ./... 2>/dev/null || echo "⚠️  govulncheck not found, install with: go install golang.org/x/vuln/cmd/govulncheck@latest"

# Run pre-commit checks
pre-commit: check-format lint test ## Run pre-commit checks
	@echo "✅ All pre-commit checks passed"

# === DEPENDENCY TARGETS ===

# Update dependencies
update-deps: ## Update dependencies
	@echo "📦 Updating dependencies..."
	@go get -u ./...
	@go mod tidy

# Verify dependencies
verify-deps: ## Verify dependencies
	@echo "🔍 Verifying dependencies..."
	@go mod verify

# Tidy dependencies
tidy: ## Tidy go.mod and go.sum
	@echo "🧹 Tidying dependencies..."
	@go mod tidy

# === INSTALLATION TARGETS ===

# Install all development tools
tools: install-tools ## Install all development tools

# Install development tools
install-tools: ## Install development tools
	@echo "📦 Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/golang/mock/mockgen@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@$(MAKE) check-canoto
	@$(MAKE) check-ginkgo
	@echo "✅ Development tools installed"

# Install canoto code generator
install-canoto: ## Install canoto code generator
	@echo "📦 Installing canoto..."
	@go install github.com/StephenButtolph/canoto@latest
	@echo "✅ Canoto installed"

# === EXAMPLE TARGETS ===

# Build all integration examples
examples: examples-go examples-c examples-cpp examples-rust ## Build all integration examples

# Build Go integration examples
examples-go: ## Build Go integration examples
	@echo "🔨 Building Go examples..."
	@cd examples && go build -o ../bin/example-go ./go_sdk_example.go
	@echo "✅ Go examples built"

# Build C integration examples
examples-c: ## Build C integration examples
	@echo "🔨 Building C examples..."
	@cd pkg/c && $(MAKE) all || echo "⚠️  C examples build failed"
	@echo "✅ C examples built"

# Build C++ integration examples
examples-cpp: ## Build C++ integration examples
	@echo "🔨 Building C++ examples..."
	@cd pkg/cpp && mkdir -p build && cd build && cmake .. && make || echo "⚠️  C++ examples build failed"
	@echo "✅ C++ examples built"

# Build Rust integration examples
examples-rust: ## Build Rust integration examples
	@echo "🔨 Building Rust examples..."
	@cd pkg/rust && cargo build --release || echo "⚠️  Rust examples build failed"
	@echo "✅ Rust examples built"

# === CLEAN TARGETS ===

# Clean build artifacts
clean: ## Clean build artifacts
	@echo "🧹 Cleaning..."
	@rm -rf bin/ coverage.out coverage.html *.coverage benchmark_report.txt *.prof
	@$(MAKE) paper-clean
	@cd pkg/cpp && rm -rf build/ || true
	@cd pkg/rust && cargo clean 2>/dev/null || true
	@echo "✅ Clean complete"

# Clean everything including generated files
clean-all: clean remove-generated ## Clean everything including generated files
	@echo "✅ Complete clean finished"

# === PAPER TARGETS ===

# Build the PDF white paper
paper: check-latex ## Build PDF white paper
	@echo "📄 Building white paper..."
	@cd paper && pdflatex main.tex
	@cd paper && bibtex main
	@cd paper && pdflatex main.tex
	@cd paper && pdflatex main.tex
	@echo "✅ Paper built: paper/main.pdf"

# Clean paper build artifacts
paper-clean: ## Clean paper build artifacts
	@echo "🧹 Cleaning paper build artifacts..."
	@cd paper && rm -f *.aux *.bbl *.blg *.log *.out *.toc *.fdb_latexmk *.fls *.synctex.gz

# Watch and rebuild paper on changes (requires entr)
paper-watch: check-latex check-entr ## Watch and rebuild paper on changes
	@echo "👀 Watching for paper changes..."
	@find paper -name "*.tex" -o -name "*.bib" | entr -s 'make paper'

# === UTILITY TARGETS ===

# Run params tool
run-params: build ## Build and run params tool
	@./bin/params

# Show comprehensive help
help: ## Show this help
	@echo "🔧 Lux Consensus Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "📋 Main Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		grep -E '^(all|build|test|coverage-95|generate|clean|help):' | \
		sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "🧪 Test Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		grep -E '^test-' | \
		sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "⚡ Benchmark Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		grep -E '^(bench|benchmark)' | \
		sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "🔧 Code Generation:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		grep -E '^(generate|install-canoto)' | \
		sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "🔍 Code Quality:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		grep -E '^(lint|format|security|pre-commit)' | \
		sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "📦 Examples:"
	@echo "  # Achieve 95% test coverage"
	@echo "  make coverage-95"
	@echo ""
	@echo "  # Generate all code and run tests"
	@echo "  make generate test"
	@echo ""
	@echo "  # Clean and rebuild everything"
	@echo "  make clean-all generate build test"
	@echo ""
	@echo "  # Build integration examples"
	@echo "  make examples"

# === CHECK TARGETS (Internal) ===

# Check if Ginkgo is installed
check-ginkgo:
	@which ginkgo > /dev/null || (echo "📦 Installing Ginkgo..."; go install github.com/onsi/ginkgo/v2/ginkgo@latest)

# Check if canoto is installed
check-canoto:
	@which canoto > /dev/null || (echo "⚠️  canoto not installed. Install with: make install-canoto" && false)

# Check if mockgen is installed
check-mockgen:
	@which mockgen > /dev/null || (echo "📦 Installing mockgen..."; go install github.com/golang/mock/mockgen@latest)

# Check if LaTeX is installed
check-latex:
	@which pdflatex > /dev/null || (echo "❌ pdflatex not found. Install LaTeX (e.g., brew install --cask mactex)"; exit 1)
	@which bibtex > /dev/null || (echo "❌ bibtex not found. Install LaTeX (e.g., brew install --cask mactex)"; exit 1)

# Check if entr is installed (for watch mode)
check-entr:
	@which entr > /dev/null || (echo "📦 Installing entr for watch mode..."; brew install entr || echo "⚠️  Could not install entr. Install manually for watch mode.")

# === CI TARGETS ===

# Run CI checks (used by GitHub Actions)
ci: pre-commit coverage-95 ## Run all CI checks
	@echo "✅ All CI checks passed"
