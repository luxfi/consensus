# Lux Node Consensus Engine

## Overview
The Lux Node now features a pluggable consensus engine architecture that supports multiple backend implementations:
- **Go**: Pure Go implementation (default)
- **C**: High-performance C implementation
- **C++**: Advanced C++ implementation with SIMD support
- **MLX**: Machine Learning accelerated implementation (Apple Silicon optimized)
- **Hybrid**: Combine multiple backends for optimal performance

## Quick Start

### 1. Using Default Go Backend
```bash
# Start node with default Go backend
./luxd

# Or explicitly specify
LUX_CONSENSUS_BACKEND=go ./luxd
```

### 2. Using MLX Backend (Apple Silicon)
```bash
# Create MLX configuration
./consensus-cli config create --backend mlx

# Start node with MLX backend
./luxd --consensus-config consensus_mlx.json
```

### 3. Using Hybrid Mode
```bash
# Use hybrid configuration for best performance
./luxd --consensus-config consensus/configs/hybrid_backend.json
```

## Configuration

### Environment Variables
```bash
# Set consensus backend
export LUX_CONSENSUS_BACKEND=mlx

# Set config path
export LUX_CONSENSUS_CONFIG=/path/to/consensus.json
```

### Configuration Files
Configs are checked in this order:
1. Command line: `--consensus-config`
2. Current directory: `./consensus.json`
3. User home: `~/.lux/consensus.json`
4. System: `/etc/lux/consensus.json`

### Example Configurations

#### Go Backend (Default)
```json
{
  "backend": "go",
  "go_config": {
    "max_goroutines": 100,
    "enable_profiling": false
  },
  "performance": {
    "cache_size": 1000,
    "batch_processing": true,
    "parallel_ops": 4
  }
}
```

#### MLX Backend (ML Accelerated)
```json
{
  "backend": "mlx",
  "mlx_config": {
    "model_path": "/models/consensus/mlx_model.bin",
    "device_type": "metal",
    "batch_size": 32,
    "enable_quantization": true
  }
}
```

#### Hybrid Mode (Optimal Performance)
```json
{
  "backend": "go",
  "hybrid_mode": {
    "primary": "go",
    "fallback": "c",
    "specializations": {
      "verify_signature": "c",
      "compute_merkle": "c",
      "validate_block": "mlx",
      "predict_consensus": "mlx"
    },
    "auto_switch": true,
    "load_threshold": 0.8
  }
}
```

## CLI Tool

### Install
```bash
cd cmd/consensus-cli
go install
```

### Commands

#### Show Configuration
```bash
consensus-cli config show
consensus-cli config show -c /path/to/config.json
```

#### Create Configuration
```bash
# Create Go backend config
consensus-cli config create --backend go

# Create MLX backend config
consensus-cli config create --backend mlx

# Create hybrid config
consensus-cli config create --backend hybrid
```

#### Test Backend
```bash
# Test Go backend
consensus-cli test --backend go

# Test MLX backend
consensus-cli test --backend mlx
```

#### Benchmark Backends
```bash
consensus-cli benchmark
```

#### Switch Backends (Demo)
```bash
consensus-cli switch go mlx
```

## Integration in Code

### Basic Usage
```go
import "github.com/luxfi/node/consensus"

// Create engine with config
config := consensus.DefaultConfig()
engine, err := consensus.NewConsensusEngine(ctx, config)

// Attach to context
ctx = engine.AttachToContext(ctx)

// Use in application
state, ok := consensus.GetConsensusState(ctx)
```

### Node Integration
```go
// In node initialization
func (n *Node) Initialize() error {
    // Initialize consensus
    consensusManager, err := NewConsensusManager(n.ctx, n.Log)
    if err != nil {
        return err
    }
    
    // Load config and initialize
    err = consensusManager.Initialize(n.Config.ConsensusConfigPath)
    if err != nil {
        return err
    }
    
    // Attach to context
    n.ctx = consensusManager.AttachToContext(n.ctx)
    
    return nil
}
```

### VM Integration
```go
// In VM initialization
func (vm *VM) Initialize(ctx context.Context) error {
    // Get consensus from context
    state, ok := consensus.GetConsensusState(ctx)
    if !ok {
        // Create default if not present
        state, _ = consensus.NewConsensusState(ctx, consensus.BackendGo)
        ctx = consensus.WithConsensusState(ctx, state)
    }
    
    vm.consensusState = state
    return nil
}
```

### Runtime Backend Switching
```go
// Switch backend at runtime
consensusManager.SwitchBackend(consensus.BackendMLX)

// Use specialized backend for operation
state := engine.SelectBackendForOperation("validate_block")
```

## Performance Considerations

### Backend Selection Guide

| Backend | Best For | Performance | Memory |
|---------|----------|-------------|---------|
| **Go** | General use, compatibility | Good | Moderate |
| **C** | High throughput, signatures | Excellent | Low |
| **C++** | SIMD operations, parallel | Excellent | Moderate |
| **MLX** | ML predictions, validation | Excellent* | High |
| **Hybrid** | Mixed workloads | Optimal | Variable |

*On Apple Silicon with Metal acceleration

### Optimization Tips

1. **Use Hybrid Mode** for production deployments
2. **Specialize operations** to appropriate backends:
   - Cryptographic operations → C/C++
   - ML predictions → MLX
   - General logic → Go
3. **Enable auto-switching** for dynamic optimization
4. **Monitor metrics** to tune performance

## Building Backend Libraries

### C Backend
```bash
cd consensus/c
make
sudo make install
```

### C++ Backend
```bash
cd consensus/cpp
cmake .
make
sudo make install
```

### MLX Backend
```bash
cd consensus/mlx
./build.sh
```

## Troubleshooting

### Backend Not Loading
```bash
# Check library path
ldd /usr/local/lib/luxconsensus.so

# Set library path
export LD_LIBRARY_PATH=/usr/local/lib:$LD_LIBRARY_PATH
```

### MLX Not Available
```bash
# Check Metal support (macOS)
system_profiler SPDisplaysDataType | grep Metal

# Install MLX dependencies
pip install mlx
```

### Performance Issues
```bash
# Enable debug mode
consensus-cli config create --backend go --debug

# Check metrics
consensus-cli test --backend go
```

## Development

### Adding New Backend

1. Implement `ConsensusState` interface
2. Add backend type to `backend.go`
3. Add configuration struct
4. Implement creation function
5. Register in engine

Example:
```go
type RustBackendConfig struct {
    WasmPath string `json:"wasm_path"`
}

func (e *ConsensusEngine) createRustBackend(ctx context.Context) (ConsensusState, error) {
    // Implementation
}
```

## Status

- ✅ **Go Backend**: Fully functional
- 🚧 **C Backend**: Interface ready, implementation pending
- 🚧 **C++ Backend**: Interface ready, implementation pending  
- 🚧 **MLX Backend**: Interface ready, implementation pending
- ✅ **Hybrid Mode**: Framework complete
- ✅ **Context Integration**: Working
- ✅ **CLI Tool**: Functional
- ✅ **Node Integration**: Ready

## Next Steps

1. Complete C/C++ backend implementations
2. Integrate MLX models for consensus prediction
3. Add performance benchmarks
4. Implement auto-tuning for hybrid mode
5. Add distributed consensus support