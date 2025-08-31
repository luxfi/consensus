# Lux Consensus Testing Guide

## Quick Start

```bash
# Build everything
make build

# Run quick test
./bin/consensus -action test

# Run simulation
./bin/sim -nodes 5 -rounds 10
```

## Manual Testing with cURL

### 1. Start the test server
```bash
./bin/server -port 9090 -network local &
```

### 2. Test endpoints

#### Health Check
```bash
curl http://localhost:9090/health
# Output: OK
```

#### Status Check
```bash
curl http://localhost:9090/status | jq
# Returns engine status and parameters
```

#### Run Consensus Test
```bash
# Simple test (GET)
curl http://localhost:9090/test | jq

# Custom test (POST)
curl -X POST http://localhost:9090/test \
  -H "Content-Type: application/json" \
  -d '{"rounds": 20, "nodes": 7}' | jq
```

#### Process Consensus Round
```bash
curl -X POST http://localhost:9090/consensus \
  -H "Content-Type: application/json" \
  -d '{
    "block_id": "test-block-001",
    "votes": {
      "node1": 1,
      "node2": 1,
      "node3": 1,
      "node4": 1,
      "node5": 0
    }
  }' | jq
```

## CLI Tools

### Consensus CLI
```bash
# Test different engines
./bin/consensus -engine chain -action test
./bin/consensus -engine dag -action test
./bin/consensus -engine pq -action test

# Check health
./bin/consensus -action health

# Get info
./bin/consensus -action info
```

### Simulator
```bash
# Basic simulation
./bin/sim -nodes 5 -rounds 10

# Advanced simulation
./bin/sim -nodes 21 -rounds 100 -failure 0.2 -latency 100ms -verbose

# Different networks
./bin/sim -network mainnet -nodes 21 -rounds 50
./bin/sim -network testnet -nodes 11 -rounds 30
./bin/sim -network local -nodes 5 -rounds 20
```

### Benchmarks
```bash
# Quick benchmark
./bin/bench -engine chain -blocks 1000

# Full benchmark
./bin/bench -engine all -duration 30s -parallel 8

# Verbose output
./bin/bench -engine pq -blocks 10000 -verbose
```

## E2E Testing

### Run all E2E tests
```bash
go test -v ./... -run TestE2E
```

### Specific E2E tests
```bash
# Test consensus engines
go test -v -run TestE2EConsensusEngine

# Test server endpoints (requires server running)
go test -v -run TestE2EConsensusServer

# Test performance
go test -v -run TestE2EPerformance

# Test simulation
go test -v -run TestE2ESimulation
```

## Unit Testing

### Run all tests
```bash
go test ./...
```

### Run with race detection
```bash
go test -race ./...
```

### Run with coverage
```bash
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run benchmarks
```bash
go test -bench=. ./...
go test -bench=. -benchtime=10s ./...
go test -bench=. -benchmem ./...
```

## Load Testing

### Simple load test
```bash
# 100 concurrent health checks
for i in {1..100}; do
  curl -s http://localhost:9090/health &
done
wait
```

### Stress test with Apache Bench (ab)
```bash
# Install ab if needed
# macOS: brew install httpd
# Linux: apt-get install apache2-utils

# 1000 requests, 10 concurrent
ab -n 1000 -c 10 http://localhost:9090/health

# POST requests
ab -n 100 -c 5 -p consensus.json -T application/json \
  http://localhost:9090/consensus
```

### Stress test with hey
```bash
# Install hey
go install github.com/rakyll/hey@latest

# Basic test
hey -n 1000 -c 10 http://localhost:9090/health

# POST with payload
hey -n 100 -c 5 -m POST \
  -H "Content-Type: application/json" \
  -d '{"block_id":"test","votes":{"n1":1,"n2":1,"n3":1}}' \
  http://localhost:9090/consensus
```

## Automated Test Script

Run the comprehensive test suite:
```bash
./test_consensus.sh
```

This script runs:
- Manual HTTP tests
- CLI tool tests
- E2E tests
- Load tests
- Unit tests
- Benchmarks

## CI/CD Testing

The GitHub Actions CI runs:
- Unit tests with race detection
- Cross-platform builds (Linux, macOS, Windows)
- Benchmarks
- Linting

Check CI status:
```bash
gh run list --repo luxfi/consensus --limit 5
gh run view <run-id> --repo luxfi/consensus
```

## Performance Requirements

Expected performance metrics:
- **Consensus finality**: < 100ms for local network
- **Throughput**: > 10,000 consensus rounds/sec
- **Concurrency**: Handle 1000+ concurrent requests
- **Memory**: < 100MB for basic operations

## Debugging

### Enable verbose logging
```bash
./bin/sim -verbose
./bin/bench -verbose
```

### Check server logs
```bash
# If running in background
tail -f consensus-server.log

# Or run in foreground
./bin/server -port 9090 -network local
```

### Profile performance
```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof
```

## Common Issues

### Port already in use
```bash
# Find process using port
lsof -i :9090
# Kill process
kill -9 <PID>
```

### Tests failing
```bash
# Check for race conditions
go test -race ./...

# Run with verbose output
go test -v ./...

# Run specific test
go test -v -run TestName ./...
```

### Benchmark variations
If benchmarks show high variation, increase duration:
```bash
go test -bench=. -benchtime=30s ./...
```