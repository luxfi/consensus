// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The β accumulator.
//!
//! Wave counts votes and Focus takes a ratio, but both fold their rounds with
//! this one function, so the rule about consecutive agreement is stated once.
//! The property worth pinning is the reset: β has to mean β *consecutive*
//! rounds, because a counter that only ever climbs would let a block collect
//! agreement across rounds that disagreed in between.

use lux_consensus::focus::accumulate;
use lux_consensus::Decision;

/// Run a sequence of verdicts and return the round each decision landed on.
fn run(verdicts: &[Option<bool>], beta: u32) -> Option<(usize, Decision)> {
    let mut preference = false;
    let mut confidence = 0u32;
    for (i, v) in verdicts.iter().enumerate() {
        if let Some(d) = accumulate(&mut preference, &mut confidence, *v, beta) {
            return Some((i, d));
        }
    }
    None
}

/// β agreeing rounds decide, and not one round sooner.
#[test]
fn beta_consecutive_rounds_decide() {
    let yes = vec![Some(true); 8];
    assert_eq!(run(&yes[..2], 3), None);
    assert_eq!(run(&yes[..3], 3), Some((2, Decision::Accept)));

    let no = vec![Some(false); 8];
    // The standing preference starts false, so the first NO agrees with it and
    // confidence reaches beta one round earlier than a switch would.
    assert_eq!(run(&no[..1], 1), Some((0, Decision::Reject)));
}

/// A round with no quorum resets the count. This is the whole point of the
/// mechanism, and the property a naive counter would lose.
#[test]
fn an_undecided_round_resets_confidence() {
    // Two agreeing rounds, a round with no quorum, then two more: never β=3
    // consecutive, so no decision across five rounds.
    let interrupted = [Some(true), Some(true), None, Some(true), Some(true)];
    assert_eq!(run(&interrupted, 3), None);

    // The same five rounds without the interruption decide on the third.
    let clean = [Some(true); 5];
    assert_eq!(run(&clean, 3), Some((2, Decision::Accept)));
}

/// A verdict that opposes the standing preference replaces it and starts the
/// count again at one, rather than merely resetting to zero — the opposing
/// round is itself evidence for the new preference.
#[test]
fn an_opposing_round_switches_preference_and_restarts_at_one() {
    let flip = [Some(true), Some(true), Some(false), Some(false), Some(false)];
    assert_eq!(run(&flip, 3), Some((4, Decision::Reject)));

    // Restarting at one, not zero: three NOs after the switch, the switch
    // itself counting as the first.
    let mut preference = false;
    let mut confidence = 0u32;
    accumulate(&mut preference, &mut confidence, Some(true), 5);
    accumulate(&mut preference, &mut confidence, Some(false), 5);
    assert_eq!((preference, confidence), (false, 1));
}

/// Alternating rounds never decide, however long they run. A block the network
/// keeps changing its mind about does not become final by attrition.
#[test]
fn alternating_rounds_never_decide() {
    let alternating: Vec<Option<bool>> =
        (0..1000).map(|i| Some(i % 2 == 0)).collect();
    assert_eq!(run(&alternating, 2), None);
}

/// β of zero decides immediately whatever the verdict — including on a round
/// with no quorum at all. Callers must configure a positive β; this pins the
/// behavior so the boundary is visible rather than surprising.
#[test]
fn beta_zero_decides_immediately() {
    let mut preference = false;
    let mut confidence = 0u32;
    assert_eq!(
        accumulate(&mut preference, &mut confidence, None, 0),
        Some(Decision::Reject),
    );
}

/// Confidence never runs away: it is bounded by the number of consecutive
/// agreeing rounds, and a decision is reported the moment it reaches β.
#[test]
fn confidence_tracks_the_run_length() {
    let mut preference = true;
    let mut confidence = 0u32;
    for round in 1..=6u32 {
        let decided = accumulate(&mut preference, &mut confidence, Some(true), 100);
        assert_eq!(decided, None);
        assert_eq!(confidence, round, "round {round}");
    }
}
