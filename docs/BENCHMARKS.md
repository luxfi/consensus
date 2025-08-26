# Consensus Benchmarks Documentation

## Overview

The Lux consensus repository includes comprehensive benchmarks for measuring performance across different consensus components.

## Running Benchmarks

### Quick Start

Run all benchmarks:
```bash
./scripts/benchmark.sh
```

### Individual Package Benchmarks

Run benchmarks for specific packages:

```bash
# Configuration benchmarks
go test -bench=. -benchmem -benchtime=1s ./config

# Post-Quantum Engine benchmarks  
go test -bench=. -benchmem -benchtime=1s ./engine/pq

# Protocol benchmarks
go test -bench=. -benchmem -benchtime=1s ./protocol/field ./protocol/quasar

# QZMQ Transport benchmarks
go test -bench=. -benchmem -benchtime=1s ./qzmq
```

## Benchmark Categories

### 1. Configuration Benchmarks (`/config`)
- `BenchmarkValidate`: Tests parameter validation speed
- `BenchmarkWithBlockTime`: Tests block time configuration
- `BenchmarkWaveConfig`: Tests Wave consensus configuration

### 2. Post-Quantum Engine (`/engine/pq`)
- `BenchmarkConsensusProcessBlock`: Measures block processing in consensus engine
- `BenchmarkConsensusIsFinalized`: Tests finality checking performance
- `BenchmarkProcessBlock`: Raw block processing speed
- `BenchmarkIsFinalized`: Direct finality check

### 3. Protocol Benchmarks
#### Field Protocol (`/protocol/field`)
- `BenchmarkNebulaService`: Tests Nebula service operations

#### Quasar Protocol (`/protocol/quasar`)
- `BenchmarkPhaseI`: Phase I consensus operations
- `BenchmarkPhaseII`: Phase II consensus operations  
- `BenchmarkCertVerify`: Certificate verification

### 4. QZMQ Transport (`/qzmq`)
- `BenchmarkEncrypt`: Encryption performance
- `BenchmarkKeyRotation`: Key rotation overhead
- `BenchmarkHandshake`: Full handshake process

## Performance Targets

### Configuration
- Parameter validation: < 10ns/op
- Block time configuration: < 5ns/op
- Wave config: < 1ns/op

### Post-Quantum Engine
- Block processing: < 500ns/op
- Finality checking: < 20ns/op
- Memory allocation: < 100B/op for block processing

### Protocols
- Phase I operations: < 1ns/op
- Phase II operations: < 50ns/op
- Certificate verification: < 1ns/op

### Transport (QZMQ)
- Encryption: < 2μs/op
- Key rotation: < 2μs/op
- Full handshake: < 100μs/op

## Continuous Integration

Benchmarks are automatically run on:
- Every push to `main` branch
- All pull requests
- Nightly at 2 AM UTC
- Manual workflow dispatch

Results are:
- Stored in GitHub Actions artifacts
- Posted as PR comments for comparison
- Tracked over time for regression detection

## Interpreting Results

### Format
```
BenchmarkName-CPUs    Iterations    Time/Op    Memory/Op    Allocs/Op
```

### Example
```
BenchmarkProcessBlock-10    2736530    457.0 ns/op    218 B/op    3 allocs/op
```
- Ran on 10 CPU cores
- Completed 2,736,530 iterations
- Each operation took 457 nanoseconds
- Allocated 218 bytes per operation
- Made 3 memory allocations per operation

## Adding New Benchmarks

1. Create benchmark function in `*_test.go` file:
```go
func BenchmarkMyFeature(b *testing.B) {
    // Setup code
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Code to benchmark
    }
}
```

2. Add to appropriate package
3. Update `scripts/benchmark.sh` if needed
4. Document in this file

## Known Issues

- `BenchmarkDecrypt` in `/qzmq` is temporarily disabled due to nonce synchronization issues

## Optimization Guidelines

When optimizing based on benchmark results:

1. **Profile First**: Use `go test -bench=. -cpuprofile=cpu.prof`
2. **Memory Analysis**: Use `go test -bench=. -memprofile=mem.prof`
3. **Focus on Hot Paths**: Optimize the most frequently called code
4. **Validate Correctness**: Ensure optimizations don't break functionality
5. **Document Changes**: Explain optimizations in commit messages

## Related Documentation

- [Performance Guide](./PERFORMANCE.md)
- [Testing Guide](./TESTING.md)
- [CI/CD Workflows](../.github/workflows/)