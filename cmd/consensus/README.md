# Lux Consensus CLI

A unified command-line tool for working with Lux consensus parameters, benchmarking, and testing.

## Installation

```bash
cd consensus/cmd/consensus
go build -o consensus
```

## Commands

### Check Parameters
Validate and analyze consensus parameters:
```bash
consensus check --k 21 --alpha-preference 13 --alpha-confidence 18 --beta 8
```

### Simulate Consensus
Run consensus simulations with various network conditions:
```bash
consensus sim --nodes 100 --rounds 50 --byzantine 0.2 --latency 10
```

### Benchmark Performance
Run local benchmarks:
```bash
consensus benchmark --nodes 100 --rounds 1000 --parallel
```

### Distributed Benchmark
Run consensus benchmarks across multiple machines:

**On coordinator (first machine):**
```bash
# Binds to port 5555, expects 100 total validators
consensus bench :5555 100
```

**On worker machines:**
```bash
# Connect to coordinator, contribute validators based on CPU cores
consensus bench 192.168.1.10:5555 100
```

Features:
- Auto-detects CPU cores and runs multiple validators per core
- Generates validator keys automatically
- Creates benchmark genesis with all validators pre-staked
- Measures pure consensus performance without blockchain state
- Displays detailed performance metrics

### Parameter Management

Interactive configuration:
```bash
consensus params interactive
```

Tune for specific network:
```bash
consensus params tune --network-size 1000 --fault-tolerance 0.2
```

Generate preset configurations:
```bash
consensus params generate --preset mainnet --output mainnet-params.json
```

### ZeroMQ Testing

Run coordinator:
```bash
consensus zmq coordinator --bind tcp://*:5555 --workers 10
```

Run worker:
```bash
consensus zmq worker --connect tcp://coordinator:5555 --node-id 1
```

## Distributed Benchmarking

The `bench` command enables easy distributed consensus testing:

1. **Automatic Resource Detection**: Each machine detects its CPU cores and allocates validators accordingly
2. **Key Generation**: Each node generates ED25519 keys for its validators
3. **Genesis Creation**: Coordinator collects all validator keys and creates a benchmark genesis
4. **Pure Consensus Testing**: Tests consensus algorithm performance without blockchain overhead
5. **Performance Metrics**: Tracks rounds/second, messages/second, and finalization rate

### Example: 3-Machine Benchmark

Machine 1 (Coordinator, 16 cores):
```bash
consensus bench :5555 480  # 480 total validators expected
# Contributes 160 validators (16 cores × 10 validators/core)
```

Machine 2 (Worker, 16 cores):
```bash
consensus bench 10.0.0.1:5555 480
# Contributes 160 validators
```

Machine 3 (Worker, 16 cores):
```bash
consensus bench 10.0.0.1:5555 480
# Contributes 160 validators
```

Result: 480 validators running across 3 machines, achieving high throughput consensus.

## Configuration

Consensus parameters can be tuned for different network sizes:

- **Local Testing**: K=5, suitable for 5-50 nodes
- **Testnet**: K=11, suitable for 50-200 nodes  
- **Mainnet**: K=21, suitable for 200+ nodes

The tool automatically adjusts parameters based on network size during benchmarking.