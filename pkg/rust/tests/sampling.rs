// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! Who gets asked, and how hard the answer has to be.
//!
//! Two numbers decide a round before any vote is cast: the peers sampled, and
//! the threshold their answers are read against. Both are derived rather than
//! configured at the point of use, and both have a range they must stay inside —
//! a brightness that ran away would let one peer own the sample, and a θ outside
//! its band would make every round either unanimous or free.

use lux_consensus::{FpcSelector, Luminance, NodeID, PhotonSampler, QuasarConfig};

fn peer(n: u8) -> NodeID {
    NodeID([n; 20])
}

// -------------------------------------------------------------- the threshold band

/// A θ range outside (0,1) is replaced by the standard band, not accepted.
///
/// The bound matters because θ is the fraction of a committee that has to agree:
/// θ = 0 makes every round a quorum and θ = 1 demands unanimity from a sample
/// that is allowed to miss peers. A selector built from a mis-parsed config must
/// land on the standard band rather than on whichever of those two the bad value
/// happened to be.
#[test]
fn a_theta_outside_the_band_falls_back_to_the_standard_one() {
    let seed = [0u8; 32];
    let standard = (0.5, 0.8);

    for bad_min in [0.0, 1.0, -1.0, 2.0] {
        assert_eq!(
            FpcSelector::new(bad_min, 0.8, seed).range(),
            standard,
            "theta_min = {bad_min} was taken as given"
        );
    }

    // A maximum that is not above the minimum is not a band at all.
    for bad_max in [0.5, 0.4, 0.0, 1.5] {
        assert_eq!(
            FpcSelector::new(0.5, bad_max, seed).range(),
            standard,
            "theta_max = {bad_max} was taken as given"
        );
    }

    // A band inside the bounds is kept, so the fallback is a repair and not a
    // policy that overrides the operator.
    assert_eq!(FpcSelector::new(0.6, 0.9, seed).range(), (0.6, 0.9));
}

/// Every θ the selector draws lands inside its own band, at every phase. The
/// threshold is `ceil(θ·k)`, so a θ that escaped the band would put the quorum
/// above k — unreachable — or at zero, where no votes are a quorum.
#[test]
fn every_drawn_threshold_stays_inside_the_band_and_the_committee() {
    let selector = FpcSelector::default();
    let (lo, hi) = selector.range();
    let k = 21;

    for phase in 0..64u64 {
        let theta = selector.theta(phase);
        assert!(
            theta >= lo && theta <= hi,
            "phase {phase} drew theta {theta}, outside [{lo}, {hi}]"
        );

        let alpha = selector.select_threshold(phase, k);
        assert!(
            alpha >= 1 && alpha <= k,
            "phase {phase} asked for {alpha} of {k}"
        );
        assert!(
            alpha > k / 2,
            "phase {phase} asked for {alpha} of {k}, which is not a majority"
        );
    }
}

// -------------------------------------------------------------------- brightness

/// Brightness is clamped at both ends, and the clamp is what keeps sampling
/// weight bounded: the multipliers compound, so an unclamped run of successes
/// grows without limit until one peer owns every sample, and an unclamped run of
/// failures decays to zero, where a peer can never be picked again and can never
/// recover. Both are permanent, and both are one long run away.
#[test]
fn brightness_is_clamped_at_both_ends() {
    let config = QuasarConfig::default();
    let mut lux = Luminance::new(&config);
    let node = peer(1);

    for _ in 0..500 {
        lux.illuminate(&node, true);
    }
    assert_eq!(
        lux.lux(&node),
        config.max_luminance,
        "a long run of successes escaped the cap"
    );

    for _ in 0..500 {
        lux.illuminate(&node, false);
    }
    assert_eq!(
        lux.lux(&node),
        config.min_luminance,
        "a long run of failures dimmed past the floor"
    );

    // The floor is above zero, so a peer that has failed everything is still
    // sampleable and can climb back.
    assert!(config.min_luminance > 0.0);
    lux.illuminate(&node, true);
    assert!(
        lux.lux(&node) > config.min_luminance,
        "a peer at the floor could not recover"
    );
}

/// An unseen peer reads at the base rather than at zero, so a validator that has
/// not been sampled yet is not thereby unsampleable. Brightness is the ratio to
/// that base, which is what makes it comparable across configs.
#[test]
fn an_unseen_peer_reads_at_the_base() {
    let config = QuasarConfig::default();
    let mut lux = Luminance::new(&config);

    assert_eq!(lux.lux(&peer(1)), config.base_luminance);
    assert_eq!(lux.brightness(&peer(1)), 1.0);
    assert_eq!(lux.node_count(), 0, "asking about a peer tracked it");
    assert_eq!(lux.total_luminance(), 0.0);

    lux.illuminate(&peer(1), true);
    assert_eq!(lux.node_count(), 1);
    assert!(lux.brightness(&peer(1)) > 1.0);
    assert_eq!(lux.total_luminance(), lux.lux(&peer(1)));
}

/// The default tracker is the one the default config describes. The two used to
/// be written out separately, which is how a tracker ended up with a floor its
/// config did not have — the same drift the config presets are stated as diffs
/// to avoid.
#[test]
fn the_default_tracker_reads_the_default_config() {
    let config = QuasarConfig::default();
    let mut from_default = Luminance::default();
    let mut from_config = Luminance::new(&config);
    let node = peer(1);

    for step in 0..40 {
        let success = step % 3 != 0;
        from_default.illuminate(&node, success);
        from_config.illuminate(&node, success);
        assert_eq!(
            from_default.lux(&node),
            from_config.lux(&node),
            "the default tracker and the default config disagree after {step} rounds"
        );
    }
}

// ---------------------------------------------------------------------- sampling

/// A sample is distinct peers, and never more of them than there are. Asking for
/// more than the whole set returns the whole set once — a sampler that padded to
/// k by repeating would count one validator several times toward a quorum.
#[test]
fn a_sample_is_distinct_and_never_larger_than_the_set() {
    let config = QuasarConfig::default();
    let peers: Vec<NodeID> = (1..=5).map(peer).collect();
    let sampler = PhotonSampler::new(peers.clone(), &config);

    for k in 0..=8 {
        let drawn = sampler.sample(k);
        assert_eq!(
            drawn.len(),
            k.min(peers.len()),
            "asking for {k} of {} returned {}",
            peers.len(),
            drawn.len()
        );

        let mut distinct = drawn.clone();
        distinct.sort();
        distinct.dedup();
        assert_eq!(distinct.len(), drawn.len(), "a peer was sampled twice at k={k}");

        for d in &drawn {
            assert!(peers.contains(d), "a peer nobody registered was sampled");
        }
    }
}

/// An empty set samples nobody. A sampler that returned a placeholder here would
/// hand a round a committee of one imaginary peer.
#[test]
fn an_empty_set_samples_nobody() {
    let sampler = PhotonSampler::new(Vec::new(), &QuasarConfig::default());
    assert!(sampler.sample(5).is_empty());
}

/// Sampling is deterministic: the same peers in the same state draw the same
/// committee. Every node runs this selection independently, so a sampler that
/// consulted a hash map's iteration order or a clock would give two honest nodes
/// different committees for one round.
#[test]
fn the_same_state_draws_the_same_committee() {
    let config = QuasarConfig::default();
    let peers: Vec<NodeID> = (1..=7).map(peer).collect();
    let mut sampler = PhotonSampler::new(peers, &config);

    sampler.update_luminance(&peer(3), true);
    sampler.update_luminance(&peer(5), false);

    let first = sampler.sample(4);
    for _ in 0..8 {
        assert_eq!(sampler.sample(4), first, "two draws from one state disagreed");
    }
}

/// A peer joins once and leaves completely. A duplicate entry would be a second
/// seat in every sample it appears in, which is the same fault a duplicate
/// registration is, one layer up.
#[test]
fn a_peer_joins_once_and_leaves_completely() {
    let config = QuasarConfig::default();
    let mut sampler = PhotonSampler::new(vec![peer(1)], &config);

    sampler.add_peer(peer(2));
    sampler.add_peer(peer(2));
    assert_eq!(sampler.sample(9).len(), 2, "a peer was added twice");

    sampler.remove_peer(&peer(1));
    let remaining = sampler.sample(9);
    assert_eq!(remaining, vec![peer(2)]);

    // Removing a peer that was never there is not a change.
    sampler.remove_peer(&peer(200));
    assert_eq!(sampler.sample(9), remaining);
}

/// The sampler's own tracker is what its weights are read from, so an update
/// through the sampler is visible in the tracker it hands back. Two trackers
/// would mean the weights that select a committee and the weights an operator
/// inspects were different numbers.
#[test]
fn the_sampler_updates_the_tracker_it_reports() {
    let config = QuasarConfig::default();
    let mut sampler = PhotonSampler::new(vec![peer(1), peer(2)], &config);

    sampler.update_luminance(&peer(1), true);

    assert!(
        sampler.luminance().brightness(&peer(1)) > 1.0,
        "an update through the sampler did not reach its tracker"
    );
    assert_eq!(sampler.luminance().brightness(&peer(2)), 1.0);
}

/// When every weight has collapsed to zero the sampler still returns a
/// committee. Weighted selection has nothing to weigh here — the scores are all
/// zero and the choice between them is arbitrary — and the alternative to
/// falling back is returning no peers, which stalls the round forever. A
/// liveness floor, not a preference.
///
/// It is reachable only from a configuration that permits total darkness: a
/// floor of zero and a failure multiplier that reaches it. The default config
/// cannot get here, which is why the floor is above zero there.
#[test]
fn a_committee_is_still_drawn_when_every_weight_has_collapsed() {
    let dark = QuasarConfig {
        base_luminance: 100.0,
        min_luminance: 0.0,
        failure_multiplier: 0.0,
        ..QuasarConfig::default()
    };
    let peers: Vec<NodeID> = (1..=5).map(peer).collect();
    let mut sampler = PhotonSampler::new(peers.clone(), &dark);

    for p in &peers {
        sampler.update_luminance(p, false);
        assert_eq!(sampler.luminance().brightness(p), 0.0);
    }

    let drawn = sampler.sample(3);
    assert_eq!(drawn.len(), 3, "a fully dimmed set sampled nobody");
    let mut distinct = drawn.clone();
    distinct.sort();
    distinct.dedup();
    assert_eq!(distinct.len(), 3, "the fallback repeated a peer");
    for d in &drawn {
        assert!(peers.contains(d));
    }
}
