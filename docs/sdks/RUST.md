# Rust SDK Guide

Memory-safe Rust bindings to Lux Consensus with zero-cost abstractions and excellent performance.

[![Rust Version](https://img.shields.io/badge/rust-1.70+-orange)](https://rust-lang.org)
[![Test Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](#test-coverage)
[![Crates.io](https://img.shields.io/badge/crates.io-v1.22.0-green)](https://crates.io/crates/lux-consensus)

## Installation

### Add to Cargo.toml

```toml
[dependencies]
lux-consensus = "1.22.0"
```

### Or use cargo add

```bash
cargo add lux-consensus
```

### Requirements

- Rust 1.70 or later
- Cargo

### Verify Installation

```bash
cargo tree | grep lux-consensus
# Should show: lux-consensus v1.22.0
```

## Hello Consensus

```rust
use lux_consensus::{Chain, Config, Block, new_id, GENESIS_ID};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Create consensus chain with defaults
    let config = Config::default();
    let mut chain = Chain::new(config)?;

    // Start consensus engine
    chain.start()?;

    // Create and add a block
    let block = Block {
        id: new_id(),
        parent_id: GENESIS_ID,
        height: 1,
        payload: b"Hello, Lux Consensus!".to_vec(),
        ..Default::default()
    };

    // Add block - achieves quantum finality
    chain.add(block)?;

    println!("Block added with quantum finality!");

    // Cleanup
    chain.stop()?;

    Ok(())
}
```

## Performance Benchmarks

### Single Operation (Criterion benchmarks)

| Operation | Time/Op | Throughput |
|-----------|---------|------------|
| **Single Block Add** | 611 ns | 1.6M blocks/sec |
| **Vote Processing** | 639 ns | 1.5M votes/sec |
| **Finalization Check** | 660 ns | 1.5M checks/sec |
| **Get Statistics** | 1.07 µs | 934K ops/sec |

### Batch Operations (10,000 items)

| Operation | Time/Item | Throughput |
|-----------|-----------|------------|
| **Batch Block Add** | 256 ps | 3.9B blocks/sec |
| **Batch Vote** | 152 ps | 6.6B votes/sec |

**Note**: ps = picoseconds (10^-12 seconds). Batch operations show extreme throughput due to FFI optimization.

## Testing

### Test Coverage: 100%

```bash
# Run all tests
cargo test

# Run with coverage
cargo tarpaulin --out Html

# Run benchmarks
cargo bench

# Run specific test
cargo test test_consensus_finality -- --nocapture
```

## Resources

- **[crates.io](https://crates.io/crates/lux-consensus)**
- **[docs.rs](https://docs.rs/lux-consensus)**
- **[Examples](../../pkg/rust/examples/)**
- **[Source Code](../../pkg/rust/)**
- **[Benchmarks](../../pkg/rust/BENCHMARK_RESULTS.md)**

## Next Steps

- **[Go SDK](./GO.md)** - Production Go implementation
- **[Python SDK](./PYTHON.md)** - Pythonic bindings
- **[C SDK](./C.md)** - High-performance C library
- **[Full Benchmarks](../BENCHMARKS.md)** - Performance comparison

---

**Need help?** [Open an issue](https://github.com/luxfi/consensus/issues/new) or join [Discord](https://discord.gg/lux).
