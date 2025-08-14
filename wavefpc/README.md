# WaveFPC - Fast-Path Consensus for Lux

WaveFPC is a drop-in Fast-Path Consensus module that accelerates owned-object transactions with **zero extra messages**. Votes piggyback in existing blocks, no QCs needed, no protocol changes required.

## What It Does

- **Owned transactions**: Execute after 2f+1 votes (milliseconds)
- **Mixed transactions**: Wait for consensus anchor (normal path)
- **Dual finality**: Optional BLS + Ringtail PQ proofs for bridges
- **Zero overhead**: Votes ride in blocks you're already sending

## Integration (5 Lines of Code)

```go
// In your block builder:
func (e *Engine) BuildBlock() *Block {
    block := &Block{...}
    e.wavefpc.OnBuildBlock(block)  // ← Add FPC votes
    return block
}

// In your gossip handler:
func (e *Engine) OnGossipBlock(block *Block) {
    e.wavefpc.OnBlockReceived(block)  // ← Process votes
}

// In your consensus accept:
func (e *Engine) AcceptBlock(block *Block) {
    e.wavefpc.OnBlockAccepted(block)  // ← Anchor to final
}

// In your VM execution:
func (vm *VM) ExecuteTransaction(tx *Transaction) error {
    if vm.wavefpc.CanExecute(tx.Ref()) {  // ← Fast path?
        return vm.executeOwned(tx)
    }
    return vm.executeNormal(tx)
}
```

## Block Format Change (Minimal)

Add two optional fields to your block payload:

```go
type BlockPayload struct {
    // ...your existing fields...
    
    // WaveFPC additions:
    FPCVotes [][]byte  // Vote references (32 bytes each)
    EpochBit bool      // Epoch fence flag
}
```

## Configuration

```go
cfg := wavefpc.Config{
    N:                 100,      // Validator count
    F:                 33,       // Byzantine tolerance
    Epoch:             1,        // Current epoch
    VoteLimitPerBlock: 256,      // Max votes per block
}

// Create classifier for your object model
cls := &MyClassifier{} // Implements OwnedInputs() and Conflicts()

// Initialize
fpc := wavefpc.New(cfg, cls, dag, pq, nodeID, validators)
```

## Safety Guarantees

1. **No double execution**: Quorum intersection prevents conflicting executables
2. **One vote per object**: Validators can't equivocate on owned objects
3. **Epoch fence**: All fast-path finals included before validator rotation

## Performance

- **Latency**: 1 network RTT for owned transactions (vs 3-5 for consensus)
- **Throughput**: 100K+ owned TPS (no signatures per tx)
- **Memory**: O(active_txs) with automatic GC
- **CPU**: Bitset operations only, no crypto in hot path

## Rollout Plan

```go
// Phase 1: Ship disabled
fpc := wavefpc.NewIntegration(ctx, cfg, cls)

// Phase 2: Shadow mode (metrics only)
fpc.Enable()

// Phase 3: Enable voting
fpc.EnableVoting()

// Phase 4: Enable fast execution
fpc.EnableExecution()
```

## Architecture

```
┌─────────────────┐
│  Your Consensus │
│     Engine      │
└────────┬────────┘
         │ OnBlock*()
    ┌────▼────┐
    │ WaveFPC │ ← Zero protocol changes
    └────┬────┘
         │ Status()
    ┌────▼────┐
    │   VM    │ ← Execute at 2f+1 votes
    └─────────┘
```

## Testing

```bash
cd wavefpc
go test -v

# Benchmarks
go test -bench=. -benchmem

# Fuzz testing
go test -fuzz=FuzzConflicts
```

## Metrics

```go
metrics := fpc.GetMetrics()
// TotalVotes:      1,234,567
// ExecutableTxs:   456,789  
// FinalTxs:        456,123
// ConflictCount:   12
// VoteLatency:     1.2ms
// FinalityLatency: 45ms
```

## FAQ

**Q: What about shared objects?**
A: They go through normal consensus. WaveFPC only accelerates owned.

**Q: Can validators equivocate?**
A: They can try, but only first vote counts per object. No safety risk.

**Q: What if network partitions during epoch?**
A: EpochBit fence ensures all fast finals are included before rotation.

**Q: How much does this add to block size?**
A: ~8KB for 256 votes (32 bytes each). Negligible vs transactions.

**Q: Do I need to change my protocol?**
A: No. WaveFPC is purely additive. Old nodes ignore the new fields.

## License

Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.