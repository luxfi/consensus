# Makefile Quick Reference

Complete guide to using the Lux Consensus Makefile.

## Quick Start

```bash
# First time setup
make install-tools    # Install all development tools
make generate         # Generate code (canoto + mocks)
make build            # Build all tools
make test             # Run tests

# Or do it all at once
make install-tools generate build test
```

## Essential Commands

### Building

```bash
make build            # Build all tools
make clean            # Clean build artifacts
make clean-all        # Clean + remove generated files
```

### Testing

```bash
make test             # Run all tests
make test-verbose     # Run tests with full output
make coverage-html    # Generate HTML coverage report
make coverage-95      # Ensure 95%+ coverage (fails if below)
make test-ai          # Test AI package only
make test-core        # Test core package only
```

### Code Generation

```bash
make generate         # Generate all code (canoto + mocks)
make generate-canoto  # Generate canoto serialization only
make generate-mocks   # Generate mock files only
make remove-generated # Remove all generated files
```

### Code Quality

```bash
make format           # Format all code
make lint             # Run linters
make security         # Check for vulnerabilities
make pre-commit       # Run all pre-commit checks
```

### Benchmarking

```bash
make bench            # Run all benchmarks
make benchmark        # Run pure consensus benchmarks
make zmq-bench        # Build ZMQ benchmark tool
make benchmark-quick  # Quick 10-node benchmark
```

## Development Workflow

### Adding New Features

```bash
# 1. Create feature branch
git checkout -b feature/my-feature

# 2. Make changes to code
# ... edit files ...

# 3. Generate any needed code
make generate

# 4. Test changes
make test

# 5. Check code quality
make pre-commit

# 6. Commit
git add .
git commit -m "feat: add my feature"
```

### Updating Interfaces

```bash
# 1. Edit interface definitions
# ... modify interfaces ...

# 2. Regenerate code
make remove-generated  # Clean old generated code
make generate          # Generate new code

# 3. Update implementations
# ... fix any compile errors ...

# 4. Test
make test
```

### Before Committing

```bash
# Run full pre-commit checks
make pre-commit

# Or manually run each step
make format           # Format code
make lint             # Check linting
make test             # Run tests
```

## Target Categories

### Main Targets
- `all` - Build and test everything (default)
- `build` - Build all tools and commands
- `test` - Run all tests
- `clean` - Clean build artifacts
- `help` - Show comprehensive help

### Test Targets
- `test-verbose` - Run tests with full output
- `test-coverage` - Generate coverage report
- `coverage-html` - Generate HTML coverage report
- `coverage-95` - Ensure 95%+ coverage
- `test-ai` - Test AI package specifically
- `test-core` - Test core package specifically
- `test-specific TEST=TestName` - Run specific test
- `test-package PKG=./ai` - Test specific package

### Benchmark Targets
- `bench` - Run all benchmarks
- `benchmark` - Pure consensus benchmarks
- `benchmark-quick` - Quick 10-node test
- `benchmark-max-tps` - Maximum throughput test
- `zmq-bench` - Build ZMQ benchmark tool

### Code Generation
- `generate` - Generate all code
- `generate-canoto` - Generate canoto files
- `generate-mocks` - Generate mock files
- `remove-generated` - Remove generated files
- `install-canoto` - Install canoto tool

### Code Quality
- `format` - Format all code
- `lint` - Run linters
- `check-format` - Check if code is formatted
- `static-analysis` - Run static analysis
- `security` - Check for vulnerabilities
- `pre-commit` - Run all pre-commit checks

### Dependency Management
- `update-deps` - Update all dependencies
- `verify-deps` - Verify dependencies
- `tidy` - Tidy go.mod

### Installation
- `tools` / `install-tools` - Install all dev tools
- `install-canoto` - Install canoto specifically

### Examples
- `examples` - Build all integration examples
- `examples-go` - Build Go examples
- `examples-c` - Build C examples
- `examples-cpp` - Build C++ examples
- `examples-rust` - Build Rust examples

### Paper Building
- `paper` - Build PDF white paper
- `paper-clean` - Clean paper artifacts
- `paper-watch` - Watch and rebuild paper

### Utilities
- `run-params` - Build and run params tool
- `ci` - Run all CI checks

## Environment Variables

### Benchmark Configuration
```bash
# Customize benchmark parameters
make run-zmq-bench NODES=100 BATCH=8192 INTERVAL=1ms ROUNDS=500
```

Variables:
- `NODES` - Number of nodes (default: 10)
- `BATCH` - Batch size (default: 4096)
- `INTERVAL` - Interval between rounds (default: 5ms)
- `ROUNDS` - Number of rounds (default: 100)

### Test Configuration
```bash
# Run specific test
make test-specific TEST=TestMyFunction

# Test specific package
make test-package PKG=./ai
```

## Common Workflows

### Starting Development
```bash
git clone https://github.com/luxfi/consensus
cd consensus
make install-tools
make generate
make build
make test
```

### Daily Development
```bash
# Pull latest
git pull

# Regenerate if needed
make generate

# Build and test
make build test

# Before committing
make pre-commit
```

### CI/CD Pipeline
```bash
# Full CI checks
make ci

# Or manually
make install-tools
make generate
make build
make pre-commit
make coverage-95
```

### Benchmarking
```bash
# Quick benchmark
make benchmark-quick

# Full benchmark suite
make bench

# Maximum TPS test
make benchmark-max-tps

# Custom benchmark
make run-zmq-bench NODES=50 BATCH=16384 INTERVAL=1ms ROUNDS=1000
```

## Troubleshooting

### "canoto: command not found"
```bash
make install-canoto
```

### "mockgen: command not found"
```bash
go install github.com/golang/mock/mockgen@latest
```

### Build fails after pull
```bash
make clean-all
make generate
make build
```

### Tests fail unexpectedly
```bash
# Clean and rebuild
make clean-all
make generate
make build
make test-verbose
```

### Coverage below 95%
```bash
# See which packages need coverage
make coverage-95

# Test specific package
make test-ai    # or test-core
```

## Tips

1. **Use `make help`** to see all available targets
2. **Run `make pre-commit`** before every commit
3. **Use `make coverage-95`** to ensure test quality
4. **Keep generated files out of git** - they regenerate automatically
5. **Run `make clean-all generate`** after pulling major changes
6. **Use specific test targets** for faster iteration:
   - `make test-ai` for AI package
   - `make test-core` for core package
   - `make test-package PKG=./mypackage` for any package

## Files and Directories

### Build Artifacts (gitignored)
- `bin/` - Built binaries
- `coverage.out` - Coverage data
- `coverage.html` - HTML coverage report
- `*.prof` - Profile data

### Generated Code (gitignored)
- `**/*.canoto.go` - Canoto serialization
- `**/mock_*.go` - Mock interfaces
- `**/*_mock.go` - Mock interfaces

### Important Files
- `Makefile` - All build targets
- `GENERATION.md` - Code generation guide
- `.gitignore` - Excluded files
- `go.mod` - Go dependencies

## See Also

- `GENERATION.md` - Detailed code generation guide
- `make help` - Full target list with descriptions
- `README.md` - Project overview
- `TESTING.md` - Testing guidelines

---

**Quick Reference Card**

```
Build:        make build
Test:         make test
Coverage:     make coverage-95
Generate:     make generate
Format:       make format
Lint:         make lint
Pre-commit:   make pre-commit
Clean:        make clean-all
Help:         make help
```
