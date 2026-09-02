// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The two identities, and the places one could be mistaken for the other.
//!
//! A block is named by 32 bytes and a validator by 20, and the whole reason they
//! are separate types is that a lossy conversion between them maps distinct
//! validators onto one signer slot. `ID` has a padding, truncating constructor
//! because a hash is often shorter or longer than the slot it goes in; `NodeID`
//! deliberately has none. This file holds that asymmetry, and the printed forms
//! an operator reads the two off a log by.
//!
//! It also holds the last two set operations on `QuasarConsensus` — retraction,
//! and handing out the set it decides with — and the certificate path's own
//! distinctness clause, which is the one place a validator's ballot could be
//! counted twice on the way into evidence.

use blst::min_pk::SecretKey;
use lux_consensus::{
    canonical_vote_message, ConsensusError, Finality, NodeID, Position, QuasarConfig,
    QuasarConsensus, Vote, VoteType, ID,
};
use lux_consensus::pop;

fn node_id(n: u8) -> NodeID {
    NodeID([n; 20])
}

// --------------------------------------------------------------- the two widths

/// `ID::from_slice` pads and truncates on purpose, and that is exactly why
/// `NodeID` does not have it.
///
/// A 32-byte slot fed a shorter hash is padded rather than refused, because the
/// callers are hash-shaped and a short digest is a real input. The same
/// convenience applied to a validator identity is the hazard the type system is
/// there to stop: truncating 32 bytes into 20 maps distinct nodes onto one
/// identity, and one identity is one signer slot and one share of the weight.
/// So the lossy direction exists here, is stated here, and has no counterpart on
/// the node.
#[test]
fn the_block_id_pads_and_truncates_where_the_node_id_cannot() {
    // Short: padded on the right, so the prefix survives.
    let short = ID::from_slice(&[0xAB, 0xCD]);
    let mut expected = [0u8; 32];
    expected[0] = 0xAB;
    expected[1] = 0xCD;
    assert_eq!(short.as_bytes(), &expected);
    assert_eq!(short.to_vec().len(), 32);

    // Long: truncated to the slot, so the tail is dropped rather than rejected.
    let long = ID::from_slice(&[0x11u8; 40]);
    assert_eq!(long.as_bytes(), &[0x11u8; 32]);

    // Which means two different inputs can land on one id — the property that
    // makes this constructor wrong for a validator.
    let a = ID::from_slice(&[7u8; 33]);
    let b = ID::from_slice(&[7u8; 34]);
    assert_eq!(a, b, "truncation is lossy, and that is the point being stated");

    // Both `From` routes agree with the constructor they wrap.
    assert_eq!(ID::from([9u8; 32]), ID::new([9u8; 32]));
    assert_eq!(ID::from(vec![9u8; 32]), ID::new([9u8; 32]));
    assert_eq!(ID::from(vec![9u8; 40]), ID::new([9u8; 32]));

    // The node identity is 20 bytes that were already 20 bytes, and there is no
    // other way in.
    assert_eq!(NodeID::from([5u8; 20]).as_bytes(), &[5u8; 20]);
}

/// The printed forms are different widths, so a node and a block cannot be
/// confused in a log or matched against the wrong grep. Sixty-four hex
/// characters is a block; forty is a validator.
#[test]
fn the_printed_forms_do_not_read_alike() {
    let block = ID::new([0xAB; 32]);
    let node = node_id(0xAB);

    assert_eq!(block.to_string(), "ab".repeat(32));
    assert_eq!(node.to_string(), "ab".repeat(20));

    assert_eq!(block.to_string().len(), 64);
    assert_eq!(node.to_string().len(), 40);
    assert_ne!(
        block.to_string(),
        node.to_string(),
        "a block and a node printed the same, so a log cannot tell them apart"
    );

    // Zero is a value, not an absence — genesis is `ID::zero()`, and it has to
    // print as something.
    assert_eq!(ID::zero().to_string(), "0".repeat(64));
}

// ------------------------------------------------------- the set, and retraction

struct Signer {
    id: [u8; 20],
    sk: SecretKey,
}

impl Signer {
    fn new(n: u8) -> Self {
        let sk = SecretKey::key_gen(&[n; 32], &[]).expect("key_gen");
        let mut id = [0u8; 20];
        id[0] = n;
        Signer { id, sk }
    }

    fn public(&self) -> [u8; 48] {
        self.sk.sk_to_pk().compress()
    }

    fn pop(&self) -> Vec<u8> {
        pop::sign(&self.sk, &self.id, &self.public())
    }
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

fn quorum_of_four() -> QuasarConfig {
    QuasarConfig {
        k: 4,
        alpha: 0.75, // ceil(0.75 · 4) = 3
        ..QuasarConfig::testnet()
    }
}

/// A retracted validator is out of the set and out of the count, and its seat is
/// free again. A retraction that only hid the member would leave its weight in
/// every denominator a floor is read against, holding the export rung above what
/// the remaining signers can reach.
#[test]
fn a_retracted_validator_leaves_the_set_and_frees_its_seat() {
    let mut quasar = QuasarConsensus::new(&quorum_of_four());
    quasar.add_validator(node_id(1), 100).expect("add");
    quasar.add_validator(node_id(2), 100).expect("add");
    assert_eq!(quasar.validator_count(), 2);

    quasar.remove_validator(&node_id(1));

    assert_eq!(quasar.validator_count(), 1);
    assert!(!quasar.is_validator(&node_id(1)));
    assert!(quasar.is_validator(&node_id(2)));

    // Retracting one that is not there is not a change.
    quasar.remove_validator(&node_id(200));
    assert_eq!(quasar.validator_count(), 1);

    // The seat is free, so the same identity can be admitted again — otherwise
    // a validator could never rejoin after leaving.
    quasar.add_validator(node_id(1), 100).expect("readmit");
    assert_eq!(quasar.validator_count(), 2);
}

/// The set a caller can read is the set the engine decides with. Two sets would
/// mean a certificate verified against the one an operator inspects while the
/// engine accepted it against another, which is a disagreement no test of either
/// alone would find.
#[test]
fn the_set_handed_out_is_the_set_that_decides() {
    let signers: Vec<Signer> = (1..=4).map(Signer::new).collect();
    let mut quasar = QuasarConsensus::new(&quorum_of_four());
    for s in &signers {
        quasar
            .add_validator_with_key(NodeID::from(s.id), 100, &s.public(), &s.pop())
            .expect("register");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let votes: Vec<Vote> = signers
        .iter()
        .map(|s| {
            Vote::new(
                ID::from(pos.block_id),
                VoteType::Preference,
                NodeID::from(s.id),
            )
            .with_signature(s.sk.sign(&message, lux_consensus::cert::DST, &[]).compress().to_vec())
        })
        .collect();

    let cert = quasar
        .create_certificate(pos, &votes)
        .expect("four signed accepts is a certificate");

    // Verified through the engine, and verified directly against the set it
    // hands out — the same verdict from the same weights.
    assert!(quasar.verify_certificate(&cert));
    assert_eq!(
        cert.verify_weighted(quasar.validators(), quasar.validators(), 0),
        Ok(())
    );
    assert_eq!(quasar.validators().len(), 4);
    assert_eq!(cert.tier, Finality::Quasar);
}

/// One validator, one vote in the evidence. A voter that sent two ballots
/// contributes one, and the second is dropped on the way in rather than
/// discovered later by the ordering clause.
///
/// It matters here rather than only at `verify` because the count that decides
/// whether to issue at all is taken over the ACCEPTED list: if duplicates
/// survived it, three ballots from two validators would clear a threshold of
/// three and the engine would try to issue a certificate two signers cannot
/// carry.
#[test]
fn a_validator_contributes_one_vote_to_the_evidence() {
    let signers: Vec<Signer> = (1..=4).map(Signer::new).collect();
    let mut quasar = QuasarConsensus::new(&quorum_of_four());
    for s in &signers {
        quasar
            .add_validator_with_key(NodeID::from(s.id), 100, &s.public(), &s.pop())
            .expect("register");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let ballot = |s: &Signer| {
        Vote::new(
            ID::from(pos.block_id),
            VoteType::Preference,
            NodeID::from(s.id),
        )
        .with_signature(s.sk.sign(&message, lux_consensus::cert::DST, &[]).compress().to_vec())
    };

    // Two validators, one of them shouting: three ballots, two signers, and the
    // threshold is three.
    let shouted = vec![ballot(&signers[0]), ballot(&signers[0]), ballot(&signers[1])];
    assert!(
        matches!(
            quasar.create_certificate(pos.clone(), &shouted),
            Err(ConsensusError::NoQuorum)
        ),
        "a repeated ballot was counted toward the quorum"
    );

    // The same three ballots from three distinct signers do carry.
    let distinct: Vec<Vote> = signers[..3].iter().map(ballot).collect();
    let cert = quasar
        .create_certificate(pos, &distinct)
        .expect("three distinct signers is a quorum");
    assert_eq!(cert.votes.len(), 3, "the evidence carries one vote per signer");
}

/// A ballot against the block is not evidence for it. `create_certificate`
/// reads preference before it reads signatures, so a correctly signed cancel
/// contributes nothing — a finality certificate says a block was accepted, and a
/// vote against it cannot be part of that claim.
#[test]
fn a_signed_cancel_is_not_evidence_of_acceptance() {
    let signers: Vec<Signer> = (1..=4).map(Signer::new).collect();
    let mut quasar = QuasarConsensus::new(&quorum_of_four());
    for s in &signers {
        quasar
            .add_validator_with_key(NodeID::from(s.id), 100, &s.public(), &s.pop())
            .expect("register");
    }

    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let cancels: Vec<Vote> = signers
        .iter()
        .map(|s| {
            Vote::new(ID::from(pos.block_id), VoteType::Cancel, NodeID::from(s.id))
                .with_signature(s.sk.sign(&message, lux_consensus::cert::DST, &[]).compress().to_vec())
        })
        .collect();

    assert!(
        matches!(
            quasar.create_certificate(pos, &cancels),
            Err(ConsensusError::NoQuorum)
        ),
        "four signed cancels were read as four accepts"
    );
}

/// The generated id fills all thirty-two bytes.
///
/// The generator writes four eight-byte lanes of an xorshift stream. A loop
/// bound one short would leave the trailing lane zero on every id it ever
/// produced — a silent halving of the space blocks are named in, and the kind of
/// fault that shows up as a collision months later rather than as a failure now.
/// Each lane is checked across several draws, because any single lane can be
/// zero by chance and a tail that is ALWAYS zero is the bug.
///
/// Distinctness is the other property this helper owes, and it is not asserted
/// here: the seed is the wall clock in nanoseconds, so whether two adjacent
/// calls differ is a fact about the host's clock granularity rather than about
/// this code, and a test of it would pass or fail by machine.
#[test]
fn a_generated_id_fills_every_lane() {
    let draws: Vec<ID> = (0..16).map(|_| lux_consensus::generate_block_id()).collect();

    for lane in 0..4 {
        let range = lane * 8..(lane + 1) * 8;
        assert!(
            draws
                .iter()
                .any(|d| d.as_bytes()[range.clone()].iter().any(|b| *b != 0)),
            "bytes {range:?} were zero in every draw — the generator does not fill them"
        );
    }
}

/// The advertised version is a version.
///
/// It reads the crate's own version out of the build environment, so asserting
/// it equals that is asserting a definition. What is worth holding is the SHAPE:
/// the neighbouring build variable is the crate NAME, and a version reported as
/// `lux-consensus` — or as a git hash, or as an empty string — is a handshake
/// value no peer can compare against its own.
#[test]
fn the_advertised_version_is_a_version() {
    let v = lux_consensus::version();
    let parts: Vec<&str> = v.split('.').collect();

    assert_eq!(parts.len(), 3, "{v:?} is not major.minor.patch");
    for part in parts {
        assert!(
            !part.is_empty() && part.chars().all(|c| c.is_ascii_digit()),
            "{v:?} has a non-numeric component {part:?}"
        );
    }
}
