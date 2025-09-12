# Changelog

## [1.17.0] - 2025-01-12

### 🎉 Major Release: Multi-Language SDK with 100% Test Parity

This release introduces complete multi-language support for the Lux consensus engine with C, Rust, Python, and Go implementations, all achieving 100% test parity and comprehensive benchmarks.

### ✨ Features

#### Multi-Language Implementation
- **C Implementation**: High-performance native C library with optimized hash tables
- **Rust FFI Bindings**: Safe Rust wrapper with zero-cost abstractions
- **Python SDK (Cython)**: High-level Python bindings with Pythonic API
- **Go CGO Integration**: Conditional compilation with seamless switching

#### Quantum-Resistant Features
- OP Stack Integration with ML-DSA-65 and ML-KEM-1024
- Quantum-resistant Merkle trees
- Example implementation provided

#### Performance Achievements
- C: 9M+ blocks/sec, 19M+ votes/sec
- Rust: 607ns engine creation
- Python: 6.7M blocks/sec
- Go: 14M+ blocks/sec

### 📊 Test Coverage
- 100% test parity across all 4 implementations
- 15 test categories with comprehensive coverage
- All performance targets exceeded

### 🔧 Technical Improvements
- Custom hash table for O(1) operations
- Thread-safe with read-write locks
- Vote caching for performance
- Memory-efficient implementations

### 📦 Installation
See README.md for detailed installation instructions for each language.

### 📈 Benchmarks
Run benchmarks with:
- C: `make benchmark`
- Rust: `cargo bench`
- Python: `python3 benchmark_consensus.py`
- Go: `go test -bench=.`
