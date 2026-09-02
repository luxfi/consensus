// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The assembled engine: which doors are shut, and who is allowed through them.
//!
//! `tests/cert.rs` holds the certificate predicate. This holds the layer above
//! it — the object a node actually drives — and the question here is not whether
//! a good block is accepted but whether anything can reach the tally without
//! being a member, and whether the engine does anything at all before it is
//! started. Both are doors that failed open once.

use lux_consensus::{
    quick_start, ConsensusError, Engine, EventHorizon, NodeID, QuasarConfig,
    QuasarConsensus, QuasarEngine, Status, VoteType, ID,
};

fn id(n: u8) -> ID {
    ID::new([n; 32])
}

fn voter(n: u8) -> NodeID {
    NodeID([n; 20])
}

/// A committee small enough to decide inside one test: k of 3, a quorum of 2,
/// and one round of agreement is finality. The engine's doors are what is under
/// test, not the depth of the confidence ladder.
fn quorum_of_three() -> QuasarConfig {
    QuasarConfig {
        k: 3,
        alpha: 0.6, // ceil(0.6 · 3) = 2
        beta: 1,
        enable_fpc: false,
        ..QuasarConfig::default()
    }
}

fn started(config: QuasarConfig, members: u8) -> QuasarEngine {
    let mut engine = QuasarEngine::new(config);
    for i in 1..=members {
        engine.add_validator(voter(i), 100).expect("add_validator");
    }
    engine.start().expect("start");
    engine
}

fn block(n: u8, height: u64) -> lux_consensus::Block {
    lux_consensus::new_block(id(n), ID::zero(), height, Vec::new())
}

// ---------------------------------------------------------------- the start door

/// An engine that was never started holds no blocks and counts no votes. The
/// check is on every entry point rather than on a flag the caller is trusted to
/// respect, because "not started" is the state a node is in while it is still
/// loading the validator set — the exact window in which counting a vote means
/// counting it against a set that is not there yet.
#[test]
fn nothing_enters_an_engine_that_was_never_started() {
    let mut engine = QuasarEngine::new(quorum_of_three());

    assert!(
        matches!(engine.add(block(1, 1)), Err(ConsensusError::NotInitialized)),
        "a block entered an unstarted engine"
    );

    let vote = lux_consensus::new_vote(id(1), VoteType::Preference, voter(1));
    assert!(
        matches!(engine.record_vote(vote), Err(ConsensusError::NotInitialized)),
        "a vote was counted by an unstarted engine"
    );
}

/// Starting twice is refused rather than repeated. A second start would reinsert
/// genesis and reset nothing else, leaving an engine whose accepted set and
/// whose height disagree about what it has decided.
#[test]
fn an_engine_is_started_once() {
    let mut engine = QuasarEngine::new(quorum_of_three());
    engine.start().expect("start");

    assert!(
        matches!(engine.start(), Err(ConsensusError::AlreadyStarted)),
        "the engine restarted over its own state"
    );
}

/// Starting seats genesis as accepted, because a chain with no accepted block
/// has no parent for the first real one to descend from.
#[test]
fn starting_seats_genesis_as_accepted() {
    let engine = started(quorum_of_three(), 3);
    let genesis = ID::zero();

    assert!(engine.is_accepted(&genesis));
    assert_eq!(engine.get_status(&genesis), Status::Accepted);
    assert_eq!(engine.height(), 0, "genesis is height zero");
}

/// Stopping shuts the doors again. A stopped engine that still accepted votes
/// would keep deciding while the node believes it has withdrawn.
#[test]
fn stopping_shuts_the_doors_again() {
    let mut engine = started(quorum_of_three(), 3);
    engine.stop().expect("stop");

    assert!(matches!(
        engine.add(block(1, 1)),
        Err(ConsensusError::NotInitialized)
    ));
    assert!(matches!(
        engine.record_vote(lux_consensus::new_vote(id(1), VoteType::Preference, voter(1))),
        Err(ConsensusError::NotInitialized)
    ));
}

/// `quick_start` is the started engine, not a constructor that leaves the caller
/// to remember the second call — an engine handed out unstarted refuses
/// everything, which is a confusing way to be safe.
#[test]
fn quick_start_hands_back_a_running_engine() {
    let mut engine = quick_start().expect("quick_start");

    assert!(engine.is_accepted(&ID::zero()), "genesis is not accepted");
    assert!(
        matches!(engine.start(), Err(ConsensusError::AlreadyStarted)),
        "quick_start returned an engine that had not been started"
    );
}

// ------------------------------------------------------------ the membership door

/// A vote from outside the set is refused, and an engine holding no validators
/// therefore counts nobody.
///
/// This is the door that used to be open: the sample was whoever happened to
/// send a message, and unregistered node ids could carry a block to Accepted
/// against an engine with an empty validator set. Failing closed on an empty set
/// is the direction to fail in — a node still loading its validators decides
/// nothing rather than deciding on strangers.
#[test]
fn an_engine_with_no_validators_counts_nobody() {
    let mut engine = QuasarEngine::new(quorum_of_three());
    engine.start().expect("start");
    engine.add(block(1, 1)).expect("add");

    for i in 1..=9u8 {
        assert!(
            matches!(
                engine.record_vote(lux_consensus::new_vote(id(1), VoteType::Preference, voter(i))),
                Err(ConsensusError::NotValidator)
            ),
            "voter {i} was counted by an engine that holds no validators"
        );
    }

    assert!(!engine.is_accepted(&id(1)), "strangers carried a block");
    assert_eq!(engine.get_status(&id(1)), Status::Processing);
}

/// A vote for a block the engine has never seen is refused before membership is
/// even consulted — there is no tally for it to join, and creating one on demand
/// would let a stranger allocate state by naming an id.
#[test]
fn a_vote_for_an_unknown_block_is_refused() {
    let mut engine = started(quorum_of_three(), 3);

    assert!(
        matches!(
            engine.record_vote(lux_consensus::new_vote(id(9), VoteType::Preference, voter(1))),
            Err(ConsensusError::BlockNotFound)
        ),
        "a vote created a tally for a block nobody proposed"
    );
}

/// The batch entry point is the single one applied in a loop, so it counts what
/// was actually recorded rather than what was offered. A batch that reported its
/// own length would tell a caller nine strangers had voted.
#[test]
fn a_batch_counts_only_the_votes_that_were_taken() {
    let mut engine = started(quorum_of_three(), 2);
    engine.add(block(1, 1)).expect("add");

    let votes = vec![
        lux_consensus::new_vote(id(1), VoteType::Preference, voter(1)), // member
        lux_consensus::new_vote(id(1), VoteType::Preference, voter(2)), // member
        lux_consensus::new_vote(id(1), VoteType::Preference, voter(3)), // stranger
        lux_consensus::new_vote(id(9), VoteType::Preference, voter(1)), // unknown block
    ];

    assert_eq!(
        engine.record_votes_batch(votes),
        2,
        "the batch reported votes it did not take"
    );
}

/// An id the engine has never been told about has no status, not a default one.
/// `Unknown` and `Processing` are different answers and a consumer that gated on
/// the second would treat every unheard-of block as in flight.
#[test]
fn an_unheard_of_block_has_no_status() {
    let engine = started(quorum_of_three(), 3);

    assert_eq!(engine.get_status(&id(200)), Status::Unknown);
    assert!(!engine.is_accepted(&id(200)));
}

// ------------------------------------------------------------------ the decision

/// Members carry a block to accepted, and the accepted height follows the block.
/// This is the path every door above exists to protect, so it has to be shown to
/// work — a gate that refuses everything passes every refusal test.
#[test]
fn members_carry_a_block_to_accepted_and_the_height_follows() {
    let mut engine = started(quorum_of_three(), 3);
    engine.add(block(1, 7)).expect("add");

    for i in 1..=3u8 {
        engine
            .record_vote(lux_consensus::new_vote(id(1), VoteType::Preference, voter(i)))
            .expect("a member's vote was refused");
    }

    assert!(engine.is_accepted(&id(1)), "three of three did not decide");
    assert_eq!(engine.get_status(&id(1)), Status::Accepted);
    assert_eq!(engine.height(), 7, "the accepted height did not follow the block");
}

/// The same committee voting to cancel rejects the block instead. Accept and
/// reject are the two ends of one fold, and a tracker that only ever reached one
/// of them would look identical on every test above.
#[test]
fn the_same_committee_voting_to_cancel_rejects_the_block() {
    let mut engine = started(quorum_of_three(), 3);
    engine.add(block(1, 3)).expect("add");

    for i in 1..=3u8 {
        engine
            .record_vote(lux_consensus::new_vote(id(1), VoteType::Cancel, voter(i)))
            .expect("a member's vote was refused");
    }

    assert_eq!(engine.get_status(&id(1)), Status::Rejected);
    assert!(!engine.is_accepted(&id(1)));
    assert_eq!(engine.height(), 0, "a rejected block moved the height");
}

// ---------------------------------------------------------------- the set itself

/// One node, one admission. A second registration of the same id is refused
/// rather than restated, because a set that took it would hold one operator in
/// two seats — two signer slots and two shares of the weight under one identity.
#[test]
fn a_node_is_admitted_once() {
    let mut quasar = QuasarConsensus::new(&quorum_of_three());
    quasar.add_validator(voter(1), 100).expect("add_validator");

    assert!(
        quasar.add_validator(voter(1), 100).is_err(),
        "one node took two seats"
    );
    assert_eq!(quasar.validator_count(), 1);
    assert!(quasar.is_validator(&voter(1)));
    assert!(!quasar.is_validator(&voter(2)));
}

/// The quorum question is asked of the set, so it is false until the set is big
/// enough — an engine that reported a quorum over two members would issue
/// certificates a third of the committee never saw.
#[test]
fn there_is_no_quorum_until_the_set_reaches_it() {
    let mut quasar = QuasarConsensus::new(&quorum_of_three());

    assert!(!quasar.has_quorum(), "an empty set reported a quorum");
    quasar.add_validator(voter(1), 100).expect("add_validator");
    assert!(!quasar.has_quorum(), "one of a two-member quorum reported one");
    quasar.add_validator(voter(2), 100).expect("add_validator");
    assert!(quasar.has_quorum());
}

/// Nothing is finalized until a certificate says so, and an engine whose ballots
/// carry no signatures produces none. This is the honest outcome rather than a
/// gap: the accept status above is Wave's preference, and finality is a separate
/// claim that needs evidence.
#[test]
fn unsigned_ballots_finalize_nothing() {
    let mut engine = started(quorum_of_three(), 3);
    engine.add(block(1, 7)).expect("add");
    for i in 1..=3u8 {
        engine
            .record_vote(lux_consensus::new_vote(id(1), VoteType::Preference, voter(i)))
            .expect("record_vote");
    }

    assert!(engine.is_accepted(&id(1)), "the preference did not converge");

    let quasar = QuasarConsensus::new(&quorum_of_three());
    assert!(
        !quasar.is_finalized(&id(1)),
        "a block with no signed evidence was reported final"
    );
    assert!(quasar.get_certificate(&id(1)).is_none());
}

// ----------------------------------------------------------------- event horizon

/// A block for a chain nobody registered is dropped, not filed. The horizon's
/// height is the count of blocks it actually holds, and a block accepted into no
/// chain would advance a height that indexes nothing.
#[test]
fn the_horizon_ignores_a_block_from_an_unregistered_chain() {
    let mut horizon = EventHorizon::new(&quorum_of_three());

    horizon.accept_block("zoo", id(1));
    assert_eq!(horizon.height(), 0, "an unregistered chain moved the height");

    horizon.register_chain("zoo".to_string());
    horizon.accept_block("zoo", id(1));
    horizon.accept_block("zoo", id(2));
    assert_eq!(horizon.height(), 2);

    // Registering an existing chain keeps what it holds, so a re-announcement
    // cannot silently drop a chain's history.
    horizon.register_chain("zoo".to_string());
    horizon.accept_block("zoo", id(3));
    assert_eq!(horizon.height(), 3);
}

/// The horizon holds one consensus, reachable both ways, so a caller registering
/// validators through it is registering them in the set the horizon decides with.
#[test]
fn the_horizon_and_its_consensus_are_one_set() {
    let mut horizon = EventHorizon::new(&quorum_of_three());

    horizon
        .quasar_mut()
        .add_validator(voter(1), 100)
        .expect("add_validator");

    assert_eq!(horizon.quasar().validator_count(), 1);
    assert!(horizon.quasar().is_validator(&voter(1)));
}

// ------------------------------------------------------------------ the vocabulary

/// Every consensus refusal prints its own sentence. Two clauses that read alike
/// are one clause in a log, and the three that carry a message must print it —
/// a `CryptoError` that dropped its detail would report only that something
/// cryptographic went wrong.
#[test]
fn every_engine_refusal_names_itself() {
    let cases = vec![
        (ConsensusError::BlockNotFound, "Block not found"),
        (ConsensusError::InvalidBlock, "Invalid block"),
        (ConsensusError::InvalidVote, "Invalid vote"),
        (ConsensusError::InvalidSignature, "Invalid signature"),
        (ConsensusError::NoQuorum, "No quorum reached"),
        (ConsensusError::AlreadyVoted, "Already voted"),
        (ConsensusError::NotValidator, "Not a validator"),
        (ConsensusError::Timeout, "Operation timeout"),
        (ConsensusError::NotInitialized, "Engine not initialized"),
        (ConsensusError::AlreadyStarted, "Engine already started"),
    ];

    let mut seen: Vec<String> = Vec::new();
    for (err, expected) in cases {
        let text = err.to_string();
        assert_eq!(text, expected, "{err:?} does not name itself");
        assert!(!seen.contains(&text), "{text:?} names two different clauses");
        seen.push(text);
    }

    // The three that carry detail must carry it through.
    assert_eq!(
        ConsensusError::CryptoError("bad point".into()).to_string(),
        "Crypto error: bad point"
    );
    assert_eq!(
        ConsensusError::NetworkError("peer gone".into()).to_string(),
        "Network error: peer gone"
    );
    assert_eq!(ConsensusError::Other("anything".into()).to_string(), "anything");
}

/// A vote is a preference for the block or against it, and the two commit types
/// are both for it. Wave counts on this predicate, so a type that fell on the
/// wrong side would be tallied as its opposite.
#[test]
fn a_commit_is_a_preference_and_a_cancel_is_not() {
    let for_it = lux_consensus::new_vote(id(1), VoteType::Preference, voter(1));
    let also_for_it = lux_consensus::new_vote(id(1), VoteType::Commit, voter(1));
    let against_it = lux_consensus::new_vote(id(1), VoteType::Cancel, voter(1));

    assert!(for_it.prefer());
    assert!(also_for_it.prefer());
    assert!(!against_it.prefer());

    // A ballot carries no signature until one is attached, and attaching one
    // does not change what it prefers.
    assert!(for_it.signature.is_empty());
    let signed = lux_consensus::new_vote(id(1), VoteType::Preference, voter(1))
        .with_signature(vec![9u8; 96]);
    assert_eq!(signed.signature, vec![9u8; 96]);
    assert!(signed.prefer());
}

/// A block still short of a quorum is in flight, not rejected. Undecided is a
/// third answer, and an engine that collapsed it into Rejected would refuse
/// every block whose votes were still arriving.
#[test]
fn a_block_short_of_a_quorum_is_in_flight_and_not_rejected() {
    let mut engine = started(quorum_of_three(), 3);
    engine.add(block(1, 4)).expect("add");

    // Two of a committee of three: fewer than k, so no round has closed.
    for i in 1..=2u8 {
        engine
            .record_vote(lux_consensus::new_vote(id(1), VoteType::Preference, voter(i)))
            .expect("record_vote");
    }

    assert_eq!(engine.get_status(&id(1)), Status::Processing);
    assert!(!engine.is_accepted(&id(1)));
    assert_ne!(engine.get_status(&id(1)), Status::Rejected);
    assert_eq!(engine.height(), 0, "an undecided block moved the height");
}

/// The engine reports the configuration it was built with, so a node that asked
/// for the mainnet committee is running the mainnet committee. The presets are
/// stated as diffs to one default, and this is the reading that would catch a
/// diff applied to the wrong field.
#[test]
fn an_engine_runs_the_configuration_it_was_given() {
    assert_eq!(QuasarEngine::testnet().config().k, QuasarConfig::testnet().k);
    assert_eq!(QuasarEngine::mainnet().config().k, QuasarConfig::mainnet().k);
    assert_eq!(QuasarEngine::default().config().k, QuasarConfig::default().k);

    // The three are genuinely different committees, so the presets are not one
    // config under three names.
    assert_ne!(QuasarConfig::testnet().k, QuasarConfig::mainnet().k);
    assert_ne!(QuasarConfig::default().k, QuasarConfig::mainnet().k);

    // Mainnet's committee is odd, so a split has a side — the reason 21 was
    // chosen over 20.
    assert_eq!(QuasarConfig::mainnet().k % 2, 1);
}
