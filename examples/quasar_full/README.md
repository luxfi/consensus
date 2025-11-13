# Quasar Full Protocol Example

Complete demonstration of the Lux Quasar consensus protocol showing all components working together to achieve sub-second quantum-resistant finality.

## What is Quasar?

Quasar is Lux's unified consensus protocol that combines multiple cutting-edge consensus techniques:

1. **Fast Probabilistic Consensus (FPC)** - Prevents stuck states with dynamic thresholds
2. **Wave Protocol** - Metastable convergence through repeated sampling
3. **Horizon/Flare** - DAG-based finality detection
4. **Post-Quantum Signatures** - Ringtail lattice + BLS fusion

Together, these achieve **2-round quantum finality in <1 second**.

## Running the Example

```bash
cd examples/quasar_full
go run main.go
```

## Running the Tests

```bash
# From consensus root
cd ../..
go test ./examples/quasar_full/... -v

# With coverage
go test ./protocol/wave/fpc/... -v -cover
go test ./protocol/flare/... -v -cover
go test ./protocol/horizon/... -v -cover
```

## What The Example Demonstrates

### Part 1: FPC Dynamic Thresholds

Shows how FPC uses a PRF (SHA-256) to select different thresholds each round:
- Prevents consensus from getting stuck
- Maintains Byzantine fault tolerance
- Deterministic but varied selection

Example output:
```
Round  0: α=13/20 (θ=0.650) [█████████████░░░░░░░]
Round  1: α=11/20 (θ=0.550) [███████████░░░░░░░░░]
Round  2: α=15/20 (θ=0.750) [███████████████░░░░░]
```

### Part 2: Wave Consensus & Block Finalization

Demonstrates REAL block finalization:
- Block creation and addition to chain
- Validator vote collection  
- Confidence building through repeated sampling
- Final consensus decision

Shows actual timing: typically <100μs for block processing.

### Part 3: Horizon DAG Finality Algorithms

Tests all 7 critical Horizon algorithms:

1. **IsReachable** - DAG connectivity analysis
2. **LCA** - Lowest Common Ancestor for parallel blocks
3. **ComputeSafePrefix** - Find finalized vertices
4. **ChooseFrontier** - Byzantine-tolerant parent selection
5. **Antichain** - Identify concurrent vertices
6. **Horizon** - Advance event horizon boundary
7. **BeyondHorizon** - Check if vertex is immutably finalized

### Part 4: Complete Quasar Flow

Visualizes the complete 2-round protocol:

**Round 1 (~500ms): Classical BFT**
- FPC threshold selection
- Wave repeated sampling
- BLS signature aggregation

**Round 2 (~300ms): Quantum Finality**
- Horizon finality detection
- Flare certificate verification
- Ringtail post-quantum signatures
- Event horizon advancement

**Total: <1 second with quantum resistance**

## Test Coverage

The test suite includes:

- ✅ FPC threshold selection (determinism + variety)
- ✅ Block finalization (single to multi-validator)
- ✅ Horizon reachability (forward + backward)
- ✅ LCA computation
- ✅ Safe prefix detection
- ✅ Frontier selection with BFT
- ✅ Event horizon advancement
- ✅ Certificate threshold validation
- ✅ Full protocol integration
- ✅ Performance benchmarks

Run `go test -v -cover` for detailed coverage report.

## Key Concepts

### Byzantine Fault Tolerance

Quasar ensures safety with `f < n/3` Byzantine faults:
- n=20 validators tolerates f=6 faults
- Certificate needs ≥2f+1 = 13 validators
- Safety: Can't have both cert and skip (mutual exclusion)

### Metastable Consensus

Wave protocol achieves convergence through:
- Random sampling of k validators
- Check if ≥α prefer this block
- Repeat until β consecutive successes
- FPC varies α to prevent stuck states

### DAG Finality

Horizon/Flare detect finality in parallel DAG:
- **Certificate**: ≥2f+1 validators support vertex
- **Skip**: ≥2f+1 validators don't support vertex
- **Safe Prefix**: Vertices that are ancestors of all frontier
- **Event Horizon**: Immutability boundary (cannot be reverted)

### Quantum Resistance

Ringtail provides post-quantum security:
- Lattice-based cryptography (NTRU/Kyber family)
- Fused with BLS for 2-round finality
- Resistant to Shor's algorithm (quantum attacks)

## Architecture

```
Quasar Protocol
    │
    ├── Round 1: Classical BFT
    │   ├── FPC (threshold selection)
    │   ├── Wave (metastable convergence)
    │   └── BLS (signature aggregation)
    │
    └── Round 2: Quantum Finality
        ├── Horizon (reachability analysis)
        ├── Flare (certificate verification)
        └── Ringtail (PQ signatures)
```

## Performance

From benchmarks in this example:

- FPC threshold selection: ~100ns per round
- Block finalization: ~30μs  
- Horizon algorithms: ~1μs per operation
- Full protocol: <1 second total

## Safety Guarantees

✓ **Safety**: No two conflicting blocks can both finalize  
✓ **Liveness**: Finality in <1s (non-adversarial)  
✓ **Byzantine Tolerance**: Works with f < n/3 faults  
✓ **Quantum Resistance**: Secure against quantum computers  

## Learn More

- [Lux Consensus README](../../README.md)
- [Quasar Protocol Documentation](../../protocol/quasar/README.md)
- [FPC Implementation](../../protocol/wave/fpc/README.md)
- [DAG Algorithms](../../core/dag/README.md)

## Related Examples

- [Simple Consensus](../simple_consensus.go) - Basic chain consensus
- [01-Simple Bridge](../01-simple-bridge/) - Cross-chain integration
- [02-AI Payment](../02-ai-payment/) - AI-powered consensus

---

**Lux Consensus v1.22.0** - Quantum-resistant finality in <1 second
