#!/usr/bin/env python3
"""
Lux Consensus MLX GPU Acceleration Example

Demonstrates Apple Silicon GPU acceleration using MLX framework.
"""

import random
import time
from lux_consensus.mlx_backend import (
    MLXConsensusBackend,
    AdaptiveMLXBatchProcessor
)


def generate_vote():
    """Generate random vote for testing"""
    voter_id = bytes([random.randint(0, 255) for _ in range(32)])
    block_id = bytes([random.randint(0, 255) for _ in range(32)])
    is_preference = random.choice([True, False])
    return (voter_id, block_id, is_preference)


def main():
    print("=== Lux Consensus MLX GPU Acceleration Demo ===\n")

    # Initialize backend
    backend = MLXConsensusBackend(
        device_type="gpu",
        batch_size=32,
        enable_quantization=True,
        cache_size=5000
    )

    print(f"Device: {backend.get_device_name()}")
    print(f"GPU Enabled: {backend.gpu_enabled}\n")

    # Performance benchmarks
    print("Performance Benchmarks:")
    print("=======================\n")

    batch_sizes = [10, 100, 1000, 10000]

    for batch_size in batch_sizes:
        # Generate test votes
        votes = [generate_vote() for _ in range(batch_size)]

        # Warm-up
        backend.process_votes_batch(votes)

        # Benchmark
        start = time.perf_counter()
        processed = backend.process_votes_batch(votes)
        end = time.perf_counter()

        duration = (end - start) * 1_000_000  # microseconds
        throughput = batch_size / (end - start) if (end - start) > 0 else 0

        print(f"Batch Size: {batch_size}")
        print(f"  Time: {duration:.0f} μs")
        print(f"  Throughput: {throughput:.0f} votes/sec")
        print(f"  Per-vote: {duration/batch_size:.0f} ns")
        print(f"  Processed: {processed}/{batch_size}\n")

    # Memory usage
    print("GPU Memory Usage:")
    print(f"  Active: {backend.get_gpu_memory_usage() / (1024*1024):.1f} MB")
    print(f"  Peak: {backend.get_peak_gpu_memory() / (1024*1024):.1f} MB\n")

    # Test block validation
    print("Testing Block Validation:")
    print("=========================\n")

    block_ids = [bytes([random.randint(0, 255) for _ in range(32)]) for _ in range(100)]

    start = time.perf_counter()
    results = backend.validate_blocks_batch(block_ids)
    end = time.perf_counter()

    duration = (end - start) * 1_000_000
    print(f"Validated 100 blocks in {duration:.0f} μs")
    print(f"Valid blocks: {sum(results)}/100\n")

    # Test adaptive batch processor
    print("Testing Adaptive Batch Processor:")
    print("==================================\n")

    processor = AdaptiveMLXBatchProcessor(backend)

    start = time.perf_counter()

    # Process 10,000 votes with adaptive batching
    for _ in range(10000):
        vote = generate_vote()
        processor.add_vote(*vote)

    processor.flush()
    end = time.perf_counter()

    duration = (end - start) * 1_000_000
    print(f"Total time: {duration:.0f} μs")
    print(f"Throughput: {processor.get_throughput():.0f} votes/sec")
    print(f"Optimal batch size: {processor.get_batch_size()}\n")

    # CPU vs GPU comparison
    print("CPU vs GPU Comparison:")
    print("======================\n")

    # CPU mode
    cpu_backend = MLXConsensusBackend(device_type="cpu", batch_size=100)
    votes = [generate_vote() for _ in range(1000)]

    start = time.perf_counter()
    cpu_backend.process_votes_batch(votes)
    end = time.perf_counter()
    cpu_time = (end - start) * 1_000_000

    # GPU mode
    gpu_backend = MLXConsensusBackend(device_type="gpu", batch_size=100)

    start = time.perf_counter()
    gpu_backend.process_votes_batch(votes)
    end = time.perf_counter()
    gpu_time = (end - start) * 1_000_000

    speedup = cpu_time / gpu_time if gpu_time > 0 else 0

    print(f"CPU Time: {cpu_time:.0f} μs")
    print(f"GPU Time: {gpu_time:.0f} μs")
    print(f"Speedup: {speedup:.1f}x\n")

    print("✅ MLX GPU acceleration working!")


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        print(f"Error: {e}")
        print("\nMake sure MLX is installed: pip3 install mlx")
        exit(1)
