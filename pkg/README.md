# Lux Consensus Source Code Organization

This directory contains implementations of the Lux consensus framework in multiple programming languages. Each language implementation provides the same consensus algorithms with language-specific optimizations and idioms.

## Directory Structure

```
src/
├── c/          # C implementation (high-performance, minimal overhead)
├── cpp/        # C++ implementation with MLX extensions (GPU acceleration)
├── go/         # Go implementation (production blockchain integration)
├── python/     # Python implementation (research and prototyping)
└── rust/       # Rust implementation (memory-safe systems programming)
```

## Language Implementations

### Go (`/src/go/`)
- **Status**: Production-ready
- **Use Case**: Core blockchain node integration
- **Import Path**: `github.com/luxfi/consensus` (via go.work at root)
- **Features**: Full integration with Lux node, concurrent processing

### C (`/src/c/`)
- **Status**: Production-ready
- **Use Case**: Embedded systems, performance-critical applications
- **Features**: Minimal dependencies, ZeroMQ networking, SIMD optimizations

### Rust (`/src/rust/`)
- **Status**: Production-ready
- **Use Case**: Memory-safe systems, async applications
- **Features**: Zero-cost abstractions, async/await, compile-time guarantees

### Python (`/src/python/`)
- **Status**: Research/Development
- **Use Case**: Prototyping, research, data analysis
- **Features**: NumPy integration, ML frameworks, visualization

### C++ (`/src/cpp/`)
- **Status**: Development
- **Use Case**: High-performance with GPU acceleration
- **Features**: MLX extensions, template metaprogramming, SIMD/GPU support

## Building

From the repository root:

```bash
# Build all implementations
make build

# Build specific language
make build-go
make build-c
make build-rust
make build-python
make build-cpp
```

## Testing

From the repository root:

```bash
# Test all implementations
make test

# Test specific language
make test-go
make test-c
make test-rust
make test-python
make test-cpp
```

## Consensus Engines

All implementations support these consensus engines (Quasar family):

1. **Wave** - Threshold voting with fast probabilistic consensus (production)
2. **Nova** - Linear chain consensus (wraps Ray sub-protocol)
3. **Nebula** - DAG consensus with safe-prefix commit (wraps Field sub-protocol)
4. **PostQuantum** - Quantum-resistant consensus with BLS + Ringtail + ML-DSA
5. **Chain** - Linear chain consensus for ordered blocks
6. **DAG** - Full directed acyclic graph consensus

## Performance Benchmarks

| Implementation | Votes/Second | Memory Usage | Latency |
|----------------|--------------|--------------|---------|
| C              | 14,000+      | < 10 MB      | < 1ms   |
| Rust           | 13,500+      | < 15 MB      | < 1ms   |
| Go             | 12,000+      | < 20 MB      | < 2ms   |
| C++ (w/ MLX)   | 15,000+      | < 25 MB      | < 1ms   |
| Python         | 5,000+       | < 50 MB      | < 5ms   |

## Protocol Compatibility

All implementations use the same binary protocol for network communication:

```
┌─────────────┬────────────┬──────────┬───────────┬──────────┐
│ Engine Type │ Node ID    │ Block ID │ Vote Type │ Reserved │
│ (1 byte)    │ (2 bytes)  │ (2 bytes)│ (1 byte)  │ (2 bytes)│
└─────────────┴────────────┴──────────┴───────────┴──────────┘
```

This ensures interoperability between different language implementations.

## Development Guidelines

1. **Consistency**: All implementations should provide the same consensus guarantees
2. **Testing**: Each implementation must have comprehensive test coverage
3. **Documentation**: Language-specific documentation in `docs/{language}/`
4. **Performance**: Optimize for language strengths while maintaining correctness
5. **Compatibility**: Maintain protocol compatibility across all implementations

## Contributing

When adding features or fixes:
1. Implement in the reference implementation (Go)
2. Port to other languages maintaining consistency
3. Add tests for all implementations
4. Update documentation

See [CONTRIBUTING.md](../CONTRIBUTING.md) for detailed guidelines.

## License

Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.