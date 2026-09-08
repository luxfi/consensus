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
    verify_over(&message(node, key), key, proof)
}

/// Verify a proof over ANY message, under the same two clauses in the same
/// order: the encodings first, then the possession.
///
/// A node-bound proof is one message among several. A committee entitles a
/// validator ON A CHAIN and signs the chain with it; a caller with a message
/// of its own would otherwise restate these clauses, and a second statement of
/// what a canonical key is is a second answer waiting to differ from this one.
pub fn verify_over(message: &[u8], key: &[u8], proof: &[u8]) -> Result<(), PopError> {
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
    if sig.verify(true, message, POP_DST, &[], &pk, true) != BLST_ERROR::BLST_SUCCESS {
        return Err(PopError::Possession);
    }
    Ok(())
}

/// Produce a node-bound proof for `node` under `secret`. The signing side of
/// [`verify`]; used by registrants and by tests.
pub fn sign(secret: &blst::min_pk::SecretKey, node: &NodeId, key: &[u8]) -> Vec<u8> {
    sign_over(secret, &message(node, key))
}

/// Produce a proof over ANY message, under the same domain tag. The signing
/// side of [`verify_over`], and the one place the tag is applied.
pub fn sign_over(secret: &blst::min_pk::SecretKey, message: &[u8]) -> Vec<u8> {
    secret.sign(message, POP_DST, &[]).compress().to_vec()
}

#[cfg(test)]
mod general_message_tests {
    use super::*;

    fn keypair() -> (blst::min_pk::SecretKey, Vec<u8>) {
        let sk = blst::min_pk::SecretKey::key_gen(&[7u8; 32], &[]).expect("key");
        let pk = sk.sk_to_pk().compress().to_vec();
        (sk, pk)
    }

    /// The node-bound pair is the general pair with one message. If these ever
    /// disagree, a registration and a committee are checking different rules.
    #[test]
    fn the_node_bound_proof_is_the_general_one_over_its_own_message() {
        let (sk, pk) = keypair();
        let node: NodeId = [3u8; NODE_LEN];
        let bound = sign(&sk, &node, &pk);
        let general = sign_over(&sk, &message(&node, &pk));
        assert_eq!(bound, general, "one proof, written two ways");
        assert!(verify(&node, &pk, &general).is_ok());
        assert!(verify_over(&message(&node, &pk), &pk, &bound).is_ok());
    }

    /// A proof for one message does not hold over another — which is the whole
    /// reason a committee signs the chain it entitles a validator on.
    #[test]
    fn a_proof_does_not_carry_to_another_message() {
        let (sk, pk) = keypair();
        let node: NodeId = [3u8; NODE_LEN];
        let here = [b"chain-a".as_slice(), &node, &pk].concat();
        let elsewhere = [b"chain-b".as_slice(), &node, &pk].concat();
        let proof = sign_over(&sk, &here);
        assert!(verify_over(&here, &pk, &proof).is_ok());
        assert_eq!(
            verify_over(&elsewhere, &pk, &proof),
            Err(PopError::Possession),
            "a proof made for one chain entitles nothing on another"
        );
    }

    /// The encoding clauses are the same clauses, in the same order: a bad key
    /// is a bad key before any pairing runs.
    #[test]
    fn the_encoding_clauses_come_first_either_way() {
        let (sk, pk) = keypair();
        let msg = b"anything".as_slice();
        let proof = sign_over(&sk, msg);
        assert_eq!(
            verify_over(msg, &pk[..KEY_LEN - 1], &proof),
            Err(PopError::Key)
        );
        assert_eq!(
            verify_over(msg, &pk, &proof[..PROOF_LEN - 1]),
            Err(PopError::Proof)
        );
    }
}
