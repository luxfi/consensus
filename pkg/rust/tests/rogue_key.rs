// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The rogue-key forgery, and why it now buys nothing.
//!
//! Registration proves possession of nothing. `ValidatorSet::insert` runs
//! `PublicKey::key_validate`, which checks the encoding, the subgroup and
//! non-identity — every one of them a property a *chosen* key can satisfy — and
//! never asks the registrant to sign anything. So an attacker can register
//!
//! ```text
//!     pk_x = g1·x − (pk_a + pk_b + pk_c)
//! ```
//!
//! and the four keys sum to `g1·x`, a key the attacker alone holds the secret
//! for. Against any verifier that sums the signers' keys and checks one
//! signature against the sum, one signature made with `x` is then a certificate
//! naming four signers of whom three never saw the block. That is the Rogue Key
//! Attack (Boneh–Drijvers–Neven 2018, §2); its standard defences are proof of
//! possession and distinct-message aggregation.
//!
//! This crate takes the third option, which is the one Go already takes: it
//! never sums keys. Every signature is checked against exactly one public key.
//! The rogue key below still registers — nothing here prevents it — and it is
//! worthless twice over. It cannot make the honest three appear to have signed,
//! and it cannot even cast its own vote, because its owner does not know a
//! secret for the key it published.

use blst::min_pk::{PublicKey, SecretKey};
use blst::{
    blst_p1, blst_p1_add_or_double, blst_p1_affine, blst_p1_affine_compress, blst_p1_cneg,
    blst_p1_from_affine, blst_p1_to_affine, blst_p1_uncompress, BLST_ERROR,
};

use lux_consensus::cert::{CertError, QuorumCert, ValidatorSet, Vote, VoteVerifier, DST};
use lux_consensus::finality::{canonical_vote_message, Finality, Id, Position};
use lux_consensus::quasar::QuasarConsensus;
use lux_consensus::{NodeID, QuasarConfig};

/// The compressed G1 sum of `keys`, negated when `negate` is set.
///
/// Plain point arithmetic on the curve public keys already live on. Nothing
/// secret is used and nothing exotic is done — this is the same addition an
/// aggregating verifier performs, run by the attacker instead.
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

/// A validator, with the key it signs with.
struct Honest {
    id: Id,
    sk: SecretKey,
}

impl Honest {
    fn new(n: u8) -> Self {
        let sk = SecretKey::key_gen(&[n; 32], &[]).expect("key_gen");
        let mut id = [0u8; 32];
        id[0] = n;
        Honest { id, sk }
    }

    fn pk(&self) -> [u8; 48] {
        self.sk.sk_to_pk().compress()
    }
}

/// The attacker: a registered key it cannot sign under, and a secret whose real
/// public key is the sum of everyone's.
struct Rogue {
    id: Id,
    pk: [u8; 48],
    sk: SecretKey,
}

/// Build the rogue key that cancels `others` out of a sum.
fn forge_key(others: &[[u8; 48]], id_byte: u8) -> Rogue {
    let sk = SecretKey::key_gen(&[0xA0 | id_byte; 32], &[]).expect("key_gen");
    let target = sk.sk_to_pk().compress();
    let minus_others = sum_g1(others, true);
    let pk = sum_g1(&[target, minus_others], false);
    let mut id = [0u8; 32];
    id[0] = id_byte;
    Rogue { id, pk, sk }
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

/// The three honest validators, the attacker, and a set holding all four.
fn stage() -> (Vec<Honest>, Rogue, ValidatorSet) {
    let honest: Vec<Honest> = (1..=3u8).map(Honest::new).collect();
    let pks: Vec<[u8; 48]> = honest.iter().map(|h| h.pk()).collect();
    let rogue = forge_key(&pks, 4);

    let mut set = ValidatorSet::new();
    for h in &honest {
        set.insert(h.id, 100, &h.pk()).expect("honest registers");
    }
    // The registration the attack needs, and it succeeds: `key_validate` has no
    // opinion about who holds the secret.
    set.insert(rogue.id, 100, &rogue.pk)
        .expect("a rogue key registers");

    (honest, rogue, set)
}

// ------------------------------------------------------------------ the arithmetic

/// The rogue key is real: a well-formed, subgroup-correct, non-identity public
/// key, and the four keys really do sum to the attacker's own. Without this the
/// refusals below could be an artefact of a malformed key rather than the
/// forgery being refused.
#[test]
fn the_rogue_key_is_well_formed_and_the_four_keys_sum_to_the_attackers() {
    let (honest, rogue, _) = stage();

    PublicKey::key_validate(&rogue.pk).expect("the rogue key passes key_validate");

    let mut all: Vec<[u8; 48]> = honest.iter().map(|h| h.pk()).collect();
    all.push(rogue.pk);
    assert_eq!(
        sum_g1(&all, false),
        rogue.sk.sk_to_pk().compress(),
        "the four keys sum to the attacker's own public key"
    );
}

/// And the sum really is signable: the attacker's one signature verifies against
/// the summed key. This is the forgery, intact, with nothing in this crate left
/// to present it to.
#[test]
fn the_attackers_signature_verifies_against_the_summed_key() {
    use blst::min_pk::Signature;

    let (honest, rogue, _) = stage();
    let message = canonical_vote_message(&position(), true);
    let forged = rogue.sk.sign(&message, DST, &[]);

    let mut all: Vec<[u8; 48]> = honest.iter().map(|h| h.pk()).collect();
    all.push(rogue.pk);
    let summed = PublicKey::uncompress(&sum_g1(&all, false)).expect("the sum is a key");

    assert_eq!(
        Signature::uncompress(&forged.compress())
            .unwrap()
            .verify(true, &message, DST, &[], &summed, true),
        BLST_ERROR::BLST_SUCCESS,
        "one signature stands for all four keys — which is why nothing may sum them"
    );
}

// -------------------------------------------------------------- the refusals

/// THE REGRESSION. The attacker's single signature, presented as the vote of
/// each named signer, is refused — at the first vote, by name.
#[test]
fn the_forged_signature_is_refused_vote_by_vote() {
    let (honest, rogue, set) = stage();
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let forged = rogue.sk.sign(&message, DST, &[]).compress().to_vec();

    let mut votes: Vec<Vote> = honest
        .iter()
        .map(|h| Vote {
            node_id: h.id,
            accept: true,
            signature: forged.clone(),
        })
        .collect();
    votes.push(Vote {
        node_id: rogue.id,
        accept: true,
        signature: forged,
    });

    let cert = QuorumCert::assemble(Finality::Quasar, pos, 3, &votes).expect("assemble");
    assert_eq!(cert.verify(&set, 0), Err(CertError::SigInvalid(0)));
}

/// The rogue registrant cannot even cast its own vote. It published
/// `g1·x − Σ pk_others` and does not know that key's secret, so there is no
/// signature it can produce that verifies under it. Registering a rogue key
/// costs the attacker its own ballot.
#[test]
fn the_rogue_registrant_cannot_sign_for_itself() {
    let (_, rogue, set) = stage();
    let message = canonical_vote_message(&position(), true);

    let own = rogue.sk.sign(&message, DST, &[]).compress().to_vec();
    assert!(
        !set.verify_vote(&rogue.id, &message, &own, 0),
        "the attacker has no secret for the key it registered"
    );
}

/// Through the public engine predicate, with a set that would have accepted the
/// forged aggregate: three honest validators sign nothing, and no certificate
/// exists to verify.
#[test]
fn the_engine_issues_nothing_from_a_forged_signature() {
    use lux_consensus::{ConsensusError, VoteType};

    let (honest, rogue, _) = stage();

    let mut config = QuasarConfig::testnet();
    config.k = 4;
    config.alpha = 0.75; // threshold 3
    let mut quasar = QuasarConsensus::new(&config);
    for h in &honest {
        quasar
            .add_validator_with_key(NodeID::from(h.id), 100, &h.pk())
            .expect("register");
    }
    quasar
        .add_validator_with_key(NodeID::from(rogue.id), 100, &rogue.pk)
        .expect("a rogue key registers here too");

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let forged = rogue.sk.sign(&message, DST, &[]).compress().to_vec();

    let ballots: Vec<lux_consensus::Vote> = honest
        .iter()
        .map(|h| h.id)
        .chain(std::iter::once(rogue.id))
        .map(|id| {
            lux_consensus::Vote::new(
                lux_consensus::ID::from(pos.block_id),
                VoteType::Preference,
                NodeID::from(id),
            )
            .with_signature(forged.clone())
        })
        .collect();

    assert!(matches!(
        quasar.create_certificate(pos, &ballots),
        Err(ConsensusError::NoQuorum)
    ));
}

// ------------------------------------------------- the header that used to float

/// A certificate makes exactly one claim — its position — and every part of it
/// is signed. The type that preceded this one carried `block_id` and `height`
/// beside the position, covered by no signature, so an honestly signed
/// certificate could be re-labelled to any block and still verify.
///
/// There is now no second field to disagree, and the only way to change what a
/// certificate names is to change the position every signature was made over,
/// which invalidates all of them.
#[test]
fn a_certificate_names_only_what_was_signed() {
    let honest: Vec<Honest> = (1..=4u8).map(Honest::new).collect();
    let mut config = QuasarConfig::testnet();
    config.k = 4;
    config.alpha = 0.75;
    let mut quasar = QuasarConsensus::new(&config);
    for h in &honest {
        quasar
            .add_validator_with_key(NodeID::from(h.id), 100, &h.pk())
            .expect("register");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let ballots: Vec<lux_consensus::Vote> = honest
        .iter()
        .map(|h| {
            lux_consensus::Vote::new(
                lux_consensus::ID::from(pos.block_id),
                lux_consensus::VoteType::Preference,
                NodeID::from(h.id),
            )
            .with_signature(h.sk.sign(&message, DST, &[]).compress().to_vec())
        })
        .collect();

    let cert = quasar
        .create_certificate(pos.clone(), &ballots)
        .expect("certificate");
    assert!(quasar.verify_certificate(&cert));
    assert_eq!(cert.position, pos, "the position is the whole claim");

    // Re-label it: the certificate now names a different block, and every
    // signature it carries stops verifying.
    let mut relabelled = cert.clone();
    relabelled.position.canonical_id = [0xEE; 32];
    assert!(!quasar.verify_certificate(&relabelled));

    let mut higher = cert;
    higher.position.height = 999_999;
    assert!(!quasar.verify_certificate(&higher));
}
