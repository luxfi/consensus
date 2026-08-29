// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! The FPC threshold, against Go.
//!
//! θ decides how large a majority of a committee is needed to move a block. Two
//! nodes that compute different θ for the same phase demand different
//! majorities of the same votes, and split. There is no adversary in that
//! story — just two implementations of one number.
//!
//! The expectations below are the Go values, not values this crate produced.
//! Regenerate them from the network's own implementation with:
//!
//! ```text
//! cd ~/work/lux/consensus && cat > /tmp/v_test.go <<'EOF'
//! package fpc
//! import ("fmt"; "math"; "testing")
//! func TestEmit(t *testing.T) {
//!     s, _ := NewSelector(0.5, 0.8, []byte("lux-fpc-default-seed-00000000000"))
//!     for p := uint64(0); p < 6; p++ {
//!         fmt.Printf("%d %#016x %d %d\n", p, math.Float64bits(s.Theta(p)),
//!             s.SelectThreshold(p, 21), s.SelectThreshold(p, 5))
//!     }
//! }
//! EOF
//! cp /tmp/v_test.go protocol/wave/fpc/emit_test.go
//! go test ./protocol/wave/fpc/ -run TestEmit -v && rm protocol/wave/fpc/emit_test.go
//! ```

use lux_consensus::{sha256, FpcSelector};

/// The Go seed, byte for byte. `FpcSelector::default()` carries this one.
const SEED: [u8; 32] = *b"lux-fpc-default-seed-00000000000";

/// phase, θ bits, α at k=21, α at k=5 — captured from the Go run above.
const GO: [(u64, u64, usize, usize); 6] = [
    (0, 0x3fe6a66cd2af9620, 15, 4),
    (1, 0x3fe28bba3aab6950, 13, 3),
    (2, 0x3fe3494fc519c669, 13, 4),
    (3, 0x3fe58b4f2da1c29b, 15, 4),
    (4, 0x3fe62fe447a76425, 15, 4),
    (5, 0x3fe4cbf55ee48fec, 14, 4),
];

/// θ to the last bit. Compared as bits, not with a tolerance: a threshold that
/// is merely close is a different threshold, and `ceil(θ·k)` will eventually
/// land on the other side of an integer and disagree about a quorum.
#[test]
fn theta_matches_go_bit_for_bit() {
    let s = FpcSelector::new(0.5, 0.8, SEED);
    for (phase, want_bits, _, _) in GO {
        let got = s.theta(phase);
        assert_eq!(
            got.to_bits(),
            want_bits,
            "phase {phase}: got {got:.17} ({:#018x}), Go says {:.17} ({want_bits:#018x})",
            got.to_bits(),
            f64::from_bits(want_bits),
        );
    }
}

/// The number that is actually used: α = ⌈θ·k⌉, the votes a block needs.
#[test]
fn alpha_matches_go() {
    let s = FpcSelector::new(0.5, 0.8, SEED);
    for (phase, _, want21, want5) in GO {
        assert_eq!(s.select_threshold(phase, 21), want21, "phase {phase}, k=21");
        assert_eq!(s.select_threshold(phase, 5), want5, "phase {phase}, k=5");
    }
}

/// `FpcSelector::default()` must be the selector the vectors describe, so the
/// default configuration is the conformant one rather than a fourth variant.
#[test]
fn default_selector_is_the_go_selector() {
    let d = FpcSelector::default();
    assert_eq!(d.range(), (0.5, 0.8));
    for (phase, want_bits, _, _) in GO {
        assert_eq!(d.theta(phase).to_bits(), want_bits, "phase {phase}");
    }
}

/// The PRF is SHA-256 itself, not something SHA-256-shaped.
///
/// These are the published FIPS 180-4 vectors. They pin the primitive
/// independently of Lux, so if `sha256` is ever swapped for a mixer that merely
/// agrees on today's six phases, this fails.
#[test]
fn sha256_is_sha256() {
    assert_eq!(
        hex::encode(sha256(b"")),
        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    );
    assert_eq!(
        hex::encode(sha256(b"abc")),
        "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
    );
    assert_eq!(
        hex::encode(sha256(
            b"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"
        )),
        "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1",
    );
}

/// The PRF input is `seed ‖ big-endian phase`, so θ is derived from the hash of
/// exactly those 40 bytes. Recomputing the composition here would only restate
/// the implementation; instead this pins the one property that composition has
/// to have — every phase is a fresh draw, and consecutive phases share nothing.
#[test]
fn each_phase_draws_independently() {
    let s = FpcSelector::new(0.5, 0.8, SEED);
    let thetas: Vec<u64> = (0..64).map(|p| s.theta(p).to_bits()).collect();
    let unique: std::collections::HashSet<_> = thetas.iter().collect();
    assert_eq!(unique.len(), thetas.len(), "phases collide");

    for (phase, _, _, _) in GO {
        let t = s.theta(phase);
        assert!((0.5..=0.8).contains(&t), "phase {phase}: θ={t} out of range");
    }
}

/// A different seed is a different threshold schedule. This is what makes the
/// per-epoch seed meaningful: an adversary who knows this epoch's α sequence
/// learns nothing about the next epoch's.
#[test]
fn a_different_seed_is_a_different_schedule() {
    let a = FpcSelector::new(0.5, 0.8, SEED);
    let mut other = SEED;
    other[31] ^= 0x01;
    let b = FpcSelector::new(0.5, 0.8, other);

    let differ = (0..16).filter(|&p| a.theta(p).to_bits() != b.theta(p).to_bits()).count();
    assert_eq!(differ, 16, "one flipped seed bit left a phase unchanged");
}
