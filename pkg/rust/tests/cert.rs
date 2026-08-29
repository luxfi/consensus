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
    CertError, QuorumCert, StakeSource, ValidatorSet, Vote, VoteVerifier, DST, SIGNATURE_LEN,
};
use lux_consensus::finality::{canonical_vote_message, Finality, Id, Position};

/// A validator: a key, an id, and a weight.
struct Signer {
    id: Id,
    sk: SecretKey,
    weight: u64,
}

impl Signer {
    /// Deterministic per `n`, so a failure reproduces exactly.
    fn new(n: u8, weight: u64) -> Self {
        let ikm = [n; 32];
        let sk = SecretKey::key_gen(&ikm, &[]).expect("key_gen");
        let mut id = [0u8; 32];
        id[0] = n;
        Signer { id, sk, weight }
    }

    fn public(&self) -> [u8; 48] {
        self.sk.sk_to_pk().compress()
    }

    fn sign(&self, message: &[u8]) -> Vec<u8> {
        self.sk.sign(message, DST, &[]).compress().to_vec()
    }

    fn vote(&self, message: &[u8]) -> Vote {
        Vote {
            node_id: self.id,
            accept: true,
            signature: self.sign(message),
        }
    }
}

/// A committee of `n` equal-weight validators, ids ascending.
fn committee(n: u8, weight: u64) -> (Vec<Signer>, ValidatorSet) {
    let signers: Vec<Signer> = (1..=n).map(|i| Signer::new(i, weight)).collect();
    let mut set = ValidatorSet::new();
    for s in &signers {
        set.insert(s.id, s.weight, &s.public()).expect("insert");
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
        set.insert(s.id, s.weight, &s.public()).expect("insert");
    }

    let cert = valid_cert(&signers, 4, 4);

    // The count-only predicate is satisfied.
    assert_eq!(cert.verify(&set, 0), Ok(()));

    // The export predicate is not: 4 of 1003, floor(2·1003/3) = 668.
    assert_eq!(
        cert.verify_weighted(&set, &set, 0),
        Err(CertError::StakeBelowSupermajority {
            voted: 4,
            total: 1003,
            need_above: 668,
        })
    );
}

/// Exactly two thirds is not a supermajority. The predicate is strict, and this
/// is the boundary it turns on.
#[test]
fn exactly_two_thirds_is_refused_and_one_more_passes() {
    // Three validators of 100. floor(2·300/3) = 200, so 200 must fail.
    let (signers, set) = committee(3, 100);
    let two = valid_cert(&signers, 2, 2);
    assert_eq!(
        two.verify_weighted(&set, &set, 0),
        Err(CertError::StakeBelowSupermajority {
            voted: 200,
            total: 300,
            need_above: 200,
        })
    );

    let three = valid_cert(&signers, 3, 3);
    assert_eq!(three.verify_weighted(&set, &set, 0), Ok(()));
}

/// A set with no stake cannot support a claim about a fraction of its stake.
#[test]
fn zero_total_stake_fails_closed() {
    let (signers, _) = committee(5, 0);
    let mut set = ValidatorSet::new();
    for s in &signers {
        set.insert(s.id, 0, &s.public()).expect("insert");
    }
    let cert = valid_cert(&signers, 4, 4);

    assert_eq!(cert.verify(&set, 0), Ok(()));
    assert_eq!(
        cert.verify_weighted(&set, &set, 0),
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
        set.insert(s.id, s.weight, &s.public()).expect("insert");
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
        fn weight(&self, _: &Id, _: u64) -> u64 {
            0
        }
        fn total_stake(&self, _: u64) -> u64 {
            0
        }
        fn validator_count(&self, _: u64) -> i64 {
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
    stranger.votes[3].node_id = [0xEE; 32];
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
#[test]
fn an_invalid_public_key_is_refused_at_registration() {
    let mut set = ValidatorSet::new();
    assert!(set.insert([1u8; 32], 100, &[0u8; 48]).is_err());
    assert!(set.insert([2u8; 32], 100, &[0xABu8; 48]).is_err());
    assert!(set.insert([3u8; 32], 100, &[]).is_err());
    // A 96-byte signature offered where a 48-byte key belongs.
    assert!(set.insert([4u8; 32], 100, &[0u8; 96]).is_err());
    assert!(set.is_empty());
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
        node_id: [0x99; 32],
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
        q.add_validator_with_key(NodeID::from(s.id), s.weight, &s.public())
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
    swapped.votes[3].node_id = [0xEEu8; 32];
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
    q.add_validator_with_key(NodeID::from(signers[0].id), 100, &signers[0].public())
        .unwrap();
    q.add_validator_with_key(NodeID::from(signers[1].id), 100, &signers[1].public())
        .unwrap();
    q.add_validator(NodeID::from(signers[2].id), 100);
    q.add_validator(NodeID::from(signers[3].id), 100);

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
        .add_validator_with_key(NodeID::from([1u8; 32]), 100, &[0u8; 48])
        .is_err());
    assert_eq!(q.validator_count(), 0);
    assert!(!q.is_validator(&NodeID::from([1u8; 32])));
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
