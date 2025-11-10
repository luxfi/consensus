"""
Lux Consensus Python SDK

High-performance consensus implementation with optional MLX GPU acceleration.
"""

try:
    from .mlx_backend import (
        MLXConsensusBackend,
        MLXConsensusModel,
        AdaptiveMLXBatchProcessor
    )
    _MLX_AVAILABLE = True
except ImportError:
    _MLX_AVAILABLE = False
    MLXConsensusBackend = None
    MLXConsensusModel = None
    AdaptiveMLXBatchProcessor = None

__all__ = [
    "MLXConsensusBackend",
    "MLXConsensusModel",
    "AdaptiveMLXBatchProcessor",
]

__version__ = "1.21.0"
