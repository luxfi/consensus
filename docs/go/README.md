# Go Implementation Documentation

## Overview

The Go implementation is the production-ready consensus engine powering the Lux blockchain. It provides high-performance, concurrent consensus with seamless integration into the Lux node.

## Installation

### Prerequisites
- Go 1.21+
- Git
- Make

### Install as Module

```bash
go get github.com/luxfi/consensus
```

### Build from Source

```bash
# Clone repository
git clone https://github.com/luxfi/consensus
cd consensus

# Build all components
make build

# Run tests
make test

# Install CLI tools
make install
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "github.com/luxfi/consensus/engine/core"
    "github.com/luxfi/ids"
)

func main() {
    // Configure consensus parameters
    params := core.ConsensusParams{
        K:                    20,
        AlphaPreference:     15,
        AlphaConfidence:     15,
        Beta:                20,
        ConcurrentPolls:     10,
        OptimalProcessing:   10,
        MaxOutstandingItems: 1000,
        MaxItemProcessingTime: 30 * time.Second,
    }
    
    // Create consensus engine
    consensus, err := core.NewCGOConsensus(params)
    if err != nil {
        log.Fatal(err)
    }
    
    // Create a block
    block := &SimpleBlock{
        id:        ids.GenerateTestID(),
        parentID:  ids.Empty,
        height:    1,
        timestamp: time.Now().Unix(),
        data:      []byte("Hello, Consensus!"),
    }
    
    // Add block to consensus
    if err := consensus.Add(block); err != nil {
        log.Fatal(err)
    }
    
    // Record votes
    for i := 0; i < 20; i++ {
        if err := consensus.RecordPoll(block.ID(), true); err != nil {
            log.Fatal(err)
        }
    }
    
    // Check if accepted
    if consensus.IsAccepted(block.ID()) {
        fmt.Printf("Block %s achieved consensus!\n", block.ID())
    }
}
```

## API Reference

### Core Types

#### `ConsensusParams`

Configuration parameters for consensus engine.

```go
type ConsensusParams struct {
    K                     int           // Consecutive successes needed
    AlphaPreference      int           // Quorum size for preference
    AlphaConfidence      int           // Quorum size for confidence
    Beta                 int           // Confidence threshold
    ConcurrentPolls      int           // Max concurrent polls
    OptimalProcessing    int           // Optimal processing size
    MaxOutstandingItems  int           // Max outstanding items
    MaxItemProcessingTime time.Duration // Max processing time per item
}
```

#### `Block` Interface

Interface that blocks must implement.

```go
type Block interface {
    ID() ids.ID              // Unique block identifier
    ParentID() ids.ID        // Parent block ID
    Height() uint64          // Block height
    Timestamp() int64        // Unix timestamp
    Bytes() []byte           // Serialized block data
    Verify(context.Context) error  // Verify block validity
    Accept(context.Context) error  // Accept block
    Reject(context.Context) error  // Reject block
}
```

#### `Consensus` Interface

Main consensus engine interface.

```go
type Consensus interface {
    Add(Block) error                      // Add block to consensus
    RecordPoll(ids.ID, bool) error       // Record poll result
    IsAccepted(ids.ID) bool              // Check if block is accepted
    GetPreference() ids.ID               // Get current preference
    Finalized() bool                     // Check if finalized
    Parameters() ConsensusParams         // Get parameters
    HealthCheck() error                  // Health check
}
```

### Engine Implementations

#### CGO Consensus (High Performance)

```go
// Using CGO for maximum performance
consensus, err := core.NewCGOConsensus(params)

// All methods are thread-safe
go func() {
    consensus.Add(block1)
}()
go func() {
    consensus.Add(block2)
}()
```

#### Pure Go Consensus

```go
// Pure Go implementation (no CGO required)
consensus, err := core.NewPureGoConsensus(params)

// Identical API to CGO version
consensus.Add(block)
consensus.RecordPoll(blockID, true)
```

### Factory Pattern

```go
// Use factory for runtime selection
factory := core.NewConsensusFactory()

// Create appropriate implementation
consensus, err := factory.CreateConsensus(params)
```

## Advanced Features

### Custom Block Implementation

```go
type MyBlock struct {
    id        ids.ID
    parentID  ids.ID
    height    uint64
    timestamp int64
    txs       []Transaction
    state     State
}

func (b *MyBlock) ID() ids.ID {
    return b.id
}

func (b *MyBlock) ParentID() ids.ID {
    return b.parentID
}

func (b *MyBlock) Height() uint64 {
    return b.height
}

func (b *MyBlock) Timestamp() int64 {
    return b.timestamp
}

func (b *MyBlock) Bytes() []byte {
    // Serialize block
    return b.marshal()
}

func (b *MyBlock) Verify(ctx context.Context) error {
    // Verify transactions
    for _, tx := range b.txs {
        if err := tx.Verify(); err != nil {
            return err
        }
    }
    
    // Verify state transition
    return b.state.Verify()
}

func (b *MyBlock) Accept(ctx context.Context) error {
    // Commit state changes
    return b.state.Commit()
}

func (b *MyBlock) Reject(ctx context.Context) error {
    // Rollback state changes
    return b.state.Rollback()
}
```

### Network Integration

```go
package main

import (
    "github.com/luxfi/consensus/network"
    "github.com/luxfi/consensus/engine/core"
)

func runNetworkNode() error {
    // Create network manager
    net, err := network.NewManager(network.Config{
        NodeID:       ids.GenerateTestNodeID(),
        ListenAddr:   "0.0.0.0:9650",
        BootstrapIPs: []string{"node1:9650", "node2:9650"},
    })
    if err != nil {
        return err
    }
    
    // Create consensus
    consensus, err := core.NewCGOConsensus(params)
    if err != nil {
        return err
    }
    
    // Register vote handler
    net.OnVote(func(nodeID ids.NodeID, vote *network.Vote) {
        consensus.RecordPoll(vote.BlockID, vote.IsPreference)
    })
    
    // Start network
    return net.Start()
}
```

### Metrics and Monitoring

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/luxfi/consensus/metrics"
)

// Create metrics collector
collector := metrics.NewCollector("consensus")

// Wrap consensus with metrics
consensus = metrics.WrapConsensus(consensus, collector)

// Register with Prometheus
prometheus.MustRegister(collector)

// Access metrics
stats := collector.GetStats()
fmt.Printf("Blocks accepted: %d\n", stats.BlocksAccepted)
fmt.Printf("Votes processed: %d\n", stats.VotesProcessed)
fmt.Printf("Average latency: %.2fms\n", stats.AverageDecisionTimeMs)
```

### Context and Cancellation

```go
// Use context for cancellation
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Verify with timeout
if err := block.Verify(ctx); err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        log.Println("Verification timed out")
    }
    return err
}

// Accept with context
if err := block.Accept(ctx); err != nil {
    return err
}
```

## Testing

### Unit Tests

```go
package consensus_test

import (
    "testing"
    "github.com/stretchr/testify/require"
    "github.com/luxfi/consensus/engine/core"
)

func TestSnowballConsensus(t *testing.T) {
    require := require.New(t)
    
    // Create consensus
    params := core.ConsensusParams{
        K:               20,
        AlphaPreference: 15,
    }
    consensus, err := core.NewCGOConsensus(params)
    require.NoError(err)
    
    // Add block
    block := createTestBlock()
    require.NoError(consensus.Add(block))
    
    // Simulate votes
    for i := 0; i < 20; i++ {
        require.NoError(consensus.RecordPoll(block.ID(), true))
    }
    
    // Verify acceptance
    require.True(consensus.IsAccepted(block.ID()))
}
```

### Benchmarks

```go
func BenchmarkVoteProcessing(b *testing.B) {
    consensus, _ := core.NewCGOConsensus(defaultParams)
    block := createTestBlock()
    consensus.Add(block)
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            consensus.RecordPoll(block.ID(), true)
        }
    })
    
    b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "votes/sec")
}
```

### Integration Tests

```go
func TestMultiNodeConsensus(t *testing.T) {
    // Create test network
    network := NewTestNetwork(t, 5)
    defer network.Shutdown()
    
    // Create nodes with consensus
    nodes := make([]*Node, 5)
    for i := range nodes {
        nodes[i] = network.NewNode(core.ConsensusParams{
            K: 20,
            AlphaPreference: 15,
        })
    }
    
    // Submit block to first node
    block := createTestBlock()
    require.NoError(t, nodes[0].SubmitBlock(block))
    
    // Wait for consensus
    require.Eventually(t, func() bool {
        for _, node := range nodes {
            if !node.IsAccepted(block.ID()) {
                return false
            }
        }
        return true
    }, 10*time.Second, 100*time.Millisecond)
}
```

### Fuzzing

```go
func FuzzConsensus(f *testing.F) {
    // Add seed corpus
    f.Add([]byte{1, 2, 3, 4})
    
    f.Fuzz(func(t *testing.T, data []byte) {
        consensus, _ := core.NewCGOConsensus(defaultParams)
        
        // Create block from fuzz data
        block := &FuzzBlock{data: data}
        
        // Should not panic
        _ = consensus.Add(block)
        _ = consensus.RecordPoll(block.ID(), true)
    })
}
```

## Performance Optimization

### Concurrent Processing

```go
// Use worker pool for vote processing
type VoteProcessor struct {
    consensus core.Consensus
    votes     chan *Vote
    workers   int
}

func (vp *VoteProcessor) Start() {
    for i := 0; i < vp.workers; i++ {
        go vp.worker()
    }
}

func (vp *VoteProcessor) worker() {
    for vote := range vp.votes {
        vp.consensus.RecordPoll(vote.BlockID, vote.IsPreference)
    }
}

func (vp *VoteProcessor) ProcessVote(vote *Vote) {
    vp.votes <- vote
}
```

### Memory Pooling

```go
import "sync"

var blockPool = sync.Pool{
    New: func() interface{} {
        return &Block{
            data: make([]byte, 0, 1024),
        }
    },
}

func getBlock() *Block {
    return blockPool.Get().(*Block)
}

func putBlock(b *Block) {
    b.Reset()
    blockPool.Put(b)
}
```

### Batch Operations

```go
// Batch multiple operations
type BatchConsensus struct {
    core.Consensus
    batch []Operation
    mu    sync.Mutex
}

func (bc *BatchConsensus) RecordPollBatch(polls []Poll) error {
    bc.mu.Lock()
    defer bc.mu.Unlock()
    
    for _, poll := range polls {
        bc.batch = append(bc.batch, poll)
    }
    
    if len(bc.batch) >= 100 {
        return bc.flush()
    }
    return nil
}

func (bc *BatchConsensus) flush() error {
    // Process batch efficiently
    for _, op := range bc.batch {
        if err := bc.Consensus.RecordPoll(op.BlockID, op.Vote); err != nil {
            return err
        }
    }
    bc.batch = bc.batch[:0]
    return nil
}
```

## Deployment

### Configuration

```yaml
# consensus.yaml
consensus:
  engine: snowball
  params:
    k: 20
    alpha_preference: 15
    alpha_confidence: 15
    beta: 20
    concurrent_polls: 10
    optimal_processing: 10
    max_outstanding_items: 1000
    max_item_processing_time: 30s

network:
  node_id: "NodeID-7Xhw2mDxuDS44j42TCB6U5579esbSt3Lg"
  listen_addr: "0.0.0.0:9650"
  bootstrap_nodes:
    - "node1.lux.network:9650"
    - "node2.lux.network:9650"

metrics:
  enabled: true
  prometheus_port: 9090
  
logging:
  level: info
  output: stdout
```

### Docker Deployment

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY . .
RUN go build -o consensus ./cmd/node

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /build/consensus /consensus

EXPOSE 9650 9090
ENTRYPOINT ["/consensus"]
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: consensus-node
spec:
  serviceName: consensus
  replicas: 5
  selector:
    matchLabels:
      app: consensus
  template:
    metadata:
      labels:
        app: consensus
    spec:
      containers:
      - name: consensus
        image: luxfi/consensus:latest
        ports:
        - containerPort: 9650
          name: consensus
        - containerPort: 9090
          name: metrics
        volumeMounts:
        - name: data
          mountPath: /data
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 100Gi
```

## CLI Tools

### consensus CLI

```bash
# Check consensus status
consensus status

# Add block
consensus block add --data "block data"

# Query block
consensus block get --id 0x1234

# Monitor consensus
consensus monitor --interval 1s

# Run benchmarks
consensus bench --duration 60s --parallel 10
```

### Development Tools

```bash
# Generate test data
go run ./cmd/generate-test-data

# Run simulator
go run ./cmd/simulator --nodes 100 --duration 10m

# Analyze consensus logs
go run ./cmd/analyze-logs consensus.log
```

## Troubleshooting

### Common Issues

1. **Build errors**
   ```bash
   # Update dependencies
   go mod tidy
   
   # Clear module cache
   go clean -modcache
   
   # Rebuild with verbose output
   go build -v ./...
   ```

2. **CGO issues**
   ```bash
   # Build without CGO
   CGO_ENABLED=0 go build
   
   # Or use pure Go implementation
   go build -tags purego
   ```

3. **Performance issues**
   - Enable pprof profiling
   - Use `go tool pprof` for analysis
   - Check goroutine leaks with `runtime.NumGoroutine()`
   - Monitor with `expvar` package

4. **Consensus stalls**
   - Check network connectivity
   - Verify parameter configuration
   - Enable debug logging
   - Use health check endpoint

## Best Practices

1. **Error Handling**: Always check and handle errors
2. **Context Usage**: Pass context for cancellation
3. **Resource Cleanup**: Use defer for cleanup
4. **Concurrency**: Use channels and goroutines properly
5. **Testing**: Write comprehensive tests with good coverage

## Examples

Complete examples in [`examples/`](../../examples/go/):
- `simple/main.go` - Basic consensus usage
- `network/main.go` - Network node implementation
- `byzantine/main.go` - Byzantine simulation
- `benchmark/main.go` - Performance testing
- `monitor/main.go` - Consensus monitoring

## License

Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.