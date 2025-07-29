## 🚀 Lux Consensus Performance

### Pure Algorithm Performance (No Network)
With no networking overhead, the algorithms run at memory speed:
- **Voting**: ~1μs per vote
- **Poll recording**: ~10μs per poll
- **Finalization check**: ~100ns
- **BLS verification**: ~100μs on AVX-512 (25× faster than reference)

### Network Capacity Analysis (10 Gb LAN)

For a 5-validator local network with high-TPS parameters:

| Resource | Formula | 10 Gb Example | Controlling Knob |
|----------|---------|---------------|------------------|
| **Wire bandwidth** | `(k-1)·batchSize·txSize/minRound` | 4×4096×250B/5ms → 6.6 Gbps | batchSize, minRound |
| **BLS work** | `(k-1)/minRound` | 4/5ms → 800 verif/s | k, minRound |
| **CPU cost** | `verif/s × t_verif` | 800×100μs → 0.08 core | BLS lib & AVX-512 |
| **Consensus slots** | `pipeline/(β·minRound)` | 1/(4×5ms) → 50 slots/s | pipeline, β |
| **TPS (per node)** | `slots/s × batchSize` | 50×4096 → ~205k TPS | batchSize, pipeline |

### Practical Parameter Envelopes

| Goal | Safe Parameters (5 validators) | LAN Usage | Cluster TPS |
|------|-------------------------------|-----------|-------------|
| **50k TPS** (dev) | batch=1000, pipeline=1, β=4, minRound=10ms | 1 Gbps | 250k |
| **100k TPS** (CI) | batch=2000, pipeline=1, β=4, minRound=10ms | 2 Gbps | 500k |
| **~1M TPS** (max) | batch=4096, pipeline=1, β=4, minRound=5ms | 6.6 Gbps | 1M-1.1M |

**Note**: These stay below 70% of a single 10 Gb port for headroom. Raising pipeline > 1 scales traffic linearly: `Gbps ≈ 6.6 × pipeline`

## 🔧 High-TPS Configuration

```go
var HighTPSParams = config.Parameters{
    K:                     5,
    AlphaPreference:       4,
    AlphaConfidence:       4,
    Beta:                  4,                      // 20ms finality
    ConcurrentRepolls:     1,                      // keep wire under 10 Gb
    OptimalProcessing:     runtime.NumCPU(),       // saturate verifier cores
    BatchSize:             4096,                   // maximize throughput
    MaxOutstandingItems:   8192,
    MaxItemProcessingTime: 3 * time.Second,
    MinRoundInterval:      5 * time.Millisecond,
}
```

## 🧮 Capacity Formulas

```
Pipeline_max  ≈ (NIC_Gbps / 8) · minRound / [(k−1) · batchSize · txSize]
Cluster_TPS   ≈ (nodes · batchSize · pipeline) / (β · minRound)
```

For a 10 Gb card with k=5, batchSize=4096, minRound=5ms:
- `pipeline_max ≈ 1.5` → cap at pipeline=1 for safety

## 🎨 Architecture for Scale

To reach 1M+ TPS on 20-64 core servers:

```
     ┌───────────┐   tx gossip   ┌─────────┐
NIC─▶│  QUIC I/O │──────────────▶│ Sampler │──┐
     └───────────┘               └─────────┘  │ mpsc
                           verify pool (N cores) │
                                               ▼
                                       ┌────────────┐
                                       │  Focus /   │
                                       │  Commit    │
                                       └────────────┘
```

- **Tile pattern**: One Linux-isolated thread per stage (like Firedancer)
- **NUMA aware**: Pin verifier threads to single L3 node
- **Lock-free**: Ring buffers between tiles avoid kernel switches

## 📊 Mathematical Guarantees

Given parameters (K, αp, αc, β) and f Byzantine nodes:
- **Safety**: Disagreement probability ≤ (f/K)^β
- **Liveness**: Terminates if > αp honest nodes online
- **Finality**: Achieved in β consecutive rounds with αc votes

## 🚀 Quick Start

```go
import (
    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/focus"
)

// Use high-TPS parameters for local testing
params := config.HighTPSParams

// Create focus instance
sb := focus.NewFocus(params, initialChoice)

// Add choices
sb.Add(choiceA)
sb.Add(choiceB)

// Record poll results (from your network layer)
votes := map[ids.ID]int{
    choiceA: 15,
    choiceB: 6,
}
sb.RecordPoll(votes)

// Check if finalized
if sb.Finalized() {
    winner := sb.Preference()
}
```

## 🧪 Testing Without Network

```go
// Create in-memory test network
network := testing.NewNetwork(seed)

// Add nodes with different latencies
node1 := network.AddNode(id1, 50*time.Millisecond)
node2 := network.AddNode(id2, 100*time.Millisecond)

// Configure specific link latency
network.SetLatency(id1, id2, 200) // 200ms between node1 and node2

// Simulate network partition
network.Partition([]ids.NodeID{id1, id2}, []ids.NodeID{id3, id4})

// Set message drop rate
network.SetDropRate(0.1) // 10% message loss

// Start simulation
ctx := context.Background()
network.Start(ctx)
```

## 📈 Benchmarks

Run benchmarks with:
```bash
go test -bench=. -benchmem ./...
```

Example results on 64-core AMD EPYC:
```
BenchmarkFocusVote-64         1000000      1042 ns/op       0 B/op       0 allocs/op
BenchmarkRecordPoll-64            100000     10234 ns/op     256 B/op       4 allocs/op
BenchmarkFinalizationCheck-64   10000000       103 ns/op       0 B/op       0 allocs/op
BenchmarkBLSVerifyAVX512-64       10000    102341 ns/op       0 B/op       0 allocs/op
```

## 🔑 Key Takeaways

- **Network is king**: A single 10 Gb link tops out at ~1M TPS with safe headroom
- **CPU is trivial**: BLS needs only ~0.1 core at 200k TPS with AVX-512
- **Keep pipeline ≤ 1**: Unless you upgrade to 25 Gb or 100 Gb fabric
- **Three levers**: Adjust `batchSize`, `minRound`, `pipeline` to hit any point on the 50k → 1M TPS curve

## 📚 Learn More

See `/example` for:
- `main.go` - Basic parameter usage
- `focus_sim.go` - Consensus simulation with Byzantine nodes
- More examples coming soon...

## License

BSD 3-Clause - See LICENSE file for details.
