// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! Signatures made by Go, checked by Rust.
//!
//! `tests/cert.rs` proves this crate is self-consistent: it signs and then
//! accepts its own signatures. That would hold just as well if the ciphersuite,
//! the domain tag or the point encoding were wrong, so long as they were
//! wrong the same way twice.
//!
//! The vectors here were produced by the Go implementation the network runs —
//! `luxfi/crypto/bls` signing `chain.CanonicalVoteMessage` — so they fail if
//! this crate disagrees with it about the domain separation tag, about which
//! group holds the key and which holds the signature, or about compressed point
//! encoding. It is the check that a Rust node and a Go node accept each other's
//! votes rather than each other's shape.
//!
//! Regenerate with:
//!
//! ```text
//! cd ~/work/lux/consensus && mkdir -p zz_vec && cat > zz_vec/main.go <<'EOF'
//! package main
//! import ("encoding/hex"; "fmt"
//!   "github.com/luxfi/consensus/engine/chain"
//!   "github.com/luxfi/crypto/bls"; "github.com/luxfi/ids")
//! func fill(b byte) ids.ID { var i ids.ID; for k := range i { i[k] = b }; return i }
//! func main() {
//!   pos := chain.VotePosition{ChainID: fill(7), Height: 42, Round: 3,
//!     BlockID: fill(11), ParentID: fill(10), CanonicalID: fill(12),
//!     ParentCanonicalID: fill(13), ExecutionStateRoot: fill(14),
//!     PayloadRoot: fill(15), ValidatorSetRoot: fill(16)}
//!   msg := chain.CanonicalVoteMessage(pos)
//!   fmt.Println(hex.EncodeToString(msg))
//!   for n := 1; n <= 3; n++ {
//!     seed := make([]byte, 32); for i := range seed { seed[i] = byte(n) }
//!     sk, _ := bls.SecretKeyFromSeed(seed); sig, _ := sk.Sign(msg)
//!     fmt.Println(hex.EncodeToString(bls.PublicKeyToCompressedBytes(sk.PublicKey())),
//!       hex.EncodeToString(bls.SignatureToBytes(sig)))
//!   }
//! }
//! EOF
//! go run ./zz_vec && rm -rf zz_vec
//! ```

use blst::min_pk::SecretKey;
use lux_consensus::cert::{NodeId, QuorumCert, ValidatorSet, Vote, VoteVerifier};
use lux_consensus::finality::{canonical_vote_message, Finality, Position};
use lux_consensus::pop;

/// The position the Go program signed over.
fn position() -> Position {
    let f = |b: u8| [b; 32];
    Position {
        chain_id: f(7),
        height: 42,
        round: 3,
        block_id: f(11),
        parent_id: f(10),
        canonical_id: f(12),
        parent_canonical_id: f(13),
        execution_state_root: f(14),
        payload_root: f(15),
        validator_set_root: f(16),
    }
}

/// The exact bytes Go's `chain.CanonicalVoteMessage` produced for that position.
const GO_MESSAGE: &str = "4c55582f636861696e2f766f74652f7632000003010707070707070707070707070707070707070707070707070707070707070707000000000000002a000000030c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f101010101010101010101010101010101010101010101010101010101010101001";

/// Compressed public key and signature per signer, from `SecretKeyFromSeed([n; 32])`.
const GO_SIGNERS: [(&str, &str); 3] = [
    (
        "95a254501b7733239ed3cec4d56737977bd09ede881d8a234560e83e5525017add3b1dcc3eabfb85e12a4131b19c253b",
        "a0888686632c21923517b3cc8c9aa5f7f75b17a952bbe540fc967d3aa6356cd6007bdb5f48a3f4819808bda3d96560520753348e22064400ecc7758c711b22a7985d2e9bcc0a43c019d27d99d07af9eba17328e0552ff2d3bf25470af7b81c58",
    ),
    (
        "ac80a5e08c712d5f08f0306ad743f7d8c215d982489b84a1d6ba805733d94c006e8938f9089a75db3ffa135af33bc69a",
        "889a66f016ec61fe21619024b57ec71b48ff213a4a4012e1823f5ce15513ccf37c1d98c2593e8e4056a75061312d5c2f16662ddc6b2c7d8a7947887414f3c0dae32081624357e41ca3ac44ca1ca9cb2f49542dc308fd395a406332f8105a003d",
    ),
    (
        "96df714a5cc9ddd2298546dce3d6d3827762a6d5b1c2a91e5ca93c9c898b1b4319cc105c493212a55b63080732ec2249",
        "867e94ed647199c69f59adb13464243f7700bab7208709bc9e5f5ff9641e91fa19fda070b34ff6613a584f2c38ecb5801394e97bf92e790b825f5771318203ea7a817c908fc9fb80fddf0076c50225e8997103b5eb15b900c3769d9e25fe2a7d",
    ),
];

/// A validator identity: 20 bytes, the width Go names a node by. The node id
/// enters no signed message, so these vectors are unchanged by its width — the
/// Go message and the Go signatures below are the same bytes they always were.
fn node_id(n: u8) -> NodeId {
    let mut id = [0u8; 20];
    id[0] = n;
    id
}

/// Ids ascending, so the assembled certificate is already canonically ordered.
fn go_set() -> (ValidatorSet, Vec<Vote>) {
    let message = canonical_vote_message(&position(), true);
    let mut set = ValidatorSet::new();
    let mut votes = Vec::new();
    for (i, (pk, sig)) in GO_SIGNERS.iter().enumerate() {
        let id = node_id(i as u8 + 1);
        let pk_bytes = hex::decode(pk).unwrap();

        // Go generated these from `SecretKeyFromSeed([n; 32])` under cgo, which
        // is `blst.KeyGen(seed, info=nil)` with the standard keygen salt —
        // byte-identical to this crate's `key_gen`. Regenerating the key from the
        // seed and getting the same public key confirms that cgo path and yields
        // the secret needed to form the proof of possession registration now
        // requires. (The non-cgo build of `luxfi/crypto` routes through CIRCL
        // with an empty HKDF salt and would derive a different key; these vectors
        // pin the cgo path, which is what Lux ships.)
        let seed = [i as u8 + 1; 32];
        let sk = SecretKey::key_gen(&seed, &[]).expect("key_gen");
        assert_eq!(
            sk.sk_to_pk().compress().to_vec(),
            pk_bytes,
            "signer {i}: Go's SecretKeyFromSeed and this crate's key_gen disagree on the seed",
        );
        let proof = pop::sign(&sk, &id, &pk_bytes);

        set.insert(id, 100, &pk_bytes, &proof)
            .expect("Go public key rejected at registration");
        let signature = hex::decode(sig).unwrap();
        assert!(
            set.verify_vote(&id, &message, &signature, 0),
            "signer {i}: Go's signature did not verify here",
        );
        votes.push(Vote {
            node_id: id,
            accept: true,
            signature,
        });
    }
    (set, votes)
}

/// The message this crate builds is the message Go signed, byte for byte. If
/// this fails nothing below can pass, and the failure is in the encoding rather
/// than the cryptography.
#[test]
fn the_message_is_the_one_go_signed() {
    assert_eq!(
        hex::encode(canonical_vote_message(&position(), true)),
        GO_MESSAGE,
    );
}

/// A Go public key is a valid group element here, and a Go signature over the
/// Go message verifies under it. This pins the ciphersuite: the tag
/// `BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_`, the key in G1 compressed to
/// 48 bytes, the signature in G2 compressed to 96.
#[test]
fn go_signatures_verify_here() {
    let (_set, votes) = go_set();
    assert_eq!(votes.len(), 3);
}

/// The whole predicate over a certificate whose every signature was made by the
/// Go implementation. This is a Rust node accepting a block that Go validators
/// certified.
#[test]
fn a_go_signed_certificate_is_accepted() {
    let (set, votes) = go_set();
    let cert = QuorumCert::assemble(Finality::Quasar, position(), 3, &votes).expect("assemble");

    assert_eq!(cert.verify(&set, 0), Ok(()));
    // 300 of 300 staked, strictly above floor(2·300/3) = 200.
    assert_eq!(cert.verify_weighted(&set, &set, 0), Ok(()));
}

/// Go's signatures are checked one at a time here, because that is the only
/// form Go produces: `engine/chain/cert.go` — "there is no aggregate field,
/// because nothing is aggregated". A Rust node that accepted an aggregate would
/// be accepting evidence this network cannot emit, under a rule Go cannot check.
///
/// And each signature is bound to this exact message: one bit elsewhere and it
/// is not a proof.
#[test]
fn each_go_signature_is_checked_on_its_own() {
    let (set, votes) = go_set();
    let message = canonical_vote_message(&position(), true);

    for v in &votes {
        assert!(set.verify_vote(&v.node_id, &message, &v.signature, 0));
    }

    let mut other = message.clone();
    other[225] ^= 0x01; // the accept byte
    for v in &votes {
        assert!(!set.verify_vote(&v.node_id, &other, &v.signature, 0));
    }
}

/// A Go signature is bound to the position Go signed it over. Presenting it
/// under any other position fails, so a Rust node cannot be made to accept a
/// block by replaying real votes from a different one.
#[test]
fn a_go_signature_does_not_travel_to_another_position() {
    let (set, votes) = go_set();

    let mut elsewhere = position();
    elsewhere.height += 1;

    let cert = QuorumCert::assemble(Finality::Quasar, elsewhere, 3, &votes).expect("assemble");
    assert!(cert.verify(&set, 0).is_err());
}

/// The domain tag itself, frozen against the network's own constant.
///
/// Every other test in this file proves conformance *empirically*: a signature
/// Go made verifies here. That check is only as good as the vectors, and the
/// header above says how to regenerate them. Change `DST` and rerun that
/// recipe and the whole file goes green again — Go and Rust would agree with
/// each other under a tag the running network never signs under, and a Rust
/// node would reject every real vote on the wire.
///
/// So the tag is pinned as a literal too. This is `dstSignature` in
/// `luxfi/crypto/bls/bls.go`, the value the live network signs consensus votes
/// under. Note the suffix: `_NUL_`, not `_POP_`. `luxfi/crypto/bls` also
/// declares `CiphersuiteSignature = "BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_"`,
/// which is neither the signature tag nor the proof-of-possession tag
/// (`BLS_POP_..._POP_`). Nothing signs under it today — the `Ciphersuite` type
/// has no users outside its own file — but it is a wrong constant sitting next
/// to the right one, and this is the assertion that catches anyone who copies it.
#[test]
fn the_domain_tag_is_the_one_the_network_signs_under() {
    assert_eq!(
        lux_consensus::cert::DST,
        b"BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_",
        "the vote ciphersuite must be luxfi/crypto/bls dstSignature, byte for byte"
    );
}
