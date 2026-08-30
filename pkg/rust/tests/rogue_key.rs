// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The rogue-key forgery, and why it now buys nothing.
//!
//! The attacker can compute
//!
//! ```text
//!     pk_x = g1·x − (pk_a + pk_b + pk_c)
//! ```
//!
//! so that the four keys sum to `g1·x`, a key the attacker alone holds the
//! secret for. Against any verifier that sums the signers' keys and checks one
//! signature against the sum, one signature made with `x` is then a certificate
//! naming four signers of whom three never saw the block. That is the Rogue Key
//! Attack (Boneh–Drijvers–Neven 2018, §2); its standard defences are proof of
//! possession and distinct-message aggregation.
//!
//! This crate closes it at both ends. `ValidatorSet::insert` demands a proof of
//! possession — a signature the key alone can make over its own (node, key) —
//! so `pk_x`, whose secret the attacker does not know, cannot be registered:
//! the rogue is refused at the door. And the accept rule never sums keys — every
//! signature is checked against exactly one public key, the form Go reads — so
//! even a key that somehow entered the set stands for no one but itself. The
//! tests below build exactly `pk_x` and show both: registration refuses it, and
//! its holder cannot sign under it.

use blst::min_pk::{PublicKey, SecretKey};
use blst::{
    blst_p1, blst_p1_add_or_double, blst_p1_affine, blst_p1_affine_compress, blst_p1_cneg,
    blst_p1_from_affine, blst_p1_to_affine, blst_p1_uncompress, BLST_ERROR,
};

use lux_consensus::cert::{
    pop_message, CertError, QuorumCert, ValidatorSet, Vote, VoteVerifier, DST, POP_DST,
};
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

    /// A genuine proof of possession — this validator holds the secret, so it
    /// can sign its own (node, key) under the proof-of-possession DST.
    fn pop(&self) -> Vec<u8> {
        self.sk
            .sign(&pop_message(&self.id, &self.pk()), POP_DST, &[])
            .compress()
            .to_vec()
    }
}

/// The attacker: a registered key it cannot sign under, and a secret whose real
/// public key is the sum of everyone's.
struct Rogue {
    id: Id,
    pk: [u8; 48],
    sk: SecretKey,
}

impl Rogue {
    /// The best proof of possession the attacker can produce for the key it
    /// registered: a signature by the only secret it holds. That secret's public
    /// key is the SUM (`target`), not the cancelling key `pk`, so this proof does
    /// not verify against `pk` — which is exactly why possession cannot be faked.
    fn forged_pop(&self) -> Vec<u8> {
        self.sk
            .sign(&pop_message(&self.id, &self.pk), POP_DST, &[])
            .compress()
            .to_vec()
    }
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
        set.insert(h.id, 100, &h.pk(), &h.pop())
            .expect("honest registers");
    }
    // The registration the attack needs no longer succeeds: proof of possession
    // has an opinion about who holds the secret, and the rogue holds none for the
    // key it published. The set the attack must work against therefore contains
    // the honest three and not the rogue — see
    // `the_rogue_key_cannot_register_without_possession`.
    assert_eq!(
        set.insert(rogue.id, 100, &rogue.pk, &rogue.forged_pop()),
        Err(CertError::PopInvalid),
        "the rogue cannot prove possession of a key it does not control"
    );

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
        Signature::uncompress(&forged.compress()).unwrap().verify(
            true,
            &message,
            DST,
            &[],
            &summed,
            true
        ),
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

/// THE POP DEFENSE. The rogue published `g1·x − Σ pk_others` and does not hold
/// that key's secret, so it cannot produce a proof of possession for it. Every
/// proof it can offer is refused, so the key never enters the set — the forgery
/// is stopped one step earlier than the per-signature check, at registration.
#[test]
fn the_rogue_key_cannot_register_without_possession() {
    let honest: Vec<Honest> = (1..=3u8).map(Honest::new).collect();
    let pks: Vec<[u8; 48]> = honest.iter().map(|h| h.pk()).collect();
    let rogue = forge_key(&pks, 4);

    let mut set = ValidatorSet::new();
    for h in &honest {
        set.insert(h.id, 100, &h.pk(), &h.pop()).expect("honest registers");
    }

    // The best proof the attacker can make — by the one secret it holds, whose
    // public key is the sum, not the registered cancelling key.
    assert_eq!(
        set.insert(rogue.id, 100, &rogue.pk, &rogue.forged_pop()),
        Err(CertError::PopInvalid),
    );
    // No proof at all, and a well-formed proof over the wrong (someone else's)
    // key, are refused just the same.
    assert_eq!(set.insert(rogue.id, 100, &rogue.pk, &[]), Err(CertError::PopInvalid));
    assert_eq!(
        set.insert(rogue.id, 100, &rogue.pk, &honest[0].pop()),
        Err(CertError::PopInvalid),
    );

    // The rogue is not a member, so nothing it signs is even looked up.
    let message = canonical_vote_message(&position(), true);
    let own = rogue.sk.sign(&message, DST, &[]).compress().to_vec();
    assert!(!set.verify_vote(&rogue.id, &message, &own, 0));
}

/// A key belongs to one validator: it cannot be registered under a second id,
/// so `nova_signer_floor` counts distinct signers and not ids one holder mints.
/// This is the other half of the floor's guarantee — proof of possession stops
/// a key you do not hold; uniqueness stops a key you do hold from voting twice.
#[test]
fn one_secret_key_cannot_be_registered_under_two_ids() {
    let h = Honest::new(1);
    let mut set = ValidatorSet::new();
    set.insert(h.id, 100, &h.pk(), &h.pop()).expect("first id");

    // The same key, a second id, and a valid proof of possession for that id —
    // still refused, because the key already belongs to the first.
    let mut second = [0u8; 32];
    second[0] = 9;
    let pop_second = h
        .sk
        .sign(&pop_message(&second, &h.pk()), POP_DST, &[])
        .compress()
        .to_vec();
    assert_eq!(
        set.insert(second, 100, &h.pk(), &pop_second),
        Err(CertError::DuplicateKey),
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
            .add_validator_with_key(NodeID::from(h.id), 100, &h.pk(), &h.pop())
            .expect("register");
    }
    // The rogue is refused here too — the engine registration path verifies the
    // same proof of possession.
    assert!(quasar
        .add_validator_with_key(NodeID::from(rogue.id), 100, &rogue.pk, &rogue.forged_pop())
        .is_err());

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
            .add_validator_with_key(NodeID::from(h.id), 100, &h.pk(), &h.pop())
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

    // Re-label a SIGNED axis: the certificate now names a different block, and
    // every signature it carries stops verifying.
    let mut relabelled = cert.clone();
    relabelled.position.canonical_id = [0xEE; 32];
    assert!(!quasar.verify_certificate(&relabelled));

    let mut higher = cert.clone();
    higher.position.height = 999_999;
    assert!(!quasar.verify_certificate(&higher));

    // Re-label the UNSIGNED transport id: verification still passes, because the
    // transport id is deliberately outside the signature — so finality must NOT
    // be keyed on it. It is keyed on the signed identity, so the relabelled
    // transport id finalizes nothing of its own, and the real signed identity is
    // the one that is finalized.
    let mut transport = cert;
    transport.position.block_id = [0xAB; 32];
    assert!(
        quasar.verify_certificate(&transport),
        "the transport id is unsigned, so mutating it cannot break verification"
    );
    assert!(
        !quasar.is_finalized(&lux_consensus::ID::from([0xAB; 32])),
        "a relabelled transport id finalizes nothing"
    );
    assert!(
        // Pinned to the literal canonical id, not `pos.signed_identity()`: the
        // production keying and the assertion must not move together, or a
        // signed_identity that ignored the canonical would pass its own test.
        quasar.is_finalized(&lux_consensus::ID::from([12u8; 32])),
        "finality is keyed on the signed identity"
    );
}

/// The per-key property the whole no-aggregate design rests on, asserted on the
/// curve and not via a set membership miss: the holder of the cancelling key
/// `g1·x − Σ pk_others` cannot sign under it. It knows a secret for the SUM, not
/// for this key, so no signature it makes verifies — whether or not the key was
/// ever registered. During a partial rollout, a peer without proof-of-possession
/// could still hand this key to a verifier, and per-key verification is then the
/// only layer left; this is that layer, tested directly.
#[test]
fn the_rogue_holder_cannot_sign_under_its_registered_key() {
    use blst::min_pk::Signature;

    let honest: Vec<Honest> = (1..=3u8).map(Honest::new).collect();
    let pks: Vec<[u8; 48]> = honest.iter().map(|h| h.pk()).collect();
    let rogue = forge_key(&pks, 4);

    let message = canonical_vote_message(&position(), true);
    let sig = rogue.sk.sign(&message, DST, &[]); // the only secret it holds
    let pk = PublicKey::uncompress(&rogue.pk).expect("the cancelling key is a valid point");

    assert_eq!(
        Signature::uncompress(&sig.compress())
            .unwrap()
            .verify(true, &message, DST, &[], &pk, true),
        BLST_ERROR::BLST_VERIFY_FAIL,
        "no signature the attacker can make verifies under the cancelling key",
    );
}

/// The proof binds the node id, not the key alone: a proof made for one node
/// does not register the same (still-unowned) key under another id. Drop the
/// node from the proof message and this passes — so it pins that the node is
/// bound, which is the defence for a victim not yet in the set.
#[test]
fn a_proof_for_one_node_does_not_register_the_key_under_another() {
    let h = Honest::new(1);
    let mut other = [0u8; 32];
    other[0] = 2;

    let mut set = ValidatorSet::new();
    // h's own proof, over (h.id, h.pk), offered for a different id and the same
    // key — which no one owns yet, so this is the proof clause failing, not the
    // duplicate-key clause.
    assert_eq!(
        set.insert(other, 100, &h.pk(), &h.pop()),
        Err(CertError::PopInvalid),
    );
    assert!(set.insert(h.id, 100, &h.pk(), &h.pop()).is_ok());
}

/// A vote-DST signature is not a proof of possession. Collapse the two domains
/// and this passes — so it pins that a signature made to vote cannot be replayed
/// to register.
#[test]
fn a_vote_dst_signature_is_not_a_proof_of_possession() {
    let h = Honest::new(1);
    let vote_dst = h
        .sk
        .sign(&pop_message(&h.id, &h.pk()), DST, &[])
        .compress()
        .to_vec();

    let mut set = ValidatorSet::new();
    assert_eq!(
        set.insert(h.id, 100, &h.pk(), &vote_dst),
        Err(CertError::PopInvalid),
    );
}

/// A point that decodes but is the identity is refused by `key_validate` before
/// any proof is examined. Weaken registration to a bare `uncompress` and this
/// key slips to the proof clause instead — so the error kind, not merely the
/// refusal, is asserted.
#[test]
fn the_identity_key_is_refused_before_the_proof() {
    // Compressed G1 identity: compression and infinity bits set, rest zero.
    let mut identity = [0u8; 48];
    identity[0] = 0xC0;

    let h = Honest::new(1);
    let mut set = ValidatorSet::new();
    assert_eq!(
        set.insert(h.id, 100, &identity, &h.pop()),
        Err(CertError::KeyEncoding),
    );
}

/// `signed_identity` is the canonical id, and the transport id only in the
/// degrade where none is set. Both branches are pinned to literals, so dropping
/// the degrade branch — or always returning the transport id — is caught.
#[test]
fn signed_identity_uses_the_transport_id_only_in_the_degrade() {
    let mut p = position(); // canonical_id = [12; 32], block_id = [11; 32]
    assert_eq!(p.signed_identity(), [12u8; 32], "non-degrade: the canonical id");
    p.canonical_id = [0u8; 32]; // EMPTY
    assert_eq!(p.signed_identity(), [11u8; 32], "degrade: the transport id");
}
