# Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

# distutils: language = c
# cython: language_level = 3

from libc.stdint cimport uint8_t, uint32_t, uint64_t
from libc.stdlib cimport malloc, free
from libc.string cimport memcpy, memset
from cpython.bytes cimport PyBytes_AsString, PyBytes_Size
import time

# C API declarations
cdef extern from "lux_consensus.h":
    # Error codes
    ctypedef enum lux_error_t:
        LUX_SUCCESS = 0
        LUX_ERROR_INVALID_PARAMS = -1
        LUX_ERROR_OUT_OF_MEMORY = -2
        LUX_ERROR_INVALID_STATE = -3
        LUX_ERROR_CONSENSUS_FAILED = -4
        LUX_ERROR_NOT_IMPLEMENTED = -5
    
    # Engine types
    ctypedef enum lux_engine_type_t:
        LUX_ENGINE_CHAIN = 0
        LUX_ENGINE_DAG = 1
        LUX_ENGINE_PQ = 2
    
    # Configuration structure
    ctypedef struct lux_consensus_config_t:
        uint32_t k
        uint32_t alpha_preference
        uint32_t alpha_confidence
        uint32_t beta
        uint32_t concurrent_polls
        uint32_t optimal_processing
        uint32_t max_outstanding_items
        uint64_t max_item_processing_time_ns
        lux_engine_type_t engine_type
    
    # Block structure
    ctypedef struct lux_block_t:
        uint8_t id[32]
        uint8_t parent_id[32]
        uint64_t height
        uint64_t timestamp
        void* data
        size_t data_size
    
    # Vote structure
    ctypedef struct lux_vote_t:
        uint8_t voter_id[32]
        uint8_t block_id[32]
        bint is_preference
    
    # Statistics structure
    ctypedef struct lux_consensus_stats_t:
        uint64_t blocks_accepted
        uint64_t blocks_rejected
        uint64_t polls_completed
        uint64_t votes_processed
        double average_decision_time_ms
    
    # Opaque engine type
    ctypedef struct lux_consensus_engine_t:
        pass
    
    # API functions
    lux_error_t lux_consensus_init()
    lux_error_t lux_consensus_cleanup()
    
    lux_error_t lux_consensus_engine_create(
        lux_consensus_engine_t** engine,
        const lux_consensus_config_t* config
    )
    
    lux_error_t lux_consensus_engine_destroy(lux_consensus_engine_t* engine)
    
    lux_error_t lux_consensus_add_block(
        lux_consensus_engine_t* engine,
        const lux_block_t* block
    )
    
    lux_error_t lux_consensus_process_vote(
        lux_consensus_engine_t* engine,
        const lux_vote_t* vote
    )
    
    lux_error_t lux_consensus_is_accepted(
        lux_consensus_engine_t* engine,
        const uint8_t* block_id,
        bint* is_accepted
    )
    
    lux_error_t lux_consensus_get_preference(
        lux_consensus_engine_t* engine,
        uint8_t* block_id
    )
    
    lux_error_t lux_consensus_poll(
        lux_consensus_engine_t* engine,
        uint32_t num_validators,
        const uint8_t** validator_ids
    )
    
    lux_error_t lux_consensus_get_stats(
        lux_consensus_engine_t* engine,
        lux_consensus_stats_t* stats
    )
    
    const char* lux_error_string(lux_error_t error)
    const char* lux_engine_type_string(lux_engine_type_t type)

# Python exception for consensus errors
class ConsensusError(Exception):
    """Exception raised for consensus engine errors"""
    pass

# Engine type enum
class EngineType:
    CHAIN = LUX_ENGINE_CHAIN
    DAG = LUX_ENGINE_DAG
    PQ = LUX_ENGINE_PQ

# Python wrapper for consensus configuration
cdef class ConsensusConfig:
    """Configuration for consensus engine"""
    cdef lux_consensus_config_t config
    
    def __init__(self, 
                 k=20,
                 alpha_preference=15,
                 alpha_confidence=15,
                 beta=20,
                 concurrent_polls=1,
                 optimal_processing=1,
                 max_outstanding_items=1024,
                 max_item_processing_time_ns=2000000000,
                 engine_type=EngineType.DAG):
        self.config.k = k
        self.config.alpha_preference = alpha_preference
        self.config.alpha_confidence = alpha_confidence
        self.config.beta = beta
        self.config.concurrent_polls = concurrent_polls
        self.config.optimal_processing = optimal_processing
        self.config.max_outstanding_items = max_outstanding_items
        self.config.max_item_processing_time_ns = max_item_processing_time_ns
        self.config.engine_type = engine_type
    
    @property
    def k(self):
        return self.config.k
    
    @property
    def alpha_preference(self):
        return self.config.alpha_preference
    
    @property
    def alpha_confidence(self):
        return self.config.alpha_confidence
    
    @property
    def beta(self):
        return self.config.beta

# Python wrapper for block
cdef class Block:
    """Block in the consensus engine"""
    cdef lux_block_t block
    cdef bytes _data
    
    def __init__(self, block_id, parent_id, height, timestamp=None, data=None):
        if len(block_id) != 32:
            raise ValueError("block_id must be 32 bytes")
        if len(parent_id) != 32:
            raise ValueError("parent_id must be 32 bytes")
        
        memcpy(self.block.id, PyBytes_AsString(block_id), 32)
        memcpy(self.block.parent_id, PyBytes_AsString(parent_id), 32)
        self.block.height = height
        self.block.timestamp = timestamp if timestamp else int(time.time())
        
        if data:
            self._data = data
            self.block.data = PyBytes_AsString(self._data)
            self.block.data_size = PyBytes_Size(self._data)
        else:
            self.block.data = NULL
            self.block.data_size = 0
    
    @property
    def id(self):
        return bytes(self.block.id[:32])
    
    @property
    def parent_id(self):
        return bytes(self.block.parent_id[:32])
    
    @property
    def height(self):
        return self.block.height
    
    @property
    def timestamp(self):
        return self.block.timestamp

# Python wrapper for vote
cdef class Vote:
    """Vote in the consensus engine"""
    cdef lux_vote_t vote
    
    def __init__(self, voter_id, block_id, is_preference=False):
        if len(voter_id) != 32:
            raise ValueError("voter_id must be 32 bytes")
        if len(block_id) != 32:
            raise ValueError("block_id must be 32 bytes")
        
        memcpy(self.vote.voter_id, PyBytes_AsString(voter_id), 32)
        memcpy(self.vote.block_id, PyBytes_AsString(block_id), 32)
        self.vote.is_preference = is_preference
    
    @property
    def voter_id(self):
        return bytes(self.vote.voter_id[:32])
    
    @property
    def block_id(self):
        return bytes(self.vote.block_id[:32])
    
    @property
    def is_preference(self):
        return self.vote.is_preference

# Python wrapper for statistics
cdef class Stats:
    """Statistics from the consensus engine"""
    cdef lux_consensus_stats_t stats
    
    @property
    def blocks_accepted(self):
        return self.stats.blocks_accepted
    
    @property
    def blocks_rejected(self):
        return self.stats.blocks_rejected
    
    @property
    def polls_completed(self):
        return self.stats.polls_completed
    
    @property
    def votes_processed(self):
        return self.stats.votes_processed
    
    @property
    def average_decision_time_ms(self):
        return self.stats.average_decision_time_ms
    
    def __repr__(self):
        return (f"Stats(blocks_accepted={self.blocks_accepted}, "
                f"blocks_rejected={self.blocks_rejected}, "
                f"polls_completed={self.polls_completed}, "
                f"votes_processed={self.votes_processed}, "
                f"average_decision_time_ms={self.average_decision_time_ms:.2f})")

# Main consensus engine wrapper
cdef class ConsensusEngine:
    """Lux Consensus Engine"""
    cdef lux_consensus_engine_t* engine
    
    def __init__(self, ConsensusConfig config):
        cdef lux_error_t err
        
        # Initialize library
        err = lux_consensus_init()
        if err != LUX_SUCCESS:
            raise ConsensusError(f"Failed to initialize: {lux_error_string(err).decode()}")
        
        # Create engine
        err = lux_consensus_engine_create(&self.engine, &config.config)
        if err != LUX_SUCCESS:
            raise ConsensusError(f"Failed to create engine: {lux_error_string(err).decode()}")
    
    def __dealloc__(self):
        if self.engine != NULL:
            lux_consensus_engine_destroy(self.engine)
            lux_consensus_cleanup()
    
    def add_block(self, Block block):
        """Add a block to the consensus engine"""
        cdef lux_error_t err = lux_consensus_add_block(self.engine, &block.block)
        if err != LUX_SUCCESS:
            raise ConsensusError(f"Failed to add block: {lux_error_string(err).decode()}")
    
    def process_vote(self, Vote vote):
        """Process a vote"""
        cdef lux_error_t err = lux_consensus_process_vote(self.engine, &vote.vote)
        if err != LUX_SUCCESS:
            raise ConsensusError(f"Failed to process vote: {lux_error_string(err).decode()}")
    
    def is_accepted(self, block_id):
        """Check if a block is accepted"""
        if len(block_id) != 32:
            raise ValueError("block_id must be 32 bytes")
        
        cdef bint accepted = False
        cdef lux_error_t err = lux_consensus_is_accepted(
            self.engine, 
            <const uint8_t*>PyBytes_AsString(block_id), 
            &accepted
        )
        if err != LUX_SUCCESS:
            raise ConsensusError(f"Failed to check acceptance: {lux_error_string(err).decode()}")
        
        return accepted
    
    def get_preference(self):
        """Get the preferred block ID"""
        cdef uint8_t block_id[32]
        cdef lux_error_t err = lux_consensus_get_preference(self.engine, block_id)
        if err != LUX_SUCCESS:
            raise ConsensusError(f"Failed to get preference: {lux_error_string(err).decode()}")
        
        return bytes(block_id[:32])
    
    def poll(self, validator_ids):
        """Poll validators"""
        cdef uint32_t num_validators = len(validator_ids)
        cdef lux_error_t err
        
        # Allocate array of validator ID pointers (at least 1 for empty case)
        cdef uint32_t alloc_size = max(1, num_validators)
        cdef uint8_t** validator_ptrs = <uint8_t**>malloc(alloc_size * sizeof(uint8_t*))
        if validator_ptrs == NULL:
            raise MemoryError("Failed to allocate memory for validator IDs")
        
        try:
            # Copy validator IDs
            for i in range(num_validators):
                if len(validator_ids[i]) != 32:
                    raise ValueError(f"validator_id[{i}] must be 32 bytes")
                validator_ptrs[i] = <uint8_t*>PyBytes_AsString(validator_ids[i])
            
            # Call poll function
            err = lux_consensus_poll(
                self.engine,
                num_validators,
                <const uint8_t**>validator_ptrs
            )
            if err != LUX_SUCCESS:
                raise ConsensusError(f"Failed to poll: {lux_error_string(err).decode()}")
        finally:
            free(validator_ptrs)
    
    def get_stats(self):
        """Get consensus statistics"""
        cdef Stats stats = Stats()
        cdef lux_error_t err = lux_consensus_get_stats(self.engine, &stats.stats)
        if err != LUX_SUCCESS:
            raise ConsensusError(f"Failed to get stats: {lux_error_string(err).decode()}")
        
        return stats

# Module-level utility functions
def error_string(error_code):
    """Get error string for an error code"""
    return lux_error_string(error_code).decode()

def engine_type_string(engine_type):
    """Get engine type string"""
    return lux_engine_type_string(engine_type).decode()