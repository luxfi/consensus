// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! # Lux Consensus Rust SDK
//!
//! Complete Quasar consensus implementation with Wave, FPC, Photon, Focus protocols.
//! Full post-quantum support via Pulsar (Module-LWE threshold) hybrid signatures.
//!
//! ## Features
//!
//! - **Wave**: Threshold voting with FPC-based adaptive thresholds
//! - **FPC**: Fast Probabilistic Consensus via PRF-derived thresholds
//! - **Photon**: Light-based validator sampling with luminance tracking
//! - **Focus**: Confidence accumulation through β consecutive rounds
//! - **Quasar**: Post-quantum finality with hybrid BLS + Pulsar signatures
//! - **Vote**: A signed vote on the wire, and the tally that turns arriving
//!   votes into a certificate the whole `cert` predicate has already passed
//! - **Zap**: The transport frame those votes travel under — the same wire Go
//!   and C++ speak
//!
//! ## Example
//!
//! ```rust,no_run
//! use lux_consensus::*;
//!
//! fn main() {
//!     // Create Quasar consensus engine (full protocol stack)
//!     let config = QuasarConfig::mainnet();
//!     let mut engine = QuasarEngine::new(config);
//!     engine.start().unwrap();
//!
//!     // The committee. Only its members' ballots are counted.
//!     for i in 0..20 {
//!         engine.add_validator(NodeID::from([i; 20]), 1).unwrap();
//!     }
//!
//!     // Add a block
//!     let block = Block::new(
//!         ID::from([1u8; 32]),
//!         ID::from([0u8; 32]),
//!         1,
//!         b"Hello, Lux!".to_vec(),
//!     );
//!     engine.add(block.clone()).unwrap();
//!
//!     // Record votes (20 for mainnet quorum)
//!     for i in 0..20 {
//!         let vote = Vote::new(
//!             block.id.clone(),
//!             VoteType::Preference,
//!             NodeID::from([i; 20]),
//!         );
//!         engine.record_vote(vote).unwrap();
//!     }
//!
//!     assert!(engine.is_accepted(&block.id));
//!     engine.stop().unwrap();
//! }
//! ```

use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant, SystemTime};


// The finality standard: the bytes a validator signs and the thresholds that
// decide. Held to the Go definitions by tests/conformance.rs.
pub mod finality;

// The quorum certificate: the predicate that decides whether a block is
// accepted. Held to `engine/chain/cert.go` by tests/cert.rs.
pub mod cert;
pub mod pop;

// The vote plane: what one validator puts on the wire, and the tally that turns
// what arrives into a certificate. It states no accept rule of its own — the
// rule is `cert`'s, and `vote::Tally::cert` is the only door out of a tally.
pub mod vote;

// The ZAP transport frame the vote plane travels under — the same wire Go's
// `luxfi/api/zap` writes and `luxcpp/zap` reads. Layer A only: nothing in it
// knows what a vote is.
pub mod zap;

/// SHA-256. The crate's one hash, from the BLS library already linked here.
///
/// Every derivation the network agrees on runs through this — there is no
/// second hash and no "close enough" mixing function. Go computes the same
/// bytes with `crypto/sha256`.
pub fn sha256(input: &[u8]) -> [u8; 32] {
    let mut out = [0u8; 32];
    // SAFETY: blst_sha256 writes exactly 32 bytes to `out` and reads `len`
    // bytes from `input`; both are sized here from the slices themselves.
    unsafe { blst::blst_sha256(out.as_mut_ptr(), input.as_ptr(), input.len()) };
    out
}

// Re-export all public types
pub use crate::finality::{
    canonical_vote_message, crash_tolerance, half_stake_floor, nova_beta, nova_quorum,
    nova_signer_floor, two_thirds_count, two_thirds_stake_floor, weighted_quasar, Finality, Position,
    QC_FINALITY, QUORUM_CERT_VERSION, VOTE_MESSAGE_LEN, VOTE_TAG,
};
pub use crate::types::*;
pub use crate::errors::*;
pub use crate::fpc::*;
pub use crate::photon::*;
pub use crate::focus::*;
pub use crate::wave::*;
pub use crate::quasar::*;
pub use crate::engine::*;
pub use crate::vote::{SignedVote, Slot, Tally, VoteTransport, VOTE};

// ============= TYPES MODULE =============

pub mod types {
    use std::fmt;
    use std::time::{Duration, SystemTime};

    /// 32-byte identifier type
    #[derive(Debug, Clone, PartialEq, Eq, Hash)]
    pub struct ID(pub [u8; 32]);

    impl ID {
        pub fn new(data: [u8; 32]) -> Self {
            ID(data)
        }

        pub fn zero() -> Self {
            ID([0u8; 32])
        }

        pub fn from_slice(data: &[u8]) -> Self {
            let mut arr = [0u8; 32];
            let len = data.len().min(32);
            arr[..len].copy_from_slice(&data[..len]);
            ID(arr)
        }

        pub fn to_vec(&self) -> Vec<u8> {
            self.0.to_vec()
        }

        pub fn as_bytes(&self) -> &[u8; 32] {
            &self.0
        }
    }

    impl From<[u8; 32]> for ID {
        fn from(data: [u8; 32]) -> Self {
            ID(data)
        }
    }

    impl From<Vec<u8>> for ID {
        fn from(data: Vec<u8>) -> Self {
            ID::from_slice(&data)
        }
    }

    impl fmt::Display for ID {
        fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
            write!(f, "{}", hex::encode(self.0))
        }
    }

    /// A validator, by the 20-byte NodeID the network names it by.
    ///
    /// It is a type of its own and not an alias of [`ID`], because a node is not
    /// a block. Go's `ids.NodeID` is 20 bytes and the proof of possession a
    /// validator registers with signs `node ‖ key` over exactly those 20 — so a
    /// set that named validators by the 32-byte block id would compute a
    /// different preimage from the same registrant and could not check the
    /// network's proofs at all. Being a distinct type, the two cannot be passed
    /// for one another by accident.
    ///
    /// There is deliberately no lossy constructor. `ID::from_slice` pads and
    /// truncates, which is convenient for a hash and is exactly the hazard here:
    /// truncating 32 bytes into 20 maps distinct nodes onto one identity, and
    /// one identity is one signer slot and one share of the weight. The only way
    /// in is 20 bytes that are already 20 bytes.
    #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
    pub struct NodeID(pub crate::pop::NodeId);

    impl NodeID {
        pub fn as_bytes(&self) -> &crate::pop::NodeId {
            &self.0
        }
    }

    impl From<crate::pop::NodeId> for NodeID {
        fn from(data: crate::pop::NodeId) -> Self {
            NodeID(data)
        }
    }

    impl fmt::Display for NodeID {
        fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
            write!(f, "{}", hex::encode(self.0))
        }
    }

    pub type Hash = ID;

    /// Block status in consensus
    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum Status {
        Unknown,
        Processing,
        Rejected,
        Accepted,
    }

    /// Consensus decision result
    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum Decision {
        Undecided,
        Accept,
        Reject,
    }

    /// Vote type for consensus
    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum VoteType {
        Preference, // Initial preference vote
        Commit,     // Final commit vote
        Cancel,     // Reject/cancel vote
    }

    /// Block in the blockchain
    #[derive(Debug, Clone)]
    pub struct Block {
        pub id: ID,
        pub parent_id: ID,
        pub height: u64,
        pub payload: Vec<u8>,
        pub timestamp: SystemTime,
    }

    impl Block {
        pub fn new(id: ID, parent_id: ID, height: u64, payload: Vec<u8>) -> Self {
            Block {
                id,
                parent_id,
                height,
                payload,
                timestamp: SystemTime::now(),
            }
        }

        pub fn genesis() -> Self {
            Block {
                id: ID::zero(),
                parent_id: ID::zero(),
                height: 0,
                payload: Vec::new(),
                timestamp: SystemTime::UNIX_EPOCH,
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
        pub timestamp: SystemTime,
    }

    impl Vote {
        pub fn new(block_id: ID, vote_type: VoteType, voter: NodeID) -> Self {
            Vote {
                block_id,
                vote_type,
                voter,
                signature: Vec::new(),
                timestamp: SystemTime::now(),
            }
        }

        pub fn with_signature(mut self, signature: Vec<u8>) -> Self {
            self.signature = signature;
            self
        }

        pub fn prefer(&self) -> bool {
            matches!(self.vote_type, VoteType::Preference | VoteType::Commit)
        }
    }

    /// A finality certificate is a [`crate::cert::QuorumCert`] — one position and
    /// the distinct signed accepts that carry it.
    ///
    /// There is one certificate type in this crate and it is the Go one. The
    /// type this name used to denote carried an aggregate signature and a pair
    /// of header fields, `block_id` and `height`, that no signature covered; it
    /// was forgeable by a registered rogue key and re-labellable to any block.
    /// Both faults were properties of its shape, so the shape is gone: what a
    /// certificate claims is its `position`, and every claim is signed.
    pub type Certificate = crate::cert::QuorumCert;

    /// Security level for Corona post-quantum crypto
    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    #[derive(Default)]
    pub enum SecurityLevel {
        Low = 2,    // Corona Level 2
        #[default]
        Medium = 3, // Corona Level 3 - Default
        High = 5,   // Corona Level 5
    }

    

    /// Quasar consensus configuration
    #[derive(Debug, Clone)]
    pub struct QuasarConfig {
        // Wave parameters
        pub k: usize,               // Sample/committee size
        pub alpha: f64,             // Fixed threshold ratio (0.5-0.8)
        pub beta: u32,              // Consecutive rounds for finality
        pub round_timeout: Duration, // Round timeout

        // FPC parameters
        pub enable_fpc: bool,       // Enable FPC adaptive thresholds
        pub theta_min: f64,         // Minimum FPC threshold (0.5)
        pub theta_max: f64,         // Maximum FPC threshold (0.8)
        pub fpc_seed: [u8; 32],     // PRF seed for FPC

        // Photon parameters
        pub base_luminance: f64,    // Base luminance in lux (100.0)
        pub max_luminance: f64,     // Maximum luminance (1000.0)
        pub min_luminance: f64,     // Minimum luminance (10.0)
        pub success_multiplier: f64, // Success brightens (1.1)
        pub failure_multiplier: f64, // Failure dims (0.9)

        // Network parameters
        pub network_timeout: Duration,
        pub max_message_size: usize,
        pub max_outstanding: usize,

        // Security parameters
        pub security_level: SecurityLevel,
        pub quantum_resistant: bool,
        pub gpu_acceleration: bool,
    }

    /// The FPC seed a node uses when no epoch seed has been derived yet.
    ///
    /// It is the seed the Go selector defaults to, byte for byte, so a default
    /// Rust node and a default Go node draw the same θ schedule. In production
    /// the seed comes from the epoch instead — `sha256(epoch ‖ chain ‖ last
    /// finalized block)` — which is unpredictable until the previous epoch
    /// finalizes.
    pub const DEFAULT_FPC_SEED: [u8; 32] = *b"lux-fpc-default-seed-00000000000";

    /// Testnet's standing seed, until epoch derivation is wired.
    pub const TESTNET_FPC_SEED: [u8; 32] = *b"lux-testnet-fpc-seed-00000000000";

    /// Mainnet's standing seed, until epoch derivation is wired.
    pub const MAINNET_FPC_SEED: [u8; 32] = *b"lux-mainnet-fpc-secure-seed-2025";

    /// The balanced configuration. The two network presets below are stated as
    /// what they change about it, so a field that ought to be shared cannot
    /// quietly drift in one of three copies.
    impl Default for QuasarConfig {
        fn default() -> Self {
            QuasarConfig {
                // Wave
                k: 20,
                alpha: 0.69, // 69% quorum - 2% above standard 67%
                beta: 20,
                round_timeout: Duration::from_millis(100),

                // FPC
                enable_fpc: true,
                theta_min: 0.5,
                theta_max: 0.8,
                fpc_seed: DEFAULT_FPC_SEED,

                // Photon
                base_luminance: 100.0,
                max_luminance: 1000.0,
                min_luminance: 10.0,
                success_multiplier: 1.1,
                failure_multiplier: 0.9,

                // Network
                network_timeout: Duration::from_secs(5),
                max_message_size: 2 * 1024 * 1024, // 2MB
                max_outstanding: 10,

                // Security
                security_level: SecurityLevel::Medium,
                quantum_resistant: true,
                gpu_acceleration: true,
            }
        }
    }

    impl QuasarConfig {
        /// Testnet: a small committee, short rounds, fixed thresholds.
        pub fn testnet() -> Self {
            QuasarConfig {
                k: 5,
                alpha: 0.6,
                beta: 5,
                round_timeout: Duration::from_millis(50),
                enable_fpc: false,
                theta_max: 0.7,
                fpc_seed: TESTNET_FPC_SEED,
                max_luminance: 500.0,
                min_luminance: 20.0,
                success_multiplier: 1.05,
                failure_multiplier: 0.95,
                network_timeout: Duration::from_secs(10),
                max_message_size: 1024 * 1024,
                max_outstanding: 5,
                security_level: SecurityLevel::Low,
                quantum_resistant: false,
                gpu_acceleration: false,
                ..QuasarConfig::default()
            }
        }

        /// Mainnet: an odd committee of 21 for tie-breaking, at the highest
        /// security level. Everything else is the balanced default.
        pub fn mainnet() -> Self {
            QuasarConfig {
                k: 21,
                fpc_seed: MAINNET_FPC_SEED,
                security_level: SecurityLevel::High,
                ..QuasarConfig::default()
            }
        }

        /// Calculate alpha threshold as integer count
        pub fn alpha_count(&self) -> usize {
            (self.alpha * self.k as f64).ceil() as usize
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
        InvalidSignature,
        NoQuorum,
        AlreadyVoted,
        NotValidator,
        Timeout,
        NotInitialized,
        AlreadyStarted,
        CryptoError(String),
        NetworkError(String),
        Other(String),
    }

    impl fmt::Display for ConsensusError {
        fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
            match self {
                ConsensusError::BlockNotFound => write!(f, "Block not found"),
                ConsensusError::InvalidBlock => write!(f, "Invalid block"),
                ConsensusError::InvalidVote => write!(f, "Invalid vote"),
                ConsensusError::InvalidSignature => write!(f, "Invalid signature"),
                ConsensusError::NoQuorum => write!(f, "No quorum reached"),
                ConsensusError::AlreadyVoted => write!(f, "Already voted"),
                ConsensusError::NotValidator => write!(f, "Not a validator"),
                ConsensusError::Timeout => write!(f, "Operation timeout"),
                ConsensusError::NotInitialized => write!(f, "Engine not initialized"),
                ConsensusError::AlreadyStarted => write!(f, "Engine already started"),
                ConsensusError::CryptoError(msg) => write!(f, "Crypto error: {}", msg),
                ConsensusError::NetworkError(msg) => write!(f, "Network error: {}", msg),
                ConsensusError::Other(msg) => write!(f, "{}", msg),
            }
        }
    }

    impl Error for ConsensusError {}

    /// Result type alias
    pub type Result<T> = std::result::Result<T, ConsensusError>;
}

// ============= FPC MODULE - Fast Probabilistic Consensus =============

pub mod fpc {
    use super::*;

    /// FPC threshold selector using PRF for deterministic phase-dependent thresholds
    ///
    /// Formula: α(phase, k) = ⌈θ(phase) · k⌉
    /// Where θ(phase) = θ_min + PRF(seed, phase) * (θ_max - θ_min)
    #[derive(Debug, Clone)]
    pub struct FpcSelector {
        theta_min: f64,
        theta_max: f64,
        seed: [u8; 32],
    }

    impl FpcSelector {
        /// Create a new FPC selector with custom parameters
        pub fn new(theta_min: f64, theta_max: f64, seed: [u8; 32]) -> Self {
            let theta_min = if theta_min > 0.0 && theta_min < 1.0 {
                theta_min
            } else {
                0.5
            };
            let theta_max = if theta_max > theta_min && theta_max <= 1.0 {
                theta_max
            } else {
                0.8
            };

            FpcSelector {
                theta_min,
                theta_max,
                seed,
            }
        }


        /// θ for a phase, from the PRF the network runs.
        ///
        /// `θ(phase) = θ_min + sha256(seed ‖ be64(phase))[0..8] / (2⁶⁴−1) · (θ_max − θ_min)`
        ///
        /// This must be SHA-256 and nothing else. Go computes the quorum
        /// threshold this way (`protocol/wave/fpc.computeTheta`), and two nodes
        /// that derive different θ for one phase require different majorities of
        /// the same committee — a fork by arithmetic, with no bad actor in it.
        /// `tests/fpc_conformance.rs` holds this to the Go values.
        fn compute_theta(&self, phase: u64) -> f64 {
            let mut input = [0u8; 40];
            input[..32].copy_from_slice(&self.seed);
            input[32..40].copy_from_slice(&phase.to_be_bytes());

            let hash = sha256(&input);

            // Big-endian, first 8 bytes — the same window and order Go reads.
            let hash_u64 = u64::from_be_bytes([
                hash[0], hash[1], hash[2], hash[3],
                hash[4], hash[5], hash[6], hash[7],
            ]);
            let normalized = (hash_u64 as f64) / (u64::MAX as f64);

            self.theta_min + normalized * (self.theta_max - self.theta_min)
        }

        /// Select threshold α for given phase and committee size k
        pub fn select_threshold(&self, phase: u64, k: usize) -> usize {
            let theta = self.compute_theta(phase);
            (theta * k as f64).ceil() as usize
        }

        /// Get raw theta value for a phase (for debugging/testing)
        pub fn theta(&self, phase: u64) -> f64 {
            self.compute_theta(phase)
        }

        /// Get configured range
        pub fn range(&self) -> (f64, f64) {
            (self.theta_min, self.theta_max)
        }
    }

    /// The default selector reads the default configuration, so the θ range and
    /// the seed have one home apiece.
    impl Default for FpcSelector {
        fn default() -> Self {
            let c = QuasarConfig::default();
            FpcSelector::new(c.theta_min, c.theta_max, c.fpc_seed)
        }
    }
}

// ============= PHOTON MODULE - Light-Based Validator Sampling =============

pub mod photon {
    use super::*;

    /// Luminance tracks node brightness based on consensus participation
    ///
    /// Successful votes increase brightness, failures decrease it.
    /// Based on real-world lighting levels:
    /// - 100 lux: Base (office lighting)
    /// - 1000 lux: Maximum (daylight)
    /// - 10 lux: Minimum (twilight)
    #[derive(Debug, Clone)]
    pub struct Luminance {
        lux: HashMap<NodeID, f64>,
        base: f64,
        max: f64,
        min: f64,
        success_mult: f64,
        failure_mult: f64,
    }

    impl Luminance {
        /// Create new luminance tracker with config
        pub fn new(config: &QuasarConfig) -> Self {
            Luminance {
                lux: HashMap::new(),
                base: config.base_luminance,
                max: config.max_luminance,
                min: config.min_luminance,
                success_mult: config.success_multiplier,
                failure_mult: config.failure_multiplier,
            }
        }

        /// Update brightness based on vote success/failure
        pub fn illuminate(&mut self, id: &NodeID, success: bool) {
            let current = self.lux.entry(*id).or_insert(self.base);

            if success {
                *current *= self.success_mult;
                if *current > self.max {
                    *current = self.max;
                }
            } else {
                *current *= self.failure_mult;
                if *current < self.min {
                    *current = self.min;
                }
            }
        }

        /// Get normalized brightness (0.1 to 10.0)
        pub fn brightness(&self, id: &NodeID) -> f64 {
            self.lux.get(id).copied().unwrap_or(self.base) / self.base
        }

        /// Get raw lux value
        pub fn lux(&self, id: &NodeID) -> f64 {
            self.lux.get(id).copied().unwrap_or(self.base)
        }

        /// Get total luminance across all nodes
        pub fn total_luminance(&self) -> f64 {
            self.lux.values().sum()
        }

        /// Get number of tracked nodes
        pub fn node_count(&self) -> usize {
            self.lux.len()
        }
    }

    /// The default tracker reads the default configuration rather than
    /// restating its five numbers, which is how the two used to disagree.
    impl Default for Luminance {
        fn default() -> Self {
            Luminance::new(&QuasarConfig::default())
        }
    }

    /// Photon sampler for peer selection
    pub struct PhotonSampler {
        peers: Vec<NodeID>,
        luminance: Luminance,
    }

    impl PhotonSampler {
        /// Create new photon sampler
        pub fn new(peers: Vec<NodeID>, config: &QuasarConfig) -> Self {
            PhotonSampler {
                peers,
                luminance: Luminance::new(config),
            }
        }

        /// Sample k peers weighted by luminance
        pub fn sample(&self, k: usize) -> Vec<NodeID> {
            if self.peers.is_empty() {
                return Vec::new();
            }

            let k = k.min(self.peers.len());

            // Calculate weights based on luminance
            let weights: Vec<f64> = self.peers
                .iter()
                .map(|p| self.luminance.brightness(p))
                .collect();

            let total_weight: f64 = weights.iter().sum();
            if total_weight == 0.0 {
                // Fallback to uniform sampling
                return self.peers.iter().take(k).cloned().collect();
            }

            // Simple deterministic weighted selection
            let mut selected = Vec::with_capacity(k);
            let mut used = vec![false; self.peers.len()];

            for i in 0..k {
                let mut best_idx = 0;
                let mut best_score = f64::MIN;

                for (idx, &weight) in weights.iter().enumerate() {
                    if used[idx] {
                        continue;
                    }
                    // Score = weight * deterministic factor based on position
                    let score = weight * ((idx + i + 1) as f64 / self.peers.len() as f64);
                    if score > best_score {
                        best_score = score;
                        best_idx = idx;
                    }
                }

                used[best_idx] = true;
                selected.push(self.peers[best_idx]);
            }

            selected
        }

        /// Update luminance after vote result
        pub fn update_luminance(&mut self, id: &NodeID, success: bool) {
            self.luminance.illuminate(id, success);
        }

        /// Add a peer
        pub fn add_peer(&mut self, peer: NodeID) {
            if !self.peers.contains(&peer) {
                self.peers.push(peer);
            }
        }

        /// Remove a peer
        pub fn remove_peer(&mut self, peer: &NodeID) {
            self.peers.retain(|p| p != peer);
        }

        /// Get luminance reference
        pub fn luminance(&self) -> &Luminance {
            &self.luminance
        }
    }
}

// ============= FOCUS MODULE - Confidence Accumulation =============

pub mod focus {
    use super::*;

    /// Focus tracks confidence building for consensus through consecutive rounds
    ///
    /// A block achieves finality when it receives β consecutive rounds of
    /// votes above the alpha threshold.
    #[derive(Debug)]
    pub struct Focus<ID: Eq + std::hash::Hash + Clone> {
        threshold: u32,     // β - consecutive rounds needed
        alpha: f64,         // Ratio threshold
        states: HashMap<ID, FocusState>,
    }

    /// Internal state for a single item
    #[derive(Debug, Clone)]
    pub struct FocusState {
        pub confidence: u32,    // Consecutive rounds count
        pub preference: bool,   // Current preference (yes/no)
        pub decided: bool,      // Has reached finality
        pub decision: Decision, // Final decision
        pub last_ratio: f64,    // Last vote ratio
    }

    impl Default for FocusState {
        fn default() -> Self {
            FocusState {
                confidence: 0,
                preference: false,
                decided: false,
                decision: Decision::Undecided,
                last_ratio: 0.0,
            }
        }
    }

    /// What one round said: a quorum for, a quorum against, or neither.
    ///
    /// A round produces this; confidence is a fold over a sequence of them.
    /// Keeping the two apart is what lets the tally be counted in whatever unit
    /// suits the caller — Wave counts votes, Focus takes a ratio — while the
    /// rule about consecutive agreement stays in exactly one place.
    pub type Verdict = Option<bool>;

    /// Fold one verdict into a running confidence, and report a decision when
    /// `beta` consecutive rounds have agreed.
    ///
    /// A verdict that agrees with the standing preference deepens confidence; a
    /// verdict that opposes it replaces the preference and starts again at one;
    /// no quorum at all resets to zero. That reset is the point of the
    /// mechanism — β must be β *consecutive* rounds, or a block could
    /// accumulate agreement across rounds that disagreed in between.
    pub fn accumulate(
        preference: &mut bool,
        confidence: &mut u32,
        verdict: Verdict,
        beta: u32,
    ) -> Option<Decision> {
        match verdict {
            Some(v) if *preference == v => *confidence += 1,
            Some(v) => {
                *preference = v;
                *confidence = 1;
            }
            None => *confidence = 0,
        }

        if *confidence >= beta {
            Some(if *preference {
                Decision::Accept
            } else {
                Decision::Reject
            })
        } else {
            None
        }
    }

    impl<ID: Eq + std::hash::Hash + Clone> Focus<ID> {
        /// Create new focus tracker
        pub fn new(threshold: u32, alpha: f64) -> Self {
            Focus {
                threshold,
                alpha,
                states: HashMap::new(),
            }
        }

        /// Update confidence based on vote ratio
        ///
        /// Returns true if decision was just reached
        pub fn update(&mut self, id: ID, yes_votes: usize, total_votes: usize) -> bool {
            if total_votes == 0 {
                return false;
            }

            let ratio = yes_votes as f64 / total_votes as f64;
            let beta = self.threshold;
            let alpha = self.alpha;
            let state = self.states.entry(id).or_default();

            if state.decided {
                return false;
            }

            state.last_ratio = ratio;

            // This round's verdict, read as a ratio against alpha.
            let verdict = if ratio >= alpha {
                Some(true)
            } else if ratio <= 1.0 - alpha {
                Some(false)
            } else {
                None
            };

            match accumulate(&mut state.preference, &mut state.confidence, verdict, beta) {
                Some(decision) => {
                    state.decided = true;
                    state.decision = decision;
                    true
                }
                None => false,
            }
        }

        /// Get state for an item
        pub fn state(&self, id: &ID) -> Option<&FocusState> {
            self.states.get(id)
        }

        /// Check if item has reached finality
        pub fn is_decided(&self, id: &ID) -> bool {
            self.states.get(id).is_some_and(|s| s.decided)
        }

        /// Get decision for an item
        pub fn decision(&self, id: &ID) -> Decision {
            self.states.get(id).map_or(Decision::Undecided, |s| s.decision)
        }

        /// Get current confidence level
        pub fn confidence(&self, id: &ID) -> u32 {
            self.states.get(id).map_or(0, |s| s.confidence)
        }

        /// Reset state for an item
        pub fn reset(&mut self, id: &ID) {
            self.states.remove(id);
        }
    }

    /// Windowed confidence tracker with time-based expiry
    pub struct WindowedFocus<ID: Eq + std::hash::Hash + Clone> {
        inner: Focus<ID>,
        window: Duration,
        last_update: HashMap<ID, Instant>,
    }

    impl<ID: Eq + std::hash::Hash + Clone> WindowedFocus<ID> {
        pub fn new(threshold: u32, alpha: f64, window: Duration) -> Self {
            WindowedFocus {
                inner: Focus::new(threshold, alpha),
                window,
                last_update: HashMap::new(),
            }
        }

        /// Update with window expiry check
        pub fn update(&mut self, id: ID, yes_votes: usize, total_votes: usize) -> bool {
            let now = Instant::now();

            // Check for window expiry
            if let Some(&last) = self.last_update.get(&id) {
                if now.duration_since(last) > self.window {
                    self.inner.reset(&id);
                }
            }

            self.last_update.insert(id.clone(), now);
            self.inner.update(id, yes_votes, total_votes)
        }

        pub fn is_decided(&self, id: &ID) -> bool {
            self.inner.is_decided(id)
        }

        pub fn decision(&self, id: &ID) -> Decision {
            self.inner.decision(id)
        }
    }
}

// ============= WAVE MODULE - Threshold Voting Protocol =============

pub mod wave {
    use super::*;

    /// Wave state for a single block
    #[derive(Debug, Clone)]
    pub struct WaveState {
        pub votes: Vec<Vote>,
        pub yes_count: usize,
        pub no_count: usize,
        pub preference: bool,
        pub confidence: u32,
        pub decided: bool,
        pub decision: Decision,
    }

    impl Default for WaveState {
        fn default() -> Self {
            WaveState {
                votes: Vec::new(),
                yes_count: 0,
                no_count: 0,
                preference: false,
                confidence: 0,
                decided: false,
                decision: Decision::Undecided,
            }
        }
    }

    /// Wave consensus engine with FPC support
    pub struct Wave {
        config: QuasarConfig,
        fpc: Option<FpcSelector>,
        phase: u64,
        states: HashMap<ID, WaveState>,
    }

    impl Wave {
        /// Create new Wave consensus
        pub fn new(config: QuasarConfig) -> Self {
            let fpc = if config.enable_fpc {
                Some(FpcSelector::new(
                    config.theta_min,
                    config.theta_max,
                    config.fpc_seed,
                ))
            } else {
                None
            };

            Wave {
                config,
                fpc,
                phase: 0,
                states: HashMap::new(),
            }
        }

        /// Get or create state for a block
        pub fn get_or_create_state(&mut self, block_id: &ID) -> &mut WaveState {
            self.states.entry(block_id.clone()).or_default()
        }

        /// Record a vote and check for consensus
        ///
        /// Returns true if decision was just reached
        pub fn record_vote(&mut self, vote: Vote) -> bool {
            let block_id = vote.block_id.clone();

            let state = self.states.entry(block_id.clone())
                .or_default();

            if state.decided {
                return false;
            }

            // Check for duplicate voter
            if state.votes.iter().any(|v| v.voter == vote.voter) {
                return false;
            }

            // Count vote
            if vote.prefer() {
                state.yes_count += 1;
            } else {
                state.no_count += 1;
            }

            state.votes.push(vote);

            // Check if we have enough votes for a decision
            self.check_consensus(&block_id)
        }

        /// Check for consensus on a block
        fn check_consensus(&mut self, block_id: &ID) -> bool {
            self.advance_phase();
            let threshold = self.threshold();
            let (k, beta) = (self.config.k, self.config.beta);

            let state = match self.states.get_mut(block_id) {
                Some(s) => s,
                None => return false,
            };

            if state.decided {
                return false;
            }

            // Need at least k votes
            if state.yes_count + state.no_count < k {
                return false;
            }

            // This round's verdict, read as counts against the threshold.
            let verdict = if state.yes_count >= threshold {
                Some(true)
            } else if state.no_count >= threshold {
                Some(false)
            } else {
                None
            };

            match crate::focus::accumulate(
                &mut state.preference,
                &mut state.confidence,
                verdict,
                beta,
            ) {
                Some(decision) => {
                    state.decided = true;
                    state.decision = decision;
                    true
                }
                None => false,
            }
        }

        /// The votes a block needs in the current phase.
        ///
        /// Pure: asking does not move the phase on. It used to, which meant the
        /// FPC schedule advanced once per vote rather than once per round.
        pub fn threshold(&self) -> usize {
            match self.fpc {
                Some(ref fpc) => fpc.select_threshold(self.phase, self.config.k),
                None => self.config.alpha_count(),
            }
        }

        /// Move to the next FPC phase.
        ///
        /// This engine has no round boundary — `check_consensus` runs on every
        /// vote — so a phase is currently a vote. That is the gap: FPC draws a
        /// threshold per ROUND, and a round is a fresh sample of k validators.
        /// Until a round loop exists, the advance is at least explicit and in
        /// one place rather than hidden inside a getter.
        fn advance_phase(&mut self) {
            if self.fpc.is_some() {
                self.phase += 1;
            }
        }

        /// Get state for a block
        pub fn state(&self, block_id: &ID) -> Option<&WaveState> {
            self.states.get(block_id)
        }

        /// Check if block is decided
        pub fn is_decided(&self, block_id: &ID) -> bool {
            self.states.get(block_id).is_some_and(|s| s.decided)
        }

        /// Get decision for a block
        pub fn decision(&self, block_id: &ID) -> Decision {
            self.states.get(block_id).map_or(Decision::Undecided, |s| s.decision)
        }

        /// Reset state for a block
        pub fn reset(&mut self, block_id: &ID) {
            self.states.remove(block_id);
        }

        /// Get current phase
        pub fn phase(&self) -> u64 {
            self.phase
        }
    }
}

// ============= QUASAR MODULE - Post-Quantum Finality =============

pub mod quasar {
    use super::*;
    use crate::cert::{ValidatorSet, Vote as CertVote, VoteVerifier};

    /// Quasar finality: the certificate side of the engine.
    ///
    /// It holds the one validator set — membership, stake and signing keys —
    /// and it will only issue or accept a certificate whose signatures verify
    /// under it. A vote from a member with no registered key contributes
    /// nothing here, which is the fail-closed direction.
    pub struct QuasarConsensus {
        validators: ValidatorSet,
        threshold: usize,
        /// Certificates this node issued, keyed by the signed identity of the
        /// position (`Position::signed_identity`) — the value every vote
        /// committed to, never the unsigned transport id. A certificate cannot
        /// be filed under a block it never attested.
        finalized: HashMap<ID, Certificate>,
    }

    impl QuasarConsensus {
        /// Create new Quasar consensus
        pub fn new(config: &QuasarConfig) -> Self {
            QuasarConsensus {
                validators: ValidatorSet::new(),
                threshold: config.alpha_count(),
                finalized: HashMap::new(),
            }
        }

        /// Register a validator this node has no signing key for.
        ///
        /// It is a member and it holds stake, so its ballots count toward
        /// preference — but it cannot contribute to a certificate until its key
        /// is known. A node already in the set is refused rather than restated:
        /// one node, one admission, on this door as on the keyed one.
        pub fn add_validator(&mut self, id: NodeID, weight: u64) -> Result<()> {
            self.validators
                .insert_unkeyed(*id.as_bytes(), weight)
                .map_err(|e| ConsensusError::CryptoError(format!("{e:?}")))
        }

        /// Register a validator with the BLS key it signs with, and its proof
        /// that it holds the matching secret.
        ///
        /// An invalid key, a key already claimed by another validator, or a
        /// proof of possession that does not verify is refused rather than
        /// stored — so a registered key is one a signature can be checked
        /// against AND one the registrant demonstrably controls. The proof
        /// travels with the validator declaration; this node does not mint it.
        pub fn add_validator_with_key(
            &mut self,
            id: NodeID,
            weight: u64,
            bls_pubkey: &[u8],
            pop: &[u8],
        ) -> Result<()> {
            self.validators
                .insert(*id.as_bytes(), weight, bls_pubkey, pop)
                .map_err(|e| ConsensusError::CryptoError(format!("{e:?}")))
        }

        /// Remove a validator
        pub fn remove_validator(&mut self, id: &NodeID) {
            self.validators.remove(id.as_bytes());
        }

        /// Get validator count
        pub fn validator_count(&self) -> usize {
            self.validators.len()
        }

        /// Whether this node may count `id`'s ballot.
        pub fn is_validator(&self, id: &NodeID) -> bool {
            self.validators.contains(id.as_bytes())
        }

        /// The validator set, for verifying certificates against.
        pub fn validators(&self) -> &ValidatorSet {
            &self.validators
        }

        /// Check if we have enough validators for consensus
        pub fn has_quorum(&self) -> bool {
            self.validators.len() >= self.threshold
        }

        /// Issue a certificate for `position` from the votes that actually
        /// support it.
        ///
        /// A vote counts only if its signature verifies under the voter's
        /// registered key over `canonical_vote_message(position, true)`. So a
        /// certificate is issued from evidence or not at all — unsigned ballots
        /// produce `NoQuorum`, not an empty certificate that later verifies.
        ///
        /// The signatures are kept, one per voter, exactly as Go keeps them.
        /// They are not summed: per-signature is the form Go reads, and it is
        /// the interoperable choice. Registration now supplies a proof of
        /// possession, so an aggregate would be sound too — but it would not be
        /// a form Go can read, so the certificate stays per-signature.
        pub fn create_certificate(
            &mut self,
            position: Position,
            votes: &[Vote],
        ) -> Result<Certificate> {
            let message = canonical_vote_message(&position, true);

            let mut accepted: Vec<CertVote> = Vec::new();
            let mut seen: std::collections::HashSet<crate::pop::NodeId> =
                std::collections::HashSet::new();

            for v in votes.iter().filter(|v| v.prefer()) {
                let id = *v.voter.as_bytes();
                if !seen.insert(id) {
                    continue;
                }
                if !self.validators.verify_vote(&id, &message, &v.signature, position.height) {
                    continue;
                }
                accepted.push(CertVote {
                    node_id: id,
                    accept: true,
                    signature: v.signature.clone(),
                });
            }

            if accepted.len() < self.threshold {
                return Err(ConsensusError::NoQuorum);
            }

            // Key finality on the identity the votes actually signed, never on
            // the transport block id, which is unsigned: a verified certificate
            // must not be relabelable to a block it never attested. See
            // `Position::signed_identity`.
            let key = ID::from(position.signed_identity());
            // Assembly sorts and dedups, so the certificate satisfies the
            // ordering clause by construction.
            let cert = Certificate::assemble(
                Finality::Quasar,
                position,
                self.threshold as u32,
                &accepted,
            )
            .map_err(|e| ConsensusError::CryptoError(e.to_string()))?;

            // Finalize only what the accept rule accepts. The count above is a
            // cheap pre-gate; the authority is the stake floor, recomputed from
            // the live set — so `is_finalized` can never report a certificate
            // `verify_certificate` would reject.
            cert.verify_weighted(&self.validators, &self.validators, 0)
                .map_err(|_| ConsensusError::NoQuorum)?;

            self.finalized.insert(key, cert.clone());
            Ok(cert)
        }

        /// Whether `cert` is evidence that its position was accepted.
        ///
        /// The whole rule, and only the rule: [`crate::cert::QuorumCert::
        /// verify_weighted`] — version, type, tier, strictly increasing distinct
        /// voters, every vote an ACCEPT, every signature checked against its own
        /// signer's key over the message rebuilt from the certificate's own
        /// position, and the tier's stake floor recomputed from this set.
        ///
        /// A certificate makes exactly one claim, its position, and every part
        /// of that claim is signed. There is no header field beside it to
        /// disagree with it, and no aggregate to check in place of the
        /// signatures.
        ///
        /// The epoch height is zero because this set is epoch-independent: its
        /// weights and membership do not vary with the argument (see the
        /// `StakeSource` impl on `ValidatorSet`). A set that reads a P-chain
        /// epoch must take that height from the caller.
        pub fn verify_certificate(&self, cert: &Certificate) -> bool {
            cert.verify_weighted(&self.validators, &self.validators, 0).is_ok()
        }

        /// Whether the position with this signed identity has quantum finality.
        /// The argument is `Position::signed_identity` — the value the votes
        /// committed to, not the transport block id, which finality is never
        /// keyed on.
        pub fn is_finalized(&self, signed_identity: &ID) -> bool {
            self.finalized.contains_key(signed_identity)
        }

        /// The certificate for a signed identity (`Position::signed_identity`),
        /// if one is finalized.
        pub fn get_certificate(&self, signed_identity: &ID) -> Option<&Certificate> {
            self.finalized.get(signed_identity)
        }
    }

    /// Event Horizon - Multi-chain block aggregation
    pub struct EventHorizon {
        quasar: QuasarConsensus,
        chains: HashMap<String, Vec<ID>>,
        height: u64,
    }

    impl EventHorizon {
        pub fn new(config: &QuasarConfig) -> Self {
            EventHorizon {
                quasar: QuasarConsensus::new(config),
                chains: HashMap::new(),
                height: 0,
            }
        }

        /// Register a chain
        pub fn register_chain(&mut self, chain_id: String) {
            self.chains.entry(chain_id).or_default();
        }

        /// Accept a block from a chain
        pub fn accept_block(&mut self, chain_id: &str, block_id: ID) {
            if let Some(blocks) = self.chains.get_mut(chain_id) {
                blocks.push(block_id);
                self.height += 1;
            }
        }

        /// Get current height
        pub fn height(&self) -> u64 {
            self.height
        }

        /// Get quasar consensus reference
        pub fn quasar(&self) -> &QuasarConsensus {
            &self.quasar
        }

        /// Get mutable quasar consensus reference
        pub fn quasar_mut(&mut self) -> &mut QuasarConsensus {
            &mut self.quasar
        }
    }
}

// ============= ENGINE MODULE - Complete Consensus Engine =============

pub mod engine {
    use super::*;

    /// Consensus engine trait
    pub trait Engine {
        fn add(&mut self, block: Block) -> Result<()>;
        fn record_vote(&mut self, vote: Vote) -> Result<()>;
        fn record_votes_batch(&mut self, votes: Vec<Vote>) -> usize;
        fn is_accepted(&self, id: &ID) -> bool;
        fn get_status(&self, id: &ID) -> Status;
        fn start(&mut self) -> Result<()>;
        fn stop(&mut self) -> Result<()>;
    }

    /// Complete Quasar consensus engine
    ///
    /// Integrates Wave voting, FPC thresholds, Photon sampling,
    /// Focus confidence, and Quasar post-quantum finality.
    pub struct QuasarEngine {
        config: QuasarConfig,
        wave: Wave,
        quasar: QuasarConsensus,
        blocks: Arc<RwLock<HashMap<ID, Block>>>,
        status: Arc<RwLock<HashMap<ID, Status>>>,
        started: Arc<RwLock<bool>>,
        height: Arc<RwLock<u64>>,
    }

    impl QuasarEngine {
        /// Create new Quasar engine with configuration
        pub fn new(config: QuasarConfig) -> Self {
            let wave = Wave::new(config.clone());
            let quasar = QuasarConsensus::new(&config);

            QuasarEngine {
                config,
                wave,
                quasar,
                blocks: Arc::new(RwLock::new(HashMap::new())),
                status: Arc::new(RwLock::new(HashMap::new())),
                started: Arc::new(RwLock::new(false)),
                height: Arc::new(RwLock::new(0)),
            }
        }

        /// Create testnet engine
        pub fn testnet() -> Self {
            QuasarEngine::new(QuasarConfig::testnet())
        }

        /// Create mainnet engine
        pub fn mainnet() -> Self {
            QuasarEngine::new(QuasarConfig::mainnet())
        }

        /// Add a validator this engine has no signing key for. Refused if it is
        /// already a member — see `QuasarConsensus::add_validator`.
        pub fn add_validator(&mut self, id: NodeID, weight: u64) -> Result<()> {
            self.quasar.add_validator(id, weight)
        }

        /// Get configuration
        pub fn config(&self) -> &QuasarConfig {
            &self.config
        }

        /// Get current height
        pub fn height(&self) -> u64 {
            *self.height.read().unwrap()
        }

        /// Accept a block (internal)
        fn accept_block(&mut self, block_id: &ID) {
            let mut status = self.status.write().unwrap();
            status.insert(block_id.clone(), Status::Accepted);

            let blocks = self.blocks.read().unwrap();
            if let Some(block) = blocks.get(block_id) {
                let mut height = self.height.write().unwrap();
                if block.height > *height {
                    *height = block.height;
                }
            }

            // Issue a certificate if the votes carry signatures that support
            // one. They often will not — this engine's ballots are unsigned —
            // and then there is no certificate, which is the honest outcome.
            let position = {
                let blocks = self.blocks.read().unwrap();
                blocks.get(block_id).map(|block| Position {
                    height: block.height,
                    block_id: *block_id.as_bytes(),
                    parent_id: *block.parent_id.as_bytes(),
                    ..Position::default()
                })
            };
            if let (Some(position), Some(votes)) =
                (position, self.wave.state(block_id).map(|s| s.votes.clone()))
            {
                let _ = self.quasar.create_certificate(position, &votes);
            }
        }
    }

    /// The balanced configuration, as a trait impl rather than an inherent
    /// method that shadows it — one `default`, and it is the standard one.
    impl Default for QuasarEngine {
        fn default() -> Self {
            QuasarEngine::new(QuasarConfig::default())
        }
    }

    impl Engine for QuasarEngine {
        fn add(&mut self, block: Block) -> Result<()> {
            if !*self.started.read().unwrap() {
                return Err(ConsensusError::NotInitialized);
            }

            let id = block.id.clone();

            {
                let mut blocks = self.blocks.write().unwrap();
                blocks.insert(id.clone(), block);
            }

            {
                let mut status = self.status.write().unwrap();
                status.insert(id.clone(), Status::Processing);
            }

            // Initialize wave state
            self.wave.get_or_create_state(&id);

            Ok(())
        }

        fn record_vote(&mut self, vote: Vote) -> Result<()> {
            if !*self.started.read().unwrap() {
                return Err(ConsensusError::NotInitialized);
            }

            // Check block exists
            {
                let blocks = self.blocks.read().unwrap();
                if !blocks.contains_key(&vote.block_id) {
                    return Err(ConsensusError::BlockNotFound);
                }
            }

            // Only members are counted. Without this the sample is whoever
            // happened to send a message: nine unregistered node ids used to
            // carry a block to Accepted against an engine holding no validators
            // at all. An empty set now counts nobody, which is the direction to
            // fail in.
            if !self.quasar.is_validator(&vote.voter) {
                return Err(ConsensusError::NotValidator);
            }

            let block_id = vote.block_id.clone();

            // Record vote in Wave
            let decided = self.wave.record_vote(vote);

            // If decided, update status
            if decided {
                let decision = self.wave.decision(&block_id);
                match decision {
                    Decision::Accept => self.accept_block(&block_id),
                    Decision::Reject => {
                        let mut status = self.status.write().unwrap();
                        status.insert(block_id, Status::Rejected);
                    }
                    Decision::Undecided => {}
                }
            }

            Ok(())
        }

        fn record_votes_batch(&mut self, votes: Vec<Vote>) -> usize {
            let mut success_count = 0;
            for vote in votes {
                if self.record_vote(vote).is_ok() {
                    success_count += 1;
                }
            }
            success_count
        }

        fn is_accepted(&self, id: &ID) -> bool {
            self.status.read().unwrap()
                .get(id)
                .is_some_and(|s| *s == Status::Accepted)
        }

        fn get_status(&self, id: &ID) -> Status {
            self.status.read().unwrap()
                .get(id)
                .copied()
                .unwrap_or(Status::Unknown)
        }

        fn start(&mut self) -> Result<()> {
            let mut started = self.started.write().unwrap();
            if *started {
                return Err(ConsensusError::AlreadyStarted);
            }

            // Initialize genesis block
            let genesis = Block::genesis();
            {
                let mut blocks = self.blocks.write().unwrap();
                blocks.insert(genesis.id.clone(), genesis.clone());
            }
            {
                let mut status = self.status.write().unwrap();
                status.insert(genesis.id, Status::Accepted);
            }

            *started = true;
            Ok(())
        }

        fn stop(&mut self) -> Result<()> {
            let mut started = self.started.write().unwrap();
            *started = false;
            Ok(())
        }
    }

}

// ============= CONVENIENCE FUNCTIONS =============

/// Quick start a consensus engine
pub fn quick_start() -> Result<QuasarEngine> {
    let mut engine = QuasarEngine::default();
    engine.start()?;
    Ok(engine)
}

/// Create a new block helper
pub fn new_block(id: ID, parent_id: ID, height: u64, payload: Vec<u8>) -> Block {
    Block::new(id, parent_id, height, payload)
}

/// Create a new vote helper
pub fn new_vote(block_id: ID, vote_type: VoteType, voter: NodeID) -> Vote {
    Vote::new(block_id, vote_type, voter)
}

/// Generate a random block ID
pub fn generate_block_id() -> ID {
    // Simple PRNG based on system time
    let now = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default();
    let seed = now.as_nanos() as u64;

    let mut state = seed;
    let mut bytes = [0u8; 32];
    for i in 0..4 {
        state ^= state << 13;
        state ^= state >> 7;
        state ^= state << 17;
        let chunk = state.to_le_bytes();
        bytes[i*8..(i+1)*8].copy_from_slice(&chunk);
    }

    ID::new(bytes)
}

/// Get SDK version
pub fn version() -> &'static str {
    env!("CARGO_PKG_VERSION")
}

// ============= TESTS =============

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_fpc_selector() {
        let fpc = FpcSelector::default();

        // Test determinism - same phase should give same theta
        let theta1 = fpc.theta(100);
        let theta2 = fpc.theta(100);
        assert_eq!(theta1, theta2);

        // Different phases should give different thetas
        let theta3 = fpc.theta(101);
        assert_ne!(theta1, theta3);

        // Theta should be in range
        for phase in 0..1000 {
            let theta = fpc.theta(phase);
            assert!((0.5..=0.8).contains(&theta), "theta {} out of range", theta);
        }
    }

    #[test]
    fn test_fpc_threshold() {
        let fpc = FpcSelector::new(0.5, 0.8, *b"test-seed-0000000000000000000000");
        let k = 20;

        let threshold = fpc.select_threshold(0, k);
        // Should be between ceil(0.5 * 20) = 10 and ceil(0.8 * 20) = 16
        assert!((10..=16).contains(&threshold));
    }

    #[test]
    fn test_luminance() {
        let config = QuasarConfig::testnet();
        let mut luminance = photon::Luminance::new(&config);

        let node = NodeID::from([1u8; 20]);

        // Initial brightness
        assert_eq!(luminance.brightness(&node), 1.0);

        // Success increases brightness
        luminance.illuminate(&node, true);
        assert!(luminance.brightness(&node) > 1.0);

        // Failure decreases brightness
        let bright_before = luminance.brightness(&node);
        luminance.illuminate(&node, false);
        assert!(luminance.brightness(&node) < bright_before);
    }

    #[test]
    fn test_focus_confidence() {
        let mut focus: focus::Focus<ID> = focus::Focus::new(5, 0.6);
        let block_id = ID::from([1u8; 32]);

        // Not decided initially
        assert!(!focus.is_decided(&block_id));

        // 5 consecutive rounds above 60% should finalize
        for _ in 0..5 {
            focus.update(block_id.clone(), 7, 10); // 70%
        }

        assert!(focus.is_decided(&block_id));
        assert_eq!(focus.decision(&block_id), Decision::Accept);
    }

    #[test]
    fn test_wave_voting() {
        let config = QuasarConfig::testnet(); // alpha=5, k=5, beta=5
        let mut wave = wave::Wave::new(config);

        let block_id = ID::from([1u8; 32]);

        // Record 5 preference votes
        for i in 0..5 {
            let vote = Vote::new(
                block_id.clone(),
                VoteType::Preference,
                NodeID::from([i; 20]),
            );
            wave.record_vote(vote);
        }

        // Should have positive preference
        let state = wave.state(&block_id).unwrap();
        assert_eq!(state.yes_count, 5);
    }

    #[test]
    fn test_quasar_engine() {
        let config = QuasarConfig::testnet();
        let mut engine = QuasarEngine::new(config);

        // Start engine
        engine.start().unwrap();

        // Add validators
        for i in 0..5 {
            engine.add_validator(NodeID::from([i; 20]), 1).unwrap();
        }

        // Add a block
        let block = Block::new(
            ID::from([1u8; 32]),
            ID::zero(),
            1,
            b"test".to_vec(),
        );
        engine.add(block.clone()).unwrap();

        // Record votes
        for i in 0..5 {
            let vote = Vote::new(
                block.id.clone(),
                VoteType::Preference,
                NodeID::from([i; 20]),
            );
            engine.record_vote(vote).unwrap();
        }

        // Should be processing or accepted
        let status = engine.get_status(&block.id);
        assert!(status == Status::Processing || status == Status::Accepted);

        engine.stop().unwrap();
    }

    #[test]
    fn test_full_consensus_flow() {
        let config = QuasarConfig::testnet();
        let mut engine = QuasarEngine::new(config.clone());
        engine.start().unwrap();

        // Add validators
        for i in 0..10 {
            engine.add_validator(NodeID::from([i; 20]), 1).unwrap();
        }

        // Create chain of blocks
        let blocks: Vec<Block> = (1..=3).map(|height| {
            let mut id = [0u8; 32];
            id[0] = height as u8;
            let mut parent_id = [0u8; 32];
            if height > 1 {
                parent_id[0] = (height - 1) as u8;
            }
            Block::new(ID::from(id), ID::from(parent_id), height, vec![])
        }).collect();

        // Add blocks
        for block in &blocks {
            engine.add(block.clone()).unwrap();
        }

        // Vote on each block (alpha=5 for testnet)
        for block in &blocks {
            for i in 0..5 {
                let vote = Vote::new(
                    block.id.clone(),
                    VoteType::Preference,
                    NodeID::from([i; 20]),
                );
                engine.record_vote(vote).unwrap();
            }
        }

        // All blocks should be accepted or processing
        for block in &blocks {
            let status = engine.get_status(&block.id);
            assert!(
                status == Status::Accepted || status == Status::Processing,
                "Block {} has unexpected status {:?}",
                block.height,
                status
            );
        }

        engine.stop().unwrap();
    }

    #[test]
    fn test_configs() {
        let default = QuasarConfig::default();
        assert_eq!(default.alpha, 0.69);
        assert_eq!(default.k, 20);
        assert_eq!(default.beta, 20);
        assert!(default.quantum_resistant);

        let testnet = QuasarConfig::testnet();
        assert_eq!(testnet.alpha, 0.6);
        assert_eq!(testnet.k, 5);
        assert!(!testnet.quantum_resistant);

        let mainnet = QuasarConfig::mainnet();
        assert_eq!(mainnet.alpha, 0.69);
        assert_eq!(mainnet.k, 21);
        assert!(mainnet.quantum_resistant);
    }
}
