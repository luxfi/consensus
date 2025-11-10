#!/usr/bin/env python3
# Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

"""
CPU-only benchmarks for Lux Consensus Python implementation.
Provides baseline performance metrics without GPU/MLX acceleration.
"""

import time
import statistics
import hashlib
import random
import json
from typing import List, Dict, Tuple, Optional
from dataclasses import dataclass
import timeit
import sys
import platform
import psutil

# Import the consensus modules
import sys
import os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

try:
    from lux_consensus import (
        ConsensusEngine, ConsensusConfig, Block, Vote,
        EngineType, ConsensusError
    )
except ImportError as e:
    print(f"Warning: Could not import lux_consensus: {e}")
    print("Attempting to import from current directory...")
    from lux_consensus import (
        ConsensusEngine, ConsensusConfig, Block, Vote,
        EngineType, ConsensusError
    )


@dataclass
class BenchmarkResult:
    """Store benchmark results with detailed metrics"""
    name: str
    count: int
    total_time: float
    mean: float
    median: float
    stdev: float
    min_time: float
    max_time: float
    throughput: float
    per_item_ns: float

    def __str__(self):
        return f"""
{self.name}:
  Items processed: {self.count:,}
  Total time: {self.total_time:.3f}s
  Mean: {self.mean*1000:.3f}ms
  Median: {self.median*1000:.3f}ms
  Std Dev: {self.stdev*1000:.3f}ms
  Min: {self.min_time*1000:.3f}ms
  Max: {self.max_time*1000:.3f}ms
  Throughput: {self.throughput:,.0f} ops/sec
  Per-item: {self.per_item_ns:.0f}ns
"""


class CPUBenchmark:
    """CPU-only benchmark suite for Lux Consensus"""

    def __init__(self, verbose: bool = True):
        self.verbose = verbose
        self.results = {}
        self.system_info = self._get_system_info()

    def _get_system_info(self) -> Dict[str, str]:
        """Gather system information"""
        return {
            'platform': platform.platform(),
            'processor': platform.processor(),
            'python': platform.python_version(),
            'cpu_count': psutil.cpu_count(logical=False),
            'cpu_threads': psutil.cpu_count(logical=True),
            'memory': f"{psutil.virtual_memory().total / (1024**3):.1f} GB"
        }

    def _print(self, msg: str):
        """Print if verbose mode"""
        if self.verbose:
            print(msg)

    def _measure_times(self, func, iterations: int) -> List[float]:
        """Measure execution times for a function"""
        times = []
        for _ in range(iterations):
            start = time.perf_counter()
            func()
            elapsed = time.perf_counter() - start
            times.append(elapsed)
        return times

    def _calculate_result(self, name: str, times: List[float], count: int) -> BenchmarkResult:
        """Calculate benchmark statistics"""
        total = sum(times)
        mean = statistics.mean(times)
        median = statistics.median(times)
        stdev = statistics.stdev(times) if len(times) > 1 else 0

        return BenchmarkResult(
            name=name,
            count=count,
            total_time=total,
            mean=mean,
            median=median,
            stdev=stdev,
            min_time=min(times),
            max_time=max(times),
            throughput=count / total if total > 0 else 0,
            per_item_ns=(total / count) * 1e9 if count > 0 else 0
        )

    def benchmark_vote_processing(self) -> Dict[str, BenchmarkResult]:
        """Benchmark vote processing at different scales"""
        self._print("\n🔵 Benchmarking Vote Processing (CPU-only)")
        self._print("-" * 50)

        results = {}

        # Setup engine
        config = ConsensusConfig(
            k=20,
            alpha_preference=15,
            alpha_confidence=15,
            beta=20,
            concurrent_polls=1,
            optimal_processing=1,
            max_outstanding_items=100000,
            max_item_processing_time_ns=2_000_000_000,
            engine_type=EngineType.DAG
        )
        engine = ConsensusEngine(config)

        # Add some blocks for voting
        for i in range(100):
            block = Block(
                block_id=bytes([i]) * 32,
                parent_id=b'\x00' * 32,
                height=i,
                timestamp=int(time.time())
            )
            engine.add_block(block)

        # Test different vote counts
        vote_counts = [1, 100, 1000, 10000]

        for count in vote_counts:
            self._print(f"\n  Testing {count:,} votes...")

            # Generate votes
            votes = []
            for i in range(count):
                vote = Vote(
                    voter_id=hashlib.sha256(f"voter_{i}".encode()).digest(),
                    block_id=bytes([i % 100]) * 32,
                    is_preference=(i % 2 == 0)
                )
                votes.append(vote)

            # Warm-up
            for vote in votes[:min(10, count)]:
                engine.process_vote(vote)

            # Benchmark single vote processing
            if count == 1:
                iterations = 1000
                times = []
                for _ in range(iterations):
                    vote = votes[0]
                    start = time.perf_counter()
                    engine.process_vote(vote)
                    elapsed = time.perf_counter() - start
                    times.append(elapsed)

                result = self._calculate_result(f"Single Vote", times, iterations)
            else:
                # Benchmark batch processing
                iterations = 10 if count >= 10000 else 100
                times = []

                for iteration in range(iterations):
                    # Reset engine for consistent state
                    engine = ConsensusEngine(config)
                    for i in range(100):
                        block = Block(
                            block_id=bytes([i]) * 32,
                            parent_id=b'\x00' * 32,
                            height=i,
                            timestamp=int(time.time())
                        )
                        engine.add_block(block)

                    # Process batch
                    start = time.perf_counter()
                    for vote in votes:
                        engine.process_vote(vote)
                    elapsed = time.perf_counter() - start
                    times.append(elapsed)

                result = self._calculate_result(f"{count:,} Votes", times, count * iterations)

            results[f"votes_{count}"] = result
            self._print(f"    Throughput: {result.throughput:,.0f} votes/sec")
            self._print(f"    Per-vote: {result.per_item_ns:.0f}ns")

        return results

    def benchmark_block_operations(self) -> Dict[str, BenchmarkResult]:
        """Benchmark block operations"""
        self._print("\n🔶 Benchmarking Block Operations (CPU-only)")
        self._print("-" * 50)

        results = {}

        config = ConsensusConfig(
            k=20,
            alpha_preference=15,
            alpha_confidence=15,
            beta=20,
            concurrent_polls=1,
            optimal_processing=1,
            max_outstanding_items=100000,
            max_item_processing_time_ns=2_000_000_000,
            engine_type=EngineType.DAG
        )

        # Test different block counts
        block_counts = [1, 100, 1000, 10000]

        for count in block_counts:
            self._print(f"\n  Testing {count:,} blocks...")

            # Generate blocks
            blocks = []
            for i in range(count):
                block = Block(
                    block_id=hashlib.sha256(f"block_{i}".encode()).digest(),
                    parent_id=hashlib.sha256(f"block_{i-1}".encode()).digest() if i > 0 else b'\x00' * 32,
                    height=i,
                    timestamp=int(time.time()) + i,
                    data=f"Block data {i}: {'x' * 100}".encode()
                )
                blocks.append(block)

            # Benchmark
            if count == 1:
                iterations = 1000
                times = []

                for _ in range(iterations):
                    engine = ConsensusEngine(config)
                    block = blocks[0]
                    start = time.perf_counter()
                    engine.add_block(block)
                    elapsed = time.perf_counter() - start
                    times.append(elapsed)

                result = self._calculate_result(f"Single Block", times, iterations)
            else:
                iterations = 10 if count >= 10000 else 100
                times = []

                for _ in range(iterations):
                    engine = ConsensusEngine(config)
                    start = time.perf_counter()
                    for block in blocks:
                        engine.add_block(block)
                    elapsed = time.perf_counter() - start
                    times.append(elapsed)

                result = self._calculate_result(f"{count:,} Blocks", times, count * iterations)

            results[f"blocks_{count}"] = result
            self._print(f"    Throughput: {result.throughput:,.0f} blocks/sec")
            self._print(f"    Per-block: {result.per_item_ns:.0f}ns")

        return results

    def benchmark_hash_operations(self) -> Dict[str, BenchmarkResult]:
        """Benchmark cryptographic hash operations"""
        self._print("\n🔐 Benchmarking Hash Operations (CPU-only)")
        self._print("-" * 50)

        results = {}
        data_sizes = [32, 256, 1024, 8192]

        for size in data_sizes:
            self._print(f"\n  Testing {size} byte hashing...")
            data = bytes(random.getrandbits(8) for _ in range(size))

            # SHA-256 benchmark
            iterations = 10000
            times = []

            for _ in range(iterations):
                start = time.perf_counter()
                hashlib.sha256(data).digest()
                elapsed = time.perf_counter() - start
                times.append(elapsed)

            result = self._calculate_result(f"SHA-256 {size}B", times, iterations)
            results[f"sha256_{size}"] = result
            self._print(f"    Throughput: {result.throughput:,.0f} hashes/sec")
            self._print(f"    Per-hash: {result.per_item_ns:.0f}ns")

        return results

    def benchmark_query_operations(self) -> Dict[str, BenchmarkResult]:
        """Benchmark consensus query operations"""
        self._print("\n🔍 Benchmarking Query Operations (CPU-only)")
        self._print("-" * 50)

        results = {}

        # Setup engine with blocks
        config = ConsensusConfig(
            k=20,
            alpha_preference=15,
            alpha_confidence=15,
            beta=20,
            concurrent_polls=1,
            optimal_processing=1,
            max_outstanding_items=10000,
            max_item_processing_time_ns=2_000_000_000,
            engine_type=EngineType.DAG
        )
        engine = ConsensusEngine(config)

        # Add test blocks
        block_ids = []
        for i in range(1000):
            block_id = hashlib.sha256(f"block_{i}".encode()).digest()
            block_ids.append(block_id)
            block = Block(
                block_id=block_id,
                parent_id=b'\x00' * 32,
                height=i,
                timestamp=int(time.time())
            )
            engine.add_block(block)

        # Benchmark is_accepted
        self._print("\n  Testing is_accepted queries...")
        iterations = 10000
        times = []

        for i in range(iterations):
            block_id = block_ids[i % len(block_ids)]
            start = time.perf_counter()
            engine.is_accepted(block_id)
            elapsed = time.perf_counter() - start
            times.append(elapsed)

        result = self._calculate_result("is_accepted", times, iterations)
        results["is_accepted"] = result
        self._print(f"    Throughput: {result.throughput:,.0f} queries/sec")
        self._print(f"    Per-query: {result.per_item_ns:.0f}ns")

        # Benchmark get_preference
        self._print("\n  Testing get_preference queries...")
        times = []

        for _ in range(iterations):
            start = time.perf_counter()
            engine.get_preference()
            elapsed = time.perf_counter() - start
            times.append(elapsed)

        result = self._calculate_result("get_preference", times, iterations)
        results["get_preference"] = result
        self._print(f"    Throughput: {result.throughput:,.0f} queries/sec")
        self._print(f"    Per-query: {result.per_item_ns:.0f}ns")

        # Benchmark get_stats
        self._print("\n  Testing get_stats queries...")
        times = []

        for _ in range(iterations):
            start = time.perf_counter()
            engine.get_stats()
            elapsed = time.perf_counter() - start
            times.append(elapsed)

        result = self._calculate_result("get_stats", times, iterations)
        results["get_stats"] = result
        self._print(f"    Throughput: {result.throughput:,.0f} queries/sec")
        self._print(f"    Per-query: {result.per_item_ns:.0f}ns")

        return results

    def benchmark_timeit_comparison(self) -> Dict[str, float]:
        """Use timeit module for precise microbenchmarks"""
        self._print("\n⏱️  Precision Microbenchmarks (timeit)")
        self._print("-" * 50)

        results = {}

        # Setup code
        setup = """
from lux_consensus import ConsensusEngine, ConsensusConfig, Block, Vote, EngineType
import hashlib
config = ConsensusConfig(
    k=20, alpha_preference=15, alpha_confidence=15, beta=20,
    concurrent_polls=1, optimal_processing=1,
    max_outstanding_items=1000, max_item_processing_time_ns=2_000_000_000,
    engine_type=EngineType.DAG
)
engine = ConsensusEngine(config)
block = Block(
    block_id=b'\\x01' * 32,
    parent_id=b'\\x00' * 32,
    height=0,
    timestamp=1234567890
)
vote = Vote(
    voter_id=b'\\x02' * 32,
    block_id=b'\\x01' * 32,
    is_preference=True
)
"""

        # Benchmark different operations
        operations = [
            ("Engine creation", "ConsensusEngine(config)", 100),
            ("Block addition", "engine.add_block(block)", 1000),
            ("Vote processing", "engine.process_vote(vote)", 1000),
            ("SHA-256 32B", "hashlib.sha256(b'x' * 32).digest()", 10000),
            ("SHA-256 1KB", "hashlib.sha256(b'x' * 1024).digest()", 10000),
        ]

        for name, stmt, number in operations:
            self._print(f"\n  {name}:")
            try:
                time_taken = timeit.timeit(stmt, setup=setup, number=number)
                per_op = (time_taken / number) * 1e9  # Convert to nanoseconds
                throughput = number / time_taken

                results[name] = {
                    'total_time': time_taken,
                    'per_op_ns': per_op,
                    'throughput': throughput
                }

                self._print(f"    Total: {time_taken:.6f}s for {number:,} operations")
                self._print(f"    Per-op: {per_op:,.0f}ns")
                self._print(f"    Throughput: {throughput:,.0f} ops/sec")
            except Exception as e:
                self._print(f"    Error: {e}")

        return results

    def compare_with_mlx(self):
        """Compare CPU results with MLX GPU benchmarks"""
        self._print("\n📊 CPU vs MLX GPU Comparison")
        self._print("-" * 50)

        # MLX GPU reference numbers (from mlx_backend.py example output)
        mlx_reference = {
            "10_votes": {"time_us": 100, "throughput": 100000},
            "100_votes": {"time_us": 200, "throughput": 500000},
            "1000_votes": {"time_us": 500, "throughput": 2000000},
            "10000_votes": {"time_us": 2000, "throughput": 5000000},
        }

        # Get our CPU results
        if 'votes_100' in self.results:
            cpu_100 = self.results['votes_100'].throughput
            mlx_100 = mlx_reference["100_votes"]["throughput"]
            speedup_100 = mlx_100 / cpu_100 if cpu_100 > 0 else 0
            self._print(f"\n  100 votes:")
            self._print(f"    CPU: {cpu_100:,.0f} votes/sec")
            self._print(f"    MLX: {mlx_100:,.0f} votes/sec")
            self._print(f"    Speedup: {speedup_100:.1f}x")

        if 'votes_1000' in self.results:
            cpu_1k = self.results['votes_1000'].throughput
            mlx_1k = mlx_reference["1000_votes"]["throughput"]
            speedup_1k = mlx_1k / cpu_1k if cpu_1k > 0 else 0
            self._print(f"\n  1,000 votes:")
            self._print(f"    CPU: {cpu_1k:,.0f} votes/sec")
            self._print(f"    MLX: {mlx_1k:,.0f} votes/sec")
            self._print(f"    Speedup: {speedup_1k:.1f}x")

        if 'votes_10000' in self.results:
            cpu_10k = self.results['votes_10000'].throughput
            mlx_10k = mlx_reference["10000_votes"]["throughput"]
            speedup_10k = mlx_10k / cpu_10k if cpu_10k > 0 else 0
            self._print(f"\n  10,000 votes:")
            self._print(f"    CPU: {cpu_10k:,.0f} votes/sec")
            self._print(f"    MLX: {mlx_10k:,.0f} votes/sec")
            self._print(f"    Speedup: {speedup_10k:.1f}x")

    def save_results(self, filename: str = "benchmark_cpu_results.json"):
        """Save benchmark results to JSON file"""
        output = {
            "system_info": self.system_info,
            "timestamp": time.time(),
            "results": {}
        }

        for key, result in self.results.items():
            if isinstance(result, BenchmarkResult):
                output["results"][key] = {
                    "name": result.name,
                    "count": result.count,
                    "total_time": result.total_time,
                    "mean": result.mean,
                    "median": result.median,
                    "stdev": result.stdev,
                    "min_time": result.min_time,
                    "max_time": result.max_time,
                    "throughput": result.throughput,
                    "per_item_ns": result.per_item_ns
                }

        with open(filename, 'w') as f:
            json.dump(output, f, indent=2)

        self._print(f"\n💾 Results saved to {filename}")

    def run_all(self):
        """Run all benchmarks"""
        print("=" * 60)
        print("🚀 LUX CONSENSUS CPU BENCHMARKS")
        print("=" * 60)

        # Print system info
        print("\n📱 System Information:")
        for key, value in self.system_info.items():
            print(f"  {key}: {value}")

        # Run benchmarks
        vote_results = self.benchmark_vote_processing()
        self.results.update(vote_results)

        block_results = self.benchmark_block_operations()
        self.results.update(block_results)

        hash_results = self.benchmark_hash_operations()
        self.results.update(hash_results)

        query_results = self.benchmark_query_operations()
        self.results.update(query_results)

        timeit_results = self.benchmark_timeit_comparison()

        # Print summary
        print("\n" + "=" * 60)
        print("📈 BENCHMARK SUMMARY")
        print("=" * 60)

        print("\n🎯 Key Performance Metrics (CPU-only):")

        if 'votes_1' in self.results:
            print(f"  Single Vote: {self.results['votes_1'].throughput:,.0f} votes/sec")
        if 'votes_100' in self.results:
            print(f"  100 Votes: {self.results['votes_100'].throughput:,.0f} votes/sec")
        if 'votes_1000' in self.results:
            print(f"  1K Votes: {self.results['votes_1000'].throughput:,.0f} votes/sec")
        if 'votes_10000' in self.results:
            print(f"  10K Votes: {self.results['votes_10000'].throughput:,.0f} votes/sec")

        print()

        if 'blocks_1' in self.results:
            print(f"  Single Block: {self.results['blocks_1'].throughput:,.0f} blocks/sec")
        if 'blocks_100' in self.results:
            print(f"  100 Blocks: {self.results['blocks_100'].throughput:,.0f} blocks/sec")
        if 'blocks_1000' in self.results:
            print(f"  1K Blocks: {self.results['blocks_1000'].throughput:,.0f} blocks/sec")
        if 'blocks_10000' in self.results:
            print(f"  10K Blocks: {self.results['blocks_10000'].throughput:,.0f} blocks/sec")

        print()

        if 'is_accepted' in self.results:
            print(f"  Query (is_accepted): {self.results['is_accepted'].per_item_ns:.0f}ns per query")
        if 'get_preference' in self.results:
            print(f"  Query (preference): {self.results['get_preference'].per_item_ns:.0f}ns per query")

        # Compare with MLX if available
        try:
            self.compare_with_mlx()
        except:
            pass

        # Save results
        self.save_results()

        print("\n✅ All CPU benchmarks completed successfully!")
        print("=" * 60)


def main():
    """Main entry point"""
    benchmark = CPUBenchmark(verbose=True)
    benchmark.run_all()


if __name__ == "__main__":
    main()