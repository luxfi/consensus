"""
Lux Consensus Python SDK

A clean, single-import interface to the Lux consensus system.
This is the main SDK surface for Python applications using Lux consensus.

Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
"""

from typing import List, Optional, Dict, Any
from dataclasses import dataclass
from datetime import datetime
from enum import IntEnum
import time

# Version info
__version__ = '1.0.0'
__all__ = [
    # Main classes
    'Chain', 'Config', 'Block', 'Vote', 'Certificate',
    # Enums
    'Status', 'Decision', 'VoteType',
    # Constants
    'GENESIS_ID',
    # Factory functions
    'default_config', 'new_chain', 'new_block', 'new_vote', 'quick_start',
    # Errors
    'ConsensusError', 'BlockNotFoundError', 'InvalidBlockError', 'NoQuorumError'
]


# ============= TYPES =============

class Status(IntEnum):
    """Block status"""
    UNKNOWN = 0
    PROCESSING = 1
    REJECTED = 2
    ACCEPTED = 3


class Decision(IntEnum):
    """Consensus decision outcome"""
    UNDECIDED = 0
    ACCEPT = 1
    REJECT = 2


class VoteType(IntEnum):
    """Vote type"""
    PREFERENCE = 0
    COMMIT = 1
    CANCEL = 2


@dataclass
class ID:
    """Identifier type (simplified for Python)"""
    value: bytes
    
    def __init__(self, *args):
        if len(args) == 1 and isinstance(args[0], bytes):
            self.value = args[0]
        else:
            # Convert list of ints to bytes
            self.value = bytes(args)
    
    def __str__(self):
        return self.value.hex()
    
    def __eq__(self, other):
        return isinstance(other, ID) and self.value == other.value
    
    def __hash__(self):
        return hash(self.value)


# Genesis block ID
GENESIS_ID = ID(b'\x00' * 32)


@dataclass
class Block:
    """Block in the blockchain"""
    id: ID
    parent_id: ID
    height: int
    payload: bytes
    time: Optional[datetime] = None
    
    def __post_init__(self):
        if self.time is None:
            self.time = datetime.now()


@dataclass
class Vote:
    """Vote on a block"""
    block_id: ID
    vote_type: VoteType
    voter: ID  # NodeID
    signature: bytes = b''


@dataclass
class Certificate:
    """Consensus certificate"""
    block_id: ID
    height: int
    votes: List[Vote]
    timestamp: datetime
    signatures: List[bytes]


@dataclass
class Config:
    """Consensus configuration"""
    # Consensus parameters
    alpha: int = 20  # Quorum size
    k: int = 20  # Sample size
    max_outstanding: int = 10  # Max outstanding polls
    max_poll_delay: float = 1.0  # Max delay between polls (seconds)
    
    # Network parameters
    network_timeout: float = 5.0  # Network timeout (seconds)
    max_message_size: int = 2 * 1024 * 1024  # 2MB
    
    # Security parameters
    security_level: int = 5  # NIST security level
    quantum_resistant: bool = True  # Use PQ crypto
    gpu_acceleration: bool = True  # Use GPU acceleration


# ============= ERRORS =============

class ConsensusError(Exception):
    """Base consensus error"""
    pass


class BlockNotFoundError(ConsensusError):
    """Block not found error"""
    pass


class InvalidBlockError(ConsensusError):
    """Invalid block error"""
    pass


class InvalidVoteError(ConsensusError):
    """Invalid vote error"""
    pass


class NoQuorumError(ConsensusError):
    """No quorum error"""
    pass


class AlreadyVotedError(ConsensusError):
    """Already voted error"""
    pass


class NotValidatorError(ConsensusError):
    """Not a validator error"""
    pass


class TimeoutError(ConsensusError):
    """Operation timeout error"""
    pass


class NotInitializedError(ConsensusError):
    """Engine not initialized error"""
    pass


# ============= ENGINE =============

class Engine:
    """Base consensus engine interface"""
    
    def add(self, block: Block) -> None:
        """Add a new block to consensus"""
        raise NotImplementedError
    
    def record_vote(self, vote: Vote) -> None:
        """Record a vote for a block"""
        raise NotImplementedError
    
    def is_accepted(self, block_id: ID) -> bool:
        """Check if a block has been accepted"""
        raise NotImplementedError
    
    def get_status(self, block_id: ID) -> Status:
        """Get the status of a block"""
        raise NotImplementedError
    
    def start(self) -> None:
        """Start the consensus engine"""
        raise NotImplementedError
    
    def stop(self) -> None:
        """Stop the consensus engine"""
        raise NotImplementedError


class Chain(Engine):
    """Linear blockchain consensus engine"""
    
    def __init__(self, config: Config):
        self.config = config
        self.blocks: Dict[ID, Block] = {}
        self.votes: Dict[ID, List[Vote]] = {}
        self.status: Dict[ID, Status] = {}
        self.last_accepted = GENESIS_ID
        self.height = 0
        self.validators: List[ID] = []
        self._started = False
    
    def add(self, block: Block) -> None:
        """Add a new block to the chain"""
        if not self._started:
            raise NotInitializedError("Engine not started")
        
        # Store the block
        self.blocks[block.id] = block
        self.status[block.id] = Status.PROCESSING
        
        # Initialize vote tracking
        if block.id not in self.votes:
            self.votes[block.id] = []
    
    def record_vote(self, vote: Vote) -> None:
        """Record a vote for a block"""
        if not self._started:
            raise NotInitializedError("Engine not started")
        
        # Check if block exists
        if vote.block_id not in self.blocks:
            raise BlockNotFoundError(f"Block {vote.block_id} not found")
        
        # Add vote
        self.votes[vote.block_id].append(vote)
        
        # Check if we have quorum
        if len(self.votes[vote.block_id]) >= self.config.alpha:
            self._accept_block(vote.block_id)
    
    def is_accepted(self, block_id: ID) -> bool:
        """Check if a block has been accepted"""
        return self.status.get(block_id) == Status.ACCEPTED
    
    def get_status(self, block_id: ID) -> Status:
        """Get the status of a block"""
        return self.status.get(block_id, Status.UNKNOWN)
    
    def start(self) -> None:
        """Start the consensus engine"""
        if self._started:
            return
        
        # Initialize genesis block
        genesis = Block(
            id=GENESIS_ID,
            parent_id=ID(b'\x00' * 32),
            height=0,
            payload=b'',
            time=datetime.now()
        )
        
        self.blocks[genesis.id] = genesis
        self.status[genesis.id] = Status.ACCEPTED
        self.last_accepted = genesis.id
        self._started = True
    
    def stop(self) -> None:
        """Stop the consensus engine"""
        self._started = False
    
    def _accept_block(self, block_id: ID) -> None:
        """Mark a block as accepted"""
        self.status[block_id] = Status.ACCEPTED
        
        block = self.blocks.get(block_id)
        if block and block.height > self.height:
            self.height = block.height
            self.last_accepted = block_id


# ============= FACTORY FUNCTIONS =============

def default_config() -> Config:
    """Returns the default consensus configuration"""
    return Config()


def new_chain(config: Optional[Config] = None) -> Chain:
    """Creates a new chain consensus engine"""
    if config is None:
        config = default_config()
    return Chain(config)


def new_block(id: ID, parent_id: ID, height: int, payload: bytes) -> Block:
    """Creates a new block with default values"""
    return Block(
        id=id,
        parent_id=parent_id,
        height=height,
        payload=payload
    )


def new_vote(block_id: ID, vote_type: VoteType, voter: ID) -> Vote:
    """Creates a new vote"""
    return Vote(
        block_id=block_id,
        vote_type=vote_type,
        voter=voter
    )


def quick_start() -> Chain:
    """Quick start a consensus engine with default config"""
    chain = new_chain()
    chain.start()
    return chain


# ============= CONVENIENCE =============

# Re-export common constants for convenience
VOTE_PREFERENCE = VoteType.PREFERENCE
VOTE_COMMIT = VoteType.COMMIT
VOTE_CANCEL = VoteType.CANCEL

STATUS_UNKNOWN = Status.UNKNOWN
STATUS_PROCESSING = Status.PROCESSING
STATUS_REJECTED = Status.REJECTED
STATUS_ACCEPTED = Status.ACCEPTED

DECIDE_UNDECIDED = Decision.UNDECIDED
DECIDE_ACCEPT = Decision.ACCEPT
DECIDE_REJECT = Decision.REJECT