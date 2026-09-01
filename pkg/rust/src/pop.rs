// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! Node-bound proof of possession — the Rust side of the standard the Go oracle
//! froze in `luxfi/conformance/vectors/pop.json`.
//!
//! A registrant proves it holds the key it registers AND that the key names the
//! node it registers it for. The pubkey-only IETF proof binds nothing but the
//! key, so it travels: an honest validator's published key and proof re-register
//! under a second identity. Binding the node closes that — a proof is valid only
//! for the one (node, key) pair it was made for.
//!
//! THE MESSAGE, byte for byte, identical in Go, Rust and C++:
//!
//! ```text
//!   offset  0 .. 19   node   — the 20-byte NodeID
//!   offset 20 .. 67   key    — compressed G1 pubkey, 48 bytes
//!                     total                          68 bytes
//! ```
//!
//! No separator, no length prefix — both fields are fixed width. The node is the
//! raw 20-byte identity a Lux validator carries, never the 32-byte block id.
//!
//! THE CIPHERSUITE is BLS12-381 `min_pk` under the proof-of-possession domain
//! `BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_` — the `_POP_` tag, never the
//! vote's `..._NUL_`, so a vote is not a proof and a proof is not a vote. Verify
//! order: encoding, then possession.

use blst::min_pk::{PublicKey, Signature};
use blst::BLST_ERROR;

/// The 20-byte validator identity — the same value Go's `ids.NodeID` and C++'s
/// NodeID carry. Distinct from the 32-byte block [`crate::finality::Id`].
pub type NodeId = [u8; 20];

/// The proof-of-possession domain. Distinct from the vote domain by `_POP_`.
pub const POP_DST: &[u8] = b"BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_";

/// Width of the node identity in the message.
pub const NODE_LEN: usize = 20;
/// Width of a compressed BLS12-381 min_pk public key (G1).
pub const KEY_LEN: usize = 48;
/// Width of a compressed BLS12-381 min_pk signature (G2) — the proof.
pub const PROOF_LEN: usize = 96;
/// The whole preimage: node ‖ key.
pub const MESSAGE_LEN: usize = NODE_LEN + KEY_LEN;

/// Why a proof was refused — the same three classes the Go oracle names, so a
/// conforming implementation rejects for the same reason, not merely rejects.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PopError {
    /// The public key is not a canonical compressed BLS12-381 G1 point: wrong
    /// width, non-canonical encoding, off-curve, outside the prime-order
    /// subgroup, or the identity.
    Key,
    /// The proof is not a canonical compressed BLS12-381 G2 point, by the same
    /// measures.
    Proof,
    /// The bytes decode but the proof does not bind this node to this key.
    Possession,
}

/// The exact bytes a node-bound proof signs: node ‖ key, 68 bytes.
pub fn message(node: &NodeId, key: &[u8]) -> Vec<u8> {
    let mut m = Vec::with_capacity(MESSAGE_LEN);
    m.extend_from_slice(node);
    m.extend_from_slice(key);
    m
}

/// Verify a node-bound proof of possession, in the order the standard fixes:
/// encoding, then possession. A port of the Go oracle's `pop.Verify`.
pub fn verify(node: &NodeId, key: &[u8], proof: &[u8]) -> Result<(), PopError> {
    if key.len() != KEY_LEN {
        return Err(PopError::Key);
    }
    let pk = PublicKey::key_validate(key).map_err(|_| PopError::Key)?;
    // One point, one encoding. blst's decode already refuses a non-canonical
    // spelling (x >= p) and a non-subgroup point; re-checking that the point
    // re-compresses to the exact input bytes is the same guard the Go oracle
    // keeps, so a decoder that were ever laxer here is caught.
    if pk.compress().as_slice() != key {
        return Err(PopError::Key);
    }
    if proof.len() != PROOF_LEN {
        return Err(PopError::Proof);
    }
    let sig = Signature::uncompress(proof).map_err(|_| PopError::Proof)?;
    // Decode is not enough: `uncompress` accepts the identity and off-subgroup
    // points, where Go's `SignatureFromBytes` refuses them. Validate the point
    // here so an identity or non-subgroup proof is refused as a bad PROOF — the
    // encoding clause — exactly as the oracle does, not later as a failed pairing.
    sig.validate(true).map_err(|_| PopError::Proof)?;
    let msg = message(node, key);
    if sig.verify(true, &msg, POP_DST, &[], &pk, true) != BLST_ERROR::BLST_SUCCESS {
        return Err(PopError::Possession);
    }
    Ok(())
}

/// Produce a node-bound proof for `node` under `secret`. The signing side of
/// [`verify`]; used by registrants and by tests.
pub fn sign(secret: &blst::min_pk::SecretKey, node: &NodeId, key: &[u8]) -> Vec<u8> {
    secret
        .sign(&message(node, key), POP_DST, &[])
        .compress()
        .to_vec()
}
