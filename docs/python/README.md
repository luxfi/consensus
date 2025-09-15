# Python Implementation Documentation

## Overview

The Python implementation provides a high-level, easy-to-use interface for consensus operations. It's ideal for research, prototyping, and integration with data science workflows.

## Installation

### Prerequisites
- Python 3.8+
- pip or conda
- ZeroMQ Python bindings

### Install via pip

```bash
# Install from PyPI (when available)
pip install lux-consensus

# Install from source
git clone https://github.com/luxfi/consensus
cd consensus/python
pip install -e .
```

### Install Dependencies

```bash
# Core dependencies
pip install pyzmq numpy asyncio typing-extensions

# Optional dependencies for performance
pip install numba cython uvloop

# Development dependencies
pip install pytest pytest-asyncio pytest-benchmark black mypy
```

## Quick Start

```python
import asyncio
from lux_consensus import Consensus, ConsensusParams, EngineType, Block, Vote, VoteType
from datetime import datetime

async def main():
    # Configure consensus parameters
    params = ConsensusParams(
        k=20,
        alpha_preference=15,
        alpha_confidence=15,
        beta=20,
        concurrent_polls=10,
        max_outstanding_items=1000
    )
    
    # Create consensus instance
    consensus = Consensus(EngineType.SNOWBALL, params)
    
    # Create and add a block
    block = Block(
        id=0x1234,
        parent_id=0x0000,
        height=1,
        timestamp=datetime.now(),
        data=b"Hello, Consensus!"
    )
    
    await consensus.add_block(block)
    
    # Process a vote
    vote = Vote(
        engine_type=EngineType.SNOWBALL,
        node_id=0x0001,
        block_id=0x1234,
        vote_type=VoteType.PREFER
    )
    
    await consensus.process_vote(vote)
    
    # Check if block is accepted
    if await consensus.is_accepted(0x1234):
        print(f"Block 0x{0x1234:04x} achieved consensus!")
    
    # Get consensus statistics
    stats = consensus.get_stats()
    print(f"Votes processed: {stats.votes_processed}")
    print(f"Blocks accepted: {stats.blocks_accepted}")

if __name__ == "__main__":
    asyncio.run(main())
```

## API Reference

### Core Classes

#### `Consensus`

Main consensus engine class.

```python
class Consensus:
    def __init__(self, engine_type: EngineType, params: ConsensusParams):
        """Initialize consensus engine."""
        
    async def add_block(self, block: Block) -> None:
        """Add a block to consensus."""
        
    async def process_vote(self, vote: Vote) -> None:
        """Process an incoming vote."""
        
    async def is_accepted(self, block_id: int) -> bool:
        """Check if a block is accepted."""
        
    def get_preference(self) -> Optional[int]:
        """Get current preference."""
        
    async def poll(self, block_id: int) -> List[Vote]:
        """Initiate polling for a block."""
        
    def get_stats(self) -> ConsensusStats:
        """Get consensus statistics."""
```

#### `ConsensusParams`

Configuration dataclass.

```python
@dataclass
class ConsensusParams:
    k: int = 20                          # Consecutive successes
    alpha_preference: int = 15           # Preference quorum
    alpha_confidence: int = 15           # Confidence quorum
    beta: int = 20                      # Confidence threshold
    concurrent_polls: int = 10          # Max concurrent polls
    max_outstanding_items: int = 1000   # Max outstanding items
    
    def validate(self) -> None:
        """Validate parameters."""
        if self.alpha_preference > self.k:
            raise ValueError("alpha_preference must be <= k")
```

#### `Block`

Block representation.

```python
@dataclass
class Block:
    id: int
    parent_id: int
    height: int
    timestamp: datetime
    data: bytes
    
    def hash(self) -> bytes:
        """Calculate block hash."""
        
    def serialize(self) -> bytes:
        """Serialize block to bytes."""
        
    @classmethod
    def deserialize(cls, data: bytes) -> 'Block':
        """Deserialize block from bytes."""
```

#### `Vote`

Vote message.

```python
@dataclass
class Vote:
    engine_type: EngineType
    node_id: int
    block_id: int
    vote_type: VoteType
    
    def pack(self) -> bytes:
        """Pack vote into 8-byte binary format."""
        
    @classmethod
    def unpack(cls, data: bytes) -> 'Vote':
        """Unpack vote from binary format."""
```

### Network Integration

#### ZeroMQ Network

```python
from lux_consensus.network import Network, NetworkConfig

async def run_network_node():
    # Configure network
    config = NetworkConfig(
        bind_address="tcp://0.0.0.0:5555",
        connect_addresses=[
            "tcp://node1:5555",
            "tcp://node2:5555"
        ]
    )
    
    # Create network and consensus
    network = Network(config)
    consensus = Consensus(EngineType.AVALANCHE, params)
    
    # Set vote handler
    async def handle_vote(vote: Vote):
        await consensus.process_vote(vote)
        
    network.on_vote(handle_vote)
    
    # Start network
    await network.start()
    
    # Broadcast vote
    vote = Vote(...)
    await network.broadcast(vote)
    
    # Run event loop
    await network.run_forever()
```

#### Asyncio Integration

```python
import asyncio
from lux_consensus import ConsensusNode

async def multi_node_simulation():
    # Create multiple nodes
    nodes = []
    for i in range(5):
        node = ConsensusNode(
            node_id=i,
            engine_type=EngineType.SNOWBALL,
            params=ConsensusParams()
        )
        nodes.append(node)
    
    # Connect nodes
    for node in nodes:
        await node.connect_to_peers(nodes)
    
    # Start all nodes
    tasks = [node.start() for node in nodes]
    await asyncio.gather(*tasks)
    
    # Submit block to first node
    block = Block(...)
    await nodes[0].submit_block(block)
    
    # Wait for consensus
    await asyncio.sleep(5)
    
    # Check consensus across all nodes
    for node in nodes:
        assert await node.is_accepted(block.id)
```

### Advanced Features

#### Custom Vote Handler

```python
from lux_consensus import VoteHandler

class CustomVoteHandler(VoteHandler):
    def __init__(self):
        self.vote_count = 0
        self.byzantine_nodes = set()
    
    async def handle_vote(self, vote: Vote, context: dict) -> bool:
        """Process vote with custom logic."""
        self.vote_count += 1
        
        # Check for Byzantine behavior
        if self.is_byzantine(vote, context):
            self.byzantine_nodes.add(vote.node_id)
            return False  # Reject vote
        
        return True  # Accept vote
    
    def is_byzantine(self, vote: Vote, context: dict) -> bool:
        """Detect Byzantine behavior."""
        # Custom Byzantine detection logic
        return False

# Use custom handler
handler = CustomVoteHandler()
consensus.set_vote_handler(handler)
```

#### Metrics and Monitoring

```python
from lux_consensus.metrics import MetricsCollector
import prometheus_client

# Enable metrics
metrics = MetricsCollector()
consensus.enable_metrics(metrics)

# Access metrics
print(f"Votes/sec: {metrics.votes_per_second}")
print(f"Latency: {metrics.average_latency_ms}ms")
print(f"Memory: {metrics.memory_usage_mb}MB")

# Export to Prometheus
prometheus_client.start_http_server(8000)

# Or get as dict
metrics_dict = metrics.to_dict()
```

#### Visualization

```python
from lux_consensus.visualize import ConsensusVisualizer
import matplotlib.pyplot as plt

# Create visualizer
viz = ConsensusVisualizer(consensus)

# Plot consensus progress
fig = viz.plot_consensus_progress()
plt.show()

# Animate consensus in real-time
viz.animate_consensus(interval=100)

# Generate network graph
graph = viz.generate_network_graph(nodes)
viz.plot_network(graph)

# Export metrics over time
viz.export_metrics_csv("consensus_metrics.csv")
```

## Data Science Integration

### NumPy Integration

```python
import numpy as np
from lux_consensus import ConsensusAnalyzer

# Analyze vote patterns
analyzer = ConsensusAnalyzer(consensus)

# Get vote matrix
vote_matrix = analyzer.get_vote_matrix()
print(f"Vote matrix shape: {vote_matrix.shape}")

# Calculate statistics
mean_votes = np.mean(vote_matrix, axis=0)
std_votes = np.std(vote_matrix, axis=0)

# Detect anomalies
anomalies = analyzer.detect_anomalies(threshold=3.0)
```

### Pandas Integration

```python
import pandas as pd

# Get consensus data as DataFrame
df = consensus.to_dataframe()

# Analyze voting patterns
vote_summary = df.groupby(['node_id', 'vote_type']).size()
print(vote_summary)

# Time series analysis
df['timestamp'] = pd.to_datetime(df['timestamp'])
df.set_index('timestamp', inplace=True)

# Resample to 1-second intervals
resampled = df.resample('1S').count()

# Export for further analysis
df.to_csv('consensus_data.csv')
df.to_parquet('consensus_data.parquet')
```

### Machine Learning

```python
from sklearn.ensemble import IsolationForest
from lux_consensus.ml import ConsensusML

# Train anomaly detector
ml = ConsensusML(consensus)
model = ml.train_anomaly_detector()

# Detect Byzantine nodes
byzantine_scores = ml.detect_byzantine_nodes(model)
byzantine_nodes = [node for node, score in byzantine_scores.items() if score > 0.8]

# Predict consensus time
features = ml.extract_features(block)
predicted_time = ml.predict_consensus_time(features)
print(f"Predicted consensus time: {predicted_time:.2f}s")
```

## Testing

### Unit Tests

```python
import pytest
from lux_consensus import Consensus, Block, Vote

@pytest.mark.asyncio
async def test_snowball_consensus():
    """Test Snowball consensus basic functionality."""
    consensus = Consensus(EngineType.SNOWBALL, ConsensusParams())
    
    # Add block
    block = Block(id=1, parent_id=0, height=1, 
                  timestamp=datetime.now(), data=b"test")
    await consensus.add_block(block)
    
    # Simulate votes
    for i in range(20):
        vote = Vote(
            engine_type=EngineType.SNOWBALL,
            node_id=i,
            block_id=1,
            vote_type=VoteType.PREFER
        )
        await consensus.process_vote(vote)
    
    assert await consensus.is_accepted(1)
```

### Integration Tests

```python
@pytest.mark.asyncio
async def test_network_consensus():
    """Test consensus over network."""
    # Create test network
    network = await create_test_network(num_nodes=5)
    
    # Submit block
    block = create_test_block()
    await network.nodes[0].submit_block(block)
    
    # Wait for consensus
    await asyncio.sleep(5)
    
    # Verify consensus
    for node in network.nodes:
        assert await node.is_accepted(block.id)
```

### Performance Tests

```python
import pytest_benchmark

def test_vote_processing_performance(benchmark):
    """Benchmark vote processing."""
    consensus = Consensus(EngineType.SNOWBALL, ConsensusParams())
    vote = create_test_vote()
    
    # Run benchmark
    result = benchmark(process_vote_sync, consensus, vote)
    
    # Assert performance requirements
    assert benchmark.stats['mean'] < 0.001  # < 1ms average
```

## Performance Optimization

### Numba JIT Compilation

```python
from numba import jit
import numpy as np

@jit(nopython=True)
def fast_vote_counting(votes: np.ndarray) -> tuple:
    """JIT-compiled vote counting."""
    prefer_count = 0
    accept_count = 0
    reject_count = 0
    
    for vote in votes:
        if vote == 1:
            prefer_count += 1
        elif vote == 2:
            accept_count += 1
        else:
            reject_count += 1
    
    return prefer_count, accept_count, reject_count

# Use in consensus
consensus.set_vote_counter(fast_vote_counting)
```

### Cython Extensions

```python
# vote_processor.pyx
from libc.stdint cimport uint8_t, uint16_t

cpdef process_votes_fast(uint8_t[:] votes):
    cdef int i
    cdef int n = votes.shape[0]
    cdef int count = 0
    
    for i in range(n):
        if votes[i] == 1:
            count += 1
    
    return count

# Use compiled extension
from lux_consensus.cython import process_votes_fast
result = process_votes_fast(vote_array)
```

### Async Optimization

```python
import uvloop

# Use uvloop for better async performance
asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())

# Batch processing
async def process_votes_batch(consensus, votes):
    """Process votes in batches for better performance."""
    batch_size = 100
    
    for i in range(0, len(votes), batch_size):
        batch = votes[i:i+batch_size]
        tasks = [consensus.process_vote(v) for v in batch]
        await asyncio.gather(*tasks)
```

## Deployment

### Docker

```dockerfile
FROM python:3.11-slim

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY . .

# Run consensus node
CMD ["python", "-m", "lux_consensus.node"]
```

### Configuration

```yaml
# config.yaml
consensus:
  engine: snowball
  params:
    k: 20
    alpha_preference: 15
    alpha_confidence: 15
    beta: 20

network:
  bind_address: "tcp://0.0.0.0:5555"
  peers:
    - "tcp://peer1:5555"
    - "tcp://peer2:5555"

monitoring:
  metrics_port: 8000
  log_level: INFO
```

### Production Deployment

```python
import logging
from lux_consensus import ProductionNode

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)

# Create production node
node = ProductionNode.from_config("config.yaml")

# Enable monitoring
node.enable_prometheus_metrics(port=8000)
node.enable_health_check(port=8080)

# Run with graceful shutdown
try:
    asyncio.run(node.run())
except KeyboardInterrupt:
    logging.info("Shutting down gracefully...")
    asyncio.run(node.shutdown())
```

## Examples

Complete examples in [`examples/`](../../examples/python/):
- `simple_consensus.py` - Basic usage
- `network_simulation.py` - Network simulation
- `byzantine_test.py` - Byzantine fault testing
- `visualization.py` - Real-time visualization
- `ml_analysis.py` - Machine learning analysis

## Troubleshooting

### Common Issues

1. **Import errors**
   ```bash
   # Ensure package is installed
   pip install -e .
   
   # Check Python path
   export PYTHONPATH=$PYTHONPATH:/path/to/consensus/python
   ```

2. **Async issues**
   ```python
   # Use proper async context
   asyncio.run(main())
   
   # Or create event loop explicitly
   loop = asyncio.new_event_loop()
   asyncio.set_event_loop(loop)
   loop.run_until_complete(main())
   ```

3. **Performance issues**
   - Use `uvloop` for better async performance
   - Enable JIT compilation with Numba
   - Use batch processing for votes

4. **Memory issues**
   - Use generators for large datasets
   - Enable garbage collection tuning
   - Monitor with `memory_profiler`

## License

Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.