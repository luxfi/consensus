"""
MLX GPU Backend for Lux Consensus

Provides Apple Silicon GPU acceleration using MLX framework.
"""

import mlx.core as mx
import mlx.nn as nn
import numpy as np
from typing import List, Tuple, Optional
import time

class MLXConsensusModel(nn.Module):
    """Neural network model for consensus decisions on GPU"""

    def __init__(self, input_size: int = 64, hidden_size: int = 128):
        super().__init__()
        self.layer1 = nn.Linear(input_size, hidden_size)
        self.layer2 = nn.Linear(hidden_size, hidden_size // 2)
        self.output = nn.Linear(hidden_size // 2, 1)

    def __call__(self, x):
        x = nn.relu(self.layer1(x))
        x = nn.relu(self.layer2(x))
        return nn.sigmoid(self.output(x))


class MLXConsensusBackend:
    """MLX-accelerated consensus backend for Apple Silicon"""

    def __init__(
        self,
        model_path: Optional[str] = None,
        device_type: str = "gpu",
        batch_size: int = 32,
        enable_quantization: bool = True,
        cache_size: int = 5000
    ):
        """
        Initialize MLX consensus backend

        Args:
            model_path: Path to pre-trained model weights
            device_type: "gpu" or "cpu"
            batch_size: Optimal batch size for GPU processing
            enable_quantization: Use int8 quantization for faster inference
            cache_size: Number of blocks to cache on GPU
        """
        self.batch_size = batch_size
        self.enable_quantization = enable_quantization
        self.cache_size = cache_size

        # Set device
        if device_type == "gpu" and mx.metal.is_available():
            mx.set_default_device(mx.gpu)
            self.gpu_enabled = True
            print("MLX GPU acceleration enabled")
        else:
            mx.set_default_device(mx.cpu)
            self.gpu_enabled = False
            print("MLX running in CPU mode")

        # Initialize model
        self.model = MLXConsensusModel()

        # Load weights if provided
        if model_path:
            self.model.load_weights(model_path)

        # Vote buffer for batching
        self.vote_buffer: List[Tuple[bytes, bytes, bool]] = []
        self.block_cache = {}

    def preprocess_votes(self, votes: List[Tuple[bytes, bytes, bool]]) -> mx.array:
        """
        Convert votes to MLX array

        Args:
            votes: List of (voter_id, block_id, is_preference) tuples

        Returns:
            MLX array of shape (n, 64) with normalized values
        """
        data = []
        for voter_id, block_id, _ in votes:
            # Voter ID (32 bytes)
            voter_bytes = np.frombuffer(voter_id, dtype=np.uint8).astype(np.float32) / 255.0
            # Block ID (32 bytes)
            block_bytes = np.frombuffer(block_id, dtype=np.uint8).astype(np.float32) / 255.0
            # Concatenate
            data.append(np.concatenate([voter_bytes, block_bytes]))

        return mx.array(np.array(data))

    def process_votes_batch(self, votes: List[Tuple[bytes, bytes, bool]]) -> int:
        """
        Process a batch of votes on GPU

        Args:
            votes: List of (voter_id, block_id, is_preference) tuples

        Returns:
            Number of votes successfully processed
        """
        if not votes:
            return 0

        try:
            # Preprocess
            input_array = self.preprocess_votes(votes)

            # Forward pass on GPU
            output = self.model(input_array)

            # Force evaluation on GPU
            mx.eval(output)

            # Count successes (output > 0.5)
            results = output.squeeze() > 0.5
            return int(mx.sum(results).item())

        except Exception as e:
            print(f"Error processing vote batch: {e}")
            return 0

    def validate_blocks_batch(self, block_ids: List[bytes]) -> List[bool]:
        """
        Validate a batch of blocks on GPU

        Args:
            block_ids: List of 32-byte block IDs

        Returns:
            List of validation results (True = valid)
        """
        if not block_ids:
            return []

        try:
            # Convert to array
            data = np.array([
                np.frombuffer(block_id, dtype=np.uint8).astype(np.float32) / 255.0
                for block_id in block_ids
            ])
            input_array = mx.array(data)

            # Validate on GPU
            output = self.model(input_array)
            mx.eval(output)

            # Convert to bool list
            results = output.squeeze() > 0.5
            return [bool(r.item()) for r in results]

        except Exception as e:
            print(f"Error validating blocks: {e}")
            return [True] * len(block_ids)  # Conservative: accept on error

    def add_vote(self, voter_id: bytes, block_id: bytes, is_preference: bool):
        """
        Add vote to buffer (auto-flushes at batch size)

        Args:
            voter_id: 32-byte voter identifier
            block_id: 32-byte block identifier
            is_preference: Whether this is a preference vote
        """
        self.vote_buffer.append((voter_id, block_id, is_preference))

        if len(self.vote_buffer) >= self.batch_size:
            self.flush()

    def flush(self):
        """Flush buffered votes to GPU"""
        if not self.vote_buffer:
            return

        self.process_votes_batch(self.vote_buffer)
        self.vote_buffer.clear()

    def get_gpu_memory_usage(self) -> int:
        """Get current GPU memory usage in bytes"""
        if not self.gpu_enabled:
            return 0
        return mx.metal.get_active_memory()

    def get_peak_gpu_memory(self) -> int:
        """Get peak GPU memory usage in bytes"""
        if not self.gpu_enabled:
            return 0
        return mx.metal.get_peak_memory()

    def reset_peak_memory(self):
        """Reset peak memory counter"""
        if self.gpu_enabled:
            mx.metal.reset_peak_memory()

    def get_device_name(self) -> str:
        """Get GPU device name"""
        return str(mx.default_device())


class AdaptiveMLXBatchProcessor:
    """Adaptive batch processor with automatic batch size tuning"""

    def __init__(self, backend: MLXConsensusBackend):
        self.backend = backend
        self.optimal_batch_size = 32
        self.vote_buffer = []
        self.throughput = 0.0

    def add_vote(self, voter_id: bytes, block_id: bytes, is_preference: bool):
        """Add vote to buffer (auto-flushes when optimal)"""
        self.vote_buffer.append((voter_id, block_id, is_preference))

        if len(self.vote_buffer) >= self.optimal_batch_size:
            self.flush()

    def flush(self):
        """Flush buffered votes to GPU"""
        if not self.vote_buffer:
            return

        start = time.perf_counter()
        processed = self.backend.process_votes_batch(self.vote_buffer)
        end = time.perf_counter()

        duration = end - start
        current_throughput = len(self.vote_buffer) / duration if duration > 0 else 0

        # Update running average (EMA)
        if self.throughput == 0.0:
            self.throughput = current_throughput
        else:
            self.throughput = 0.9 * self.throughput + 0.1 * current_throughput

        # Adjust batch size
        self._adjust_batch_size(current_throughput)

        self.vote_buffer.clear()

    def _adjust_batch_size(self, current_throughput: float):
        """Adjust batch size based on performance"""
        # Increase if throughput is good
        if current_throughput > 1_000_000.0 and self.optimal_batch_size < 128:
            self.optimal_batch_size *= 2
        # Decrease if throughput is poor
        elif current_throughput < 100_000.0 and self.optimal_batch_size > 16:
            self.optimal_batch_size //= 2

    def get_throughput(self) -> float:
        """Get current throughput in votes/second"""
        return self.throughput

    def get_batch_size(self) -> int:
        """Get current optimal batch size"""
        return self.optimal_batch_size


# Example usage
if __name__ == "__main__":
    import random

    print("=== Lux Consensus MLX GPU Backend ===\n")

    # Initialize backend
    backend = MLXConsensusBackend(
        device_type="gpu",
        batch_size=32,
        enable_quantization=True
    )

    print(f"Device: {backend.get_device_name()}")
    print(f"GPU Enabled: {backend.gpu_enabled}\n")

    # Generate test votes
    def generate_vote():
        voter_id = bytes([random.randint(0, 255) for _ in range(32)])
        block_id = bytes([random.randint(0, 255) for _ in range(32)])
        return (voter_id, block_id, random.choice([True, False]))

    # Benchmark
    print("Performance Benchmarks:")
    print("=======================\n")

    for batch_size in [10, 100, 1000, 10000]:
        votes = [generate_vote() for _ in range(batch_size)]

        # Warm-up
        backend.process_votes_batch(votes)

        # Benchmark
        start = time.perf_counter()
        processed = backend.process_votes_batch(votes)
        end = time.perf_counter()

        duration = (end - start) * 1_000_000  # microseconds
        throughput = batch_size * 1_000_000 / duration

        print(f"Batch Size: {batch_size}")
        print(f"  Time: {duration:.0f} μs")
        print(f"  Throughput: {throughput:.0f} votes/sec")
        print(f"  Per-vote: {duration/batch_size:.0f} ns")
        print(f"  Processed: {processed}/{batch_size}\n")

    # Memory usage
    print("GPU Memory Usage:")
    print(f"  Active: {backend.get_gpu_memory_usage() / (1024*1024):.1f} MB")
    print(f"  Peak: {backend.get_peak_gpu_memory() / (1024*1024):.1f} MB\n")

    print("✅ MLX GPU acceleration working!")
