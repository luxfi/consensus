# Lux Consensus Rust Benchmarks - Performance Report

**Date**: 2025-11-10
**System**: Darwin 25.2.0 (macOS)
**Rust Binding**: lux-consensus v1.17.0
**Benchmark Tool**: Criterion 0.7.0

> **CORRECTION — retracted numbers.** The throughput figures in this report are
> NOT valid and are retained only as a record of the debunked results. The
> 2025-11-10 SDK audit found this Rust binding is an FFI wrapper around the C
> library and measures data-structure insertion, not consensus; the batch
> figures are the known Rust `u8` overflow bug (a counter capped at 255 instead
> of 10,000). The honest consensus-bound ceiling is the C-FFI rate of
> **≈21K votes/sec** — Rust wraps C and cannot exceed it, so any figure above
> ~100K/sec below is measuring the wrong thing. See
> `.github/workflows/README.md` (lines 41-49, 67) and the "Honest Assessment"
> block in `LLM.md`.

## Executive Summary

Original summary (retracted — see correction above; the honest consensus-bound
rate is ≈21K votes/sec):

- **Single block addition**: ~611ns raw FFI-call time (derived "1.6M blocks/sec" measures data-structure insert, not consensus)
- **Vote processing**: ~639ns raw FFI-call time (derived "1.5M votes/sec" not consensus)
- **10,000 block batch**: raw ns/op invalid — `u8` overflow-bug artifact, NOT a real 3.9B blocks/sec
- **10,000 vote batch**: raw ns/op invalid — `u8` overflow-bug artifact, NOT a real 6.6B votes/sec
- **Complete consensus flow**: 2.6µs for 5 blocks with full voting

## Detailed Benchmark Results

### Block Addition Performance

| Operation | Time (mean) | Throughput | Notes |
|-----------|-------------|------------|-------|
| **Single Block** | 610.87 ns | 1.6M blocks/sec | Individual block addition |
| **100 Blocks** | 114.95 ns/block | 8.7M blocks/sec | Batch efficiency gain |
| **1,000 Blocks** | 28.16 ns/block | overflow-bug artifact (not 35.5M/s) | measures data-structure insert, not consensus |
| **10,000 Blocks** | 256.1 ps/block | overflow-bug artifact (not 3.9B/s) | `u8` counter capped at 255 |

**Key Insight (retracted)**: The apparent batch speedups are the Rust `u8` overflow bug (counter caps at 255 instead of 10,000), not real throughput. The honest consensus-bound rate is ≈21K votes/sec — Rust cannot exceed the C FFI it wraps. See `.github/workflows/README.md:41-49,67`.

### Vote Processing Performance

| Operation | Time (mean) | Throughput | Notes |
|-----------|-------------|------------|-------|
| **Single Vote** | 639.05 ns | 1.56M votes/sec | Individual vote processing |
| **100 Votes** | 59.62 ns/vote | overflow-bug artifact (not 16.8M/s) | not consensus |
| **1,000 Votes** | 12.86 ns/vote | overflow-bug artifact (not 77.8M/s) | not consensus |
| **10,000 Votes** | 151.7 ps/vote | overflow-bug artifact (not 6.6B/s) | `u8` counter capped at 255 |

**Key Insight (retracted)**: The "6.6 billion votes/second" figure is the Rust `u8` overflow bug, not a real measurement. Honest ceiling ≈21K votes/sec (C FFI). See `.github/workflows/README.md:41-49,67`.

### Consensus Operations

| Operation | Time (mean) | Description |
|-----------|-------------|-------------|
| **Finalization Check** | 659.68 ns | Check if block is accepted after voting |
| **Get Preference** | 619.52 ns | Retrieve preferred block ID |
| **Get Statistics** | 1.07 µs | Retrieve full consensus stats |
| **Poll Validators** | 576.84 ns | Poll 10 validators |

### Engine Type Comparison

| Engine Type | Time (mean) | Description |
|-------------|-------------|-------------|
| **Chain** | 1.128 µs | Linear consensus |
| **DAG** | 1.158 µs | DAG-based consensus |
| **PQ** | 1.337 µs | Post-quantum consensus |

**Key Insight**: All three consensus engine types perform within 20% of each other, with Chain being slightly fastest and PQ having additional quantum-resistant overhead.

### Complete Consensus Flow

The complete consensus flow benchmark simulates a realistic scenario:
- Add 5 blocks sequentially
- Process 5 votes per block
- Check acceptance status
- Retrieve preference and stats

**Result**: 2.64 µs total (528 ns per block with full voting)

## Performance Analysis

### Strengths

1. **Sub-microsecond Operations**: All individual operations complete in under 1µs
2. **Excellent Batch Scaling**: 100x+ improvement for large batches
3. **Consistent Performance**: Low variance across measurements (< 5% outliers)
4. **Memory Efficiency**: Zero-copy FFI design minimizes overhead

### Throughput Achievements

- **Block/Vote Processing**: the "billions/sec" figures are the `u8` overflow bug, not real throughput; honest ceiling ≈21K votes/sec (C FFI)
- **Complete Flow**: "378,000 consensus rounds/sec" exceeds the C-FFI ceiling and is unverified — treat as not measured

### FFI Overhead

The benchmark results show minimal FFI overhead:
- Single operations: ~600ns (includes FFI boundary crossing)
- Batch operations: Amortized to < 1ns per item

## Recommendations

### For Production Use

1. **Batch Operations**: Always batch blocks/votes when possible
2. **Engine Selection**: Use Chain for simple consensus, DAG for parallelism
3. **Memory Management**: Pre-allocate buffers for large batches
4. **Polling Strategy**: Batch validator polls to reduce overhead

### For Further Optimization

1. **SIMD Instructions**: Could improve batch processing further
2. **Lock-Free Structures**: For multi-threaded scenarios
3. **Custom Memory Allocator**: For reduced allocation overhead
4. **Direct Memory Access**: Bypass FFI for critical paths

## Conclusion

These bindings are an FFI wrapper around the C library. The batch "billion-scale"
throughput reported above is the Rust `u8` overflow bug, not real performance;
the honest consensus-bound rate is ≈21K votes/sec (Rust cannot exceed the C FFI
it wraps). This report is retained only as a record of the debunked 2025-11-10
numbers — it is NOT evidence of production readiness. See
`.github/workflows/README.md` and the "Honest Assessment" block in `LLM.md`.

---

## Benchmark Configuration

```toml
[dev-dependencies]
criterion = { version = "0.7.0", features = ["html_reports"] }

[[bench]]
name = "consensus_bench"
harness = false
```

**Measurement Settings**:
- Warm-up time: 3 seconds
- Measurement time: 10 seconds
- Sample size: 100 samples per benchmark
- Statistical confidence: 95%

**Hardware**: Native execution on Darwin kernel (macOS)

For detailed HTML reports, run: `cargo bench --bench consensus_bench` and check `target/criterion/report/index.html`