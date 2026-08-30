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
//!             NodeID::from([i; 32]),
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

// The FPC threshold PRF, the same primitive Go's protocol/wave/fpc uses.
use sha2::{Digest, Sha256};

// The finality standard: the bytes a validator signs and the thresholds that
// decide. Held to the Go definitions by tests/conformance.rs.
pub mod finality;

// ============= BLS MODULE - The Signature Primitive =============

/// The signature scheme Lux consensus votes are carried under.
///
/// Every parameter here is read off `github.com/luxfi/crypto/bls` and none of
/// them is a free choice: the ciphersuite string is the domain separation tag
/// baked into every signature on the network, and the two lengths follow from
/// it. `BLS12381G2` in the tag means signatures live in G2, so a public key is a
/// compressed G1 point at 48 bytes and a signature is a compressed G2 point at
/// 96 bytes.
///
/// This module exists because those two lengths were inverted here — the
/// aggregator declared "48-byte aggregated G1 signature" and sliced every vote
/// to 48 bytes before handing it to a G2 decoder. That decoder rejects 48 bytes
/// every time, so the aggregate was the 48 zero bytes of the error path, always,
/// and the verifier's only signature test was that the field was non-empty. Zero
/// bytes are not empty. Forty validators voting with no signatures produced a
/// certificate that verified.
pub mod bls {
    use crate::finality::{Keys, Node};
    use blst::min_pk::{PublicKey, Signature};
    use blst::BLST_ERROR;
    use std::collections::HashMap;

    /// The ciphersuite, byte for byte what `luxfi/crypto/bls` signs and verifies
    /// under. A signature made under any other tag is not a vote on this network.
    pub const DST: &[u8] = b"BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_";

    /// A compressed G1 public key.
    pub const PUBLIC_KEY_LEN: usize = 48;

    /// A compressed G2 signature.
    pub const SIGNATURE_LEN: usize = 96;

    /// The signing keys of a validator set, by node.
    ///
    /// Resolution is the authorization: a node absent from the registry has no
    /// key, and a vote from it is refused rather than skipped. There is no
    /// "unknown signer" outcome that is not a refusal.
    #[derive(Debug, Default, Clone)]
    pub struct Registry {
        keys: HashMap<Node, PublicKey>,
    }

    impl Registry {
        pub fn new() -> Self {
            Registry { keys: HashMap::new() }
        }

        /// Register a validator's compressed public key.
        ///
        /// The key is validated on the way in — length, subgroup, and not the
        /// identity — so a malformed or identity key is refused once here rather
        /// than being carried to every later verification. An identity public
        /// key verifies a signature over any message.
        pub fn insert(&mut self, node: Node, compressed: &[u8]) -> bool {
            if compressed.len() != PUBLIC_KEY_LEN {
                return false;
            }
            let Ok(pk) = PublicKey::uncompress(compressed) else {
                return false;
            };
            if pk.validate().is_err() {
                return false;
            }
            self.keys.insert(node, pk);
            true
        }

        pub fn len(&self) -> usize {
            self.keys.len()
        }

        pub fn is_empty(&self) -> bool {
            self.keys.is_empty()
        }
    }

    impl Keys for Registry {
        fn verify(&self, node: &Node, message: &[u8], signature: &[u8]) -> bool {
            let Some(pk) = self.keys.get(node) else {
                return false;
            };
            if signature.len() != SIGNATURE_LEN {
                return false;
            }
            let Ok(sig) = Signature::uncompress(signature) else {
                return false;
            };
            // Both group checks on. A subgroup check costs a pairing-free
            // multiplication and refuses a small-order point; skipping it to
            // save that is how a signature over one message is replayed as a
            // signature over another.
            sig.verify(true, message, DST, &[], pk, true) == BLST_ERROR::BLST_SUCCESS
        }
    }

    #[cfg(test)]
    mod tests {
        use super::*;
        use crate::finality::{
            canonical_vote_message, Cert, Finality, Position, Refusal, Vote, NODE_LEN,
        };
        use blst::min_pk::SecretKey;

        /// A committee of `n` validators with real keys, and the registry that
        /// resolves them. Node ids ascend so an assembled certificate is already
        /// in canonical order.
        fn committee(n: u8) -> (Vec<SecretKey>, Vec<Node>, Registry) {
            let mut keys = Vec::new();
            let mut nodes = Vec::new();
            let mut registry = Registry::new();
            for i in 0..n {
                let sk = SecretKey::key_gen(&[i.wrapping_add(1); 32], &[]).expect("key_gen");
                let node = [i; NODE_LEN];
                assert!(registry.insert(node, &sk.sk_to_pk().compress()), "register {i}");
                keys.push(sk);
                nodes.push(node);
            }
            (keys, nodes, registry)
        }

        fn position(chain: u8) -> Position {
            Position {
                chain_id: [chain; 32],
                height: 9,
                round: 3,
                block_id: [0x22; 32],
                parent_id: [0x33; 32],
                canonical_id: [0x44; 32],
                parent_canonical_id: [0x55; 32],
                execution_state_root: [0x66; 32],
                payload_root: [0x77; 32],
                validator_set_root: [0x88; 32],
            }
        }

        /// A signature the network made, over the message the network signs,
        /// verifies here. Without this the refusals below would be satisfied by
        /// a verifier that refuses everything.
        #[test]
        fn a_real_signature_verifies() {
            let (keys, nodes, registry) = committee(4);
            let pos = position(0x11);
            let message = canonical_vote_message(&pos, true);

            let votes = keys
                .iter()
                .zip(&nodes)
                .map(|(sk, node)| Vote {
                    node: *node,
                    accept: true,
                    signature: sk.sign(&message, DST, &[]).compress().to_vec(),
                })
                .collect();

            let cert = Cert::assemble(pos, Finality::Quasar, 3, votes).expect("assemble");
            assert_eq!(cert.verify(&registry), Ok(()));
            assert_eq!(
                cert.votes[0].signature.len(),
                SIGNATURE_LEN,
                "a signature is a compressed G2 point"
            );
        }

        /// THE regression. Forty registered validators, every one of them voting
        /// with no signature at all, and the certificate must not verify.
        ///
        /// The predicate this replaces answered the question "is the aggregate
        /// signature field non-empty?", and the aggregator it asked about filled
        /// that field with forty-eight zero bytes whenever it had nothing to
        /// aggregate. Zero bytes are not empty, so the answer was yes, and this
        /// certificate — no signatures, no signers who signed anything — was
        /// finality.
        #[test]
        fn a_certificate_carrying_no_signatures_is_refused() {
            let (_, nodes, registry) = committee(40);
            let votes = nodes
                .iter()
                .map(|node| Vote { node: *node, accept: true, signature: Vec::new() })
                .collect();

            let cert = Cert::assemble(position(0x11), Finality::Quasar, 21, votes).expect("assemble");
            assert_eq!(cert.verify(&registry), Err(Refusal::Signature));
        }

        /// Forty-eight zero bytes are refused as explicitly as none: that exact
        /// value is what the broken aggregator produced on every input, so it is
        /// the one string a verifier must never mistake for a signature.
        #[test]
        fn the_broken_aggregate_is_refused() {
            let (_, nodes, registry) = committee(40);
            let votes = nodes
                .iter()
                .map(|node| Vote { node: *node, accept: true, signature: vec![0u8; 48] })
                .collect();

            let cert = Cert::assemble(position(0x11), Finality::Quasar, 21, votes).expect("assemble");
            assert_eq!(cert.verify(&registry), Err(Refusal::Signature));

            // And the same length a compressed signature actually is, all zero.
            let zeros = vec![0u8; SIGNATURE_LEN];
            assert!(!registry.verify(&nodes[0], b"any message", &zeros));
        }

        /// A validator with no registered key cannot contribute a vote, however
        /// well-formed the signature it presents.
        #[test]
        fn an_unregistered_signer_is_refused() {
            let (keys, nodes, registry) = committee(4);
            let pos = position(0x11);
            let message = canonical_vote_message(&pos, true);

            let mut votes: Vec<Vote> = keys
                .iter()
                .zip(&nodes)
                .map(|(sk, node)| Vote {
                    node: *node,
                    accept: true,
                    signature: sk.sign(&message, DST, &[]).compress().to_vec(),
                })
                .collect();

            // A stranger, signing the right message with a key nobody registered.
            let stranger = SecretKey::key_gen(&[0xEE; 32], &[]).expect("key_gen");
            votes.push(Vote {
                node: [0xFF; NODE_LEN],
                accept: true,
                signature: stranger.sign(&message, DST, &[]).compress().to_vec(),
            });

            let cert = Cert::assemble(pos, Finality::Quasar, 3, votes).expect("assemble");
            assert_eq!(cert.verify(&registry), Err(Refusal::Signature));
        }

        /// A signature made over one position cannot be presented in a
        /// certificate for another. The message is derived from the certificate's
        /// own position, so moving a valid signature to a different block
        /// invalidates it — this is what binds a vote to what it voted on.
        #[test]
        fn a_signature_does_not_travel_between_positions() {
            let (keys, nodes, registry) = committee(4);
            let signed = position(0x11);
            let message = canonical_vote_message(&signed, true);

            let votes: Vec<Vote> = keys
                .iter()
                .zip(&nodes)
                .map(|(sk, node)| Vote {
                    node: *node,
                    accept: true,
                    signature: sk.sign(&message, DST, &[]).compress().to_vec(),
                })
                .collect();

            // Same votes, a certificate claiming a different chain.
            let moved = Cert::assemble(position(0x99), Finality::Quasar, 3, votes.clone()).expect("assemble");
            assert_eq!(moved.verify(&registry), Err(Refusal::Signature));

            // And the accept flag is bound too: an accept signature is not a
            // reject signature over the same position.
            assert!(!registry.verify(
                &nodes[0],
                &canonical_vote_message(&signed, false),
                &votes[0].signature
            ));
        }

        /// One validator cannot be counted twice, and a certificate whose votes
        /// are not strictly increasing is refused whatever its byte form.
        #[test]
        fn a_duplicate_signer_is_refused() {
            let (keys, nodes, registry) = committee(4);
            let pos = position(0x11);
            let message = canonical_vote_message(&pos, true);
            let sig = keys[0].sign(&message, DST, &[]).compress().to_vec();

            let twice = vec![
                Vote { node: nodes[0], accept: true, signature: sig.clone() },
                Vote { node: nodes[0], accept: true, signature: sig.clone() },
            ];
            assert_eq!(
                Cert::assemble(pos.clone(), Finality::Quasar, 2, twice.clone()).unwrap_err(),
                Refusal::Order,
                "assembly must refuse a duplicate signer"
            );

            // And if one is built around the assembler, verification refuses it too.
            let forged = Cert {
                version: crate::finality::QUORUM_CERT_VERSION,
                role: crate::finality::QC_FINALITY,
                tier: Finality::Quasar,
                position: pos,
                threshold: 2,
                votes: twice,
            };
            assert_eq!(forged.verify(&registry), Err(Refusal::Order));
        }

        /// A certificate cannot relabel itself to a higher rung. The threshold is
        /// re-derived from the live set, so a Nova quorum of stake presented as
        /// Quasar fails the two-thirds check.
        #[test]
        fn a_rung_cannot_be_forged_upward() {
            struct Set;
            impl crate::finality::Stake for Set {
                // Five validators, equal weight.
                fn weight(&self, node: &Node, _height: u64) -> u64 {
                    if node[0] < 5 {
                        100
                    } else {
                        0
                    }
                }
                fn total(&self, _height: u64) -> u64 {
                    500
                }
                fn count(&self, _height: u64) -> i64 {
                    5
                }
            }

            let (keys, nodes, registry) = committee(5);
            let pos = position(0x11);
            let message = canonical_vote_message(&pos, true);
            let sign = |i: usize| Vote {
                node: nodes[i],
                accept: true,
                signature: keys[i].sign(&message, DST, &[]).compress().to_vec(),
            };

            // Three of five: 300 of 500 is a majority, and is NOT two thirds
            // (the floor is 333, and the predicate is strictly greater).
            let three = vec![sign(0), sign(1), sign(2)];
            let nova = Cert::assemble(pos.clone(), Finality::Nova, 3, three.clone()).expect("assemble");
            assert_eq!(nova.verify_stake(&registry, &Set), Ok(()), "three of five is a Nova majority");

            let relabeled = Cert::assemble(pos.clone(), Finality::Quasar, 3, three).expect("assemble");
            assert_eq!(
                relabeled.verify_stake(&registry, &Set),
                Err(Refusal::BelowStake),
                "a Nova quorum relabeled Quasar must not export"
            );

            // Four of five is 400 of 500, which does exceed the 333 floor.
            let four = vec![sign(0), sign(1), sign(2), sign(3)];
            let quasar = Cert::assemble(pos, Finality::Quasar, 4, four).expect("assemble");
            assert_eq!(quasar.verify_stake(&registry, &Set), Ok(()));
        }

        /// An unresolved validator set fails closed. A majority of an unknown set
        /// is not a majority, and answering "yes" there is how a node with a
        /// transiently empty view self-accepts.
        #[test]
        fn an_unresolved_set_fails_closed() {
            struct Nothing;
            impl crate::finality::Stake for Nothing {
                fn weight(&self, _node: &Node, _height: u64) -> u64 {
                    0
                }
                fn total(&self, _height: u64) -> u64 {
                    0
                }
                fn count(&self, _height: u64) -> i64 {
                    0
                }
            }

            let (keys, nodes, registry) = committee(4);
            let pos = position(0x11);
            let message = canonical_vote_message(&pos, true);
            let votes = keys
                .iter()
                .zip(&nodes)
                .map(|(sk, node)| Vote {
                    node: *node,
                    accept: true,
                    signature: sk.sign(&message, DST, &[]).compress().to_vec(),
                })
                .collect();

            let cert = Cert::assemble(pos, Finality::Quasar, 3, votes).expect("assemble");
            assert_eq!(cert.verify(&registry), Ok(()), "the signatures are real");
            assert_eq!(
                cert.verify_stake(&registry, &Nothing),
                Err(Refusal::BelowStake),
                "an unresolved set must not certify"
            );
        }

        /// The lengths, stated outright. Go's `PublicKeyLen` is 48 and its
        /// `SignatureLen` is 96; this module had them the other way round, which
        /// is why nothing it decoded ever parsed.
        #[test]
        fn the_group_sizes_are_go_s_way_round() {
            let sk = SecretKey::key_gen(&[7u8; 32], &[]).expect("key_gen");
            assert_eq!(sk.sk_to_pk().compress().len(), PUBLIC_KEY_LEN);
            assert_eq!(sk.sign(b"x", DST, &[]).compress().len(), SIGNATURE_LEN);
            assert_eq!(PUBLIC_KEY_LEN, 48, "a compressed G1 point");
            assert_eq!(SIGNATURE_LEN, 96, "a compressed G2 point");

            // The inversion, demonstrated: a signature truncated to the length
            // the old aggregator assumed does not decode.
            let sig = sk.sign(b"x", DST, &[]).compress();
            assert!(
                blst::min_pk::Signature::uncompress(&sig[..48]).is_err(),
                "48 bytes is not a G2 signature — the old aggregator's every input"
            );

            let mut registry = Registry::new();
            assert!(!registry.insert([1u8; NODE_LEN], &sig), "96 bytes is not a G1 public key");
            assert!(registry.is_empty());
        }
    }
}

// Re-export all public types
pub use crate::finality::{
    canonical_vote_message, crash_tolerance, equal_stake_quasar, half_stake_floor, nova_beta,
    nova_quorum, nova_signer_floor, two_thirds_stake_floor, weighted_quasar, Finality, Position,
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
            write!(f, "{}", hex::encode(&self.0))
        }
    }

    /// Node identifier (32 bytes)
    pub type NodeID = ID;
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

    /// Quasar signature (BLS + Corona)
    #[derive(Debug, Clone)]
    pub struct QuasarSignature {
        pub bls_sig: Vec<u8>,        // BLS signature (48 bytes)
        pub corona_sig: Vec<u8>,     // Corona post-quantum signature
        pub signer: NodeID,
    }

    /// Security level for Corona post-quantum crypto
    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum SecurityLevel {
        Low = 2,    // Corona Level 2
        Medium = 3, // Corona Level 3 - Default
        High = 5,   // Corona Level 5
    }

    impl Default for SecurityLevel {
        fn default() -> Self {
            SecurityLevel::Medium
        }
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

    impl QuasarConfig {
        /// Default configuration (balanced)
        pub fn default() -> Self {
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
                fpc_seed: *b"lux-consensus-fpc-default-seed!!", // 32 bytes

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

        /// Testnet configuration (fast, relaxed)
        pub fn testnet() -> Self {
            QuasarConfig {
                k: 5,
                alpha: 0.6,
                beta: 5,
                round_timeout: Duration::from_millis(50),
                enable_fpc: false,
                theta_min: 0.5,
                theta_max: 0.7,
                fpc_seed: *b"lux-testnet-fpc-seed-00000000000",
                base_luminance: 100.0,
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
            }
        }

        /// Mainnet configuration (production, secure)
        pub fn mainnet() -> Self {
            QuasarConfig {
                k: 21, // Odd number for tie-breaking
                alpha: 0.69,
                beta: 20,
                round_timeout: Duration::from_millis(100),
                enable_fpc: true,
                theta_min: 0.5,
                theta_max: 0.8,
                fpc_seed: *b"lux-mainnet-fpc-secure-seed-2025",
                base_luminance: 100.0,
                max_luminance: 1000.0,
                min_luminance: 10.0,
                success_multiplier: 1.1,
                failure_multiplier: 0.9,
                network_timeout: Duration::from_secs(5),
                max_message_size: 2 * 1024 * 1024,
                max_outstanding: 10,
                security_level: SecurityLevel::High,
                quantum_resistant: true,
                gpu_acceleration: true,
            }
        }

        /// Calculate alpha threshold as integer count
        pub fn alpha_count(&self) -> usize {
            (self.alpha * self.k as f64).ceil() as usize
        }
    }

    impl Default for QuasarConfig {
        fn default() -> Self {
            QuasarConfig::default()
        }
    }

    // Legacy Config alias for backward compatibility
    pub type Config = QuasarConfig;
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

    /// The label the epoch seed is taken under, so this digest cannot collide
    /// with any other digest the protocol takes over similar bytes.
    pub const SEED_DOMAIN: &[u8] = b"lux.consensus.fpc.seed";

    /// The exact bytes [`derive_epoch_seed`] hashes.
    ///
    /// ```text
    /// domain || be64(epoch) || be64(len(chain_id)) || chain_id
    ///        || be64(len(prev_block_hash)) || prev_block_hash
    /// ```
    ///
    /// Every variable-length field is written at its length, so the preimage
    /// reads back apart into exactly the three inputs that produced it. Written
    /// end to end they would not: a chain calling itself `lux-mainnet||H`
    /// derives, binding no parent at all, the seed `lux-mainnet` derives at
    /// parent `H` — trading the parent away for a name the chain picks itself,
    /// when the parent is the one input nobody can know in advance.
    ///
    /// This is `protocol/wave/fpc.EpochSeedPreimage`. The corpus records these
    /// bytes, so this crate is held to what it hashed and not only to what came
    /// out.
    pub fn epoch_seed_preimage(epoch: u64, chain_id: &[u8], prev_block_hash: &[u8]) -> Vec<u8> {
        let mut out =
            Vec::with_capacity(SEED_DOMAIN.len() + 24 + chain_id.len() + prev_block_hash.len());
        out.extend_from_slice(SEED_DOMAIN);
        out.extend_from_slice(&epoch.to_be_bytes());
        out.extend_from_slice(&(chain_id.len() as u64).to_be_bytes());
        out.extend_from_slice(chain_id);
        out.extend_from_slice(&(prev_block_hash.len() as u64).to_be_bytes());
        out.extend_from_slice(prev_block_hash);
        out
    }

    /// The per-epoch threshold seed: `sha256(epoch_seed_preimage(..))`.
    ///
    /// This is `protocol/wave/fpc.DeriveEpochSeed`, and it is the only way a seed
    /// should be produced. A hardcoded seed — which is what this module carried
    /// before — is public forever, so an adversary reads every future θ off it
    /// and knows in advance the exact round where a committee is thinnest.
    /// Binding `prev_block_hash` closes that: it is unknown until the previous
    /// epoch finalizes, so no party can compute the next epoch's thresholds
    /// while the current one is still open, and no party can steer them by
    /// choosing one of the inputs.
    ///
    /// The output is 32 bytes because it is a digest; feeding it straight into
    /// [`FpcSelector::new`] is the whole intended path.
    pub fn derive_epoch_seed(epoch: u64, chain_id: &[u8], prev_block_hash: &[u8]) -> [u8; 32] {
        let mut h = Sha256::new();
        h.update(epoch_seed_preimage(epoch, chain_id, prev_block_hash));
        h.finalize().into()
    }

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

        /// Create default selector
        pub fn default() -> Self {
            FpcSelector::new(0.5, 0.8, *b"lux-fpc-default-seed-00000000000")
        }

        /// Compute θ for a given phase.
        ///
        /// The PRF is SHA-256 over `seed || be64(phase)`, which is what
        /// `protocol/wave/fpc.(*Selector).computeTheta` computes. What used to
        /// stand here was a SipHash-style mixer commented "SHA256-like"; like
        /// is not equal, and it disagreed with Go on all sixteen phases. At
        /// phase 0 with k=20 it accepted a round on eleven votes where Go
        /// requires fifteen.
        ///
        /// θ sets α, the count at which a wave round accepts, so two nodes
        /// deriving different θ from the same seed do not agree about when a
        /// round decides. That is the Nova rung — reorgable, and not the
        /// two-thirds-stake certificate a bridge reads — so the damage is
        /// divergent preference and reorg behaviour rather than a broken
        /// settlement guarantee. It is still two implementations running
        /// different rules on identical votes.
        fn compute_theta(&self, phase: u64) -> f64 {
            // PRF input: seed || phase (as big-endian u64)
            let mut input = [0u8; 40];
            input[..32].copy_from_slice(&self.seed);
            input[32..40].copy_from_slice(&phase.to_be_bytes());

            let hash: [u8; 32] = Sha256::digest(input).into();

            // Convert first 8 bytes to u64, normalize to [0, 1]
            let hash_u64 = u64::from_be_bytes([
                hash[0], hash[1], hash[2], hash[3],
                hash[4], hash[5], hash[6], hash[7],
            ]);
            let normalized = (hash_u64 as f64) / (u64::MAX as f64);

            // Scale to [theta_min, theta_max]
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

        /// Create with default parameters
        pub fn default() -> Self {
            Luminance {
                lux: HashMap::new(),
                base: 100.0,
                max: 1000.0,
                min: 10.0,
                success_mult: 1.1,
                failure_mult: 0.9,
            }
        }

        /// Update brightness based on vote success/failure
        pub fn illuminate(&mut self, id: &NodeID, success: bool) {
            let current = self.lux.entry(id.clone()).or_insert(self.base);

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

                for (idx, (_peer, &weight)) in self.peers.iter().zip(weights.iter()).enumerate() {
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
                selected.push(self.peers[best_idx].clone());
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
            let state = self.states.entry(id).or_insert_with(FocusState::default);

            if state.decided {
                return false;
            }

            state.last_ratio = ratio;

            // Check if ratio exceeds alpha threshold
            if ratio >= self.alpha {
                // Voting YES
                if state.preference {
                    // Same preference, increment confidence
                    state.confidence += 1;
                } else {
                    // Preference switched, reset
                    state.preference = true;
                    state.confidence = 1;
                }
            } else if ratio <= 1.0 - self.alpha {
                // Voting NO (below inverse threshold)
                if !state.preference {
                    state.confidence += 1;
                } else {
                    state.preference = false;
                    state.confidence = 1;
                }
            } else {
                // In the uncertain zone, reset confidence
                state.confidence = 0;
            }

            // Check for finality
            if state.confidence >= self.threshold {
                state.decided = true;
                state.decision = if state.preference {
                    Decision::Accept
                } else {
                    Decision::Reject
                };
                return true;
            }

            false
        }

        /// Get state for an item
        pub fn state(&self, id: &ID) -> Option<&FocusState> {
            self.states.get(id)
        }

        /// Check if item has reached finality
        pub fn is_decided(&self, id: &ID) -> bool {
            self.states.get(id).map_or(false, |s| s.decided)
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
            self.states.entry(block_id.clone()).or_insert_with(WaveState::default)
        }

        /// Record a vote and check for consensus
        ///
        /// Returns true if decision was just reached
        pub fn record_vote(&mut self, vote: Vote) -> bool {
            let block_id = vote.block_id.clone();

            let state = self.states.entry(block_id.clone())
                .or_insert_with(WaveState::default);

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
            // A round is counted only once the sample is full, and the phase
            // advances with the round — never with the vote. Go advances at
            // `countVotes`, past the same gate. Advancing per vote ran the phase
            // k times faster, so two nodes that saw one round each read θ from
            // different phases and applied different accept counts to the same
            // votes.
            let decided_or_short = match self.states.get(block_id) {
                Some(s) => s.decided || s.yes_count + s.no_count < self.config.k,
                None => true,
            };
            if decided_or_short {
                return false;
            }

            let threshold = self.advance_round();

            let state = match self.states.get_mut(block_id) {
                Some(s) => s,
                None => return false,
            };

            // Check for quorum
            if state.yes_count >= threshold {
                if state.preference {
                    state.confidence += 1;
                } else {
                    state.preference = true;
                    state.confidence = 1;
                }
            } else if state.no_count >= threshold {
                if !state.preference {
                    state.confidence += 1;
                } else {
                    state.preference = false;
                    state.confidence = 1;
                }
            } else {
                state.confidence = 0;
            }

            // Check for finality (β consecutive rounds)
            if state.confidence >= self.config.beta {
                state.decided = true;
                state.decision = if state.preference {
                    Decision::Accept
                } else {
                    Decision::Reject
                };
                return true;
            }

            false
        }

        /// Open the next round and return the vote count it accepts on.
        ///
        /// Under FPC the count is θ(phase)·k rounded up, and the phase is the
        /// round number — so this both advances and reads, and there is no way
        /// to read the threshold without having counted a round. Without FPC the
        /// count is the configured α, fixed for every round.
        pub fn advance_round(&mut self) -> usize {
            if let Some(ref fpc) = self.fpc {
                self.phase += 1;
                fpc.select_threshold(self.phase, self.config.k)
            } else {
                self.config.alpha_count()
            }
        }

        /// The vote count the current round accepts on, without opening a new
        /// one. For status and metrics; the round itself reads its count from
        /// [`Wave::advance_round`].
        pub fn threshold(&self) -> usize {
            match self.fpc {
                Some(ref fpc) => fpc.select_threshold(self.phase, self.config.k),
                None => self.config.alpha_count(),
            }
        }

        /// Get state for a block
        pub fn state(&self, block_id: &ID) -> Option<&WaveState> {
            self.states.get(block_id)
        }

        /// Check if block is decided
        pub fn is_decided(&self, block_id: &ID) -> bool {
            self.states.get(block_id).map_or(false, |s| s.decided)
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

    use crate::bls;
    use crate::finality::{Cert, Refusal};

    /// Validator in the Quasar consensus
    #[derive(Debug, Clone)]
    pub struct Validator {
        pub id: NodeID,
        pub weight: u64,
        pub active: bool,
        /// A compressed G1 point, 48 bytes. The comment here read "96 bytes",
        /// which is a signature, not a key.
        pub bls_pubkey: Option<Vec<u8>>,
        pub corona_pubkey: Option<Vec<u8>>,
    }

    /// The validator set, the round threshold, and the certificates this node
    /// has proved.
    ///
    /// Two identity widths meet here and they are not reconciled. The sampler's
    /// `NodeID` is 32 bytes — this crate's `ID` — while a certificate names its
    /// signers in the 20 bytes `luxfi/ids.NodeID` is and the wire carries. So
    /// `add_validator` populates the sampling set and `register_key` populates
    /// the signing set, and nothing converts between them, because there is no
    /// honest conversion: a 32-byte ed25519 key is not a Lux node id. Until the
    /// sampler is moved onto the 20-byte identity the network uses, the two sets
    /// have to be populated from the same source by the caller.
    pub struct QuasarConsensus {
        validators: HashMap<NodeID, Validator>,
        keys: bls::Registry,
        threshold: usize,
        finalized: HashMap<finality::Id, Cert>,
    }

    impl QuasarConsensus {
        /// Create new Quasar consensus
        pub fn new(config: &QuasarConfig) -> Self {
            QuasarConsensus {
                validators: HashMap::new(),
                keys: bls::Registry::new(),
                threshold: config.alpha_count(),
                finalized: HashMap::new(),
            }
        }

        /// Register the signing key a certificate's signatures are checked
        /// against. Refuses a key that is the wrong length, off the curve, off
        /// the subgroup, or the identity — an identity key verifies a signature
        /// over any message at all.
        pub fn register_key(&mut self, node: finality::Node, compressed: &[u8]) -> bool {
            self.keys.insert(node, compressed)
        }

        /// How many signing keys are registered.
        pub fn key_count(&self) -> usize {
            self.keys.len()
        }

        /// Add a validator
        pub fn add_validator(&mut self, id: NodeID, weight: u64) {
            self.validators.insert(id.clone(), Validator {
                id,
                weight,
                active: true,
                bls_pubkey: None,
                corona_pubkey: None,
            });
        }

        /// Add a validator with cryptographic keys
        pub fn add_validator_with_keys(
            &mut self,
            id: NodeID,
            weight: u64,
            bls_pubkey: Option<Vec<u8>>,
            corona_pubkey: Option<Vec<u8>>,
        ) {
            self.validators.insert(id.clone(), Validator {
                id,
                weight,
                active: true,
                bls_pubkey,
                corona_pubkey,
            });
        }

        /// Remove a validator
        pub fn remove_validator(&mut self, id: &NodeID) {
            self.validators.remove(id);
        }

        /// Get validator count
        pub fn validator_count(&self) -> usize {
            self.validators.len()
        }

        /// Check if we have enough validators for consensus
        pub fn has_quorum(&self) -> bool {
            self.validators.len() >= self.threshold
        }

        /// Record a certificate, once every signature in it has verified.
        ///
        /// The certificate carries its own position, and each signature is
        /// checked against the signer's registered key over the message that
        /// position derives — so a certificate is recorded because it was
        /// proved, never because it was well-formed.
        ///
        /// What stood here instead built a certificate out of unverified votes,
        /// filled its signature field from an aggregator that could not decode
        /// its own inputs, and offered a verifier whose only signature test was
        /// that the field was non-empty. The aggregator wrote forty-eight zero
        /// bytes on failure, which is not empty, so it always passed.
        pub fn record(&mut self, cert: Cert) -> std::result::Result<(), Refusal> {
            cert.verify(&self.keys)?;
            self.finalized.insert(cert.position.canonical(), cert);
            Ok(())
        }

        /// Check a certificate against the registered validator keys without
        /// recording it.
        pub fn verify(&self, cert: &Cert) -> std::result::Result<(), Refusal> {
            cert.verify(&self.keys)
        }

        /// Whether a canonical block has a recorded certificate.
        pub fn is_finalized(&self, canonical: &finality::Id) -> bool {
            self.finalized.contains_key(canonical)
        }

        /// The recorded certificate for a canonical block.
        pub fn certificate(&self, canonical: &finality::Id) -> Option<&Cert> {
            self.finalized.get(canonical)
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
            self.chains.entry(chain_id).or_insert_with(Vec::new);
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

        /// Create with default config
        pub fn default() -> Self {
            QuasarEngine::new(QuasarConfig::default())
        }

        /// Create testnet engine
        pub fn testnet() -> Self {
            QuasarEngine::new(QuasarConfig::testnet())
        }

        /// Create mainnet engine
        pub fn mainnet() -> Self {
            QuasarEngine::new(QuasarConfig::mainnet())
        }

        /// Add a validator
        pub fn add_validator(&mut self, id: NodeID, weight: u64) {
            self.quasar.add_validator(id, weight);
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

            // No certificate is issued here, and that is deliberate. A
            // certificate binds a position — a chain id, a round, an execution
            // state root, a validator set root — and this engine's block carries
            // an id, a parent, a height and a payload. Nothing it can assemble
            // is a statement the network signs, so what stood here manufactured
            // a finality artifact out of a position it did not have and votes it
            // had not checked. A caller holding a real position and the signing
            // keys assembles the certificate; see `finality::Cert` and
            // `QuasarConsensus::record`.
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
                .map_or(false, |s| *s == Status::Accepted)
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

    // Legacy Chain type alias for backward compatibility
    pub type Chain = QuasarEngine;
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
            assert!(theta >= 0.5 && theta <= 0.8, "theta {} out of range", theta);
        }
    }

    #[test]
    fn test_fpc_threshold() {
        let fpc = FpcSelector::new(0.5, 0.8, *b"test-seed-0000000000000000000000");
        let k = 20;

        let threshold = fpc.select_threshold(0, k);
        // Should be between ceil(0.5 * 20) = 10 and ceil(0.8 * 20) = 16
        assert!(threshold >= 10 && threshold <= 16);
    }

    #[test]
    fn test_luminance() {
        let config = QuasarConfig::testnet();
        let mut luminance = photon::Luminance::new(&config);

        let node = NodeID::from([1u8; 32]);

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
                NodeID::from([i; 32]),
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
            engine.add_validator(NodeID::from([i; 32]), 1);
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
                NodeID::from([i; 32]),
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
            engine.add_validator(NodeID::from([i; 32]), 1);
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
                    NodeID::from([i; 32]),
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
