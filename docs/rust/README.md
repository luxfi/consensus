# Rust Implementation Documentation

## Overview

The Rust implementation provides memory-safe, high-performance consensus with zero-cost abstractions. It leverages Rust's ownership system to prevent data races and ensure thread safety without runtime overhead.

## Installation

### Prerequisites
- Rust 1.70+ (install via [rustup](https://rustup.rs/))
- Cargo (included with Rust)
- ZeroMQ 4.3+ development libraries

### Adding to Your Project

Add to your `Cargo.toml`:

```toml
[dependencies]
lux-consensus = "0.1.0"
zmq = "0.10"
tokio = { version = "1.35", features = ["full"] }
serde = { version = "1.0", features = ["derive"] }
```

### Building from Source

```bash
# Clone the repository
git clone https://github.com/luxfi/consensus
cd consensus/rust

# Build in release mode
cargo build --release

# Run tests
cargo test

# Run benchmarks
cargo bench
```

## Quick Start

```rust
use lux_consensus::{
    Consensus, ConsensusParams, EngineType,
    Block, Vote, VoteType,
};
use std::time::SystemTime;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Create consensus with Snowball engine
    let params = ConsensusParams {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 10,
        max_outstanding_items: 1000,
    };
    
    let mut consensus = Consensus::new(EngineType::Snowball, params)?;
    
    // Create and add a block
    let block = Block {
        id: 0x1234,
        parent_id: 0x0000,
        height: 1,
        timestamp: SystemTime::now(),
        data: vec![1, 2, 3, 4],
    };
    
    consensus.add_block(block).await?;
    
    // Process a vote
    let vote = Vote {
        engine_type: EngineType::Snowball,
        node_id: 0x0001,
        block_id: 0x1234,
        vote_type: VoteType::Prefer,
    };
    
    consensus.process_vote(vote).await?;
    
    // Check if block is accepted
    if consensus.is_accepted(0x1234).await {
        println!("Block 0x1234 achieved consensus!");
    }
    
    Ok(())
}
```

## API Reference

### Core Types

#### `Consensus`
The main consensus engine struct.

```rust
impl Consensus {
    /// Creates a new consensus instance
    pub fn new(engine: EngineType, params: ConsensusParams) -> Result<Self>;
    
    /// Adds a block to consensus
    pub async fn add_block(&mut self, block: Block) -> Result<()>;
    
    /// Processes an incoming vote
    pub async fn process_vote(&mut self, vote: Vote) -> Result<()>;
    
    /// Checks if a block is accepted
    pub async fn is_accepted(&self, block_id: u16) -> bool;
    
    /// Gets the current preference
    pub fn get_preference(&self) -> Option<u16>;
    
    /// Initiates a poll for a block
    pub async fn poll(&mut self, block_id: u16) -> Result<Vec<Vote>>;
}
```

#### `ConsensusParams`
Configuration parameters for consensus.

```rust
#[derive(Debug, Clone)]
pub struct ConsensusParams {
    pub k: usize,                    // Consecutive successes
    pub alpha_preference: usize,     // Preference quorum
    pub alpha_confidence: usize,     // Confidence quorum
    pub beta: usize,                // Confidence threshold
    pub concurrent_polls: usize,    // Max concurrent polls
    pub max_outstanding_items: usize, // Max outstanding items
}
```

#### `Block`
Represents a block in the consensus.

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Block {
    pub id: u16,
    pub parent_id: u16,
    pub height: u64,
    pub timestamp: SystemTime,
    pub data: Vec<u8>,
}
```

#### `Vote`
Represents a vote message.

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Vote {
    pub engine_type: EngineType,
    pub node_id: u16,
    pub block_id: u16,
    pub vote_type: VoteType,
}
```

### Network Integration

#### Using ZeroMQ

```rust
use lux_consensus::network::{Network, NetworkConfig};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Create network configuration
    let config = NetworkConfig {
        bind_address: "tcp://0.0.0.0:5555".to_string(),
        connect_addresses: vec![
            "tcp://node1:5555".to_string(),
            "tcp://node2:5555".to_string(),
        ],
    };
    
    // Create network instance
    let mut network = Network::new(config)?;
    
    // Start network listener
    let mut consensus = Consensus::new(EngineType::Avalanche, params)?;
    
    network.start(move |vote| {
        // Process incoming votes
        consensus.process_vote(vote).await?;
    }).await?;
    
    // Broadcast a vote
    let vote = Vote { /* ... */ };
    network.broadcast(vote).await?;
    
    Ok(())
}
```

### Advanced Features

#### Custom Vote Handler

```rust
use lux_consensus::{VoteHandler, VoteContext};

struct MyVoteHandler;

impl VoteHandler for MyVoteHandler {
    fn handle_vote(&mut self, vote: Vote, ctx: &VoteContext) -> Result<()> {
        println!("Processing vote from node {}", vote.node_id);
        
        // Custom logic
        if ctx.is_byzantine(vote.node_id) {
            return Err("Byzantine node detected".into());
        }
        
        Ok(())
    }
}

// Register handler
consensus.set_vote_handler(Box::new(MyVoteHandler));
```

#### Async Stream Processing

```rust
use futures::StreamExt;
use lux_consensus::VoteStream;

// Create vote stream
let mut vote_stream = VoteStream::new("tcp://0.0.0.0:5555")?;

// Process votes as they arrive
while let Some(vote) = vote_stream.next().await {
    match vote {
        Ok(v) => consensus.process_vote(v).await?,
        Err(e) => eprintln!("Vote error: {}", e),
    }
}
```

#### Metrics and Monitoring

```rust
use lux_consensus::metrics::Metrics;

// Enable metrics collection
consensus.enable_metrics();

// Get metrics
let metrics = consensus.get_metrics();
println!("Votes processed: {}", metrics.votes_processed);
println!("Blocks accepted: {}", metrics.blocks_accepted);
println!("Average latency: {:?}", metrics.avg_latency);

// Export Prometheus metrics
let prometheus_metrics = metrics.to_prometheus();
```

## Traits and Generics

### Engine Trait

```rust
pub trait Engine: Send + Sync {
    /// Process a vote
    fn process_vote(&mut self, vote: Vote) -> Result<()>;
    
    /// Check if block is accepted
    fn is_accepted(&self, block_id: u16) -> bool;
    
    /// Get preference
    fn get_preference(&self) -> Option<u16>;
    
    /// Poll for block
    fn poll(&mut self, block_id: u16) -> Vec<Vote>;
}
```

### Custom Engine Implementation

```rust
use lux_consensus::{Engine, EngineState};

struct MyCustomEngine {
    state: EngineState,
}

impl Engine for MyCustomEngine {
    fn process_vote(&mut self, vote: Vote) -> Result<()> {
        // Custom consensus logic
        self.state.record_vote(vote);
        Ok(())
    }
    
    fn is_accepted(&self, block_id: u16) -> bool {
        self.state.is_accepted(block_id)
    }
    
    fn get_preference(&self) -> Option<u16> {
        self.state.preference
    }
    
    fn poll(&mut self, block_id: u16) -> Vec<Vote> {
        // Generate votes for polling
        vec![]
    }
}

// Use custom engine
let engine = Box::new(MyCustomEngine::new());
let consensus = Consensus::with_engine(engine, params)?;
```

## Error Handling

```rust
use lux_consensus::{ConsensusError, ErrorKind};

match consensus.add_block(block).await {
    Ok(()) => println!("Block added successfully"),
    Err(ConsensusError::DuplicateBlock(id)) => {
        eprintln!("Block {} already exists", id);
    }
    Err(ConsensusError::InvalidParent(id)) => {
        eprintln!("Invalid parent for block {}", id);
    }
    Err(e) => {
        eprintln!("Consensus error: {}", e);
    }
}

// Custom error handling
fn handle_consensus_error(e: ConsensusError) {
    match e.kind() {
        ErrorKind::Network => {
            // Retry network operation
        }
        ErrorKind::Byzantine => {
            // Handle Byzantine fault
        }
        ErrorKind::Timeout => {
            // Handle timeout
        }
        _ => {
            // Generic error handling
        }
    }
}
```

## Testing

### Unit Tests

```rust
#[cfg(test)]
mod tests {
    use super::*;
    
    #[tokio::test]
    async fn test_snowball_consensus() {
        let params = ConsensusParams::default();
        let mut consensus = Consensus::new(EngineType::Snowball, params).unwrap();
        
        let block = Block {
            id: 1,
            parent_id: 0,
            height: 1,
            timestamp: SystemTime::now(),
            data: vec![],
        };
        
        consensus.add_block(block).await.unwrap();
        
        // Simulate votes
        for i in 0..20 {
            let vote = Vote {
                engine_type: EngineType::Snowball,
                node_id: i,
                block_id: 1,
                vote_type: VoteType::Prefer,
            };
            consensus.process_vote(vote).await.unwrap();
        }
        
        assert!(consensus.is_accepted(1).await);
    }
}
```

### Integration Tests

```rust
#[tokio::test]
async fn test_network_consensus() {
    // Start test network
    let network = TestNetwork::new(5).await;
    
    // Create consensus nodes
    let nodes = network.create_nodes(EngineType::Avalanche).await;
    
    // Submit block to first node
    nodes[0].add_block(test_block()).await.unwrap();
    
    // Wait for consensus
    tokio::time::sleep(Duration::from_secs(5)).await;
    
    // Verify all nodes agree
    for node in &nodes {
        assert!(node.is_accepted(1).await);
    }
}
```

### Benchmarks

```rust
use criterion::{black_box, criterion_group, criterion_main, Criterion};

fn bench_vote_processing(c: &mut Criterion) {
    let rt = tokio::runtime::Runtime::new().unwrap();
    
    c.bench_function("process_vote", |b| {
        b.iter(|| {
            rt.block_on(async {
                let mut consensus = create_consensus();
                let vote = create_vote();
                consensus.process_vote(black_box(vote)).await
            })
        })
    });
}

criterion_group!(benches, bench_vote_processing);
criterion_main!(benches);
```

## Performance Optimization

### Zero-Copy Serialization

```rust
use lux_consensus::zero_copy::{ZeroCopyVote, VoteBuffer};

// Use zero-copy for network operations
let buffer = VoteBuffer::new(1024);
let vote = ZeroCopyVote::from_buffer(&buffer)?;

// Process without allocation
consensus.process_zero_copy_vote(&vote).await?;
```

### SIMD Optimizations

```rust
// Enable SIMD features in Cargo.toml
[features]
simd = ["packed_simd"]

// Use SIMD-optimized operations
use lux_consensus::simd::SimdConsensus;

let consensus = SimdConsensus::new(params)?;
consensus.process_votes_simd(&votes).await?;
```

### Memory Pool

```rust
use lux_consensus::pool::{MemoryPool, PooledBlock};

// Create memory pool
let pool = MemoryPool::new(1000);

// Allocate from pool
let block = PooledBlock::new_in(&pool, Block { /* ... */ });

// Automatic deallocation when dropped
consensus.add_pooled_block(block).await?;
```

## Examples

Complete examples in [`examples/`](../../examples/rust/):
- `basic_consensus.rs` - Simple consensus usage
- `network_node.rs` - Network node implementation
- `byzantine_simulation.rs` - Byzantine fault simulation
- `benchmark.rs` - Performance benchmarking
- `custom_engine.rs` - Custom consensus engine

## Best Practices

1. **Error Handling**: Always use `Result` types and handle errors explicitly
2. **Async/Await**: Use async for I/O operations to maximize throughput
3. **Lifetimes**: Minimize allocations with lifetime annotations
4. **Generics**: Use generics for flexible, reusable code
5. **Testing**: Write comprehensive unit and integration tests

## Troubleshooting

### Common Issues

1. **Compilation errors**
   ```bash
   # Update Rust
   rustup update
   
   # Clean build
   cargo clean && cargo build
   ```

2. **Async runtime panics**
   - Ensure you're using `#[tokio::main]` or creating runtime explicitly
   - Check for blocking operations in async contexts

3. **Performance issues**
   - Enable release mode: `cargo build --release`
   - Use `cargo flamegraph` for profiling
   - Check for unnecessary clones

4. **Memory usage**
   - Use `valgrind` or `heaptrack` for memory profiling
   - Enable jemalloc: `jemallocator = "0.5"`

## License

Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.