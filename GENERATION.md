# Code Generation Guide

This document explains how to regenerate generated code files in the Lux Consensus repository.

## Overview

The repository uses code generation for:
1. **Canoto Protocol Buffers** - Efficient serialization (*.canoto.go)
2. **Mock Interfaces** - Testing utilities (*_mock.go, mock_*.go)

All generated files are excluded from git (see `.gitignore`) and must be regenerated when needed.

## Quick Start

```bash
# Install all required tools
make install-tools

# Generate all code
make generate

# Or generate specific types
make generate-canoto    # Just canoto files
make generate-mocks     # Just mock files
```

## Canoto Code Generation

### What is Canoto?

Canoto is a protocol buffer code generator for Go that creates efficient serialization code.

### Installation

```bash
# Install canoto
make install-canoto

# Or manually
go install github.com/StephenButtolph/canoto@latest
```

### Files Generated

Located in `engine/bft/`:
- `block.canoto.go` - Generated from `block.go`
- `storage.canoto.go` - Generated from `storage.go`
- `qc.canoto.go` - Generated from `qc.go`

### Manual Generation

```bash
# From repository root
cd engine/bft
canoto block.go storage.go qc.go

# Or use Makefile
make generate-canoto
```

### When to Regenerate

Regenerate canoto files when you modify:
- Block structure definitions
- Storage interface changes
- QC (Quorum Certificate) modifications

## Mock Generation

### What are Mocks?

Mocks are test doubles generated from interfaces using `mockgen`.

### Installation

```bash
# mockgen is installed with other tools
make install-tools

# Or manually
go install github.com/golang/mock/mockgen@latest
```

### Generation

Mocks are generated using `go:generate` directives in source files:

```go
//go:generate mockgen -destination=mock_interface.go -package=mypackage . MyInterface
```

To regenerate all mocks:

```bash
make generate-mocks

# Or manually
go generate ./...
```

## Removing Generated Files

To clean all generated files (useful before regenerating):

```bash
# Remove all generated files
make remove-generated

# Then regenerate
make generate
```

## Build Process Integration

### First Time Setup

```bash
# 1. Clone repository
git clone https://github.com/luxfi/consensus
cd consensus

# 2. Install tools
make install-tools

# 3. Generate code
make generate

# 4. Build
make build

# 5. Test
make test
```

### Development Workflow

```bash
# After modifying interfaces or proto definitions:

# 1. Remove old generated code
make remove-generated

# 2. Regenerate
make generate

# 3. Test changes
make test

# 4. Commit (generated files are excluded automatically)
git add .
git commit -m "Update interfaces"
```

## CI/CD Integration

GitHub Actions and CI systems should:

```yaml
- name: Setup
  run: |
    make install-tools
    make generate

- name: Build
  run: make build

- name: Test
  run: make coverage-95
```

## Troubleshooting

### "canoto: command not found"

```bash
# Install canoto
make install-canoto

# Verify installation
which canoto
canoto -version
```

### "mockgen: command not found"

```bash
# Install mockgen
go install github.com/golang/mock/mockgen@latest

# Verify installation
which mockgen
```

### Generated files causing merge conflicts

```bash
# Remove generated files (they shouldn't be in git anyway)
make remove-generated

# Pull latest changes
git pull

# Regenerate
make generate
```

### Canoto generation fails

```bash
# Check source files are valid
cd engine/bft
go build .

# Try manual generation with verbose output
canoto -v block.go storage.go qc.go

# Check canoto version
canoto -version
```

## Verification

After generation, verify everything works:

```bash
# Run tests
make test

# Check build
make build

# Run benchmarks
make bench
```

## Additional Resources

- Canoto: https://github.com/StephenButtolph/canoto
- Mockgen: https://github.com/golang/mock
- Go Generate: https://go.dev/blog/generate

## Best Practices

1. **Never edit generated files manually** - They will be overwritten
2. **Always regenerate after interface changes**
3. **Verify tests pass after regeneration**
4. **Keep generation tools up-to-date**: `make install-tools`
5. **Document `//go:generate` directives** in source files
6. **Run `make remove-generated`** before major updates

## Makefile Targets Reference

```bash
make generate              # Generate all code
make generate-canoto       # Generate canoto files only
make generate-mocks        # Generate mocks only
make remove-generated      # Remove all generated files
make install-tools         # Install all code gen tools
make install-canoto        # Install canoto only
```

---

**Last Updated:** 2025-11-06
**Canoto Version:** v0.17.2
**Mockgen Version:** Latest
