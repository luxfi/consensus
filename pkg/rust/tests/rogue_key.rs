// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The rogue-key forgery against the aggregate certificate path.
//!
//! `ValidatorSet::verify_aggregate` sums the signers' public keys and checks one
//! signature against the sum. That is only sound if every registered key is one
//! whose owner proved possession of the matching secret. Nothing here demands
//! that: `insert` runs `PublicKey::key_validate`, which checks the encoding, the
//! subgroup and non-identity — all properties a chosen key can satisfy — and
//! never asks the registrant to sign anything.
//!
//! So an attacker registers
//!
//! ```text
//!     pk_x = g1·x − (pk_a + pk_b + pk_c)
//! ```
//!
//! and the sum of all four keys collapses to `g1·x`, a key the attacker holds
//! the secret for alone. One signature made with `x` verifies against that sum,
//! and the certificate names four signers of whom three never saw the block.
//!
//! The construction is Rogue Key Attack, Boneh–Drijvers–Neven 2018, §2; the two
//! standard defences are proof of possession (RFC 9380 / draft-irtf-cfrg-bls-
//! signature POP) and distinct-message aggregation. Neither is present here.

use blst::min_pk::{PublicKey, SecretKey, Signature};
use blst::{
    blst_p1, blst_p1_add_or_double, blst_p1_affine, blst_p1_affine_compress, blst_p1_cneg,
    blst_p1_from_affine, blst_p1_to_affine, blst_p1_uncompress, BLST_ERROR,
};

use lux_consensus::cert::{ValidatorSet, DST};
use lux_consensus::finality::{canonical_vote_message, Id, Position};
use lux_consensus::quasar::QuasarConsensus;
use lux_consensus::{Certificate, NodeID, QuasarConfig, ID};

/// The compressed G1 sum of `keys`, negated when `negate` is set.
///
/// Plain point arithmetic on the curve public keys already live on. Nothing
/// secret is used and nothing exotic is done: this is the same addition
/// `AggregatePublicKey::aggregate` performs, run by the attacker instead of by
/// the verifier.
fn sum_g1(keys: &[[u8; 48]], negate: bool) -> [u8; 48] {
    unsafe {
        let mut acc = blst_p1::default();
        let mut started = false;
        for k in keys {
            let mut aff = blst_p1_affine::default();
            assert_eq!(
                blst_p1_uncompress(&mut aff, k.as_ptr()),
                BLST_ERROR::BLST_SUCCESS,
                "the attacker can read a published key"
            );
            let mut p = blst_p1::default();
            blst_p1_from_affine(&mut p, &aff);
            if started {
                let prev = acc;
                blst_p1_add_or_double(&mut acc, &prev, &p);
            } else {
                acc = p;
                started = true;
            }
        }
        if negate {
            blst_p1_cneg(&mut acc, true);
        }
        let mut out_aff = blst_p1_affine::default();
        blst_p1_to_affine(&mut out_aff, &acc);
        let mut out = [0u8; 48];
        blst_p1_affine_compress(out.as_mut_ptr(), &out_aff);
        out
    }
}

/// A validator: an id, a key it never uses, and its published public key.
struct Honest {
    id: Id,
    pk: [u8; 48],
}

fn honest(n: u8) -> Honest {
    let sk = SecretKey::key_gen(&[n; 32], &[]).expect("key_gen");
    let mut id = [0u8; 32];
    id[0] = n;
    Honest { id, pk: sk.sk_to_pk().compress() }
}

fn position() -> Position {
    Position {
        chain_id: [7u8; 32],
        height: 42,
        round: 3,
        block_id: [11u8; 32],
        parent_id: [10u8; 32],
        canonical_id: [12u8; 32],
        parent_canonical_id: [13u8; 32],
        execution_state_root: [14u8; 32],
        payload_root: [15u8; 32],
        validator_set_root: [16u8; 32],
    }
}

/// The attacker's registered key and the signature it makes on its own.
struct Rogue {
    id: Id,
    pk: [u8; 48],
    sk: SecretKey,
}

/// Build the rogue key that cancels `others` out of the aggregate.
fn forge_key(others: &[[u8; 48]], id_byte: u8) -> Rogue {
    let sk = SecretKey::key_gen(&[0xA0 | id_byte; 32], &[]).expect("key_gen");
    let target = sk.sk_to_pk().compress();
    let minus_others = sum_g1(others, true);
    let pk = sum_g1(&[target, minus_others], false);
    let mut id = [0u8; 32];
    id[0] = id_byte;
    Rogue { id, pk, sk }
}

// ------------------------------------------------------- the forgery, at the set

/// THE FORGERY. Three honest validators never signed anything, and a
/// certificate naming all four of them verifies.
#[test]
fn a_rogue_key_forges_an_aggregate_three_validators_never_signed() {
    let a = honest(1);
    let b = honest(2);
    let c = honest(3);
    let rogue = forge_key(&[a.pk, b.pk, c.pk], 4);

    let mut set = ValidatorSet::new();
    set.insert(a.id, 100, &a.pk).expect("honest a");
    set.insert(b.id, 100, &b.pk).expect("honest b");
    set.insert(c.id, 100, &c.pk).expect("honest c");

    // The registration the attack needs. `key_validate` checks the encoding, the
    // subgroup and non-identity; a rogue key satisfies all three, so this
    // succeeds and the attack is already won.
    set.insert(rogue.id, 100, &rogue.pk)
        .expect("a rogue key registers");

    let message = canonical_vote_message(&position(), true);

    // One signature, made by the attacker alone.
    let forged = rogue.sk.sign(&message, DST, &[]).compress().to_vec();

    let signers = [a.id, b.id, c.id, rogue.id];
    assert!(
        !set.verify_aggregate(&signers, &message, &forged),
        "a certificate naming four signers verified against one attacker's signature"
    );
}

/// The forgery is not an artefact of the honest keys never being used: the same
/// three validators' real signatures verify individually, so they are ordinary
/// members of the set, and it is the aggregate rule alone that fails.
#[test]
fn the_same_set_verifies_the_honest_validators_individually() {
    use lux_consensus::cert::VoteVerifier;

    let sk = SecretKey::key_gen(&[1u8; 32], &[]).expect("key_gen");
    let a = honest(1);
    let mut set = ValidatorSet::new();
    set.insert(a.id, 100, &a.pk).expect("honest a");

    let message = canonical_vote_message(&position(), true);
    let sig = sk.sign(&message, DST, &[]).compress().to_vec();
    assert!(set.verify_vote(&a.id, &message, &sig, 42));
}

// ------------------------------------------------ the forgery, through the engine

/// The same forgery through the public certificate predicate. Nothing in this
/// test touches an internal: a certificate is built by hand, as a relayed
/// certificate arrives, and `verify_certificate` accepts it.
#[test]
fn a_rogue_key_forges_a_certificate_through_verify_certificate() {
    let a = honest(1);
    let b = honest(2);
    let c = honest(3);
    let rogue = forge_key(&[a.pk, b.pk, c.pk], 4);

    let mut config = QuasarConfig::mainnet();
    config.k = 4;
    config.alpha = 0.75;
    let mut quasar = QuasarConsensus::new(&config);

    for (id, pk) in [(a.id, a.pk), (b.id, b.pk), (c.id, c.pk), (rogue.id, rogue.pk)] {
        quasar
            .add_validator_with_key(NodeID::from(id), 100, &pk)
            .expect("register");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let forged = rogue.sk.sign(&message, DST, &[]).compress().to_vec();

    // Strictly increasing by id, as the predicate requires.
    let mut ids = [a.id, b.id, c.id, rogue.id];
    ids.sort();

    let cert = Certificate {
        block_id: ID::from(pos.block_id),
        height: pos.height,
        position: pos,
        signers: ids.iter().map(|i| NodeID::from(*i)).collect(),
        aggregated_sig: forged,
        quantum_sigs: Vec::new(),
        timestamp: std::time::SystemTime::now(),
    };

    assert!(
        !quasar.verify_certificate(&cert),
        "verify_certificate accepted a certificate three of its four signers never signed"
    );
}

// --------------------------------------------------- the unbound header fields

/// `Certificate.block_id` and `Certificate.height` are not covered by any
/// signature. The message is rebuilt from `position` alone, so both header
/// fields can be set to anything and the certificate still verifies — an
/// honestly signed certificate is re-labelled to a block nobody voted on.
#[test]
fn the_certificate_header_is_bound_to_what_was_signed() {
    let ikm: Vec<SecretKey> = (1..=4u8)
        .map(|i| SecretKey::key_gen(&[i; 32], &[]).expect("key_gen"))
        .collect();

    let mut config = QuasarConfig::mainnet();
    config.k = 4;
    config.alpha = 0.75;
    let mut quasar = QuasarConsensus::new(&config);

    let mut ids: Vec<Id> = Vec::new();
    for (n, sk) in ikm.iter().enumerate() {
        let mut id = [0u8; 32];
        id[0] = n as u8 + 1;
        ids.push(id);
        quasar
            .add_validator_with_key(NodeID::from(id), 100, &sk.sk_to_pk().compress())
            .expect("register");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);

    let sigs: Vec<Signature> = ikm
        .iter()
        .map(|sk| sk.sign(&message, DST, &[]))
        .collect();
    let refs: Vec<&Signature> = sigs.iter().collect();
    let agg = blst::min_pk::AggregateSignature::aggregate(&refs, false)
        .expect("aggregate")
        .to_signature()
        .compress()
        .to_vec();

    // Everything signed and honest, except the two header fields.
    let cert = Certificate {
        block_id: ID::from([0xEE; 32]),
        height: 999_999,
        position: pos,
        signers: ids.iter().map(|i| NodeID::from(*i)).collect(),
        aggregated_sig: agg,
        quantum_sigs: Vec::new(),
        timestamp: std::time::SystemTime::now(),
    };

    assert!(
        !quasar.verify_certificate(&cert),
        "a certificate verified while claiming a block and a height nobody signed"
    );
}

/// A sanity check on the arithmetic: the rogue key is a real, well-formed,
/// subgroup-correct public key, and the sum of the four keys really is the
/// attacker's target. Without this the failures above could be an artefact of a
/// malformed key rather than the forgery.
#[test]
fn the_rogue_key_is_well_formed_and_the_sum_is_the_attackers_key() {
    let a = honest(1);
    let b = honest(2);
    let c = honest(3);
    let rogue = forge_key(&[a.pk, b.pk, c.pk], 4);

    // blst accepts it: encoding, subgroup, not the identity.
    PublicKey::key_validate(&rogue.pk).expect("the rogue key passes key_validate");

    let sum = sum_g1(&[a.pk, b.pk, c.pk, rogue.pk], false);
    assert_eq!(
        sum,
        rogue.sk.sk_to_pk().compress(),
        "the four keys sum to the attacker's own public key"
    );
}
