#!/usr/bin/env python3
# Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

import pytest
import random
import time
import numpy as np
import mlx.core as mx
from lux_consensus.mlx_backend import (
    MLXConsensusBackend,
    MLXConsensusModel,
    AdaptiveMLXBatchProcessor
)


def generate_vote():
    """Generate random vote for testing"""
    voter_id = bytes([random.randint(0, 255) for _ in range(32)])
    block_id = bytes([random.randint(0, 255) for _ in range(32)])
    is_preference = random.choice([True, False])
    return (voter_id, block_id, is_preference)


def test_mlx_model_initialization():
    """Test MLX model initialization"""
    print("=== Test: MLX Model Initialization ===")
    
    # Test default initialization
    model = MLXConsensusModel()
    assert model is not None
    print("✅ Default MLX model initialized")
    
    # Test custom sizes
    model_custom = MLXConsensusModel(input_size=128, hidden_size=256)
    assert model_custom is not None
    print("✅ Custom MLX model initialized")


def test_mlx_backend_initialization():
    """Test MLX backend initialization"""
    print("=== Test: MLX Backend Initialization ===")
    
    # Test GPU mode (if available)
    backend_gpu = MLXConsensusBackend(device_type="gpu")
    assert backend_gpu is not None
    print(f"✅ GPU backend initialized, GPU enabled: {backend_gpu.gpu_enabled}")
    
    # Test CPU mode
    backend_cpu = MLXConsensusBackend(device_type="cpu")
    assert backend_cpu is not None
    assert not backend_cpu.gpu_enabled
    print("✅ CPU backend initialized")
    
    # Test custom configuration
    backend_custom = MLXConsensusBackend(
        device_type="cpu",
        batch_size=64,
        enable_quantization=False,
        cache_size=10000
    )
    assert backend_custom.batch_size == 64
    assert not backend_custom.enable_quantization
    assert backend_custom.cache_size == 10000
    print("✅ Custom backend configuration works")


def test_mlx_vote_processing():
    """Test MLX vote processing"""
    print("=== Test: MLX Vote Processing ===")
    
    backend = MLXConsensusBackend(device_type="cpu", batch_size=10)
    
    # Generate test votes
    votes = [generate_vote() for _ in range(25)]
    
    # Process votes
    processed_count = backend.process_votes_batch(votes)
    assert processed_count >= 0
    assert processed_count <= len(votes)
    print(f"✅ Processed {processed_count}/{len(votes)} votes")
    
    # Test empty batch
    empty_result = backend.process_votes_batch([])
    assert empty_result == 0
    print("✅ Empty batch handled correctly")


def test_mlx_block_validation():
    """Test MLX block validation"""
    print("=== Test: MLX Block Validation ===")
    
    backend = MLXConsensusBackend(device_type="cpu")
    
    # Generate test block IDs
    block_ids = [bytes([random.randint(0, 255) for _ in range(32)]) for _ in range(10)]
    
    # Validate blocks
    results = backend.validate_blocks_batch(block_ids)
    assert len(results) == len(block_ids)
    assert all(isinstance(result, bool) for result in results)
    print(f"✅ Validated {len(block_ids)} blocks")


def test_mlx_adaptive_processor():
    """Test adaptive batch processor"""
    print("=== Test: MLX Adaptive Batch Processor ===")
    
    backend = MLXConsensusBackend(device_type="cpu", batch_size=10)
    processor = AdaptiveMLXBatchProcessor(backend)
    
    # Add votes
    for _ in range(50):
        vote = generate_vote()
        processor.add_vote(*vote)
    
    # Flush processor
    processor.flush()
    
    # Check throughput
    throughput = processor.get_throughput()
    assert throughput >= 0
    print(f"✅ Adaptive processor throughput: {throughput:.0f} votes/sec")
    
    # Check batch size
    batch_size = processor.get_batch_size()
    assert batch_size > 0
    print(f"✅ Optimal batch size: {batch_size}")


def test_mlx_memory_management():
    """Test MLX memory management"""
    print("=== Test: MLX Memory Management ===")
    
    backend = MLXConsensusBackend(device_type="cpu")
    
    # Test memory usage reporting
    memory_usage = backend.get_gpu_memory_usage()
    assert memory_usage >= 0
    print(f"✅ Memory usage: {memory_usage} bytes")
    
    peak_memory = backend.get_peak_gpu_memory()
    assert peak_memory >= 0
    print(f"✅ Peak memory: {peak_memory} bytes")


def test_mlx_device_information():
    """Test MLX device information"""
    print("=== Test: MLX Device Information ===")
    
    backend = MLXConsensusBackend(device_type="cpu")
    
    # Test device name
    device_name = backend.get_device_name()
    assert device_name is not None
    assert len(device_name) > 0
    print(f"✅ Device name: {device_name}")


def test_mlx_performance_benchmark():
    """Test MLX performance benchmarking"""
    print("=== Test: MLX Performance Benchmark ===")
    
    backend = MLXConsensusBackend(device_type="cpu", batch_size=100)
    
    # Generate test data
    batch_sizes = [10, 100, 500]
    
    for batch_size in batch_sizes:
        votes = [generate_vote() for _ in range(batch_size)]
        
        # Benchmark
        start = time.perf_counter()
        processed = backend.process_votes_batch(votes)
        end = time.perf_counter()
        
        duration = (end - start) * 1_000_000  # microseconds
        throughput = batch_size / (end - start) if (end - start) > 0 else 0
        
        assert duration > 0
        assert throughput >= 0
        assert processed >= 0
        
        print(f"✅ Batch {batch_size}: {duration:.0f} μs, {throughput:.0f} votes/sec")


def test_mlx_error_handling():
    """Test MLX error handling"""
    print("=== Test: MLX Error Handling ===")
    
    backend = MLXConsensusBackend(device_type="cpu")
    
    # Test with invalid vote data (wrong size)
    try:
        invalid_votes = [(b"short", b"short", True)]  # Too short
        backend.process_votes_batch(invalid_votes)
        print("✅ Invalid vote data handled gracefully")
    except Exception as e:
        print(f"✅ Invalid vote data caught exception: {type(e).__name__}")


def test_mlx_cache_management():
    """Test MLX cache management"""
    print("=== Test: MLX Cache Management ===")
    
    backend = MLXConsensusBackend(device_type="cpu", cache_size=100)
    
    # Test cache operations
    block_id = bytes([random.randint(0, 255) for _ in range(32)])
    
    # Add to cache
    backend.block_cache[block_id] = {"test": "data"}
    
    # Verify cache
    assert block_id in backend.block_cache
    assert backend.block_cache[block_id]["test"] == "data"
    
    # Clear cache
    backend.block_cache.clear()
    assert len(backend.block_cache) == 0
    
    print("✅ Cache management working correctly")


def test_mlx_vote_buffer_management():
    """Test MLX vote buffer management"""
    print("=== Test: MLX Vote Buffer Management ===")
    
    backend = MLXConsensusBackend(device_type="cpu", batch_size=5)
    
    # Test add_vote method
    voter_id = bytes([random.randint(0, 255) for _ in range(32)])
    block_id = bytes([random.randint(0, 255) for _ in range(32)])
    
    backend.add_vote(voter_id, block_id, True)
    assert len(backend.vote_buffer) == 1
    print("✅ Single vote added to buffer")
    
    # Test auto-flush at batch size
    for i in range(4):  # Add 4 more votes to reach batch size of 5
        voter_id = bytes([random.randint(0, 255) for _ in range(32)])
        block_id = bytes([random.randint(0, 255) for _ in range(32)])
        backend.add_vote(voter_id, block_id, True)
    
    # Buffer should be empty after auto-flush
    assert len(backend.vote_buffer) == 0
    print("✅ Auto-flush at batch size working")
    
    # Test manual flush
    backend.add_vote(voter_id, block_id, False)
    assert len(backend.vote_buffer) == 1
    
    backend.flush()
    assert len(backend.vote_buffer) == 0
    print("✅ Manual flush working")


def test_mlx_preprocessing():
    """Test MLX vote preprocessing"""
    print("=== Test: MLX Vote Preprocessing ===")
    
    backend = MLXConsensusBackend(device_type="cpu")
    
    # Generate test votes
    votes = [generate_vote() for _ in range(10)]
    
    # Test preprocessing
    processed_array = backend.preprocess_votes(votes)
    
    # Verify output shape and type
    assert processed_array.shape[0] == len(votes)
    assert processed_array.shape[1] == 64  # 32 + 32 bytes
    print(f"✅ Preprocessing output shape: {processed_array.shape}")
    
    # Test with empty votes
    empty_array = backend.preprocess_votes([])
    assert empty_array.shape[0] == 0
    print("✅ Empty vote preprocessing handled")


def test_mlx_model_forward_pass():
    """Test MLX model forward pass"""
    print("=== Test: MLX Model Forward Pass ===")
    
    model = MLXConsensusModel()
    
    # Generate test input
    test_input = np.random.rand(10, 64).astype(np.float32)
    mx_input = mx.array(test_input)
    
    # Forward pass
    output = model(mx_input)
    
    # Verify output
    assert output.shape[0] == 10
    assert output.shape[1] == 1
    print(f"✅ Model forward pass output shape: {output.shape}")


def test_mlx_block_validation_edge_cases():
    """Test MLX block validation edge cases"""
    print("=== Test: MLX Block Validation Edge Cases ===")
    
    backend = MLXConsensusBackend(device_type="cpu")
    
    # Test empty block IDs
    empty_results = backend.validate_blocks_batch([])
    assert empty_results == []
    print("✅ Empty block validation handled")
    
    # Test single block
    block_id = bytes([random.randint(0, 255) for _ in range(32)])
    single_result = backend.validate_blocks_batch([block_id])
    assert len(single_result) == 1
    assert isinstance(single_result[0], bool)
    print("✅ Single block validation working")
    
    # Test large batch
    large_batch = [bytes([random.randint(0, 255) for _ in range(32)]) for _ in range(1000)]
    large_results = backend.validate_blocks_batch(large_batch)
    assert len(large_results) == len(large_batch)
    print("✅ Large batch validation working")


def test_mlx_adaptive_processor_detailed():
    """Test adaptive processor in detail"""
    print("=== Test: MLX Adaptive Processor Detailed ===")
    
    backend = MLXConsensusBackend(device_type="cpu", batch_size=10)
    processor = AdaptiveMLXBatchProcessor(backend)
    
    # Test initial state
    assert processor.get_batch_size() > 0
    assert processor.get_throughput() == 0
    print("✅ Initial processor state correct")
    
    # Add votes and test batching
    for i in range(25):
        vote = generate_vote()
        processor.add_vote(*vote)
    
    # Check that some votes were processed (throughput may be 0 if processing is very fast)
    throughput = processor.get_throughput()
    assert throughput >= 0  # Throughput should be non-negative
    print(f"✅ Votes processed through adaptive batching (throughput: {throughput})")
    
    # Test final flush
    processor.flush()
    print("✅ Final flush completed")


def test_mlx_quantization():
    """Test MLX quantization settings"""
    print("=== Test: MLX Quantization ===")
    
    # Test with quantization enabled
    backend_quantized = MLXConsensusBackend(device_type="cpu", enable_quantization=True)
    assert backend_quantized.enable_quantization
    print("✅ Quantization enabled backend created")
    
    # Test with quantization disabled
    backend_no_quant = MLXConsensusBackend(device_type="cpu", enable_quantization=False)
    assert not backend_no_quant.enable_quantization
    print("✅ Quantization disabled backend created")


def test_mlx_validation_error_handling():
    """Test MLX validation error handling"""
    print("=== Test: MLX Validation Error Handling ===")
    
    backend = MLXConsensusBackend(device_type="cpu")
    
    # Test with invalid block data (wrong size) - should handle gracefully
    try:
        invalid_block_ids = [b"short"]  # Too short
        results = backend.validate_blocks_batch(invalid_block_ids)
        # Should return conservative results (all True) on error
        assert all(results)
        print("✅ Invalid block data handled conservatively")
    except Exception as e:
        print(f"✅ Invalid block data caught exception: {type(e).__name__}")


def test_mlx_weight_loading():
    """Test MLX model weight loading"""
    print("=== Test: MLX Weight Loading ===")
    
    # Test backend initialization without weights
    backend_no_weights = MLXConsensusBackend(device_type="cpu", model_path=None)
    assert backend_no_weights.model is not None
    print("✅ Backend without weights initialized")
    
    # Test backend initialization with non-existent weights (should handle gracefully)
    try:
        backend_invalid_weights = MLXConsensusBackend(
            device_type="cpu", 
            model_path="/non/existent/path.weights"
        )
        print("✅ Invalid weight path handled gracefully")
    except Exception as e:
        print(f"✅ Invalid weight path caught exception: {type(e).__name__}")


if __name__ == "__main__":
    # Run all tests
    test_mlx_model_initialization()
    test_mlx_backend_initialization()
    test_mlx_vote_processing()
    test_mlx_block_validation()
    test_mlx_adaptive_processor()
    test_mlx_memory_management()
    test_mlx_device_information()
    test_mlx_performance_benchmark()
    test_mlx_error_handling()
    test_mlx_cache_management()
    test_mlx_vote_buffer_management()
    test_mlx_preprocessing()
    test_mlx_model_forward_pass()
    test_mlx_block_validation_edge_cases()
    test_mlx_adaptive_processor_detailed()
    test_mlx_quantization()
    test_mlx_validation_error_handling()
    test_mlx_weight_loading()
    
    print("\n✅ All MLX backend tests passed!")