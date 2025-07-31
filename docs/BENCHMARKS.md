# Consensus Benchmarks Guide

This document explains how to run and interpret the benchmark results for the Lux consensus implementation.

## Running Benchmarks

### Local Execution

```bash
# Run all benchmarks
make benchmark

# Run ZeroMQ transport benchmarks only
cd networking/zmq4
go test -bench=. -benchmem -benchtime=30s -run=^$

# Run consensus algorithm benchmarks only
go test -bench=. -benchmem -benchtime=30s -run=^$ ./benchmark/...

# Run specific benchmark
go test -bench=BenchmarkZMQTransportThroughput -benchmem -benchtime=30s -run=^$ ./networking/zmq4
```

### CI/CD Execution

Benchmarks run automatically in CI:
- **On every push to main**: Results are stored for historical comparison
- **On pull requests**: Results are commented on the PR
- **Nightly runs**: Scheduled at 2 AM UTC for performance tracking
- **Manual trigger**: Via GitHub Actions workflow dispatch

## Viewing Results

### GitHub Actions

1. **Pull Request Comments**: Benchmark results are automatically posted as comments
2. **Action Summary**: View results in the workflow run summary
3. **Artifacts**: Download detailed results from workflow artifacts
4. **Historical Data**: Compare with previous runs at `/actions/workflows/benchmark.yml`

### Result Format

```
BenchmarkZMQTransportThroughput-8    1000000    1053 ns/op    36 B/op    1 allocs/op
```

- **-8**: Number of CPU cores used
- **1000000**: Number of iterations
- **1053 ns/op**: Nanoseconds per operation
- **36 B/op**: Bytes allocated per operation
- **1 allocs/op**: Memory allocations per operation

## ZeroMQ Transport Benchmarks

### Throughput Test
Measures raw message sending speed:
- **Good**: < 1 microsecond per message
- **Acceptable**: < 10 microseconds per message
- **Poor**: > 10 microseconds per message

### Latency Test
Measures round-trip time:
- **Good**: < 50 microseconds RTT
- **Acceptable**: < 200 microseconds RTT
- **Poor**: > 200 microseconds RTT

### Concurrent Test
Tests performance under concurrent load:
- Measures scalability with multiple senders
- Should show near-linear scaling up to CPU count

### Large Message Test
Tests performance with various message sizes:
- 1KB, 10KB, 100KB, 1MB messages
- Throughput should scale with message size

### Multicast Test
Tests one-to-many message distribution:
- Performance with 5, 10, 20 receivers
- Should show minimal degradation with more receivers

## Consensus Algorithm Benchmarks

### Ballot Operations
- Vote recording and tallying performance
- Should handle 100k+ operations/second

### Message Verification
- Cryptographic verification speed
- Target: < 100 microseconds per verification

### State Transitions
- Consensus state machine performance
- Should complete in microseconds

## Performance Alerts

CI will alert if performance degrades by more than 50% compared to the baseline.

## Optimization Tips

1. **Profile First**: Use `go test -cpuprofile=cpu.prof` to identify bottlenecks
2. **Memory Usage**: Monitor allocations with `-memprofile=mem.prof`
3. **Concurrency**: Ensure proper goroutine usage with `go test -race`
4. **Message Batching**: Consider batching for throughput improvements

## Historical Performance

View performance trends:
- GitHub Actions benchmark history
- Performance regression alerts
- Comparison with previous releases

## Troubleshooting

### High Latency
- Check network configuration
- Verify no background processes
- Ensure proper CPU governor settings

### Memory Issues
- Look for allocation hotspots
- Check for memory leaks with extended runs
- Profile heap usage

### Inconsistent Results
- Increase benchmark time with `-benchtime=60s`
- Run multiple times and average
- Check system load during tests