// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The certificate predicate, by mutation.
//!
//! A certificate is the only evidence a block was accepted, so the interesting
//! question is never "does a good certificate pass" — it is "does every
//! near-miss fail, and fail for the stated reason". Each test below takes one
//! valid certificate and breaks exactly one thing.
//!
//! Four of these reproduce forgeries that the previous structural check
//! accepted: a certificate carrying 96 zero bytes, one carrying 0xAB repeated,
//! one signed by nobody in the validator set, and one assembled from votes cast
//! over a different position.

use blst::min_pk::SecretKey;
use lux_consensus::cert::{
    CertError, NodeId, QuorumCert, Registration, StakeSource, ValidatorSet, Vote, VoteVerifier,
    DST, SIGNATURE_LEN,
};
use lux_consensus::finality::{
    canonical_vote_message, two_thirds_count, two_thirds_stake_floor, Finality, Position,
};
use lux_consensus::pop;

/// A validator: a key, an id, and a weight.
struct Signer {
    id: NodeId,
    sk: SecretKey,
    weight: u64,
}

impl Signer {
    /// Deterministic per `n`, so a failure reproduces exactly.
    fn new(n: u8, weight: u64) -> Self {
        let ikm = [n; 32];
        let sk = SecretKey::key_gen(&ikm, &[]).expect("key_gen");
        let mut id = [0u8; 20];
        id[0] = n;
        Signer { id, sk, weight }
    }

    fn public(&self) -> [u8; 48] {
        self.sk.sk_to_pk().compress()
    }

    fn sign(&self, message: &[u8]) -> Vec<u8> {
        self.sk.sign(message, DST, &[]).compress().to_vec()
    }

    /// The proof of possession this validator presents at registration: a
    /// signature over its own (node, key) under the proof-of-possession DST.
    fn pop(&self) -> Vec<u8> {
        pop::sign(&self.sk, &self.id, &self.public())
    }

    fn vote(&self, message: &[u8]) -> Vote {
        Vote {
            node_id: self.id,
            accept: true,
            signature: self.sign(message),
        }
    }
}

/// A validator identity from one byte: 20 bytes, the width Go names a node by.
fn node(n: u8) -> NodeId {
    let mut id = [0u8; 20];
    id[0] = n;
    id
}

/// A committee of `n` equal-weight validators, ids ascending.
fn committee(n: u8, weight: u64) -> (Vec<Signer>, ValidatorSet) {
    let signers: Vec<Signer> = (1..=n).map(|i| Signer::new(i, weight)).collect();
    let mut set = ValidatorSet::new();
    for s in &signers {
        set.insert(s.id, s.weight, &s.public(), &s.pop()).expect("insert");
    }
    (signers, set)
}

/// A position with every slot populated, so the canonical-degrade path is not
/// what is under test here.
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

/// A valid Quasar certificate from the first `k` members of the committee.
fn valid_cert(signers: &[Signer], k: usize, threshold: u32) -> QuorumCert {
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<Vote> = signers[..k].iter().map(|s| s.vote(&message)).collect();
    QuorumCert::assemble(Finality::Quasar, pos, threshold, &votes).expect("assemble")
}

// ---------------------------------------------------------------- the happy path

#[test]
fn a_real_certificate_verifies() {
    let (signers, set) = committee(5, 100);
    let cert = valid_cert(&signers, 4, 4);

    assert_eq!(cert.verify(&set, 0), Ok(()));
    // 400 of 500 staked > floor(2·500/3) = 333.
    assert_eq!(cert.verify_weighted(&set, &set, 0), Ok(()));
}

// ------------------------------------------------- the forgeries that used to pass

/// The previous check ended `!cert.aggregated_sig.is_empty()`, and an aggregate
/// of nothing is 48 zero bytes — non-empty, so it passed. A signature of zero
/// is not a signature.
#[test]
fn a_zero_signature_is_refused() {
    let (signers, set) = committee(5, 100);
    let mut cert = valid_cert(&signers, 4, 4);
    for v in &mut cert.votes {
        v.signature = vec![0u8; SIGNATURE_LEN];
    }
    assert_eq!(cert.verify(&set, 0), Err(CertError::SigInvalid(0)));
}

/// 96 bytes of 0xAB — well-formed length, no relationship to any key.
#[test]
fn a_garbage_signature_is_refused() {
    let (signers, set) = committee(5, 100);
    let mut cert = valid_cert(&signers, 4, 4);
    for v in &mut cert.votes {
        v.signature = vec![0xABu8; SIGNATURE_LEN];
    }
    assert_eq!(cert.verify(&set, 0), Err(CertError::SigInvalid(0)));
}

/// Nine anonymous node ids used to drive a block to Accepted against a set with
/// zero validators registered. A voter with no key in the set has nothing to
/// check against, which is exactly as good as a bad signature.
#[test]
fn a_voter_outside_the_set_is_refused() {
    let (signers, _) = committee(5, 100);
    let cert = valid_cert(&signers, 4, 4);

    let empty = ValidatorSet::new();
    assert_eq!(cert.verify(&empty, 0), Err(CertError::SigInvalid(0)));

    // And one stranger among four members fails at that stranger's index.
    let (_, mut set) = committee(5, 100);
    set.remove(&signers[2].id);
    assert_eq!(cert.verify(&set, 0), Err(CertError::SigInvalid(2)));
}

/// Real signatures, real validators — over a different position. The message is
/// derived from the certificate's own position, so votes cannot be lifted from
/// one block onto another.
#[test]
fn votes_signed_over_another_position_are_refused() {
    let (signers, set) = committee(5, 100);

    let mut elsewhere = position();
    elsewhere.height += 1;
    let other_message = canonical_vote_message(&elsewhere, true);

    let votes: Vec<Vote> = signers[..4].iter().map(|s| s.vote(&other_message)).collect();
    let cert = QuorumCert::assemble(Finality::Quasar, position(), 4, &votes).expect("assemble");

    assert_eq!(cert.verify(&set, 0), Err(CertError::SigInvalid(0)));
}

/// An accept signature and a reject signature over one position are distinct
/// messages, so a reject cannot be presented as an accept.
#[test]
fn a_reject_signature_cannot_pass_as_an_accept() {
    let (signers, set) = committee(5, 100);
    let pos = position();
    let reject_message = canonical_vote_message(&pos, false);

    let votes: Vec<Vote> = signers[..4]
        .iter()
        .map(|s| Vote {
            node_id: s.id,
            accept: true, // claims accept
            signature: s.sign(&reject_message), // signed reject
        })
        .collect();
    let cert = QuorumCert::assemble(Finality::Quasar, pos, 4, &votes).expect("assemble");

    assert_eq!(cert.verify(&set, 0), Err(CertError::SigInvalid(0)));
}

// ------------------------------------------------------------- structural clauses

/// One validator counted twice is the cheapest way to manufacture a quorum.
#[test]
fn a_duplicate_voter_is_refused() {
    let (signers, set) = committee(5, 100);
    let mut cert = valid_cert(&signers, 4, 4);
    cert.votes[1] = cert.votes[0].clone();

    assert_eq!(cert.verify(&set, 0), Err(CertError::NotStrictlyIncreasing(1)));
}

/// Order is part of the certificate. Without it the same votes have many
/// encodings, and a cert can be reshaped into a "new" one.
#[test]
fn a_reordered_certificate_is_refused() {
    let (signers, set) = committee(5, 100);
    let mut cert = valid_cert(&signers, 4, 4);
    cert.votes.reverse();

    assert_eq!(cert.verify(&set, 0), Err(CertError::NotStrictlyIncreasing(1)));
}

/// A finality certificate witnesses acceptance. A reject vote inside one is a
/// contradiction, caught before its signature is even considered.
#[test]
fn a_reject_vote_inside_a_finality_cert_is_refused() {
    let (signers, set) = committee(5, 100);
    let mut cert = valid_cert(&signers, 4, 4);
    cert.votes[2].accept = false;

    assert_eq!(cert.verify(&set, 0), Err(CertError::VoteNotAccept(2)));
}

/// The threshold is checked against the count of *verified* votes.
#[test]
fn too_few_votes_is_refused() {
    let (signers, set) = committee(5, 100);
    let mut cert = valid_cert(&signers, 4, 4);
    cert.votes.truncate(3);

    assert_eq!(
        cert.verify(&set, 0),
        Err(CertError::BelowThreshold { have: 3, need: 4 })
    );
}

/// A cert may not name a tier that is not an accept tier. Checked before any
/// signature work, so a wire-decoded garbage tier fails closed on the
/// count-only path too.
#[test]
fn a_non_accept_tier_is_refused() {
    let (signers, set) = committee(5, 100);
    for tier in [Finality::Photon, Finality::Wave, Finality::Horizon] {
        let mut cert = valid_cert(&signers, 4, 4);
        cert.tier = tier;
        assert_eq!(cert.verify(&set, 0), Err(CertError::UnknownTier(tier)));
    }
}

#[test]
fn a_wrong_version_or_type_is_refused() {
    let (signers, set) = committee(5, 100);

    let mut cert = valid_cert(&signers, 4, 4);
    cert.version = 2;
    assert_eq!(
        cert.verify(&set, 0),
        Err(CertError::Version { got: 2, want: 3 })
    );

    let mut cert = valid_cert(&signers, 4, 4);
    cert.qc_type = 9;
    assert_eq!(cert.verify(&set, 0), Err(CertError::Type { got: 9, want: 1 }));
}

#[test]
fn an_empty_or_thresholdless_cert_is_refused() {
    let (signers, set) = committee(5, 100);

    let mut cert = valid_cert(&signers, 4, 4);
    cert.threshold = 0;
    assert_eq!(cert.verify(&set, 0), Err(CertError::ThresholdZero));

    let mut cert = valid_cert(&signers, 4, 4);
    cert.votes.clear();
    assert_eq!(cert.verify(&set, 0), Err(CertError::NoVotes));
}

// ------------------------------------------------------------------ the stake rung

/// Count is not stake. Four of five validators sign, which passes the count
/// clause, but they hold 4 of 1003 stake — nowhere near two thirds.
#[test]
fn a_count_quorum_without_the_stake_is_refused() {
    let signers: Vec<Signer> = (1..=5)
        .map(|i| Signer::new(i, if i == 5 { 999 } else { 1 }))
        .collect();
    let mut set = ValidatorSet::new();
    for s in &signers {
        set.insert(s.id, s.weight, &s.public(), &s.pop()).expect("insert");
    }

    let cert = valid_cert(&signers, 4, 4);

    // The count-only predicate is satisfied.
    assert_eq!(cert.verify(&set, 0), Ok(()));

    // The export predicate is not: 4 of 1003, floor(2·1003/3) = 668.
    assert_eq!(
        cert.verify_weighted(&set, &set, 0),
        Err(CertError::StakeBelowSupermajority {
            voted: 4,
            signer: 1003,
            need_above: 668,
        })
    );
}

/// Exactly two thirds is not a supermajority. The predicate is strict, and this
/// is the boundary it turns on.
///
/// Six validators, so the boundary lands inside the set AND the set is a
/// Byzantine committee — a three-validator set has the same boundary and cannot
/// export at all, which would make the passing half of this case untestable.
#[test]
fn exactly_two_thirds_is_refused_and_one_more_passes() {
    // Six validators of 100. floor(2·600/3) = 400, so 400 must fail.
    let (signers, set) = committee(6, 100);
    let four = valid_cert(&signers, 4, 4);
    assert_eq!(
        four.verify_weighted(&set, &set, 0),
        Err(CertError::StakeBelowSupermajority {
            voted: 400,
            signer: 600,
            need_above: 400,
        })
    );

    let five = valid_cert(&signers, 5, 5);
    assert_eq!(five.verify_weighted(&set, &set, 0), Ok(()));
}

/// An epoch that resolves to no stake at all cannot support a claim about a
/// fraction of its stake.
///
/// The signatures still verify — membership and stake are separate facts, read
/// from separate sources, and this is the case where the weighing one is empty.
/// It is stated against a stake source rather than by registering zero-weight
/// validators, because registration now refuses those outright: see
/// `a_zero_weight_signer_is_refused_at_registration`.
#[test]
fn zero_total_stake_fails_closed() {
    struct NoStake(i64);
    impl StakeSource for NoStake {
        fn weight(&self, _: &NodeId, _: u64) -> u64 {
            0
        }
        fn signer_stake(&self, _: u64) -> u64 {
            0
        }
        fn signer_count(&self, _: u64) -> i64 {
            self.0
        }
    }

    let (signers, set) = committee(5, 100);
    let cert = valid_cert(&signers, 4, 4);

    assert_eq!(cert.verify(&set, 0), Ok(()));
    assert_eq!(
        cert.verify_weighted(&set, &NoStake(5), 0),
        Err(CertError::StakeZero { epoch_height: 0 })
    );
}

/// Nova is a stake majority AND a signer floor. The floor is what a stake
/// predicate cannot express: one validator holding 996 of 1000 has a majority
/// on its own, and must still not ignite alone.
#[test]
fn nova_needs_both_the_majority_and_the_signer_floor() {
    let signers: Vec<Signer> = (1..=5)
        .map(|i| Signer::new(i, if i == 1 { 996 } else { 1 }))
        .collect();
    let mut set = ValidatorSet::new();
    for s in &signers {
        set.insert(s.id, s.weight, &s.public(), &s.pop()).expect("insert");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);

    // The stake majority holder, alone.
    let alone =
        QuorumCert::assemble(Finality::Nova, pos.clone(), 1, &[signers[0].vote(&message)]).unwrap();
    assert_eq!(alone.verify(&set, 0), Ok(()));
    assert_eq!(
        alone.verify_weighted(&set, &set, 0),
        Err(CertError::SignerFloor { have: 1, need: 3, n: 5 })
    );

    // Three signers including it: floor met, and 998 of 1000 is a majority.
    let votes: Vec<Vote> = signers[..3].iter().map(|s| s.vote(&message)).collect();
    let three = QuorumCert::assemble(Finality::Nova, pos, 3, &votes).unwrap();
    assert_eq!(three.verify_weighted(&set, &set, 0), Ok(()));
}

/// Nova over a set the node cannot resolve is not a weaker accept, it is no
/// accept. `nova_quorum(0)` is 1, so without this a node with a transiently
/// empty view would self-accept.
#[test]
fn nova_over_an_unresolved_set_fails_closed() {
    let (signers, _) = committee(5, 100);
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<Vote> = signers[..3].iter().map(|s| s.vote(&message)).collect();
    let cert = QuorumCert::assemble(Finality::Nova, pos, 3, &votes).unwrap();

    // A verifier that knows the keys, over a set that reports nothing.
    let (_, keys) = committee(5, 100);
    struct Empty;
    impl StakeSource for Empty {
        fn weight(&self, _: &NodeId, _: u64) -> u64 {
            0
        }
        fn signer_stake(&self, _: u64) -> u64 {
            0
        }
        fn signer_count(&self, _: u64) -> i64 {
            0
        }
    }
    assert_eq!(
        cert.verify_weighted(&keys, &Empty, 0),
        Err(CertError::UnresolvedSet { n: 0 })
    );
}

// -------------------------------------------------- signatures, one per signer

/// Every signature is checked against exactly one key. That is what a
/// certificate is: n independent statements, not one statement about a sum.
///
/// The mutations below are the ones an aggregate over summed keys could not
/// tell apart — the same evidence re-ordered, a signer swapped for a stranger,
/// one signature short — and each is refused by name.
#[test]
fn each_signature_is_checked_against_its_own_signer() {
    let (signers, set) = committee(4, 100);
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<Vote> = signers.iter().map(|s| s.vote(&message)).collect();

    let cert = QuorumCert::assemble(Finality::Quasar, pos.clone(), 4, &votes).expect("assemble");
    assert_eq!(cert.verify(&set, 0), Ok(()));

    // One voter's signature over a different voter's slot.
    let mut swapped = cert.clone();
    swapped.votes[1].signature = cert.votes[0].signature.clone();
    assert_eq!(swapped.verify(&set, 0), Err(CertError::SigInvalid(1)));

    // A stranger in place of a member: the id resolves to no key.
    let mut stranger = cert.clone();
    stranger.votes[3].node_id = [0xEE; 20];
    assert_eq!(stranger.verify(&set, 0), Err(CertError::SigInvalid(3)));

    // One id repeated — the ordering clause catches it before any key is read.
    let mut dup = cert.clone();
    dup.votes[1].node_id = dup.votes[0].node_id;
    assert_eq!(dup.verify(&set, 0), Err(CertError::NotStrictlyIncreasing(1)));

    // Zero, garbage, and the wrong length.
    for bad in [
        vec![0u8; SIGNATURE_LEN],
        vec![0xABu8; SIGNATURE_LEN],
        cert.votes[2].signature[..95].to_vec(),
        Vec::new(),
    ] {
        let mut broken = cert.clone();
        broken.votes[2].signature = bad;
        assert_eq!(broken.verify(&set, 0), Err(CertError::SigInvalid(2)));
    }

    // One signature short of the declared threshold.
    let short = QuorumCert::assemble(Finality::Quasar, pos, 4, &votes[..3]);
    assert_eq!(short, Err(CertError::BelowThreshold { have: 3, need: 4 }));
}

/// A single flipped bit anywhere in the signed message invalidates every
/// signature over it. This is the binding the whole certificate rests on.
#[test]
fn every_byte_of_the_message_is_bound() {
    let (signers, set) = committee(1, 100);
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let signature = signers[0].sign(&message);

    assert!(set.verify_vote(&signers[0].id, &message, &signature, 0));

    for byte in 0..message.len() {
        for bit in [0u8, 3, 7] {
            let mut tampered = message.clone();
            tampered[byte] ^= 1 << bit;
            assert!(
                !set.verify_vote(&signers[0].id, &tampered, &signature, 0),
                "byte {byte} bit {bit} was not bound",
            );
        }
    }
}

// -------------------------------------------------------------------- registration

/// A key that is not a valid group element never enters the set, so there is no
/// state in which a registered validator has nothing to verify against.
///
/// The error KIND is asserted, not merely the refusal: Go names which clause a
/// registration died on, and a port that refuses everything for one reason is
/// not the same standard.
#[test]
fn an_invalid_public_key_is_refused_at_registration() {
    let s = Signer::new(1, 100);
    let mut set = ValidatorSet::new();

    // 48 bytes that are not a point, in the two shapes a wire decode produces.
    assert_eq!(set.insert(node(1), 100, &[0u8; 48], &s.pop()), Err(CertError::KeyEncoding));
    assert_eq!(set.insert(node(2), 100, &[0xABu8; 48], &s.pop()), Err(CertError::KeyEncoding));
    // A 96-byte signature offered where a 48-byte key belongs.
    assert_eq!(set.insert(node(4), 100, &[0u8; 96], &s.pop()), Err(CertError::KeyEncoding));
    // No key at all is not a malformed key: it names the other door.
    assert_eq!(set.insert(node(3), 100, &[], &s.pop()), Err(CertError::NoKey));

    assert!(set.is_empty(), "nothing refused left a member behind");
}

// ---------------------------------------------------- the admission rule, by clause
//
// `ValidatorSet::insert` is the port of Go's `validators.Register`. These pin it
// clause by clause and in its order, because the whole value of the rule is that
// a registration the network refuses is refused here FOR THE SAME REASON.

/// POSSESSION IS REQUIRED. A key is admitted only with a proof that this node
/// holds its secret. Every proof that is not that one is refused, and the key
/// never becomes a member — so `nova_signer_floor` counts holders of secrets.
#[test]
fn a_key_without_its_proof_is_refused() {
    let s = Signer::new(1, 100);
    let other = Signer::new(2, 100);

    for (what, proof) in [
        ("no proof at all", Vec::new()),
        ("96 zero bytes", vec![0u8; SIGNATURE_LEN]),
        ("96 bytes of garbage", vec![0xABu8; SIGNATURE_LEN]),
        ("a proof one byte short", s.pop()[..95].to_vec()),
        ("another validator's proof", other.pop()),
        // The right secret, the right key, the WRONG node — the proof binds the
        // identity, so it does not travel to a second one.
        ("this key's proof, made for another node", pop::sign(&s.sk, &other.id, &s.public())),
        // A signature by this secret over this preimage, in the VOTE domain.
        // Collapse the two tags and this passes.
        (
            "a vote-domain signature over the proof preimage",
            s.sk.sign(&pop::message(&s.id, &s.public()), DST, &[]).compress().to_vec(),
        ),
    ] {
        let mut set = ValidatorSet::new();
        assert_eq!(
            set.insert(s.id, 100, &s.public(), &proof),
            Err(CertError::PopInvalid),
            "{what} was accepted as a proof of possession",
        );
        assert!(set.is_empty(), "{what} left a member behind");
    }

    // And the real proof is admitted, so the refusals above are the clause
    // biting and not registration refusing everything.
    let mut set = ValidatorSet::new();
    assert_eq!(set.insert(s.id, 100, &s.public(), &s.pop()), Ok(()));
    assert!(set.can_verify(&s.id));
}

/// ONE KEY, ONE NODE. A second node cannot claim a key already counted, even
/// holding the secret and presenting a genuine proof for its own id — which is
/// what makes a floor on distinct voters a floor on distinct signers rather than
/// on ids one holder can mint at will.
#[test]
fn one_key_belongs_to_one_node() {
    let s = Signer::new(1, 100);
    let second = node(9);

    let mut set = ValidatorSet::new();
    set.insert(s.id, 100, &s.public(), &s.pop()).expect("first id");

    // A genuine, node-bound proof for the second id — possession is satisfied,
    // and uniqueness is what refuses it.
    let proof = pop::sign(&s.sk, &second, &s.public());
    assert_eq!(pop::verify(&second, &s.public(), &proof), Ok(()), "the proof is genuine");
    assert_eq!(set.insert(second, 100, &s.public(), &proof), Err(CertError::DuplicateKey));

    assert_eq!(set.len(), 1, "the second id never became a member");
    assert!(!set.contains(&second));
}

/// ONE NODE, ONE KEY. A node is admitted exactly once, so one operator cannot
/// occupy several signer slots and several shares of the weight — which
/// possession cannot catch, because each of those proofs is genuine.
#[test]
fn one_node_holds_one_key() {
    let a = Signer::new(1, 100);
    let b = Signer::new(2, 100);

    let mut set = ValidatorSet::new();
    set.insert(a.id, 100, &a.public(), &a.pop()).expect("admitted");

    // The same node under a SECOND key it genuinely holds.
    let second = pop::sign(&b.sk, &a.id, &b.public());
    assert_eq!(pop::verify(&a.id, &b.public(), &second), Ok(()), "the proof is genuine");
    assert_eq!(set.insert(a.id, 100, &b.public(), &second), Err(CertError::DuplicateNode));

    // The identical registration offered twice fails on the KEY axis first,
    // which is the order Go iterates them in.
    assert_eq!(set.insert(a.id, 100, &a.public(), &a.pop()), Err(CertError::DuplicateKey));

    // Neither door restates a member: an unkeyed admission of a keyed node is
    // refused, and so is the reverse — a re-admission cannot quietly de-key a
    // validator or change its weight.
    assert_eq!(set.insert_unkeyed(a.id, 5), Err(CertError::DuplicateNode));
    set.insert_unkeyed(b.id, 5).expect("a new node, no key");
    assert_eq!(set.insert(b.id, 100, &b.public(), &b.pop()), Err(CertError::DuplicateNode));

    // Nothing moved.
    assert_eq!(set.len(), 2);
    assert_eq!(set.weight(&a.id, 0), 100);
    assert_eq!(set.public_key(&a.id).map(|k| k.compress()), Some(a.public()));
    assert!(!set.can_verify(&b.id));

    // Re-keying is a retraction and a fresh admission, and nothing else: the old
    // key is freed with the member, and the node comes back under the new one.
    set.remove(&a.id);
    assert!(!set.contains(&a.id));
    set.insert(a.id, 100, &b.public(), &second).expect("re-admitted under a new key");
    assert_eq!(set.public_key(&a.id).map(|k| k.compress()), Some(b.public()));
}

/// A KEYED SIGNER WITH NO STAKE is a phantom: it raises the count of distinct
/// signers a floor is read against without raising the weight. Refused at the
/// door, so the disagreement between "how many signed" and "how much signed"
/// cannot be introduced by registration.
#[test]
fn a_zero_weight_signer_is_refused_at_registration() {
    let s = Signer::new(1, 100);
    let mut set = ValidatorSet::new();

    assert_eq!(set.insert(s.id, 0, &s.public(), &s.pop()), Err(CertError::ZeroWeight));
    assert!(set.is_empty());

    // The clause ORDER, pinned: a registration that is both weightless and
    // unproven is refused for its weight, because Go checks the O(1) clause
    // first and a port that reordered them would name a different reason — and
    // would spend a pairing on a registration that was inadmissible on its face.
    assert_eq!(set.insert(s.id, 0, &s.public(), &[]), Err(CertError::ZeroWeight));
    // No key beats both, as it does in Go.
    assert_eq!(set.insert(s.id, 0, &[], &[]), Err(CertError::NoKey));

    // A member with no KEY may hold no stake: it can never sign, so it is no
    // phantom signer — it only raises `n`, which is the direction that makes
    // every floor harder rather than easier. Go's flatten carries these too.
    set.insert_unkeyed(s.id, 0).expect("a keyless member may be weightless");
    assert_eq!(set.len(), 1);
    assert!(!set.can_verify(&s.id));
}

/// The proof registration checks is the NODE-BOUND one the network signs, and
/// the node in it is the 20-byte NodeID: the preimage is `node ‖ key`, 68 bytes.
///
/// A proof made over the id padded to 32 bytes — the width this set used to name
/// its validators by — is refused. So the two spellings are not both valid, and
/// this crate's registration and `pop`'s frozen Go vectors are one standard.
#[test]
fn the_proof_is_bound_to_the_twenty_byte_identity() {
    let s = Signer::new(1, 100);
    assert_eq!(s.id.len(), 20);
    assert_eq!(pop::message(&s.id, &s.public()).len(), 68);

    let mut padded = Vec::new();
    padded.extend_from_slice(&s.id);
    padded.extend_from_slice(&[0u8; 12]); // the id as 32 bytes, zero-extended
    padded.extend_from_slice(&s.public());
    let wrong_width = s.sk.sign(&padded, pop::POP_DST, &[]).compress().to_vec();

    let mut set = ValidatorSet::new();
    assert_eq!(
        set.insert(s.id, 100, &s.public(), &wrong_width),
        Err(CertError::PopInvalid),
        "a proof over the 32-byte spelling of the id was accepted",
    );
    assert_eq!(set.insert(s.id, 100, &s.public(), &s.pop()), Ok(()));
}

/// Assembly sorts and drops non-accepts, so an assembled cert satisfies the
/// ordering clause by construction whatever order the votes arrived in.
#[test]
fn assembly_is_canonical() {
    let (signers, set) = committee(5, 100);
    let pos = position();
    let message = canonical_vote_message(&pos, true);

    let mut votes: Vec<Vote> = signers.iter().map(|s| s.vote(&message)).collect();
    votes.reverse();
    votes.push(votes[0].clone()); // a duplicate
    votes.push(Vote {
        node_id: [0x99; 20],
        accept: false, // a reject
        signature: vec![0u8; SIGNATURE_LEN],
    });

    let cert = QuorumCert::assemble(Finality::Quasar, pos, 5, &votes).expect("assemble");
    assert_eq!(cert.votes.len(), 5);
    assert!(cert.votes.windows(2).all(|w| w[0].node_id < w[1].node_id));
    assert_eq!(cert.verify(&set, 0), Ok(()));
}

// ------------------------------------------------- the probabilistic engine's cert

/// `QuasarConsensus` issues a certificate only from votes that carry a verified
/// signature, and the certificate it issues verifies. Unsigned ballots — which
/// is what the probabilistic engine actually collects today — produce no
/// certificate rather than an empty one that later passes.
#[test]
fn the_engine_issues_certificates_only_from_signatures() {
    use lux_consensus::{ConsensusError, NodeID, QuasarConfig, QuasarConsensus, VoteType};

    let (signers, _) = committee(4, 100);
    let mut config = QuasarConfig::testnet();
    config.k = 4;
    config.alpha = 0.75; // threshold = 3
    let mut q = QuasarConsensus::new(&config);

    for s in &signers {
        q.add_validator_with_key(NodeID::from(s.id), s.weight, &s.public(), &s.pop())
            .expect("register");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);

    // Unsigned ballots: members, but no evidence.
    let unsigned: Vec<lux_consensus::Vote> = signers
        .iter()
        .map(|s| {
            lux_consensus::Vote::new(
                lux_consensus::ID::from(pos.block_id),
                VoteType::Preference,
                NodeID::from(s.id),
            )
        })
        .collect();
    assert!(matches!(
        q.create_certificate(pos.clone(), &unsigned),
        Err(ConsensusError::NoQuorum)
    ));

    // The same ballots, signed.
    let signed: Vec<lux_consensus::Vote> = signers
        .iter()
        .map(|s| {
            lux_consensus::Vote::new(
                lux_consensus::ID::from(pos.block_id),
                VoteType::Preference,
                NodeID::from(s.id),
            )
            .with_signature(s.sign(&message))
        })
        .collect();

    let cert = q.create_certificate(pos.clone(), &signed).expect("certificate");
    assert_eq!(cert.votes.len(), 4);
    assert_eq!(cert.tier, Finality::Quasar);
    assert!(q.verify_certificate(&cert));

    // The forgeries, against this path too.
    for bad in [
        vec![0u8; SIGNATURE_LEN],
        vec![0xABu8; SIGNATURE_LEN],
        Vec::new(),
    ] {
        let mut broken = cert.clone();
        broken.votes[0].signature = bad;
        assert!(!q.verify_certificate(&broken));
    }

    // Moved to another position, the signatures are no longer a proof.
    let mut moved = cert.clone();
    moved.position.height += 1;
    assert!(!q.verify_certificate(&moved));

    // A signer swapped for a stranger: the id resolves to no key.
    let mut swapped = cert.clone();
    swapped.votes[3].node_id = [0xEEu8; 20];
    assert!(!q.verify_certificate(&swapped));

    // Three of the four, which is the declared threshold but not the export
    // stake floor — 300 of 400 does clear floor(2·400/3) = 266, so it stands.
    // Two do not.
    let mut two = cert;
    two.votes.truncate(2);
    two.threshold = 2;
    assert!(!q.verify_certificate(&two));
}

/// A member with no registered key holds stake and may have its ballot counted,
/// but can never contribute to a certificate.
#[test]
fn a_keyless_member_cannot_be_certified() {
    use lux_consensus::{ConsensusError, NodeID, QuasarConfig, QuasarConsensus, VoteType};

    let (signers, _) = committee(4, 100);
    let mut config = QuasarConfig::testnet();
    config.k = 4;
    config.alpha = 0.75; // threshold = 3
    let mut q = QuasarConsensus::new(&config);

    // Two keyed, two not.
    q.add_validator_with_key(NodeID::from(signers[0].id), 100, &signers[0].public(), &signers[0].pop())
        .unwrap();
    q.add_validator_with_key(NodeID::from(signers[1].id), 100, &signers[1].public(), &signers[1].pop())
        .unwrap();
    q.add_validator(NodeID::from(signers[2].id), 100).unwrap();
    q.add_validator(NodeID::from(signers[3].id), 100).unwrap();

    assert_eq!(q.validator_count(), 4);
    for s in &signers {
        assert!(q.is_validator(&NodeID::from(s.id)));
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<lux_consensus::Vote> = signers
        .iter()
        .map(|s| {
            lux_consensus::Vote::new(
                lux_consensus::ID::from(pos.block_id),
                VoteType::Preference,
                NodeID::from(s.id),
            )
            .with_signature(s.sign(&message))
        })
        .collect();

    // All four signed, but only two can be checked — below the threshold of 3.
    assert!(matches!(
        q.create_certificate(pos, &votes),
        Err(ConsensusError::NoQuorum)
    ));
}

/// An invalid key is refused at registration, so it never becomes a member.
#[test]
fn the_engine_refuses_an_invalid_key() {
    use lux_consensus::{NodeID, QuasarConfig, QuasarConsensus};

    let mut q = QuasarConsensus::new(&QuasarConfig::testnet());
    assert!(q
        .add_validator_with_key(NodeID::from([1u8; 20]), 100, &[0u8; 48], &[])
        .is_err());
    assert_eq!(q.validator_count(), 0);
    assert!(!q.is_validator(&NodeID::from([1u8; 20])));
}

/// Assembly cannot manufacture a quorum it does not have.
#[test]
fn assembly_refuses_to_under_fill() {
    let (signers, _) = committee(5, 100);
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<Vote> = signers[..2].iter().map(|s| s.vote(&message)).collect();

    assert_eq!(
        QuorumCert::assemble(Finality::Quasar, pos.clone(), 4, &votes),
        Err(CertError::BelowThreshold { have: 2, need: 4 })
    );
    assert_eq!(
        QuorumCert::assemble(Finality::Quasar, pos, 0, &votes),
        Err(CertError::ThresholdZero)
    );
}

/// The issue path finalizes only what the accept rule accepts: a certificate
/// that meets the vote COUNT but not the stake floor is refused, so
/// `is_finalized` can never report a certificate `verify_certificate` rejects.
/// Three dust-stake voters clear the count of three and fall far below the
/// two-thirds floor of a set dominated by one heavy validator.
#[test]
fn the_issue_path_applies_the_stake_floor_not_only_the_count() {
    use lux_consensus::{ConsensusError, NodeID, QuasarConfig, QuasarConsensus, VoteType};

    let signers: Vec<Signer> = vec![
        Signer::new(1, 1),
        Signer::new(2, 1),
        Signer::new(3, 1),
        Signer::new(4, 10_000),
    ];
    let mut config = QuasarConfig::testnet();
    config.k = 4;
    config.alpha = 0.75; // threshold = 3, met by the three dust voters
    let mut q = QuasarConsensus::new(&config);
    for s in &signers {
        q.add_validator_with_key(NodeID::from(s.id), s.weight, &s.public(), &s.pop())
            .expect("register");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let ballots: Vec<lux_consensus::Vote> = signers[..3]
        .iter()
        .map(|s| {
            lux_consensus::Vote::new(
                lux_consensus::ID::from(pos.block_id),
                VoteType::Preference,
                NodeID::from(s.id),
            )
            .with_signature(s.sign(&message))
        })
        .collect();

    // Count is met (3 of 3), stake is not (3 of 10_003, floor is > 6668).
    assert!(matches!(
        q.create_certificate(pos.clone(), &ballots),
        Err(ConsensusError::NoQuorum)
    ));
    assert!(
        !q.is_finalized(&lux_consensus::ID::from(pos.signed_identity())),
        "nothing below the stake floor is finalized"
    );
}

// ------------------------------------------------------------- the weight total

/// Go's `Register` runs one clause after uniqueness that this port had dropped:
/// `math.Add64` on the running total, refusing the WHOLE set if it overflows.
/// Four validators of 2^63 sum to 2^65. Without the clause both sides of the
/// weighted predicate clamp to `u64::MAX` — total AND voted — and
/// `u64::MAX > floor(2·u64::MAX/3)`, so a certificate carrying half the stake
/// reads as an export supermajority. The set that would do it is refused here,
/// at the member whose weight the total cannot hold.
#[test]
fn a_set_whose_weights_overflow_is_refused_at_admission() {
    let huge = 1u64 << 63;
    let signers: Vec<Signer> = (1..=4).map(|i| Signer::new(i, huge)).collect();
    let mut set = ValidatorSet::new();

    let first = &signers[0];
    set.insert(first.id, first.weight, &first.public(), &first.pop())
        .expect("the first 2^63 fits");
    // Every seat here is keyed, so the two sums coincide — and both are checked,
    // because it is their agreement on a keyed set that makes them meaningful.
    assert_eq!(set.signer_stake(0), huge);
    assert_eq!(set.carried(), huge);

    // 2^63 + 2^63 is 2^64. Refused, and nothing of it is kept.
    let second = &signers[1];
    assert_eq!(
        set.insert(second.id, second.weight, &second.public(), &second.pop()),
        Err(CertError::WeightOverflow)
    );
    assert_eq!(set.len(), 1, "a refused registration is not a member");
    assert_eq!(set.signer_stake(0), huge, "nor does its weight count");
    assert!(!set.can_verify(&second.id), "nor is its key registered");

    // And through the whole-set door, for the same reason.
    let registrations: Vec<Registration> = signers
        .iter()
        .map(|s| Registration {
            node: s.id,
            public_key: s.public().to_vec(),
            proof: s.pop(),
            weight: s.weight,
        })
        .collect();
    assert_eq!(
        ValidatorSet::register(registrations).unwrap_err(),
        CertError::WeightOverflow
    );
}

/// The unkeyed door counts weight toward what the set CARRIES, so it carries the
/// same clause — as Go's `FlattenValidatorSet` does, which checks the overflow
/// before it even looks at the key.
///
/// Representability and quorum are different questions here, and this pins both:
/// the weight is in `carried` and refuses a set that cannot hold it, and it is
/// absent from `signer_stake`, because a member that cannot sign moves no floor.
#[test]
fn an_unkeyed_member_cannot_overflow_the_total_either() {
    let mut set = ValidatorSet::new();
    set.insert_unkeyed(node(1), u64::MAX).expect("the first fits");
    assert_eq!(
        set.insert_unkeyed(node(2), 1),
        Err(CertError::WeightOverflow)
    );
    assert_eq!(set.len(), 1);
    assert_eq!(set.carried(), u64::MAX);
    assert_eq!(
        set.signer_stake(0),
        0,
        "no seat can sign, so no stake is behind any possible quorum"
    );
    assert_eq!(set.signer_count(0), 0);
}

/// The largest representable set is admissible, and the total is exact at the
/// boundary rather than clamped to it — a set that sums to `u64::MAX` is a real
/// set, and only the member past it is refused. Retracting one frees exactly its
/// weight, so the total stays the sum of what is actually there.
#[test]
fn the_total_is_exact_at_the_boundary_and_after_a_retraction() {
    let mut set = ValidatorSet::new();
    set.insert_unkeyed(node(1), u64::MAX - 10).expect("insert");
    set.insert_unkeyed(node(2), 10).expect("insert");
    assert_eq!(set.carried(), u64::MAX);

    assert_eq!(set.insert_unkeyed(node(3), 1), Err(CertError::WeightOverflow));

    set.remove(&node(2));
    assert_eq!(set.carried(), u64::MAX - 10);
    set.insert_unkeyed(node(3), 10).expect("the freed room is real");
    assert_eq!(set.carried(), u64::MAX);
}

/// The other half of the same clause, on the predicate side.
///
/// This is the exact certificate the missing clause admitted: four validators of
/// 2^63, two of them voting — half the stake — read against a stake source that
/// clamps, which is what the set itself used to do. The floor is computed from
/// the total, so when both the total and the voted sum clamp to `u64::MAX` the
/// comparison is `u64::MAX > floor(2·u64::MAX/3)` and QUASAR returns `Ok` on 50%.
///
/// It now returns the overflow instead. A sum that cannot be represented is not
/// a sum, and the predicate says so rather than comparing two saturated numbers:
/// no admitted set can reach it, so reaching it is evidence about the source.
#[test]
fn a_clamping_stake_source_is_refused_rather_than_read() {
    /// The pre-fix arithmetic, kept whole so the difference is the fix and
    /// nothing else: a total folded on demand with `saturating_add`.
    struct Clamping {
        weights: Vec<(NodeId, u64)>,
    }
    impl StakeSource for Clamping {
        fn weight(&self, node: &NodeId, _epoch_height: u64) -> u64 {
            self.weights
                .iter()
                .find(|(id, _)| id == node)
                .map(|(_, w)| *w)
                .unwrap_or(0)
        }
        fn signer_stake(&self, _epoch_height: u64) -> u64 {
            self.weights.iter().fold(0u64, |a, (_, w)| a.saturating_add(*w))
        }
        fn signer_count(&self, _epoch_height: u64) -> i64 {
            self.weights.len() as i64
        }
    }

    let huge = 1u64 << 63;
    // Registered at a weight the set can hold, so the keys resolve and the
    // signatures verify — the stake is the only thing under test.
    let (signers, set) = committee(4, 1);
    let clamping = Clamping {
        weights: signers.iter().map(|s| (s.id, huge)).collect(),
    };
    assert_eq!(
        clamping.signer_stake(0),
        u64::MAX,
        "the source clamps, as the set used to"
    );

    // Two of four: half the stake, and every signature genuine.
    let cert = valid_cert(&signers, 2, 2);
    assert_eq!(cert.verify(&set, 0), Ok(()));
    assert_eq!(
        cert.verify_weighted(&set, &clamping, 0),
        Err(CertError::WeightOverflow),
        "half the stake is not two thirds of it, whatever the arithmetic clamps to"
    );

    // The same refusal one rung down, so neither tier reads a clamped total.
    // Three of four, because Nova's signer floor is read before its stake and
    // two would stop there — the stake clause has to be reached to be tested.
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<Vote> = signers[..3].iter().map(|s| s.vote(&message)).collect();
    let nova = QuorumCert::assemble(Finality::Nova, pos, 3, &votes).expect("assemble");
    assert_eq!(
        nova.verify_weighted(&set, &clamping, 0),
        Err(CertError::WeightOverflow)
    );
}

// ------------------------------------------------------------ the whole-set door

/// `register` admits the set or none of it. An inadmissible member fails the
/// call, and the members that were already good do not survive it — which is the
/// whole difference from a loop of `insert`, where they do, leaving a set whose
/// `n` and total describe a set nobody registered.
#[test]
fn one_bad_registration_refuses_the_whole_set() {
    let signers: Vec<Signer> = (1..=4).map(|i| Signer::new(i, 100)).collect();
    let mut registrations: Vec<Registration> = signers
        .iter()
        .map(|s| Registration {
            node: s.id,
            public_key: s.public().to_vec(),
            proof: s.pop(),
            weight: s.weight,
        })
        .collect();
    // The last one presents a proof bound to another node.
    registrations[3].proof = signers[0].pop();

    assert_eq!(
        ValidatorSet::register(registrations.clone()).unwrap_err(),
        CertError::PopInvalid
    );

    // The same registrations one at a time: three go in, and the caller is left
    // holding a set of three where four were registered — n and the total both
    // short, with nothing to say so. That is what the whole-set door removes.
    let mut partial = ValidatorSet::new();
    let mut refused = 0;
    for r in &registrations {
        if partial
            .insert(r.node, r.weight, &r.public_key, &r.proof)
            .is_err()
        {
            refused += 1;
        }
    }
    assert_eq!((partial.len(), partial.signer_stake(0), refused), (3, 300, 1));

    // Repaired, the set is admitted whole.
    registrations[3].proof = signers[3].pop();
    let set = ValidatorSet::register(registrations).expect("register");
    assert_eq!((set.len(), set.signer_stake(0)), (4, 400));
    for s in &signers {
        assert!(set.can_verify(&s.id));
    }
}

/// Which member a bad set is refused on is decided by node id, not by the order a
/// caller happened to build its input — Go sorts before it checks for exactly
/// this reason. Two nodes handed the same registrations in different orders must
/// refuse the same set for the same reason, or they disagree about a set they
/// both rejected.
#[test]
fn the_refusal_does_not_depend_on_the_input_order() {
    let signers: Vec<Signer> = (1..=4).map(|i| Signer::new(i, 100)).collect();
    let registration = |s: &Signer, weight: u64, proof: Vec<u8>| Registration {
        node: s.id,
        public_key: s.public().to_vec(),
        proof,
        weight,
    };

    // Two faults, at two node ids: node 2 stakes nothing, node 4 proves nothing.
    // Sorted, node 2 is reached first, so ZeroWeight is the answer either way.
    let low = registration(&signers[1], 0, signers[1].pop());
    let high = registration(&signers[3], 100, signers[0].pop());
    let good = registration(&signers[0], 100, signers[0].pop());

    assert_eq!(
        ValidatorSet::register(vec![good.clone(), low.clone(), high.clone()]).unwrap_err(),
        CertError::ZeroWeight
    );
    assert_eq!(
        ValidatorSet::register(vec![high, good, low]).unwrap_err(),
        CertError::ZeroWeight,
        "the same set refused on the same clause, whatever order it arrived in"
    );
}

/// The proof path needs a key. Go's `Register` refuses a keyless registration
/// rather than admitting a member that can never sign; a member this node holds
/// no key for comes in through the unkeyed door instead.
#[test]
fn the_whole_set_door_refuses_a_keyless_registration() {
    let s = Signer::new(1, 100);
    assert_eq!(
        ValidatorSet::register(vec![Registration {
            node: s.id,
            public_key: Vec::new(),
            proof: s.pop(),
            weight: 100,
        }])
        .unwrap_err(),
        CertError::NoKey
    );
}

// ------------------------------------------------- the export distinct-signer floor

/// A skewed committee: the first member holds `heavy`, the rest hold one each.
/// This is the shape the export count floor exists for.
fn whale(n: u8, heavy: u64) -> (Vec<Signer>, ValidatorSet) {
    let signers: Vec<Signer> = (1..=n)
        .map(|i| Signer::new(i, if i == 1 { heavy } else { 1 }))
        .collect();
    let mut set = ValidatorSet::new();
    for s in &signers {
        set.insert(s.id, s.weight, &s.public(), &s.pop()).expect("insert");
    }
    (signers, set)
}

/// Stake cannot buy export finality.
///
/// One validator holds a hundred of a hundred and four — more than two thirds
/// several times over — and signs alone. A rung that read only stake would export
/// that certificate, which would make "Byzantine supermajority" a statement about
/// one key. The count floor is what refuses it, and the refusal has to land on the
/// COUNT: a stake refusal here would mean the lone signer never held two thirds
/// and the case proved nothing.
#[test]
fn a_lone_holder_of_two_thirds_cannot_export() {
    let (signers, set) = whale(5, 100);

    // The premise: the stake half is satisfied outright. floor(2·104/3) = 69.
    assert_eq!(two_thirds_stake_floor(104), 69);

    assert_eq!(
        valid_cert(&signers, 1, 1).verify_weighted(&set, &set, 0),
        Err(CertError::SignerFloor { have: 1, need: 4, n: 5 }),
    );
    // Three is still one short of floor(2·5/3)+1 = 4, and the stake is untouched.
    assert_eq!(
        valid_cert(&signers, 3, 3).verify_weighted(&set, &set, 0),
        Err(CertError::SignerFloor { have: 3, need: 4, n: 5 }),
    );
    // At the floor the same stake carries: the count was the binding clause.
    assert_eq!(valid_cert(&signers, 4, 4).verify_weighted(&set, &set, 0), Ok(()));
}

/// Neither half is sufficient. The four light members meet the count floor
/// exactly and hold four of a hundred and four, and they are refused on stake —
/// the mirror of the case above, and together they are the whole rule.
#[test]
fn meeting_the_count_without_the_stake_is_refused_too() {
    let (signers, set) = whale(5, 100);
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<Vote> = signers[1..].iter().map(|s| s.vote(&message)).collect();
    let cert = QuorumCert::assemble(Finality::Quasar, pos, 4, &votes).expect("assemble");

    assert_eq!(
        cert.verify_weighted(&set, &set, 0),
        Err(CertError::StakeBelowSupermajority { voted: 4, signer: 104, need_above: 69 }),
    );
}

/// The floor is the supermajority in seats, recomputed from its definition —
/// the smallest k whose 3k strictly exceeds 2n — sharing no code with the closed
/// form. It is never above the set, so it is never a rung nothing can satisfy.
#[test]
fn the_export_floor_is_the_supermajority_in_seats() {
    for (n, want) in [(1, 1), (2, 2), (3, 3), (4, 3), (5, 4), (11, 8), (21, 15), (41, 28), (100, 67)] {
        assert_eq!(two_thirds_count(n), want, "two_thirds_count({n})");
    }
    for n in 1i64..=1000 {
        let mut k = 0i64;
        while 3 * k <= 2 * n {
            k += 1;
        }
        assert_eq!(two_thirds_count(n), k, "n={n}: the smallest k with 3k>2n");
        assert!(k <= n, "n={n}: a floor of {k} is above the set");
        // The same supermajority the stake half demands of n unit weights.
        assert!(k as u64 > two_thirds_stake_floor(n as u64), "n={n}");
    }
}

/// A source reporting stake for a set it says is empty has no set to read two
/// thirds of. `two_thirds_count(0)` is 1, so computing the floor would hand a lone
/// signer a floor of one; the case is refused instead.
#[test]
fn an_export_certificate_over_an_unresolved_set_fails_closed() {
    struct Unresolved(ValidatorSet);
    impl StakeSource for Unresolved {
        fn weight(&self, node: &NodeId, h: u64) -> u64 {
            self.0.weight(node, h)
        }
        fn signer_stake(&self, h: u64) -> u64 {
            self.0.signer_stake(h)
        }
        fn signer_count(&self, _: u64) -> i64 {
            0
        }
    }

    let (signers, set) = committee(4, 100);
    let cert = valid_cert(&signers, 4, 4);
    // Against the real set this certificate exports.
    assert_eq!(cert.verify_weighted(&set, &set, 0), Ok(()));
    assert_eq!(
        cert.verify_weighted(&set, &Unresolved(set.clone()), 0),
        Err(CertError::UnresolvedSet { n: 0 }),
    );
}

// ------------------------------------------------------- the keyless denominator

/// R5, the STAKE half: a member that cannot sign moves no stake floor.
///
/// Six validators hold a hundred each and a key; a seventh holds three hundred
/// and no key, so a third of what the set carries belongs to a member that can
/// never cast a vote — the point at which a denominator read over the membership
/// roll puts the export rung permanently out of reach.
///
/// The COUNT floor is deliberately the same number either way here — the smallest
/// k with 3k>2n is five for both six and seven — so the stake denominator is the
/// only thing that decides this case, and a fix that moved only the count fails it.
///
/// Every validator that CAN sign does. That is the entire signing set and the
/// strongest certificate this set is capable of producing: if it is refused, no
/// certificate is ever accepted here and export finality is stranded for good.
/// Read over the membership roll it IS refused — 600 does not exceed
/// floor(2·900/3) = 600, and no quorum could ever reach 601, because the stake it
/// falls short by is held by a spectator.
#[test]
fn keyless_stake_is_in_no_floor_and_export_still_reaches() {
    let (signers, mut set) = committee(6, 100);
    let mut spectator = [0u8; 20];
    spectator[0] = 200;
    set.insert_unkeyed(spectator, 300).expect("a keyless member");

    // What the set carries, and what can actually sign.
    assert_eq!(set.carried(), 900, "the chain carries nine hundred");
    assert_eq!(set.signer_stake(0), 600, "six hundred of it can sign");
    assert_eq!(set.len(), 7, "seven members");
    assert_eq!(set.signer_count(0), 6, "six signers");
    assert_eq!(set.weight(&spectator, 0), 0, "a spectator weighs nothing");
    assert!(!set.can_verify(&spectator));

    // The fixture reproduces R5 only if the roll-denominator would have stranded
    // it — otherwise this test proves nothing.
    assert!(
        set.signer_stake(0) <= two_thirds_stake_floor(set.carried()),
        "fixture does not reproduce R5: the signing set clears the roll floor anyway"
    );
    // And it isolates the STAKE half only if the count floor is the same number
    // over the signers and over the roll.
    assert_eq!(
        two_thirds_count(set.signer_count(0)),
        two_thirds_count(set.len() as i64),
        "the count floors differ, so this case does not turn on stake alone"
    );

    // The whole signing set signs, and the export rung admits it.
    let cert = valid_cert(&signers, 6, 6);
    assert!(cert.voter_count() >= two_thirds_count(set.signer_count(0)));
    assert_eq!(cert.verify(&set, 0), Ok(()));
    assert_eq!(
        cert.verify_weighted(&set, &set, 0),
        Ok(()),
        "export refused with every signer in the set agreeing"
    );

    // And the rung is still a rung: four of six is short of two thirds of the
    // stake that can sign, so the floor was moved off the spectator, not removed.
    let four = valid_cert(&signers, 4, 4);
    assert_eq!(
        four.verify_weighted(&set, &set, 0),
        Err(CertError::StakeBelowSupermajority {
            voted: 400,
            signer: 600,
            need_above: 400,
        })
    );
}

/// R5, the COUNT half, isolated: a member that cannot sign moves no count floor
/// either.
///
/// Four validators hold a hundred each and a key; two more hold ONE each and no
/// key. The keyless weight is a rounding error on purpose, so the stake floor is
/// cleared under either denominator — 400 exceeds floor(2·400/3) = 266 and
/// floor(2·402/3) = 268 alike — and stake cannot be what decides.
///
/// The count can. The smallest k with 3k>2n is three over the four signers and
/// five over the roll of six, and four signatures is every one this set is able
/// to produce. Read over the roll, two members holding two units between them
/// strand the export rung of a chain whose four real validators all agree.
#[test]
fn keyless_seats_are_in_no_count_floor_either() {
    let (signers, mut set) = committee(4, 100);
    for i in 0..2u8 {
        let mut spectator = [0u8; 20];
        spectator[0] = 200 + i;
        set.insert_unkeyed(spectator, 1).expect("a keyless member");
    }

    assert_eq!(set.signer_count(0), 4, "four signers");
    assert_eq!(set.len(), 6, "six members");
    assert_eq!(set.signer_stake(0), 400);
    assert_eq!(set.carried(), 402);

    // The stake half is satisfied under EITHER denominator, so it is not what
    // decides — that is what makes this case about the count and nothing else.
    assert!(set.signer_stake(0) > two_thirds_stake_floor(set.signer_stake(0)));
    assert!(set.signer_stake(0) > two_thirds_stake_floor(set.carried()));
    // And the two count floors really do differ, or the case proves nothing.
    assert!(
        two_thirds_count(set.len() as i64) > 4,
        "the roll floor is within reach of four signers; nothing is stranded"
    );

    let cert = valid_cert(&signers, 4, 4);
    assert_eq!(
        cert.verify_weighted(&set, &set, 0),
        Ok(()),
        "export refused with all four signers agreeing: the COUNT floor was read \
         over seats that cannot sign"
    );

    // The rung is still a rung, and its edge is sharp on the signer denominator:
    // three signers sit exactly ON the floor and carry, two sit below it.
    assert_eq!(valid_cert(&signers, 3, 3).verify_weighted(&set, &set, 0), Ok(()));
    assert!(valid_cert(&signers, 2, 2).verify_weighted(&set, &set, 0).is_err());
}

// ------------------------------------------------------ the Byzantine committee

/// The export rung's floor on the SET.
///
/// A supermajority is a claim about a fault budget: f = (n-1)/3 validators may be
/// arbitrarily malicious and the rest still agree on one history. Below four
/// signers that budget is ZERO. One, two or three parties produce a unanimous
/// certificate carrying every unit of the signer stake, and it tolerates nothing
/// — a single compromised key is not one fault absorbed by a margin, it is a
/// forged export certificate every verifier accepts.
///
/// Neither quorum floor catches it, and that is the point: both are read over n,
/// so both shrink with it. At n=1, `two_thirds_count(1)` is 1 and one signature is
/// a supermajority of one, over a stake floor the same signature clears outright.
#[test]
fn an_export_certificate_needs_a_byzantine_committee() {
    for n in 1..4u8 {
        let (signers, set) = committee(n, 100);
        let cert = valid_cert(&signers, n as usize, u32::from(n));

        // Both quorum floors are MET, so neither can be what refuses it.
        assert!(cert.voter_count() >= two_thirds_count(set.signer_count(0)), "n={n}");
        assert!(
            u64::from(n) * 100 > two_thirds_stake_floor(set.signer_stake(0)),
            "n={n}: unanimity does not clear the stake floor, so this case does not \
             reach the committee clause"
        );

        assert_eq!(
            cert.verify_weighted(&set, &set, 0),
            Err(CertError::MinCommittee { n: i64::from(n), need: 4 }),
            "n={n}: a unanimous certificate over a set with no Byzantine fault budget \
             minted export finality"
        );
    }

    // And at the floor the same shape carries: this is a floor on the set, not a
    // ban on small chains certifying anything.
    let (signers, set) = committee(4, 100);
    assert_eq!(
        valid_cert(&signers, 4, 4).verify_weighted(&set, &set, 0),
        Ok(()),
        "the minimum Byzantine committee cannot export"
    );
}

/// The clause belongs to the export rung and must not migrate down the ladder.
///
/// Nova authorizes LOCAL EXECUTION, which the chain can still reorg away, and is
/// crash-fault-safe rather than Byzantine-safe by construction. A four-signer
/// floor there would stop a small or a partitioned chain making any progress, in
/// exchange for a guarantee the rung never offered.
#[test]
fn nova_ignites_below_the_byzantine_committee() {
    for n in 1..4u8 {
        let (signers, set) = committee(n, 100);
        let pos = position();
        let message = canonical_vote_message(&pos, true);
        let votes: Vec<Vote> = signers.iter().map(|s| s.vote(&message)).collect();
        let cert = QuorumCert::assemble(Finality::Nova, pos, u32::from(n), &votes)
            .expect("assemble");
        assert_eq!(
            cert.verify_weighted(&set, &set, 0),
            Ok(()),
            "n={n}: a unanimous NOVA certificate was refused — the export rung's \
             committee floor has leaked down a rung"
        );
    }
}

/// The spectator cannot buy its way into a tally by being named as a voter: it
/// has no key, so `verify` refuses the certificate before any floor is read.
#[test]
fn a_keyless_member_named_as_a_voter_is_refused() {
    let (signers, mut set) = committee(3, 100);
    let ghost = Signer::new(9, 200);
    // Admitted WITHOUT its key, so the set knows the stake and not the signer.
    set.insert_unkeyed(ghost.id, 200).expect("a keyless member");

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let mut votes: Vec<Vote> = signers[..2].iter().map(|s| s.vote(&message)).collect();
    // A real signature under a key the set does not hold for it.
    votes.push(ghost.vote(&message));
    let cert = QuorumCert::assemble(Finality::Quasar, pos, 3, &votes).expect("assemble");

    assert!(
        matches!(cert.verify(&set, 0), Err(CertError::SigInvalid(_))),
        "a vote from a member with no registered key must not verify"
    );
}
