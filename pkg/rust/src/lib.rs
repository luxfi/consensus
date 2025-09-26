// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

use libc::{c_char, c_void, size_t};
use std::ffi::CStr;
use std::fmt;
use std::ptr;

// FFI bindings to C library
#[repr(C)]
pub struct LuxConsensusEngine {
    _private: [u8; 0],
}

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub enum LuxError {
    Success = 0,
    InvalidParams = -1,
    OutOfMemory = -2,
    InvalidState = -3,
    ConsensusFailed = -4,
    NotImplemented = -5,
}

impl fmt::Display for LuxError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            LuxError::Success => write!(f, "Success"),
            LuxError::InvalidParams => write!(f, "Invalid parameters"),
            LuxError::OutOfMemory => write!(f, "Out of memory"),
            LuxError::InvalidState => write!(f, "Invalid state"),
            LuxError::ConsensusFailed => write!(f, "Consensus failed"),
            LuxError::NotImplemented => write!(f, "Not implemented"),
        }
    }
}

impl std::error::Error for LuxError {}

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub enum LuxEngineType {
    Chain = 0,
    DAG = 1,
    PQ = 2,
}

#[repr(C)]
#[derive(Debug, Clone)]
pub struct LuxConsensusConfig {
    pub k: u32,
    pub alpha_preference: u32,
    pub alpha_confidence: u32,
    pub beta: u32,
    pub concurrent_polls: u32,
    pub optimal_processing: u32,
    pub max_outstanding_items: u32,
    pub max_item_processing_time_ns: u64,
    pub engine_type: LuxEngineType,
}

#[repr(C)]
#[derive(Debug)]
pub struct LuxBlock {
    pub id: [u8; 32],
    pub parent_id: [u8; 32],
    pub height: u64,
    pub timestamp: u64,
    pub data: *mut c_void,
    pub data_size: size_t,
}

#[repr(C)]
#[derive(Debug)]
pub struct LuxVote {
    pub voter_id: [u8; 32],
    pub block_id: [u8; 32],
    pub is_preference: bool,
}

#[repr(C)]
#[derive(Debug, Default)]
pub struct LuxConsensusStats {
    pub blocks_accepted: u64,
    pub blocks_rejected: u64,
    pub polls_completed: u64,
    pub votes_processed: u64,
    pub average_decision_time_ms: f64,
}

// External C functions
extern "C" {
    fn lux_consensus_init() -> LuxError;
    fn lux_consensus_cleanup() -> LuxError;
    
    fn lux_consensus_engine_create(
        engine: *mut *mut LuxConsensusEngine,
        config: *const LuxConsensusConfig,
    ) -> LuxError;
    
    fn lux_consensus_engine_destroy(engine: *mut LuxConsensusEngine) -> LuxError;
    
    fn lux_consensus_add_block(
        engine: *mut LuxConsensusEngine,
        block: *const LuxBlock,
    ) -> LuxError;
    
    fn lux_consensus_process_vote(
        engine: *mut LuxConsensusEngine,
        vote: *const LuxVote,
    ) -> LuxError;
    
    fn lux_consensus_is_accepted(
        engine: *mut LuxConsensusEngine,
        block_id: *const u8,
        is_accepted: *mut bool,
    ) -> LuxError;
    
    fn lux_consensus_get_preference(
        engine: *mut LuxConsensusEngine,
        block_id: *mut u8,
    ) -> LuxError;
    
    fn lux_consensus_poll(
        engine: *mut LuxConsensusEngine,
        num_validators: u32,
        validator_ids: *const *const u8,
    ) -> LuxError;
    
    fn lux_consensus_get_stats(
        engine: *mut LuxConsensusEngine,
        stats: *mut LuxConsensusStats,
    ) -> LuxError;
    
    fn lux_error_string(error: LuxError) -> *const c_char;
    fn lux_engine_type_string(engine_type: LuxEngineType) -> *const c_char;
}

// High-level Rust API
pub struct ConsensusEngine {
    engine: *mut LuxConsensusEngine,
}

impl ConsensusEngine {
    /// Initialize the consensus library
    pub fn init() -> Result<(), LuxError> {
        let result = unsafe { lux_consensus_init() };
        match result {
            LuxError::Success => Ok(()),
            err => Err(err),
        }
    }
    
    /// Cleanup the consensus library
    pub fn cleanup() -> Result<(), LuxError> {
        let result = unsafe { lux_consensus_cleanup() };
        match result {
            LuxError::Success => Ok(()),
            err => Err(err),
        }
    }
    
    /// Create a new consensus engine
    pub fn new(config: &LuxConsensusConfig) -> Result<Self, LuxError> {
        let mut engine: *mut LuxConsensusEngine = ptr::null_mut();
        let result = unsafe { lux_consensus_engine_create(&mut engine, config) };
        
        match result {
            LuxError::Success => Ok(ConsensusEngine { engine }),
            err => Err(err),
        }
    }
    
    /// Add a block to the consensus engine
    pub fn add_block(&mut self, block: &LuxBlock) -> Result<(), LuxError> {
        let result = unsafe { lux_consensus_add_block(self.engine, block) };
        match result {
            LuxError::Success => Ok(()),
            err => Err(err),
        }
    }
    
    /// Process a vote
    pub fn process_vote(&mut self, vote: &LuxVote) -> Result<(), LuxError> {
        let result = unsafe { lux_consensus_process_vote(self.engine, vote) };
        match result {
            LuxError::Success => Ok(()),
            err => Err(err),
        }
    }
    
    /// Check if a block is accepted
    pub fn is_accepted(&self, block_id: &[u8; 32]) -> Result<bool, LuxError> {
        let mut accepted = false;
        let result = unsafe {
            lux_consensus_is_accepted(self.engine, block_id.as_ptr(), &mut accepted)
        };
        
        match result {
            LuxError::Success => Ok(accepted),
            err => Err(err),
        }
    }
    
    /// Get the preferred block ID
    pub fn get_preference(&self) -> Result<[u8; 32], LuxError> {
        let mut block_id = [0u8; 32];
        let result = unsafe {
            lux_consensus_get_preference(self.engine, block_id.as_mut_ptr())
        };
        
        match result {
            LuxError::Success => Ok(block_id),
            err => Err(err),
        }
    }
    
    /// Poll validators
    pub fn poll(&mut self, validator_ids: &[[u8; 32]]) -> Result<(), LuxError> {
        let validator_ptrs: Vec<*const u8> = validator_ids
            .iter()
            .map(|id| id.as_ptr())
            .collect();
        
        let result = unsafe {
            lux_consensus_poll(
                self.engine,
                validator_ids.len() as u32,
                validator_ptrs.as_ptr(),
            )
        };
        
        match result {
            LuxError::Success => Ok(()),
            err => Err(err),
        }
    }
    
    /// Get consensus statistics
    pub fn get_stats(&self) -> Result<LuxConsensusStats, LuxError> {
        let mut stats = LuxConsensusStats::default();
        let result = unsafe {
            lux_consensus_get_stats(self.engine, &mut stats)
        };
        
        match result {
            LuxError::Success => Ok(stats),
            err => Err(err),
        }
    }
    
    /// Get error string for an error code
    pub fn error_string(error: LuxError) -> String {
        unsafe {
            let c_str = lux_error_string(error);
            CStr::from_ptr(c_str).to_string_lossy().into_owned()
        }
    }
    
    /// Get engine type string
    pub fn engine_type_string(engine_type: LuxEngineType) -> String {
        unsafe {
            let c_str = lux_engine_type_string(engine_type);
            CStr::from_ptr(c_str).to_string_lossy().into_owned()
        }
    }
}

impl Drop for ConsensusEngine {
    fn drop(&mut self) {
        unsafe {
            lux_consensus_engine_destroy(self.engine);
        }
    }
}

// Safety: ConsensusEngine can be sent between threads
unsafe impl Send for ConsensusEngine {}
unsafe impl Sync for ConsensusEngine {}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_initialization() {
        assert!(ConsensusEngine::init().is_ok());
        assert!(ConsensusEngine::cleanup().is_ok());
    }
    
    #[test]
    fn test_engine_creation() {
        ConsensusEngine::init().unwrap();
        
        let config = LuxConsensusConfig {
            k: 20,
            alpha_preference: 15,
            alpha_confidence: 15,
            beta: 20,
            concurrent_polls: 1,
            optimal_processing: 1,
            max_outstanding_items: 1024,
            max_item_processing_time_ns: 2000000000,
            engine_type: LuxEngineType::DAG,
        };
        
        let engine = ConsensusEngine::new(&config);
        assert!(engine.is_ok());
        
        ConsensusEngine::cleanup().unwrap();
    }
    
    #[test]
    fn test_block_operations() {
        ConsensusEngine::init().unwrap();
        
        let config = LuxConsensusConfig {
            k: 20,
            alpha_preference: 15,
            alpha_confidence: 15,
            beta: 20,
            concurrent_polls: 1,
            optimal_processing: 1,
            max_outstanding_items: 1024,
            max_item_processing_time_ns: 2000000000,
            engine_type: LuxEngineType::DAG,
        };
        
        let mut engine = ConsensusEngine::new(&config).unwrap();
        
        let block = LuxBlock {
            id: [1; 32],
            parent_id: [0; 32],
            height: 1,
            timestamp: 1234567890,
            data: ptr::null_mut(),
            data_size: 0,
        };
        
        assert!(engine.add_block(&block).is_ok());
        
        let is_accepted = engine.is_accepted(&block.id).unwrap();
        assert!(!is_accepted);
        
        ConsensusEngine::cleanup().unwrap();
    }
    
    #[test]
    fn test_voting() {
        ConsensusEngine::init().unwrap();
        
        let config = LuxConsensusConfig {
            k: 20,
            alpha_preference: 2,
            alpha_confidence: 2,
            beta: 3,
            concurrent_polls: 1,
            optimal_processing: 1,
            max_outstanding_items: 1024,
            max_item_processing_time_ns: 2000000000,
            engine_type: LuxEngineType::DAG,
        };
        
        let mut engine = ConsensusEngine::new(&config).unwrap();
        
        let block = LuxBlock {
            id: [2; 32],
            parent_id: [0; 32],
            height: 1,
            timestamp: 1234567890,
            data: ptr::null_mut(),
            data_size: 0,
        };
        
        engine.add_block(&block).unwrap();
        
        for i in 0..3 {
            let vote = LuxVote {
                voter_id: [i; 32],
                block_id: block.id,
                is_preference: false,
            };
            assert!(engine.process_vote(&vote).is_ok());
        }
        
        let stats = engine.get_stats().unwrap();
        assert_eq!(stats.votes_processed, 3);
        
        ConsensusEngine::cleanup().unwrap();
    }
}