# Migration Guide: Sampler to Emitter

This guide helps developers migrate from the old `Sampler/Sample` pattern to the new `Emitter/Emit` pattern introduced in v2.0.0.

## Overview

The refactoring introduces a light/quantum theme throughout the consensus mechanism, replacing probabilistic sampling with photon emission metaphors.

## Quick Migration

### 1. Update Imports

```go
// Old
import "github.com/luxfi/consensus/prism"

// New
import "github.com/luxfi/consensus/photon"
```

### 2. Update Type Names

```go
// Old
var sampler prism.Sampler

// New
var emitter photon.Emitter[types.NodeID]
```

### 3. Update Method Calls

```go
// Old
nodes, err := sampler.Sample(ctx, k)

// New
nodes, err := emitter.Emit(ctx, k, seed)
```

### 4. Update Constructors

```go
// Old
sampler := prism.NewUniformSampler(peers)

// New
emitter := photon.NewUniformEmitter(peers, photon.DefaultEmitterOptions())
```

## Detailed Changes

### Interface Changes

#### Old Sampler Interface
```go
type Sampler interface {
    Sample(ctx context.Context, k int) ([]NodeID, error)
}
```

#### New Emitter Interface
```go
type Emitter[T comparable] interface {
    Emit(ctx context.Context, k int, seed uint64) ([]T, error)
    Report(node T, success bool) // New: performance feedback
}
```

### Key Differences

1. **Generic Type Support**: Emitter uses generics for flexibility
2. **Seed Parameter**: Explicit seed for deterministic selection
3. **Performance Tracking**: New `Report()` method for luminance updates
4. **Luminance System**: Nodes tracked by brightness (10-1000 lux)

### Luminance Tracking

The new system tracks node performance using light metaphors:

```go
// Report successful consensus participation
emitter.Report(nodeID, true)  // Increases node brightness

// Report failed participation
emitter.Report(nodeID, false) // Decreases node brightness
```

Brightness affects selection probability:
- 1000 lux: Maximum brightness (10x selection weight)
- 100 lux: Default brightness (1x selection weight)
- 10 lux: Minimum brightness (0.1x selection weight)

## Package Structure Changes

### Old Structure
```
prism/
├── sampler.go     # Sampling + DAG geometry
├── uniform.go     # Uniform sampling
└── weighted.go    # Weighted sampling
```

### New Structure
```
photon/            # K-of-N selection (NEW)
├── emitter.go     # Emitter interface
├── uniform.go     # Uniform emission
└── luminance.go   # Brightness tracking

prism/             # DAG geometry only
├── cut.go         # DAG cuts
├── frontier.go    # Frontier management
└── refraction.go  # Path analysis
```

## Configuration Updates

### Old Configuration
```go
type Config struct {
    SampleSize int
    Weighted   bool
}
```

### New Configuration
```go
type EmitterOptions struct {
    MinPeers int
    MaxPeers int
    Stake    func(NodeID) float64      // Stake weighting
    Latency  func(NodeID) time.Duration // Latency penalty
}

// Use defaults
opts := photon.DefaultEmitterOptions()
```

## Testing Updates

Update your tests to use the new interface:

```go
// Old test
func TestSampling(t *testing.T) {
    sampler := &mockSampler{}
    nodes, _ := sampler.Sample(ctx, 5)
    // assertions...
}

// New test
func TestEmission(t *testing.T) {
    emitter := &mockEmitter{}
    nodes, _ := emitter.Emit(ctx, 5, 12345)
    emitter.Report(nodes[0], true) // Track performance
    // assertions...
}
```

## Backward Compatibility

For gradual migration, you can create an adapter:

```go
type SamplerAdapter struct {
    emitter photon.Emitter[types.NodeID]
}

func (s *SamplerAdapter) Sample(ctx context.Context, k int) ([]types.NodeID, error) {
    return s.emitter.Emit(ctx, k, uint64(time.Now().UnixNano()))
}
```

## Performance Considerations

The new emitter system has minimal overhead:
- Emission: ~3μs per operation
- Luminance update: ~72ns per operation
- Zero allocations for brightness tracking

## Common Issues

### Issue: "undefined: prism.Sampler"
**Solution**: Update import to `photon.Emitter`

### Issue: "wrong number of arguments in call to Emit"
**Solution**: Add seed parameter: `Emit(ctx, k, seed)`

### Issue: "cannot use sampler (type *UniformSampler) as type Emitter"
**Solution**: Replace with `photon.NewUniformEmitter()`

## Support

For questions or issues with migration:
- Check [CHANGELOG.md](./CHANGELOG.md) for detailed changes
- Review [LLM.md](./LLM.md) for architecture details
- Open an issue on GitHub for specific problems

---

*Migration guide for Lux Consensus v2.0.0 - Photon/Emitter Refactoring*