// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! # Lux Consensus TypeScript/Node.js Bindings
//!
//! NAPI-RS bindings for the Lux Quasar consensus SDK.
//! Provides native performance with TypeScript type safety.
//!
//! ## Features
//!
//! - **Wave Consensus**: Threshold voting with FPC-based adaptive thresholds
//! - **FPC**: Fast Probabilistic Consensus with PRF-derived thresholds
//! - **Photon**: Light-based validator sampling with luminance tracking
//! - **Focus**: Confidence accumulation through consecutive rounds
//! - **Quasar**: Post-quantum finality with hybrid signatures

use napi::bindgen_prelude::*;
use napi_derive::napi;
use serde::{Deserialize, Serialize};
use std::sync::{Arc, Mutex};

// Import the Engine trait to access its methods
use lux_consensus::Engine;

// Re-use core types from lux-consensus
use lux_consensus::{
    Block as CoreBlock,
    QuasarEngine as CoreQuasarEngine,
    QuasarConfig as CoreQuasarConfig,
    FpcSelector as CoreFpcSelector,
    ID as CoreID,
    NodeID as CoreNodeID,
    Status as CoreStatus,
    Vote as CoreVote,
    VoteType as CoreVoteType,
    SecurityLevel as CoreSecurityLevel,
};

// ============= TypeScript-friendly Types =============

/// Block identifier (32-byte hex string)
#[napi]
pub struct BlockId {
    inner: [u8; 32],
}

#[napi]
impl BlockId {
    #[napi(constructor)]
    pub fn new(hex_string: String) -> Result<Self> {
        let bytes = hex::decode(&hex_string)
            .map_err(|e| Error::from_reason(format!("Invalid hex: {}", e)))?;
        if bytes.len() != 32 {
            return Err(Error::from_reason("Block ID must be 32 bytes"));
        }
        let mut arr = [0u8; 32];
        arr.copy_from_slice(&bytes);
        Ok(BlockId { inner: arr })
    }

    /// Create zero ID (genesis parent)
    #[napi(factory)]
    pub fn zero() -> Self {
        BlockId { inner: [0u8; 32] }
    }

    #[napi]
    pub fn to_hex(&self) -> String {
        hex::encode(&self.inner)
    }

    #[napi]
    pub fn to_bytes(&self) -> Vec<u8> {
        self.inner.to_vec()
    }

    #[napi]
    pub fn is_zero(&self) -> bool {
        self.inner == [0u8; 32]
    }
}

/// Node identifier (32-byte hex string)
#[napi]
pub struct NodeId {
    inner: [u8; 32],
}

#[napi]
impl NodeId {
    #[napi(constructor)]
    pub fn new(hex_string: String) -> Result<Self> {
        let bytes = hex::decode(&hex_string)
            .map_err(|e| Error::from_reason(format!("Invalid hex: {}", e)))?;
        if bytes.len() != 32 {
            return Err(Error::from_reason("Node ID must be 32 bytes"));
        }
        let mut arr = [0u8; 32];
        arr.copy_from_slice(&bytes);
        Ok(NodeId { inner: arr })
    }

    #[napi]
    pub fn to_hex(&self) -> String {
        hex::encode(&self.inner)
    }

    #[napi]
    pub fn to_bytes(&self) -> Vec<u8> {
        self.inner.to_vec()
    }
}

/// Block status
#[napi]
pub enum BlockStatus {
    Unknown,
    Processing,
    Rejected,
    Accepted,
}

impl From<CoreStatus> for BlockStatus {
    fn from(status: CoreStatus) -> Self {
        match status {
            CoreStatus::Unknown => BlockStatus::Unknown,
            CoreStatus::Processing => BlockStatus::Processing,
            CoreStatus::Rejected => BlockStatus::Rejected,
            CoreStatus::Accepted => BlockStatus::Accepted,
        }
    }
}

/// Vote type
#[napi]
pub enum VoteType {
    /// Preference vote during sampling
    Preference,
    /// Commit vote for finalization
    Commit,
    /// Cancel/reject vote
    Cancel,
}

impl From<VoteType> for CoreVoteType {
    fn from(vt: VoteType) -> Self {
        match vt {
            VoteType::Preference => CoreVoteType::Preference,
            VoteType::Commit => CoreVoteType::Commit,
            VoteType::Cancel => CoreVoteType::Cancel,
        }
    }
}

// ============= Configuration Types =============

/// Complete Quasar consensus configuration
#[napi(object)]
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct QuasarConfig {
    // Wave parameters
    /// Sample/committee size (default: 20)
    pub k: u32,
    /// Fixed threshold ratio 0.5-0.8 (default: 0.69 = 69%)
    pub alpha: f64,
    /// Consecutive rounds for finality (default: 20)
    pub beta: u32,
    /// Round timeout in milliseconds (default: 100)
    pub round_timeout_ms: u32,

    // FPC parameters
    /// Enable FPC adaptive thresholds (default: true)
    pub enable_fpc: bool,
    /// Minimum FPC threshold (default: 0.5)
    pub theta_min: f64,
    /// Maximum FPC threshold (default: 0.8)
    pub theta_max: f64,
    /// PRF seed for FPC (32-byte hex, optional)
    pub fpc_seed: Option<String>,

    // Photon parameters
    /// Base luminance in lux (default: 100.0)
    pub base_luminance: f64,
    /// Maximum luminance (default: 1000.0)
    pub max_luminance: f64,
    /// Minimum luminance (default: 10.0)
    pub min_luminance: f64,
    /// Success multiplier (default: 1.1)
    pub success_multiplier: f64,
    /// Failure multiplier (default: 0.9)
    pub failure_multiplier: f64,

    // Network parameters
    /// Network timeout in milliseconds (default: 5000)
    pub network_timeout_ms: u32,
    /// Max message size in bytes (default: 2MB)
    pub max_message_size: u32,
    /// Max outstanding polls (default: 10)
    pub max_outstanding: u32,

    // Security parameters
    /// Security level: 2=Low, 3=Medium, 5=High (default: 3)
    pub security_level: u32,
    /// Enable quantum-resistant signatures (default: true)
    pub quantum_resistant: bool,
    /// Enable GPU acceleration (default: true)
    pub gpu_acceleration: bool,
}

impl Default for QuasarConfig {
    fn default() -> Self {
        QuasarConfig {
            k: 20,
            alpha: 0.69,
            beta: 20,
            round_timeout_ms: 100,
            enable_fpc: true,
            theta_min: 0.5,
            theta_max: 0.8,
            fpc_seed: None,
            base_luminance: 100.0,
            max_luminance: 1000.0,
            min_luminance: 10.0,
            success_multiplier: 1.1,
            failure_multiplier: 0.9,
            network_timeout_ms: 5000,
            max_message_size: 2 * 1024 * 1024,
            max_outstanding: 10,
            security_level: 3,
            quantum_resistant: true,
            gpu_acceleration: true,
        }
    }
}

impl From<&QuasarConfig> for CoreQuasarConfig {
    fn from(cfg: &QuasarConfig) -> Self {
        let fpc_seed = if let Some(ref seed_hex) = cfg.fpc_seed {
            if let Ok(bytes) = hex::decode(seed_hex) {
                if bytes.len() == 32 {
                    let mut arr = [0u8; 32];
                    arr.copy_from_slice(&bytes);
                    arr
                } else {
                    *b"lux-consensus-fpc-default-seed!!"
                }
            } else {
                *b"lux-consensus-fpc-default-seed!!"
            }
        } else {
            *b"lux-consensus-fpc-default-seed!!"
        };

        let security_level = match cfg.security_level {
            2 => CoreSecurityLevel::Low,
            5 => CoreSecurityLevel::High,
            _ => CoreSecurityLevel::Medium,
        };

        CoreQuasarConfig {
            k: cfg.k as usize,
            alpha: cfg.alpha,
            beta: cfg.beta,
            round_timeout: std::time::Duration::from_millis(cfg.round_timeout_ms as u64),
            enable_fpc: cfg.enable_fpc,
            theta_min: cfg.theta_min,
            theta_max: cfg.theta_max,
            fpc_seed,
            base_luminance: cfg.base_luminance,
            max_luminance: cfg.max_luminance,
            min_luminance: cfg.min_luminance,
            success_multiplier: cfg.success_multiplier,
            failure_multiplier: cfg.failure_multiplier,
            network_timeout: std::time::Duration::from_millis(cfg.network_timeout_ms as u64),
            max_message_size: cfg.max_message_size as usize,
            max_outstanding: cfg.max_outstanding as usize,
            security_level,
            quantum_resistant: cfg.quantum_resistant,
            gpu_acceleration: cfg.gpu_acceleration,
        }
    }
}

// Alias for backward compatibility
pub type ConsensusConfig = QuasarConfig;

/// Block data for consensus
#[napi(object)]
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Block {
    /// Block ID (32-byte hex string)
    pub id: String,
    /// Parent block ID (32-byte hex string)
    pub parent_id: String,
    /// Block height
    pub height: u32,
    /// Block payload (hex-encoded)
    pub payload: String,
    /// Block timestamp (Unix milliseconds)
    pub timestamp: i64,
}

impl Block {
    fn to_core(&self) -> Result<CoreBlock> {
        let id_bytes = hex::decode(&self.id)
            .map_err(|e| Error::from_reason(format!("Invalid block ID: {}", e)))?;
        let parent_bytes = hex::decode(&self.parent_id)
            .map_err(|e| Error::from_reason(format!("Invalid parent ID: {}", e)))?;
        let payload = hex::decode(&self.payload)
            .map_err(|e| Error::from_reason(format!("Invalid payload: {}", e)))?;

        if id_bytes.len() != 32 || parent_bytes.len() != 32 {
            return Err(Error::from_reason("Block IDs must be 32 bytes"));
        }

        let mut id_arr = [0u8; 32];
        let mut parent_arr = [0u8; 32];
        id_arr.copy_from_slice(&id_bytes);
        parent_arr.copy_from_slice(&parent_bytes);

        Ok(CoreBlock::new(
            CoreID::from(id_arr),
            CoreID::from(parent_arr),
            self.height as u64,
            payload,
        ))
    }
}

/// Vote on a block
#[napi(object)]
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Vote {
    /// Block ID being voted on (hex string)
    pub block_id: String,
    /// Vote type (0=Preference, 1=Commit, 2=Cancel)
    pub vote_type: u32,
    /// Voter node ID (hex string)
    pub voter: String,
    /// Optional signature (hex string)
    pub signature: Option<String>,
}

impl Vote {
    fn to_core(&self) -> Result<CoreVote> {
        let block_id = hex::decode(&self.block_id)
            .map_err(|e| Error::from_reason(format!("Invalid block ID: {}", e)))?;
        let voter = hex::decode(&self.voter)
            .map_err(|e| Error::from_reason(format!("Invalid voter ID: {}", e)))?;

        if block_id.len() != 32 || voter.len() != 32 {
            return Err(Error::from_reason("IDs must be 32 bytes"));
        }

        let vote_type = match self.vote_type {
            0 => CoreVoteType::Preference,
            1 => CoreVoteType::Commit,
            2 => CoreVoteType::Cancel,
            _ => return Err(Error::from_reason("Invalid vote type")),
        };

        let mut block_arr = [0u8; 32];
        let mut voter_arr = [0u8; 32];
        block_arr.copy_from_slice(&block_id);
        voter_arr.copy_from_slice(&voter);

        Ok(CoreVote::new(
            CoreID::from(block_arr),
            vote_type,
            CoreNodeID::from(voter_arr),
        ))
    }
}

// ============= FPC Selector =============

/// FPC (Fast Probabilistic Consensus) threshold selector
///
/// Computes phase-dependent thresholds using PRF-based selection.
/// Formula: α(phase, k) = ⌈θ(phase) · k⌉
#[napi]
pub struct FpcSelector {
    inner: CoreFpcSelector,
}

#[napi]
impl FpcSelector {
    /// Create a new FPC selector with given theta range
    #[napi(constructor)]
    pub fn new(theta_min: f64, theta_max: f64, seed_hex: Option<String>) -> Result<Self> {
        let seed = if let Some(s) = seed_hex {
            let bytes = hex::decode(&s)
                .map_err(|e| Error::from_reason(format!("Invalid seed hex: {}", e)))?;
            if bytes.len() != 32 {
                return Err(Error::from_reason("Seed must be 32 bytes"));
            }
            let mut arr = [0u8; 32];
            arr.copy_from_slice(&bytes);
            arr
        } else {
            *b"lux-fpc-default-seed-00000000000"
        };

        Ok(FpcSelector {
            inner: CoreFpcSelector::new(theta_min, theta_max, seed),
        })
    }

    /// Get the threshold for a given phase and k
    #[napi]
    pub fn threshold(&self, phase: u32, k: u32) -> u32 {
        self.inner.select_threshold(phase as u64, k as usize) as u32
    }

    /// Get theta value for a phase (between theta_min and theta_max)
    #[napi]
    pub fn theta(&self, phase: u32) -> f64 {
        self.inner.theta(phase as u64)
    }
}

// ============= Consensus Engine =============

/// Lux Quasar Consensus Engine for TypeScript/Node.js
///
/// Thread-safe consensus engine with native performance.
/// Implements complete Quasar consensus with:
/// - Wave: Threshold voting with FPC support
/// - FPC: Fast Probabilistic Consensus
/// - Photon: Light-based validator sampling
/// - Focus: Confidence accumulation
/// - Quasar: Post-quantum finality
#[napi]
pub struct ConsensusEngine {
    inner: Arc<Mutex<CoreQuasarEngine>>,
    config: QuasarConfig,
}

#[napi]
impl ConsensusEngine {
    /// Create a new consensus engine with configuration
    #[napi(constructor)]
    pub fn new(config: QuasarConfig) -> Result<Self> {
        let core_config = CoreQuasarConfig::from(&config);
        let engine = CoreQuasarEngine::new(core_config);
        Ok(ConsensusEngine {
            inner: Arc::new(Mutex::new(engine)),
            config,
        })
    }

    /// Start the consensus engine
    #[napi]
    pub fn start(&self) -> Result<()> {
        let mut engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;
        engine.start()
            .map_err(|e| Error::from_reason(format!("Start error: {:?}", e)))
    }

    /// Stop the consensus engine
    #[napi]
    pub fn stop(&self) -> Result<()> {
        let mut engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;
        engine.stop()
            .map_err(|e| Error::from_reason(format!("Stop error: {:?}", e)))
    }

    /// Add a block to the consensus
    #[napi]
    pub fn add_block(&self, block: Block) -> Result<()> {
        let core_block = block.to_core()?;
        let mut engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;
        engine.add(core_block)
            .map_err(|e| Error::from_reason(format!("Add error: {:?}", e)))
    }

    /// Record a vote on a block
    #[napi]
    pub fn record_vote(&self, vote: Vote) -> Result<()> {
        let core_vote = vote.to_core()?;
        let mut engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;
        engine.record_vote(core_vote)
            .map_err(|e| Error::from_reason(format!("Block not found: {:?}", e)))
    }

    /// Check if a block has been accepted
    #[napi]
    pub fn is_accepted(&self, block_id: String) -> Result<bool> {
        let id_bytes = hex::decode(&block_id)
            .map_err(|e| Error::from_reason(format!("Invalid block ID: {}", e)))?;
        if id_bytes.len() != 32 {
            return Err(Error::from_reason("Block ID must be 32 bytes"));
        }
        let mut arr = [0u8; 32];
        arr.copy_from_slice(&id_bytes);

        let engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;
        Ok(engine.is_accepted(&CoreID::from(arr)))
    }

    /// Get the status of a block
    #[napi]
    pub fn get_status(&self, block_id: String) -> Result<BlockStatus> {
        let id_bytes = hex::decode(&block_id)
            .map_err(|e| Error::from_reason(format!("Invalid block ID: {}", e)))?;
        if id_bytes.len() != 32 {
            return Err(Error::from_reason("Block ID must be 32 bytes"));
        }
        let mut arr = [0u8; 32];
        arr.copy_from_slice(&id_bytes);

        let engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;
        Ok(engine.get_status(&CoreID::from(arr)).into())
    }

    /// Process multiple votes in batch (optimized)
    #[napi]
    pub fn record_votes_batch(&self, votes: Vec<Vote>) -> Result<u32> {
        let mut engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;

        let core_votes: Vec<CoreVote> = votes
            .into_iter()
            .filter_map(|v| v.to_core().ok())
            .collect();

        Ok(engine.record_votes_batch(core_votes) as u32)
    }

    /// Add a validator to the engine
    #[napi]
    pub fn add_validator(&self, node_id: String, weight: u32) -> Result<()> {
        let id_bytes = hex::decode(&node_id)
            .map_err(|e| Error::from_reason(format!("Invalid node ID: {}", e)))?;
        if id_bytes.len() != 32 {
            return Err(Error::from_reason("Node ID must be 32 bytes"));
        }
        let mut arr = [0u8; 32];
        arr.copy_from_slice(&id_bytes);

        let mut engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;
        engine.add_validator(CoreNodeID::from(arr), weight as u64);
        Ok(())
    }

    /// Get the current configuration
    #[napi]
    pub fn get_config(&self) -> QuasarConfig {
        self.config.clone()
    }

    /// Get current block height
    #[napi]
    pub fn get_height(&self) -> Result<u32> {
        let engine = self.inner.lock()
            .map_err(|e| Error::from_reason(format!("Lock error: {}", e)))?;
        Ok(engine.height() as u32)
    }
}

// ============= Utility Functions =============

/// Generate a random 32-byte block ID
#[napi]
pub fn generate_block_id() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let mut bytes = [0u8; 32];
    bytes[..16].copy_from_slice(&now.to_le_bytes());
    // Add some randomness from pointer address
    let ptr = &bytes as *const _ as u64;
    bytes[16..24].copy_from_slice(&ptr.to_le_bytes());
    // Add more entropy
    let extra = (now ^ (ptr as u128)) as u64;
    bytes[24..32].copy_from_slice(&extra.to_le_bytes());
    hex::encode(bytes)
}

/// Create default Quasar configuration (production)
#[napi]
pub fn default_config() -> QuasarConfig {
    QuasarConfig::default()
}

/// Create testnet configuration (lower thresholds for testing)
#[napi]
pub fn testnet_config() -> QuasarConfig {
    QuasarConfig {
        k: 5,
        alpha: 0.6,
        beta: 5,
        round_timeout_ms: 50,
        enable_fpc: false,
        theta_min: 0.5,
        theta_max: 0.7,
        fpc_seed: None,
        base_luminance: 100.0,
        max_luminance: 500.0,
        min_luminance: 20.0,
        success_multiplier: 1.05,
        failure_multiplier: 0.95,
        network_timeout_ms: 10000,
        max_message_size: 1024 * 1024,
        max_outstanding: 5,
        security_level: 2,
        quantum_resistant: false,
        gpu_acceleration: false,
    }
}

/// Create mainnet configuration (production settings)
#[napi]
pub fn mainnet_config() -> QuasarConfig {
    QuasarConfig {
        k: 21,
        alpha: 0.69,
        beta: 20,
        round_timeout_ms: 100,
        enable_fpc: true,
        theta_min: 0.5,
        theta_max: 0.8,
        fpc_seed: None,
        base_luminance: 100.0,
        max_luminance: 1000.0,
        min_luminance: 10.0,
        success_multiplier: 1.1,
        failure_multiplier: 0.9,
        network_timeout_ms: 5000,
        max_message_size: 2 * 1024 * 1024,
        max_outstanding: 10,
        security_level: 5,
        quantum_resistant: true,
        gpu_acceleration: true,
    }
}

/// Get SDK version
#[napi]
pub fn version() -> String {
    env!("CARGO_PKG_VERSION").to_string()
}

/// Compute FPC threshold for given parameters
#[napi]
pub fn fpc_threshold(phase: u32, k: u32, theta_min: f64, theta_max: f64) -> u32 {
    let selector = CoreFpcSelector::new(theta_min, theta_max, *b"lux-fpc-default-seed-00000000000");
    selector.select_threshold(phase as u64, k as usize) as u32
}

// ============= Tests =============

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_consensus_engine() {
        let config = testnet_config();
        let engine = ConsensusEngine::new(config).unwrap();
        engine.start().unwrap();

        // Create and add a block
        let block = Block {
            id: "01".repeat(32),
            parent_id: "00".repeat(32),
            height: 1,
            payload: hex::encode(b"test payload"),
            timestamp: 0,
        };
        engine.add_block(block.clone()).unwrap();

        // Record votes (need ceil(0.6 * 5) = 3 votes for testnet)
        for i in 0..5 {
            let vote = Vote {
                block_id: "01".repeat(32),
                vote_type: 0, // Preference
                voter: format!("{:02x}", i).repeat(32),
                signature: None,
            };
            engine.record_vote(vote).unwrap();
        }

        // Check acceptance
        assert!(engine.is_accepted("01".repeat(32)).unwrap());
        engine.stop().unwrap();
    }

    #[test]
    fn test_block_id() {
        let id = BlockId::new("00".repeat(32)).unwrap();
        assert_eq!(id.to_hex(), "00".repeat(32));
        assert!(id.is_zero());
    }

    #[test]
    fn test_fpc_selector() {
        let selector = FpcSelector::new(0.5, 0.8, None).unwrap();
        let threshold = selector.threshold(0, 20);
        assert!(threshold >= 10 && threshold <= 16);
    }

    #[test]
    fn test_configs() {
        let default = default_config();
        assert_eq!(default.alpha, 0.69);
        assert_eq!(default.k, 20);

        let testnet = testnet_config();
        assert_eq!(testnet.alpha, 0.6);
        assert_eq!(testnet.k, 5);

        let mainnet = mainnet_config();
        assert_eq!(mainnet.alpha, 0.69);
        assert_eq!(mainnet.k, 21);
    }
}
