// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! # Lux Consensus Rust SDK
//!
//! A clean, single-import interface to the Lux consensus system.
//! This is the main SDK surface for Rust applications using Lux consensus.
//!
//! ## Example
//!
//! ```rust
//! use lux_consensus::*;  // Single clean import!
//!
//! fn main() -> Result<(), Box<dyn std::error::Error>> {
//!     // Create and start engine
//!     let config = Config::default();
//!     let mut chain = Chain::new(config);
//!     chain.start()?;
//!
//!     // Create and add a block
//!     let block = Block::new(
//!         ID::from([1, 2, 3]),
//!         GENESIS_ID,
//!         1,
//!         b"Hello, Lux!".to_vec(),
//!     );
//!     chain.add(block.clone())?;
//!
//!     // Record votes
//!     for i in 0..20 {
//!         let vote = Vote::new(
//!             block.id.clone(),
//!             VoteType::Preference,
//!             NodeID::from([i as u8]),
//!         );
//!         chain.record_vote(vote)?;
//!     }
//!
//!     // Check if accepted
//!     assert!(chain.is_accepted(&block.id));
//!     Ok(())
//! }
//! ```

// No top-level imports needed, they're in the modules

// Re-export everything for single-import convenience
pub use crate::types::*;
pub use crate::engine::*;
pub use crate::errors::*;

// ============= TYPES MODULE =============

pub mod types {
    use std::fmt;
    use std::time::{Duration, SystemTime};
    
    /// Identifier type
    #[derive(Debug, Clone, PartialEq, Eq, Hash)]
    pub struct ID(pub Vec<u8>);
    
    impl ID {
        pub fn new(data: Vec<u8>) -> Self {
            ID(data)
        }
    }
    
    impl From<[u8; 3]> for ID {
        fn from(data: [u8; 3]) -> Self {
            ID(data.to_vec())
        }
    }
    
    impl From<[u8; 32]> for ID {
        fn from(data: [u8; 32]) -> Self {
            ID(data.to_vec())
        }
    }
    
    impl fmt::Display for ID {
        fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
            write!(f, "{}", hex::encode(&self.0))
        }
    }
    
    /// Node identifier
    pub type NodeID = ID;
    pub type Hash = ID;
    
    /// Genesis block ID
    pub const GENESIS_ID: ID = ID(Vec::new());
    
    /// Block status
    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum Status {
        Unknown,
        Processing,
        Rejected,
        Accepted,
    }
    
    /// Consensus decision
    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum Decision {
        Undecided,
        Accept,
        Reject,
    }
    
    /// Vote type
    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum VoteType {
        Preference,
        Commit,
        Cancel,
    }
    
    /// Block in the blockchain
    #[derive(Debug, Clone)]
    pub struct Block {
        pub id: ID,
        pub parent_id: ID,
        pub height: u64,
        pub payload: Vec<u8>,
        pub time: SystemTime,
    }
    
    impl Block {
        pub fn new(id: ID, parent_id: ID, height: u64, payload: Vec<u8>) -> Self {
            Block {
                id,
                parent_id,
                height,
                payload,
                time: SystemTime::now(),
            }
        }
    }
    
    /// Vote on a block
    #[derive(Debug, Clone)]
    pub struct Vote {
        pub block_id: ID,
        pub vote_type: VoteType,
        pub voter: NodeID,
        pub signature: Vec<u8>,
    }
    
    impl Vote {
        pub fn new(block_id: ID, vote_type: VoteType, voter: NodeID) -> Self {
            Vote {
                block_id,
                vote_type,
                voter,
                signature: Vec::new(),
            }
        }
        
        pub fn with_signature(mut self, signature: Vec<u8>) -> Self {
            self.signature = signature;
            self
        }
    }
    
    /// Consensus certificate
    #[derive(Debug, Clone)]
    pub struct Certificate {
        pub block_id: ID,
        pub height: u64,
        pub votes: Vec<Vote>,
        pub timestamp: SystemTime,
        pub signatures: Vec<Vec<u8>>,
    }
    
    /// Consensus configuration
    #[derive(Debug, Clone)]
    pub struct Config {
        // Consensus parameters
        pub alpha: usize,           // Quorum size
        pub k: usize,                // Sample size
        pub max_outstanding: usize,  // Max outstanding polls
        pub max_poll_delay: Duration, // Max delay between polls
        
        // Network parameters
        pub network_timeout: Duration,
        pub max_message_size: usize,
        
        // Security parameters
        pub security_level: u32,
        pub quantum_resistant: bool,
        pub gpu_acceleration: bool,
    }
    
    impl Default for Config {
        fn default() -> Self {
            Config {
                alpha: 20,
                k: 20,
                max_outstanding: 10,
                max_poll_delay: Duration::from_secs(1),
                
                network_timeout: Duration::from_secs(5),
                max_message_size: 2 * 1024 * 1024, // 2MB
                
                security_level: 5,    // NIST Level 5
                quantum_resistant: true,
                gpu_acceleration: true,
            }
        }
    }
}

// ============= ERRORS MODULE =============

pub mod errors {
    use std::error::Error;
    use std::fmt;
    
    /// Consensus error type
    #[derive(Debug)]
    pub enum ConsensusError {
        BlockNotFound,
        InvalidBlock,
        InvalidVote,
        NoQuorum,
        AlreadyVoted,
        NotValidator,
        Timeout,
        NotInitialized,
        Other(String),
    }
    
    impl fmt::Display for ConsensusError {
        fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
            match self {
                ConsensusError::BlockNotFound => write!(f, "Block not found"),
                ConsensusError::InvalidBlock => write!(f, "Invalid block"),
                ConsensusError::InvalidVote => write!(f, "Invalid vote"),
                ConsensusError::NoQuorum => write!(f, "No quorum"),
                ConsensusError::AlreadyVoted => write!(f, "Already voted"),
                ConsensusError::NotValidator => write!(f, "Not a validator"),
                ConsensusError::Timeout => write!(f, "Operation timeout"),
                ConsensusError::NotInitialized => write!(f, "Engine not initialized"),
                ConsensusError::Other(msg) => write!(f, "{}", msg),
            }
        }
    }
    
    impl Error for ConsensusError {}
    
    /// Result type alias
    pub type Result<T> = std::result::Result<T, ConsensusError>;
}

// ============= ENGINE MODULE =============

pub mod engine {
    use super::*;
    use std::collections::HashMap;
    use std::sync::{Arc, Mutex};
    
    /// Consensus engine trait
    pub trait Engine {
        fn add(&mut self, block: Block) -> Result<()>;
        fn record_vote(&mut self, vote: Vote) -> Result<()>;
        fn is_accepted(&self, id: &ID) -> bool;
        fn get_status(&self, id: &ID) -> Status;
        fn start(&mut self) -> Result<()>;
        fn stop(&mut self) -> Result<()>;
    }
    
    /// Linear blockchain consensus engine
    pub struct Chain {
        config: Config,
        blocks: Arc<Mutex<HashMap<ID, Block>>>,
        votes: Arc<Mutex<HashMap<ID, Vec<Vote>>>>,
        status: Arc<Mutex<HashMap<ID, Status>>>,
        last_accepted: Arc<Mutex<ID>>,
        height: Arc<Mutex<u64>>,
        started: Arc<Mutex<bool>>,
    }
    
    impl Chain {
        pub fn new(config: Config) -> Self {
            Chain {
                config,
                blocks: Arc::new(Mutex::new(HashMap::new())),
                votes: Arc::new(Mutex::new(HashMap::new())),
                status: Arc::new(Mutex::new(HashMap::new())),
                last_accepted: Arc::new(Mutex::new(GENESIS_ID)),
                height: Arc::new(Mutex::new(0)),
                started: Arc::new(Mutex::new(false)),
            }
        }
        
        fn accept_block(&mut self, id: ID) {
            let mut status = self.status.lock().unwrap();
            status.insert(id.clone(), Status::Accepted);
            
            let blocks = self.blocks.lock().unwrap();
            if let Some(block) = blocks.get(&id) {
                let mut height = self.height.lock().unwrap();
                if block.height > *height {
                    *height = block.height;
                    let mut last = self.last_accepted.lock().unwrap();
                    *last = id;
                }
            }
        }
    }
    
    impl Engine for Chain {
        fn add(&mut self, block: Block) -> Result<()> {
            if !*self.started.lock().unwrap() {
                return Err(ConsensusError::NotInitialized);
            }
            
            let id = block.id.clone();
            self.blocks.lock().unwrap().insert(id.clone(), block);
            self.status.lock().unwrap().insert(id.clone(), Status::Processing);
            self.votes.lock().unwrap().entry(id).or_insert_with(Vec::new);
            
            Ok(())
        }
        
        fn record_vote(&mut self, vote: Vote) -> Result<()> {
            if !*self.started.lock().unwrap() {
                return Err(ConsensusError::NotInitialized);
            }
            
            {
                let blocks = self.blocks.lock().unwrap();
                if !blocks.contains_key(&vote.block_id) {
                    return Err(ConsensusError::BlockNotFound);
                }
            } // Drop blocks lock here
            
            let should_accept = {
                let mut votes = self.votes.lock().unwrap();
                let block_votes = votes.entry(vote.block_id.clone()).or_insert_with(Vec::new);
                block_votes.push(vote.clone());
                block_votes.len() >= self.config.alpha
            }; // Drop votes lock here
            
            if should_accept {
                self.accept_block(vote.block_id);
            }
            
            Ok(())
        }
        
        fn is_accepted(&self, id: &ID) -> bool {
            self.status.lock().unwrap()
                .get(id)
                .map_or(false, |s| *s == Status::Accepted)
        }
        
        fn get_status(&self, id: &ID) -> Status {
            self.status.lock().unwrap()
                .get(id)
                .copied()
                .unwrap_or(Status::Unknown)
        }
        
        fn start(&mut self) -> Result<()> {
            let mut started = self.started.lock().unwrap();
            if *started {
                return Ok(());
            }
            
            // Initialize genesis block
            let genesis = Block::new(
                ID::new(vec![0; 32]),
                ID::new(vec![0; 32]),
                0,
                Vec::new(),
            );
            
            self.blocks.lock().unwrap().insert(genesis.id.clone(), genesis.clone());
            self.status.lock().unwrap().insert(genesis.id.clone(), Status::Accepted);
            *self.last_accepted.lock().unwrap() = genesis.id;
            
            *started = true;
            Ok(())
        }
        
        fn stop(&mut self) -> Result<()> {
            *self.started.lock().unwrap() = false;
            Ok(())
        }
    }
}

// ============= CONVENIENCE FUNCTIONS =============

/// Quick start a consensus engine
pub fn quick_start() -> Result<Chain> {
    let config = Config::default();
    let mut chain = Chain::new(config);
    chain.start()?;
    Ok(chain)
}

/// Create a new block helper
pub fn new_block(id: ID, parent_id: ID, height: u64, payload: Vec<u8>) -> Block {
    Block::new(id, parent_id, height, payload)
}

/// Create a new vote helper
pub fn new_vote(block_id: ID, vote_type: VoteType, voter: NodeID) -> Vote {
    Vote::new(block_id, vote_type, voter)
}