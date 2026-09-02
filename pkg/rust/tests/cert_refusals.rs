// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! What a refusal SAYS, and the two set operations only a retraction reaches.
//!
//! `tests/cert.rs` breaks one valid certificate at a time and asks whether it
//! still passes. This file asks the other half: when the gate refuses, is the
//! refusal the right one — does it name the clause that actually failed and
//! carry the numbers to act on? A gate that refuses everything for one reason is
//! as useless as one that accepts everything, and a refusal nobody can read is a
//! refusal nobody can debug.
//!
//! One stake clause is reachable only at the accept rung and only from a source
//! that reports members holding nothing, and one pair of totals moves only on a
//! retraction, so both live here rather than in the mutation file.

use blst::min_pk::SecretKey;
use lux_consensus::cert::{
    CertError, NodeId, QuorumCert, StakeSource, ValidatorSet, Vote, VoteVerifier, DST,
};
use lux_consensus::finality::{canonical_vote_message, signer_floor, Finality, Position};
use lux_consensus::pop;

/// A validator: a key, an id, and a weight. Deterministic per `n`, so a failure
/// reproduces exactly.
struct Signer {
    id: NodeId,
    sk: SecretKey,
    weight: u64,
}

impl Signer {
    fn new(n: u8, weight: u64) -> Self {
        let sk = SecretKey::key_gen(&[n; 32], &[]).expect("key_gen");
        let mut id = [0u8; 20];
        id[0] = n;
        Signer { id, sk, weight }
    }

    fn public(&self) -> [u8; 48] {
        self.sk.sk_to_pk().compress()
    }

    fn pop(&self) -> Vec<u8> {
        pop::sign(&self.sk, &self.id, &self.public())
    }

    fn vote(&self, message: &[u8]) -> Vote {
        Vote {
            node_id: self.id,
            accept: true,
            signature: self.sk.sign(message, DST, &[]).compress().to_vec(),
        }
    }
}

fn node(n: u8) -> NodeId {
    let mut id = [0u8; 20];
    id[0] = n;
    id
}

fn committee(n: u8, weight: u64) -> (Vec<Signer>, ValidatorSet) {
    let signers: Vec<Signer> = (1..=n).map(|i| Signer::new(i, weight)).collect();
    let mut set = ValidatorSet::new();
    for s in &signers {
        set.insert(s.id, s.weight, &s.public(), &s.pop()).expect("insert");
    }
    (signers, set)
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

/// A certificate from the first `k` of a committee of `n`, declaring the quorum
/// that set DERIVES for `tier` — the only threshold `verify_weighted` admits, so
/// every row below turns on the clause it is about and not on a number the
/// certificate named for itself.
fn cert(signers: &[Signer], k: usize, n: i64, tier: Finality) -> QuorumCert {
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<Vote> = signers[..k].iter().map(|s| s.vote(&message)).collect();
    let mut c = QuorumCert::assemble(tier, pos, votes.len() as u32, &votes).expect("assemble");
    c.threshold = signer_floor(tier, n) as u32;
    c
}

// -------------------------------------------------------- the accept rung's stake

/// A stake source that answers a set's three questions independently, so a test
/// can state a combination no live set could produce and watch the gate refuse
/// it. Signature checking stays with the real set; only the weights are
/// fabricated.
struct Fabricated<'a> {
    set: &'a ValidatorSet,
    signer_stake: u64,
    signer_count: i64,
    weight: u64,
}

impl VoteVerifier for Fabricated<'_> {
    fn verify_vote(&self, node: &NodeId, message: &[u8], sig: &[u8], h: u64) -> bool {
        self.set.verify_vote(node, message, sig, h)
    }
}

impl StakeSource for Fabricated<'_> {
    fn weight(&self, _node: &NodeId, _epoch_height: u64) -> u64 {
        self.weight
    }
    fn signer_stake(&self, _epoch_height: u64) -> u64 {
        self.signer_stake
    }
    fn signer_count(&self, _epoch_height: u64) -> i64 {
        self.signer_count
    }
}

/// A majority of nothing is not a majority. The export rung has the same clause
/// and `zero_total_stake_fails_closed` holds it there, but Nova reaches it by a
/// different route — its signer floor is read FIRST, so a source reporting
/// members who hold nothing gets past the count before the stake refuses it.
/// This is the case that separates "there are no validators" from "there are
/// validators and they hold nothing", and only the second reaches this line.
#[test]
fn nova_fails_closed_when_the_source_reports_no_signer_stake() {
    let (signers, set) = committee(5, 100);
    let c = cert(&signers, 4, 5, Finality::Nova);

    let source = Fabricated {
        set: &set,
        signer_stake: 0,
        signer_count: 5,
        weight: 100,
    };

    // The count floor is cleared — four of five — so only the stake can refuse.
    assert_eq!(
        c.verify_weighted(&source, &source, 9),
        Err(CertError::StakeZero { epoch_height: 9 }),
        "a nova certificate cleared a majority of nothing"
    );
}

// ------------------------------------------------------------------ retraction

/// Removing a member the set never held is not a change. Each of the two sums is
/// the weight still present under its own map, so a map that loses nothing loses
/// no weight — and a retraction that decremented on a miss would walk both
/// totals down toward a floor that everything clears.
#[test]
fn retracting_a_member_the_set_never_held_moves_neither_total() {
    let (_, mut set) = committee(4, 100);

    let carried = set.carried();
    let signer_stake = set.signer_stake(0);
    let len = set.len();

    set.remove(&node(200));

    assert_eq!(set.carried(), carried, "the carried total moved on a miss");
    assert_eq!(
        set.signer_stake(0),
        signer_stake,
        "the signer stake moved on a miss"
    );
    assert_eq!(set.len(), len, "the membership roll moved on a miss");
}

/// A member with no key is in the carried total and in no floor, so retracting
/// one takes its weight out of the first and leaves the second alone. Retraction
/// is the one operation that can push the two sums out of step: a `remove` that
/// decremented both would make a spectator's departure shrink the denominator
/// every floor is read against, which is the R5 clause running backwards.
#[test]
fn retracting_an_unkeyed_member_leaves_the_signable_stake_alone() {
    let (_, mut set) = committee(4, 100);
    let spectator = node(9);
    set.insert_unkeyed(spectator, 500).expect("insert_unkeyed");

    assert_eq!(set.carried(), 900, "the spectator's weight is carried");
    assert_eq!(
        set.signer_stake(0),
        400,
        "the spectator's weight is in no floor"
    );

    set.remove(&spectator);

    assert_eq!(set.carried(), 400, "the carried total kept a departed member");
    assert_eq!(
        set.signer_stake(0),
        400,
        "retracting a spectator moved the stake a floor is read against"
    );
    assert!(!set.contains(&spectator));
}

/// Retracting a member that COULD sign takes its weight out of both sums and
/// frees its key. Both halves matter: a signable total left standing would hold
/// the export floor above what the remaining signers can ever reach, and a key
/// left claimed would make a re-key impossible — one key belongs to one node,
/// and the node that held it is gone.
#[test]
fn retracting_a_keyed_member_frees_both_the_stake_and_the_key() {
    let (signers, mut set) = committee(4, 100);
    let departing = &signers[3];

    set.remove(&departing.id);

    assert_eq!(set.carried(), 300, "the carried total kept a departed signer");
    assert_eq!(
        set.signer_stake(0),
        300,
        "the departed signer's weight is still in the export floor"
    );
    assert_eq!(set.signer_count(0), 3);
    assert!(!set.can_verify(&departing.id));

    // The key is free, so the same secret can be seated under a fresh identity —
    // with a fresh proof, because the proof binds the pair and not the key.
    let reseated = node(77);
    let proof = pop::sign(&departing.sk, &reseated, &departing.public());
    set.insert(reseated, 100, &departing.public(), &proof)
        .expect("a retracted key could not be registered again");
    assert!(set.can_verify(&reseated));
}

// -------------------------------------------------------------- the refusal text

/// Every refusal names its own clause and carries the numbers it was decided on.
///
/// This is the mistake it exists to catch: a match arm copied from the one above
/// it, printing the neighbouring clause's name or the neighbouring clause's
/// fields. That is not cosmetic. A `StakeBelowMajority` that says "quasar" sends
/// an operator to the wrong rung, and an arm that drops its numbers leaves them
/// with a refusal they cannot act on. The distinctness check is the other half:
/// two clauses that print the same sentence are one clause to whoever reads the
/// log, however carefully they are separated in the type.
#[test]
fn every_refusal_names_its_own_clause_and_carries_its_numbers() {
    let cases: Vec<(CertError, Vec<&str>)> = vec![
        (
            CertError::Version { got: 9, want: 1 },
            vec!["version", "9", "1"],
        ),
        (CertError::Type { got: 3, want: 1 }, vec!["type", "3", "1"]),
        (
            CertError::UnknownTier(Finality::Photon),
            vec!["tier", Finality::Photon.name()],
        ),
        (CertError::ThresholdZero, vec!["threshold", "zero"]),
        (CertError::NoVotes, vec!["no votes"]),
        (CertError::NotStrictlyIncreasing(4), vec!["increasing", "4"]),
        (CertError::VoteNotAccept(2), vec!["accept", "2"]),
        (CertError::SigInvalid(7), vec!["signature", "7"]),
        (
            CertError::BelowThreshold { have: 2, need: 5 },
            vec!["threshold", "2", "5"],
        ),
        (CertError::UnresolvedSet { n: -1 }, vec!["unresolved", "-1"]),
        (
            CertError::MinCommittee { n: 3, need: 4 },
            vec!["3", "4", "supermajority"],
        ),
        (
            CertError::SignerFloor {
                have: 2,
                need: 4,
                n: 5,
            },
            vec!["2", "4", "5"],
        ),
        (
            CertError::StakeZero { epoch_height: 88 },
            vec!["stake is zero", "88"],
        ),
        (
            CertError::StakeBelowMajority {
                voted: 10,
                signer: 30,
                need_above: 15,
            },
            vec!["nova", "10", "30", "15"],
        ),
        (
            CertError::StakeBelowSupermajority {
                voted: 11,
                signer: 31,
                need_above: 20,
            },
            vec!["quasar", "11", "31", "20"],
        ),
        (CertError::KeyEncoding, vec!["public key", "point"]),
        (CertError::NoKey, vec!["no public key"]),
        (CertError::ZeroWeight, vec!["zero weight"]),
        (CertError::DuplicateKey, vec!["key", "more than one node"]),
        (CertError::DuplicateNode, vec!["node", "more than once"]),
        (CertError::PopInvalid, vec!["possession"]),
        (CertError::WeightOverflow, vec!["overflow"]),
    ];

    let mut seen: Vec<String> = Vec::new();
    for (err, must_say) in &cases {
        let text = err.to_string();
        for fragment in must_say {
            assert!(
                text.contains(fragment),
                "{err:?} prints {text:?}, which does not say {fragment:?}"
            );
        }
        assert!(
            !seen.contains(&text),
            "{err:?} prints {text:?}, which another clause already prints — \
             the two are one clause to whoever reads the log"
        );
        seen.push(text);
    }

    // The two stake clauses differ in more than their numbers: each says which
    // rung refused, and an operator reading only the message has to be able to
    // tell which floor they missed.
    let nova = CertError::StakeBelowMajority {
        voted: 1,
        signer: 3,
        need_above: 1,
    }
    .to_string();
    let quasar = CertError::StakeBelowSupermajority {
        voted: 1,
        signer: 3,
        need_above: 2,
    }
    .to_string();
    assert!(nova.contains("nova") && !nova.contains("quasar"));
    assert!(quasar.contains("quasar") && !quasar.contains("nova"));
}

/// The table above checks the formatter. This checks that the formatter is what
/// a caller actually gets back from the gate, with the real numbers in it — the
/// two are separate facts, and a refusal that reached the caller as a bare
/// discriminant would satisfy the first and fail an operator at the second.
#[test]
fn a_real_shortfall_reports_the_stake_it_actually_had() {
    // One holder of a hundred and four minimum registrations. The four light seats
    // sign: that MEETS the export count floor floor(2·5/3)+1 = 4, so the refusal is
    // the stake clause and the numbers in it are the tally, the signer stake and
    // the floor — four of a hundred and four against floor(2·104/3) = 69. An equal
    // set cannot state this row: there the count and the stake bind at one edge.
    let signers: Vec<Signer> = (1..=5u8)
        .map(|i| Signer::new(i, if i == 1 { 100 } else { 1 }))
        .collect();
    let mut set = ValidatorSet::new();
    for sgn in &signers {
        set.insert(sgn.id, sgn.weight, &sgn.public(), &sgn.pop()).expect("insert");
    }

    let text = cert(&signers[1..], 4, 5, Finality::Quasar)
        .verify_weighted(&set, &set, 0)
        .expect_err("four minimum registrations are not an export supermajority")
        .to_string();

    assert!(
        text.contains('4') && text.contains("104") && text.contains("69"),
        "the refusal {text:?} does not say how short the quorum was"
    );
}
