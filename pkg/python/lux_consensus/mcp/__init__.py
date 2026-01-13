"""MCP mesh consensus for AI agents.

Reference: https://github.com/luxfi/consensus
"""

from .consensus import State, Result, Consensus, run
from .mcp_mesh import MCPMesh, MCPAgent, create_mesh, run_mcp_consensus

__all__ = [
    # Core
    "Consensus",
    "State",
    "Result",
    "run",
    # MCP Mesh
    "MCPMesh",
    "MCPAgent",
    "run_mcp_consensus",
    "create_mesh",
]
