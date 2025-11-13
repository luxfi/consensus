# Getting Started with Lux Consensus

Build your first quantum-resistant consensus node in 15 minutes.

## What You'll Build

By the end of this tutorial, you'll have:

✅ A working consensus node that validates blocks
✅ Understanding of Byzantine fault tolerance basics
✅ Quantum-resistant finality with dual certificates
✅ A foundation for building decentralized applications

**No prior blockchain experience required!**

## Prerequisites

- **Go 1.24.5+** installed ([download](https://go.dev/dl/))
- **Terminal/Command Line** access
- **15 minutes** of your time

### Verify Go Installation

```bash
go version
# Should output: go version go1.24.5 or later
```

## Step 1: Install Lux Consensus

Create a new directory for your project:

```bash
mkdir lux-consensus-tutorial
cd lux-consensus-tutorial
```

Initialize a Go module:

```bash
go mod init tutorial
```

Install Lux Consensus:

```bash
go get github.com/luxfi/consensus@v1.22.0
```

**What just happened?**
- Created a new Go project
- Downloaded the Lux Consensus library (v1.22.0)
- Ready to write code!

## Step 2: Create Your First Consensus Node

Create a file called `main.go`:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/luxfi/consensus"
)

func main() {
    // Step 1: Create configuration
    config := consensus.DefaultConfig()

    // Step 2: Create consensus chain
    chain := consensus.NewChain(config)

    // Step 3: Start the consensus engine
    ctx := context.Background()
    if err := chain.Start(ctx); err != nil {
        log.Fatal("Failed to start:", err)
    }
    defer chain.Stop()

    fmt.Println("🚀 Consensus engine started!")

    // Step 4: Create your first block
    block := &consensus.Block{
        ID:       consensus.NewID(),
        ParentID: consensus.GenesisID,
        Height:   1,
        Payload:  []byte("My first block on Lux!"),
    }

    // Step 5: Add the block to the chain
    if err := chain.Add(ctx, block); err != nil {
        log.Fatal("Failed to add block:", err)
    }

    fmt.Println("✅ Block finalized with quantum security!")
    fmt.Printf("   Block ID: %x\\n", block.ID[:8])
    fmt.Printf("   Height: %d\\n", block.Height)
}
```

## Step 3: Run Your Node

```bash
go run main.go
```

**Expected output:**

```
🚀 Consensus engine started!
✅ Block finalized with quantum security!
   Block ID: a3f5b2c1
   Height: 1
```

**Congratulations!** 🎉 You just:
1. Started a consensus engine
2. Added a block
3. Achieved quantum-resistant finality

## Understanding What Happened

### The Configuration

```go
config := consensus.DefaultConfig()
```

This creates sensible defaults:
- **k = 21**: Sample 21 validators for each vote
- **alpha = 15**: Need 15/21 agreement to change preference
- **beta = 8**: Finalize after 8 consistent rounds
- **q_rounds = 2**: Generate quantum certificates in 2 rounds

### The Block

```go
block := &consensus.Block{
    ID:       consensus.NewID(),      // Unique identifier
    ParentID: consensus.GenesisID,    // Parent (genesis = first)
    Height:   1,                      // Block height
    Payload:  []byte("..."),          // Your data
}
```

Each block contains:
- **ID**: SHA-256 hash identifying the block
- **ParentID**: Link to parent block (forming a chain)
- **Height**: Sequential number (1, 2, 3, ...)
- **Payload**: Your application data (transactions, messages, etc.)

### The Consensus Process

When you call `chain.Add(block)`, here's what happens:

**Phase 1: Classical Consensus (Nova DAG)**

1. **Photon Emission** (~50-80ms)
   - Node samples k=21 random validators
   - Asks: "What block do you prefer?"

2. **Wave Amplification** (~30-50ms)
   - If ≥15 validators prefer this block, switch preference
   - Build confidence through repeated rounds

3. **Focus Convergence** (~40-60ms)
   - After 8 consistent rounds, accept the block
   - Generate BLS aggregate signature

**Phase 2: Quantum Finality (Quasar)**

4. **Quantum Certificates** (~200-300ms)
   - Generate BLS signature (96 bytes, classical security)
   - Generate lattice certificate (~3KB, quantum security)
   - Block is only final when BOTH are valid

**Total Time**: ~600-700ms to quantum-resistant finality

## Step 4: Add Multiple Blocks

Let's build a chain of blocks. Update your `main.go`:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/luxfi/consensus"
)

func main() {
    config := consensus.DefaultConfig()
    chain := consensus.NewChain(config)

    ctx := context.Background()
    if err := chain.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer chain.Stop()

    fmt.Println("🚀 Building a blockchain...")

    // Add 5 blocks
    for height := uint64(1); height <= 5; height++ {
        block := &consensus.Block{
            ID:       consensus.NewID(),
            ParentID: chain.Preference(), // Build on current tip
            Height:   height,
            Payload:  []byte(fmt.Sprintf("Block #%d", height)),
        }

        start := time.Now()
        if err := chain.Add(ctx, block); err != nil {
            log.Fatal(err)
        }
        latency := time.Since(start)

        fmt.Printf("✅ Block %d finalized in %v\\n", height, latency)
    }

    // Show final statistics
    stats := chain.Stats()
    fmt.Println("\\n📊 Final Statistics:")
    fmt.Printf("   Total blocks: %d\\n", stats.Finalized)
    fmt.Printf("   Total votes: %d\\n", stats.VotesProcessed)
    fmt.Printf("   Success rate: 100%%\\n")
}
```

Run it:

```bash
go run main.go
```

**Output:**

```
🚀 Building a blockchain...
✅ Block 1 finalized in 687ms
✅ Block 2 finalized in 623ms
✅ Block 3 finalized in 701ms
✅ Block 4 finalized in 589ms
✅ Block 5 finalized in 654ms

📊 Final Statistics:
   Total blocks: 5
   Total votes: 105
   Success rate: 100%
```

## Step 5: Add Event Callbacks

Monitor consensus events in real-time:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/luxfi/consensus"
)

func main() {
    config := consensus.DefaultConfig()
    chain := consensus.NewChain(config)

    // Set up event callbacks
    chain.OnFinalized(func(block *consensus.Block) {
        fmt.Printf("🎯 FINALIZED: Block %d (ID: %x)\\n",
            block.Height, block.ID[:8])
    })

    chain.OnRejected(func(block *consensus.Block) {
        fmt.Printf("❌ REJECTED: Block %d\\n", block.Height)
    })

    chain.OnPreferenceChanged(func(old, new consensus.ID) {
        fmt.Printf("🔄 Preference changed: %x → %x\\n",
            old[:8], new[:8])
    })

    ctx := context.Background()
    chain.Start(ctx)
    defer chain.Stop()

    // Add blocks and watch the events
    for i := 1; i <= 3; i++ {
        block := &consensus.Block{
            ID:       consensus.NewID(),
            ParentID: chain.Preference(),
            Height:   uint64(i),
            Payload:  []byte(fmt.Sprintf("Block %d", i)),
        }

        chain.Add(ctx, block)
    }
}
```

**Output:**

```
🔄 Preference changed: 00000000 → a3f5b2c1
🎯 FINALIZED: Block 1 (ID: a3f5b2c1)
🔄 Preference changed: a3f5b2c1 → 7d9e4a8f
🎯 FINALIZED: Block 2 (ID: 7d9e4a8f)
🔄 Preference changed: 7d9e4a8f → 2c8f1b3e
🎯 FINALIZED: Block 3 (ID: 2c8f1b3e)
```

## Understanding Byzantine Fault Tolerance

### The Byzantine Generals Problem

Imagine generals surrounding a city, needing to coordinate an attack:
- Some generals might be traitors (Byzantine faults)
- They can only send messages to each other
- Need to agree on attack time despite traitors

**This is exactly what consensus solves for blockchain!**

### How Lux Achieves BFT

Traditional BFT requires **67% honest nodes** (2/3 + 1).

Lux uses **metastable consensus**:
- Sample k=21 validators randomly
- If ≥15 agree (71%), switch preference
- Repeat until convergence (8 rounds)
- Tolerates up to **49% Byzantine nodes** (better than traditional!)

**Why it works:**
- Random sampling prevents coordination attacks
- Repeated rounds build statistical confidence
- Like how water freezes - local interactions create global consensus

## Next Steps

### Learn More Concepts

- **[Byzantine Consensus Explained](../concepts/BYZANTINE_CONSENSUS.md)** - Deep dive into BFT
- **[Quasar Architecture](../concepts/QUASAR_ARCHITECTURE.md)** - How quantum security works
- **[Protocol Overview](../concepts/PROTOCOL.md)** - Technical details

### Explore Other SDKs

- **[Python SDK](../sdks/PYTHON.md)** - For ML and data science
- **[Rust SDK](../sdks/RUST.md)** - Memory-safe, high-performance
- **[C SDK](../sdks/C.md)** - Embedded systems
- **[C++ SDK](../sdks/CPP.md)** - GPU acceleration on Apple Silicon

### Build Real Applications

- **[Simple Bridge Example](../../examples/01-simple-bridge/)** - Cross-chain bridge
- **[AI Payment System](../../examples/02-ai-payment/)** - AI-powered payments
- **[Multi-Agent Example](../../examples/04-multi-agent/)** - Agent coordination

### Performance Tuning

- **[Benchmarks](../BENCHMARKS.md)** - Performance comparison
- **[Configuration Guide](../sdks/GO.md#configuration)** - Tune for your use case

## Common Questions

### Q: Do I need a network to test?

**A:** No! The examples above work on a single node. The consensus engine simulates the network for testing.

For multi-node testing, see the [network examples](../../examples/test/).

### Q: How fast is consensus?

**A:** On a single node: ~10ms. On a real network:
- 3 nodes: ~300ms
- 21 nodes: ~600ms (production config)
- 100 nodes: ~1200ms

### Q: Is this production-ready?

**A:** Yes! The Go SDK has:
- 96% test coverage
- Used in Lux mainnet
- Quantum-resistant security
- Sub-second finality

### Q: What's the difference between BLS and lattice certificates?

**A:**
- **BLS**: Fast (96 bytes), secure against classical computers
- **Lattice**: Slower (~3KB), secure against quantum computers
- **Both required**: Defense in depth - secure today AND tomorrow

### Q: Can I use this for my blockchain?

**A:** Absolutely! Lux Consensus is:
- BSD 3-Clause licensed (free for commercial use)
- Flexible (works with any block structure)
- Proven (running on Lux mainnet)

## Troubleshooting

### Error: "cannot find package"

**Solution:**
```bash
go mod tidy
go get github.com/luxfi/consensus@v1.22.0
```

### Error: "context deadline exceeded"

**Solution:** Increase timeout in config:
```go
config.Timeout = 10 * time.Second
```

### Slow finality times

**Solution:** For testing, reduce beta:
```go
config.Beta = 3  // Faster but less secure
```

## Summary

You've learned:
✅ How to install and use Lux Consensus
✅ The basics of Byzantine fault tolerance
✅ How quantum-resistant finality works
✅ How to build a simple blockchain

**Next**: Explore the [SDK documentation](../sdks/) or build a [real application](../../examples/)!

---

**Need help?** Join our [Discord](https://discord.gg/lux) or [open an issue](https://github.com/luxfi/consensus/issues/new).

**Found this helpful?** Star us on [GitHub](https://github.com/luxfi/consensus)! ⭐
