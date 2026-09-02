// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! Confidence over rounds, through the two trackers that hold it.
//!
//! `tests/confidence.rs` holds the fold itself — `focus::accumulate`, the one
//! place the β-consecutive rule lives. This file holds the two things that call
//! it: Wave, which reads a round as counts against a threshold, and Focus, which
//! reads one as a ratio against alpha. The fold being right does not make either
//! caller right: a tracker that re-entered a decided item, counted one validator
//! twice, or decided on fewer than k answers would pass every test of the fold.

use std::time::Duration;

use lux_consensus::{
    new_vote, Decision, Focus, NodeID, QuasarConfig, VoteType, Wave, WindowedFocus, ID,
};

fn id(n: u8) -> ID {
    ID::new([n; 32])
}

fn voter(n: u8) -> NodeID {
    NodeID([n; 20])
}

/// A committee of three, a quorum of two, one round of agreement. Small enough
/// that a decision is reachable inside a test, so the interesting cases are the
/// ones that must NOT reach it.
fn quick() -> QuasarConfig {
    QuasarConfig {
        k: 3,
        alpha: 0.6, // ceil(0.6 · 3) = 2
        beta: 1,
        enable_fpc: false,
        ..QuasarConfig::default()
    }
}

fn yes(block: u8, from: u8) -> lux_consensus::Vote {
    new_vote(id(block), VoteType::Preference, voter(from))
}

fn no(block: u8, from: u8) -> lux_consensus::Vote {
    new_vote(id(block), VoteType::Cancel, voter(from))
}

// ------------------------------------------------------------------------- wave

/// One validator, one ballot. A repeat from the same voter is dropped rather
/// than tallied, so a peer cannot reach a quorum by answering the same question
/// several times — which is the distinctness clause a certificate enforces by
/// ordering, held here at the round instead.
#[test]
fn a_validator_votes_once_per_block() {
    let mut wave = Wave::new(quick());

    assert!(!wave.record_vote(yes(1, 1)));
    for _ in 0..10 {
        assert!(!wave.record_vote(yes(1, 1)), "a repeat decided the block");
    }

    let state = wave.state(&id(1)).expect("state");
    assert_eq!(state.yes_count, 1, "one voter was counted {} times", state.yes_count);
    assert_eq!(state.votes.len(), 1);
    assert!(!wave.is_decided(&id(1)));
}

/// Fewer than k answers is not a round. Deciding on two of a committee of three
/// would be deciding on whoever replied first, which is a different — and
/// smaller — quorum than the one the parameters describe.
#[test]
fn fewer_than_k_answers_is_not_a_round() {
    let mut wave = Wave::new(quick());

    assert!(!wave.record_vote(yes(1, 1)));
    assert!(!wave.record_vote(yes(1, 2)));
    assert_eq!(
        wave.state(&id(1)).expect("state").confidence,
        0,
        "confidence accumulated before k answers were in"
    );

    assert!(wave.record_vote(yes(1, 3)), "k answers did not close the round");
    assert!(wave.is_decided(&id(1)));
    assert_eq!(wave.decision(&id(1)), Decision::Accept);
}

/// A decided block is finished. Later votes are dropped rather than folded in,
/// so a decision cannot be walked back by whoever answers last — the whole point
/// of β consecutive rounds is that the answer stops moving.
#[test]
fn a_decided_block_does_not_reopen() {
    let mut wave = Wave::new(quick());
    for i in 1..=3u8 {
        wave.record_vote(yes(1, i));
    }
    assert_eq!(wave.decision(&id(1)), Decision::Accept);

    for i in 4..=9u8 {
        assert!(
            !wave.record_vote(no(1, i)),
            "voter {i} reopened a decided block"
        );
    }
    assert_eq!(wave.decision(&id(1)), Decision::Accept);
}

/// The same committee against the block decides the other way. Both ends of the
/// fold have to be reachable through Wave's counting, or the reject branch is
/// only ever exercised through `accumulate` directly.
#[test]
fn a_committee_against_the_block_rejects_it() {
    let mut wave = Wave::new(quick());
    for i in 1..=3u8 {
        wave.record_vote(no(1, i));
    }

    assert!(wave.is_decided(&id(1)));
    assert_eq!(wave.decision(&id(1)), Decision::Reject);
    assert_eq!(wave.state(&id(1)).expect("state").no_count, 3);
}

/// A split committee decides nothing. With a quorum of two out of three, one
/// answer either way clears neither threshold, and the round has to produce no
/// verdict rather than falling to a default.
#[test]
fn a_split_committee_reaches_no_verdict() {
    let config = QuasarConfig {
        k: 4,
        alpha: 0.75, // ceil(0.75 · 4) = 3
        beta: 1,
        enable_fpc: false,
        ..QuasarConfig::default()
    };
    let mut wave = Wave::new(config);

    for i in 1..=2u8 {
        wave.record_vote(yes(1, i));
    }
    for i in 3..=4u8 {
        wave.record_vote(no(1, i));
    }

    assert!(!wave.is_decided(&id(1)), "two against two decided");
    assert_eq!(wave.decision(&id(1)), Decision::Undecided);
    assert_eq!(wave.state(&id(1)).expect("state").confidence, 0);
}

/// Asking what the threshold is does not move the phase on.
///
/// It used to: the phase advanced inside the getter, so the FPC schedule stepped
/// once per QUESTION rather than once per round, and two nodes that read the
/// threshold a different number of times drew different θ for the same round —
/// a fork by arithmetic with no bad actor in it.
#[test]
fn asking_for_the_threshold_does_not_advance_the_phase() {
    let mut wave = Wave::new(QuasarConfig {
        enable_fpc: true,
        ..quick()
    });

    let phase = wave.phase();
    let first = wave.threshold();
    for _ in 0..20 {
        assert_eq!(wave.threshold(), first, "the threshold moved while being read");
    }
    assert_eq!(wave.phase(), phase, "reading the threshold advanced the phase");

    // A vote is what advances it — the round, not the question.
    wave.record_vote(yes(1, 1));
    assert!(wave.phase() > phase, "a recorded vote did not advance the phase");
}

/// With FPC off the threshold is the configured count and the phase never moves,
/// so a node running fixed thresholds cannot drift into a schedule it is not
/// following.
#[test]
fn a_fixed_threshold_never_draws_a_phase() {
    let config = quick();
    let expected = config.alpha_count();
    let mut wave = Wave::new(config);

    assert_eq!(wave.threshold(), expected);
    for i in 1..=3u8 {
        wave.record_vote(yes(1, i));
    }
    assert_eq!(wave.phase(), 0, "a fixed-threshold engine advanced a phase");
    assert_eq!(wave.threshold(), expected, "the fixed threshold moved");
}

/// A block nobody has voted on has no state, and its decision is undecided
/// rather than absent — the two are different answers, and a consumer reading
/// the second as a rejection would reject every block it had not yet heard of.
#[test]
fn an_unheard_of_block_is_undecided_and_holds_no_state() {
    let mut wave = Wave::new(quick());

    assert!(wave.state(&id(9)).is_none());
    assert!(!wave.is_decided(&id(9)));
    assert_eq!(wave.decision(&id(9)), Decision::Undecided);

    // Opening a tally for it is explicit, and starts empty.
    let state = wave.get_or_create_state(&id(9));
    assert_eq!((state.yes_count, state.no_count, state.confidence), (0, 0, 0));
    assert!(!state.decided);
    assert!(wave.state(&id(9)).is_some());
}

/// Resetting drops the tally, so the same block can be voted on again from
/// nothing. A reset that left the counts would carry the old round's agreement
/// into the new one, which is exactly what β consecutive is supposed to prevent.
#[test]
fn resetting_a_block_drops_what_was_counted() {
    let mut wave = Wave::new(quick());
    for i in 1..=3u8 {
        wave.record_vote(yes(1, i));
    }
    assert!(wave.is_decided(&id(1)));

    wave.reset(&id(1));
    assert!(wave.state(&id(1)).is_none());
    assert_eq!(wave.decision(&id(1)), Decision::Undecided);

    // And the same voters can answer again.
    assert!(!wave.record_vote(yes(1, 1)));
    assert_eq!(wave.state(&id(1)).expect("state").yes_count, 1);
}

// ------------------------------------------------------------------------ focus

/// A round with no answers in it is not a round. Zero votes is silence, not a
/// verdict, and folding it as one would reset confidence every time a sample
/// came back empty — turning a partition into a permanent stall.
#[test]
fn a_round_with_no_answers_is_not_folded() {
    let mut focus: Focus<u8> = Focus::new(3, 0.6);

    focus.update(1, 3, 3);
    assert_eq!(focus.confidence(&1), 1);

    assert!(!focus.update(1, 0, 0), "an empty round returned a decision");
    assert_eq!(
        focus.confidence(&1),
        1,
        "an empty round was folded in and moved the confidence"
    );
}

/// β consecutive rounds decide, and a round that reaches no quorum starts the
/// count again. The fold is held in `tests/confidence.rs`; this is the same rule
/// read as a RATIO against alpha, which is the reading Focus applies and Wave
/// does not.
#[test]
fn beta_consecutive_ratios_decide_and_a_middling_round_resets() {
    let mut focus: Focus<u8> = Focus::new(3, 0.7);

    // 3 of 4 is 0.75, over alpha.
    assert!(!focus.update(1, 3, 4));
    assert!(!focus.update(1, 3, 4));
    assert_eq!(focus.confidence(&1), 2);

    // 2 of 4 is 0.5, which is neither above alpha nor below 1-alpha: no quorum.
    assert!(!focus.update(1, 2, 4));
    assert_eq!(focus.confidence(&1), 0, "a middling round did not reset the run");
    assert!(!focus.is_decided(&1));

    for _ in 0..2 {
        assert!(!focus.update(1, 4, 4));
    }
    assert!(focus.update(1, 4, 4), "three consecutive rounds did not decide");
    assert!(focus.is_decided(&1));
    assert_eq!(focus.decision(&1), Decision::Accept);
}

/// A run of rounds below `1 - alpha` decides against the item, so the tracker
/// reaches both ends of the fold from a ratio as well as from counts.
#[test]
fn a_run_of_rounds_against_the_item_rejects_it() {
    let mut focus: Focus<u8> = Focus::new(2, 0.7);

    // 1 of 4 is 0.25, below 1 - 0.7.
    assert!(!focus.update(1, 1, 4));
    assert!(focus.update(1, 1, 4));
    assert_eq!(focus.decision(&1), Decision::Reject);
}

/// A decided item is finished: further rounds return no new decision and do not
/// move its state. Reporting `true` again would tell a caller a second decision
/// had been reached about a block that had already been settled once.
#[test]
fn a_decided_item_does_not_reopen() {
    let mut focus: Focus<u8> = Focus::new(1, 0.6);
    assert!(focus.update(1, 4, 4));
    let settled = focus.decision(&1);

    for _ in 0..5 {
        assert!(!focus.update(1, 0, 4), "a decided item reported a new decision");
    }
    assert_eq!(focus.decision(&1), settled);
    assert_eq!(focus.state(&1).expect("state").last_ratio, 1.0);
}

/// An item nobody has reported on reads as undecided with no confidence, rather
/// than as absent state a caller has to special-case.
#[test]
fn an_unreported_item_is_undecided_with_no_confidence() {
    let focus: Focus<u8> = Focus::new(3, 0.6);

    assert!(focus.state(&7).is_none());
    assert!(!focus.is_decided(&7));
    assert_eq!(focus.decision(&7), Decision::Undecided);
    assert_eq!(focus.confidence(&7), 0);
}

/// Resetting an item drops its run, so a block being re-litigated starts from
/// nothing rather than from the agreement it had before.
#[test]
fn resetting_an_item_drops_its_run() {
    let mut focus: Focus<u8> = Focus::new(3, 0.6);
    focus.update(1, 4, 4);
    focus.update(1, 4, 4);
    assert_eq!(focus.confidence(&1), 2);

    focus.reset(&1);
    assert_eq!(focus.confidence(&1), 0);
    assert!(focus.state(&1).is_none());
}

// --------------------------------------------------------------- windowed focus

/// Inside the window a run accumulates as it does without one — the window is an
/// expiry, not a second rule about what counts.
#[test]
fn a_run_inside_the_window_accumulates_normally() {
    let mut focus: WindowedFocus<u8> = WindowedFocus::new(3, 0.6, Duration::from_secs(3600));

    assert!(!focus.update(1, 4, 4));
    assert!(!focus.update(1, 4, 4));
    assert!(focus.update(1, 4, 4), "three rounds inside the window did not decide");
    assert!(focus.is_decided(&1));
    assert_eq!(focus.decision(&1), Decision::Accept);
}

/// A gap longer than the window drops the run. β must be β CONSECUTIVE rounds,
/// and two rounds an hour apart are not consecutive in any sense a liveness
/// argument can use — without the expiry a block could collect its agreement
/// across a partition it never actually survived.
#[test]
fn a_gap_longer_than_the_window_drops_the_run() {
    let window = Duration::from_millis(1);
    let mut focus: WindowedFocus<u8> = WindowedFocus::new(2, 0.6, window);

    assert!(!focus.update(1, 4, 4));

    // Longer than the window, so the next round starts a fresh run rather than
    // completing this one.
    std::thread::sleep(window * 5);

    assert!(
        !focus.update(1, 4, 4),
        "a round after the window elapsed completed a run it should have started"
    );
    assert!(!focus.is_decided(&1));
    assert_eq!(focus.decision(&1), Decision::Undecided);

    // Immediately after, the fresh run completes.
    assert!(focus.update(1, 4, 4), "the fresh run did not accumulate");
    assert_eq!(focus.decision(&1), Decision::Accept);
}
