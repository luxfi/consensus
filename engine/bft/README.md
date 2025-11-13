# BFT Consensus Engine

Thin wrapper around [`github.com/luxfi/bft`](https://github.com/luxfi/bft) (Simplex BFT) for integration with Lux consensus.

## About Simplex BFT

Simplex is a state-of-the-art Byzantine Fault Tolerant consensus protocol:
- **Peer-reviewed**: Published in TCC 2023
- **Simple**: No view change sub-protocol
- **Censorship-resistant**: Leader rotation instead of timeouts
- **MPL Licensed**: Maintained as external package

Paper: https://eprint.iacr.org/2023/463

## Architecture

```
Lux Consensus Package
    └── engine/bft/ (this package)
         └── Thin wrapper glue code
              └── github.com/luxfi/bft
                   └── Full Simplex BFT implementation
```

**Design**: Keep BFT as external MPL dependency, minimal glue code here.

## Usage

```go
import (
    "github.com/luxfi/consensus/engine/bft"
    luxbft "github.com/luxfi/bft"
)

// Create BFT engine with Simplex configuration
cfg := bft.Config{
    NodeID:      "node-1",
    Validators:  []string{"node-1", "node-2", "node-3", "node-4"},
    EpochLength: 100,
    EpochConfig: luxbft.EpochConfig{
        // Full Simplex configuration
        MaxProposalWait: 5 * time.Second,
        // ... see github.com/luxfi/bft for all options
    },
}

engine, err := bft.New(cfg)
if err != nil {
    panic(err)
}

// Access underlying Simplex for advanced features
simplex := engine.GetSimplex()
simplex.ProposeBlock(block)
```

## Direct Simplex Usage

For full control, use Simplex directly:

```go
import luxbft "github.com/luxfi/bft"

cfg := luxbft.EpochConfig{
    // Full Simplex configuration
}

epoch, err := luxbft.NewEpoch(cfg)
```

## Why Keep BFT External?

1. **License**: BFT is MPL (different from consensus package license)
2. **Maintenance**: BFT is actively maintained as separate project
3. **Size**: BFT is a complete BFT implementation (~87KB epoch.go alone)
4. **Separation**: Clean boundary between protocols

## Integration

The wrapper provides:
- `Start/Stop()` methods matching consensus engine interface
- `HealthCheck()` for monitoring
- `GetSimplex()` for direct access to full Simplex features

See `wrapper.go` for implementation (minimal glue code).

## Testing

```bash
# Test wrapper
go test ./engine/bft

# Test full Simplex
cd ~/work/lux/bft
go test .
```

## Learn More

- [Simplex BFT Repository](https://github.com/luxfi/bft)
- [Simplex Paper (TCC 2023)](https://eprint.iacr.org/2023/463)
- [Lux Consensus Overview](../../README.md)
