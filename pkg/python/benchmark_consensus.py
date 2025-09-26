#!/usr/bin/env python3
# Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

import time
import statistics
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
from lux_consensus import (
    ConsensusEngine, ConsensusConfig, Block, Vote,
    EngineType, ConsensusError
)

class BenchmarkResults:
    def __init__(self, name):
        self.name = name
        self.times = []
        self.throughput = 0
        
    def add_time(self, elapsed):
        self.times.append(elapsed)
        
    def calculate_stats(self):
        if not self.times:
            return
        self.mean = statistics.mean(self.times)
        self.median = statistics.median(self.times)
        self.stdev = statistics.stdev(self.times) if len(self.times) > 1 else 0
        self.min = min(self.times)
        self.max = max(self.times)
        
    def print_results(self):
        print(f"\n{self.name}:")
        print(f"  Mean: {self.mean*1000:.3f}ms")
        print(f"  Median: {self.median*1000:.3f}ms")
        print(f"  Std Dev: {self.stdev*1000:.3f}ms")
        print(f"  Min: {self.min*1000:.3f}ms")
        print(f"  Max: {self.max*1000:.3f}ms")
        if self.throughput > 0:
            print(f"  Throughput: {self.throughput:.0f} ops/sec")

def benchmark_engine_creation():
    """Benchmark consensus engine creation"""
    print("Running engine creation benchmark...")
    results = BenchmarkResults("Engine Creation")
    
    config = ConsensusConfig(
        k=20,
        alpha_preference=15,
        alpha_confidence=15,
        beta=20,
        concurrent_polls=1,
        optimal_processing=1,
        max_outstanding_items=1024,
        max_item_processing_time_ns=2_000_000_000,
        engine_type=EngineType.DAG
    )
    
    # Warmup
    for _ in range(10):
        engine = ConsensusEngine(config)
        del engine
    
    # Benchmark
    iterations = 100
    for _ in range(iterations):
        start = time.perf_counter()
        engine = ConsensusEngine(config)
        elapsed = time.perf_counter() - start
        results.add_time(elapsed)
        del engine
    
    results.calculate_stats()
    results.throughput = iterations / sum(results.times)
    results.print_results()
    return results

def benchmark_block_operations():
    """Benchmark block operations"""
    print("\nRunning block operations benchmark...")
    
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
    
    results = {}
    
    # Single block addition
    print("  Single block addition...")
    engine = ConsensusEngine(config)
    single_results = BenchmarkResults("Single Block Add")
    
    for i in range(1000):
        block_id = bytes([(i >> 8) & 0xFF, i & 0xFF]) + b'\x00' * 30
        block = Block(
            block_id=block_id,
            parent_id=b'\x00' * 32,
            height=i,
            timestamp=int(time.time())
        )
        
        start = time.perf_counter()
        engine.add_block(block)
        elapsed = time.perf_counter() - start
        single_results.add_time(elapsed)
    
    single_results.calculate_stats()
    single_results.throughput = 1000 / sum(single_results.times)
    results['single'] = single_results
    
    # Batch block operations
    for batch_size in [100, 1000]:
        print(f"  Batch {batch_size} blocks...")
        batch_results = BenchmarkResults(f"Batch {batch_size} Blocks")
        
        for run in range(10):
            engine = ConsensusEngine(config)
            
            start = time.perf_counter()
            for i in range(batch_size):
                block_id = bytes([(i >> 16) & 0xFF, (i >> 8) & 0xFF, i & 0xFF]) + b'\x00' * 29
                block = Block(
                    block_id=block_id,
                    parent_id=b'\x00' * 32,
                    height=i,
                    timestamp=int(time.time())
                )
                engine.add_block(block)
            elapsed = time.perf_counter() - start
            batch_results.add_time(elapsed)
            del engine
        
        batch_results.calculate_stats()
        batch_results.throughput = batch_size / batch_results.mean
        results[f'batch_{batch_size}'] = batch_results
    
    # Print results
    for result in results.values():
        result.print_results()
    
    return results

def benchmark_vote_processing():
    """Benchmark vote processing"""
    print("\nRunning vote processing benchmark...")
    
    config = ConsensusConfig(
        k=20,
        alpha_preference=15,
        alpha_confidence=15,
        beta=20,
        concurrent_polls=1,
        optimal_processing=1,
        max_outstanding_items=1024,
        max_item_processing_time_ns=2_000_000_000,
        engine_type=EngineType.DAG
    )
    
    results = {}
    
    # Setup engine with blocks
    engine = ConsensusEngine(config)
    for i in range(100):
        block_id = bytes([i]) * 32
        block = Block(
            block_id=block_id,
            parent_id=b'\x00' * 32,
            height=i,
            timestamp=int(time.time())
        )
        engine.add_block(block)
    
    # Single vote processing
    print("  Single vote processing...")
    single_results = BenchmarkResults("Single Vote")
    
    for i in range(1000):
        voter_id = bytes([(i >> 8) & 0xFF, i & 0xFF]) + b'\x00' * 30
        vote = Vote(
            voter_id=voter_id,
            block_id=bytes([i % 100]) * 32,
            is_preference=(i % 2 == 0)
        )
        
        start = time.perf_counter()
        engine.process_vote(vote)
        elapsed = time.perf_counter() - start
        single_results.add_time(elapsed)
    
    single_results.calculate_stats()
    single_results.throughput = 1000 / sum(single_results.times)
    results['single'] = single_results
    
    # Batch vote processing
    for batch_size in [1000, 10000]:
        print(f"  Batch {batch_size} votes...")
        batch_results = BenchmarkResults(f"Batch {batch_size} Votes")
        
        for run in range(5):
            start = time.perf_counter()
            for i in range(batch_size):
                voter_id = bytes([(i >> 16) & 0xFF, (i >> 8) & 0xFF, i & 0xFF]) + b'\x00' * 29
                vote = Vote(
                    voter_id=voter_id,
                    block_id=bytes([i % 100]) * 32,
                    is_preference=(i % 2 == 0)
                )
                engine.process_vote(vote)
            elapsed = time.perf_counter() - start
            batch_results.add_time(elapsed)
        
        batch_results.calculate_stats()
        batch_results.throughput = batch_size / batch_results.mean
        results[f'batch_{batch_size}'] = batch_results
    
    # Print results
    for result in results.values():
        result.print_results()
    
    return results

def benchmark_query_operations():
    """Benchmark query operations"""
    print("\nRunning query operations benchmark...")
    
    config = ConsensusConfig(
        k=20,
        alpha_preference=15,
        alpha_confidence=15,
        beta=20,
        concurrent_polls=1,
        optimal_processing=1,
        max_outstanding_items=1024,
        max_item_processing_time_ns=2_000_000_000,
        engine_type=EngineType.DAG
    )
    
    engine = ConsensusEngine(config)
    
    # Add blocks
    block_ids = []
    for i in range(1000):
        block_id = bytes([(i >> 8) & 0xFF, i & 0xFF]) + b'\x00' * 30
        block_ids.append(block_id)
        block = Block(
            block_id=block_id,
            parent_id=b'\x00' * 32,
            height=i,
            timestamp=int(time.time())
        )
        engine.add_block(block)
    
    results = {}
    
    # is_accepted query
    print("  is_accepted queries...")
    accepted_results = BenchmarkResults("is_accepted")
    
    for i in range(1000):
        block_id = block_ids[i % 1000]
        start = time.perf_counter()
        is_accepted = engine.is_accepted(block_id)
        elapsed = time.perf_counter() - start
        accepted_results.add_time(elapsed)
    
    accepted_results.calculate_stats()
    accepted_results.throughput = 1000 / sum(accepted_results.times)
    results['is_accepted'] = accepted_results
    
    # get_preference query
    print("  get_preference queries...")
    pref_results = BenchmarkResults("get_preference")
    
    for _ in range(1000):
        start = time.perf_counter()
        pref = engine.get_preference()
        elapsed = time.perf_counter() - start
        pref_results.add_time(elapsed)
    
    pref_results.calculate_stats()
    pref_results.throughput = 1000 / sum(pref_results.times)
    results['get_preference'] = pref_results
    
    # get_stats query
    print("  get_stats queries...")
    stats_results = BenchmarkResults("get_stats")
    
    for _ in range(1000):
        start = time.perf_counter()
        stats = engine.get_stats()
        elapsed = time.perf_counter() - start
        stats_results.add_time(elapsed)
    
    stats_results.calculate_stats()
    stats_results.throughput = 1000 / sum(stats_results.times)
    results['get_stats'] = stats_results
    
    # Print results
    for result in results.values():
        result.print_results()
    
    return results

def benchmark_concurrent_operations():
    """Benchmark concurrent operations"""
    print("\nRunning concurrent operations benchmark...")
    
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
    
    results = {}
    
    for num_threads in [1, 2, 4, 8]:
        print(f"  Testing with {num_threads} threads...")
        thread_results = BenchmarkResults(f"{num_threads} Threads")
        
        for run in range(5):
            engine = ConsensusEngine(config)
            operations_per_thread = 1000
            
            def worker(thread_id):
                for i in range(operations_per_thread):
                    block_id = bytes([thread_id, (i >> 8) & 0xFF, i & 0xFF]) + b'\x00' * 29
                    block = Block(
                        block_id=block_id,
                        parent_id=b'\x00' * 32,
                        height=i,
                        timestamp=int(time.time())
                    )
                    engine.add_block(block)
            
            start = time.perf_counter()
            with ThreadPoolExecutor(max_workers=num_threads) as executor:
                futures = [executor.submit(worker, i) for i in range(num_threads)]
                for future in as_completed(futures):
                    future.result()
            elapsed = time.perf_counter() - start
            thread_results.add_time(elapsed)
            
            del engine
        
        thread_results.calculate_stats()
        thread_results.throughput = (num_threads * operations_per_thread) / thread_results.mean
        results[f'threads_{num_threads}'] = thread_results
    
    # Print results
    for result in results.values():
        result.print_results()
    
    return results

def benchmark_memory_usage():
    """Benchmark memory usage with large datasets"""
    print("\nRunning memory usage benchmark...")
    
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
    
    results = {}
    
    for size in [1000, 10000]:
        print(f"  Testing with {size} blocks with data...")
        size_results = BenchmarkResults(f"{size} Blocks with Data")
        
        for run in range(3):
            engine = ConsensusEngine(config)
            
            start = time.perf_counter()
            for i in range(size):
                block_id = bytes([(i >> 24) & 0xFF, (i >> 16) & 0xFF, 
                                  (i >> 8) & 0xFF, i & 0xFF]) + b'\x00' * 28
                block_data = f"Block data {i}".encode()
                block = Block(
                    block_id=block_id,
                    parent_id=b'\x00' * 32,
                    height=i,
                    timestamp=int(time.time()),
                    data=block_data
                )
                engine.add_block(block)
            elapsed = time.perf_counter() - start
            size_results.add_time(elapsed)
            
            del engine
        
        size_results.calculate_stats()
        size_results.throughput = size / size_results.mean
        results[f'size_{size}'] = size_results
    
    # Print results
    for result in results.values():
        result.print_results()
    
    return results

def main():
    print("=" * 60)
    print("Lux Consensus Python Benchmarks")
    print("=" * 60)
    
    all_results = {}
    
    # Run all benchmarks
    all_results['engine_creation'] = benchmark_engine_creation()
    all_results['block_operations'] = benchmark_block_operations()
    all_results['vote_processing'] = benchmark_vote_processing()
    all_results['query_operations'] = benchmark_query_operations()
    all_results['concurrent_operations'] = benchmark_concurrent_operations()
    all_results['memory_usage'] = benchmark_memory_usage()
    
    # Print summary
    print("\n" + "=" * 60)
    print("BENCHMARK SUMMARY")
    print("=" * 60)
    
    print("\nKey Performance Metrics:")
    print(f"  Engine Creation: {all_results['engine_creation'].mean*1000:.3f}ms average")
    
    if 'block_operations' in all_results:
        block_results = all_results['block_operations']
        if 'single' in block_results:
            print(f"  Single Block Add: {block_results['single'].throughput:.0f} blocks/sec")
        if 'batch_1000' in block_results:
            print(f"  Batch Block Add (1000): {block_results['batch_1000'].throughput:.0f} blocks/sec")
    
    if 'vote_processing' in all_results:
        vote_results = all_results['vote_processing']
        if 'single' in vote_results:
            print(f"  Single Vote Process: {vote_results['single'].throughput:.0f} votes/sec")
        if 'batch_10000' in vote_results:
            print(f"  Batch Vote Process (10000): {vote_results['batch_10000'].throughput:.0f} votes/sec")
    
    if 'query_operations' in all_results:
        query_results = all_results['query_operations']
        if 'is_accepted' in query_results:
            print(f"  Query (is_accepted): {query_results['is_accepted'].mean*1000000:.1f}μs average")
    
    print("\n✅ All benchmarks completed successfully!")

if __name__ == "__main__":
    main()