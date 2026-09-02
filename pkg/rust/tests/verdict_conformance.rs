// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! Rust against Go, on the DECISION.
//!
//! `tests/conformance.rs` holds this crate to the bytes Go emits — the signed
//! message, the certificate wire, the floors as numbers. That is necessary and
//! it is not sufficient: an implementation can reproduce every byte and still
//! finalize on the wrong number of signers, because encoding and deciding are
//! different questions. A build carrying no weighted predicate at all passes a
//! corpus that only ever asks it to encode.
//!
//! So this harness reads the `verdict` section and asks the other question. Each
//! case states a validator set, the distinct signers a certificate carries and
//! the rung it attests; Go recorded what its predicate decided, and this crate
//! must decide the same. Nothing is restated here — the expectation is read from
//! the corpus and the answer comes from `Cert::verify_weighted`.
//!
//! Signatures are not the subject. Every vote is resolved as correctly signed,
//! exactly as the Go oracle did, so a case can only turn on the weighted half:
//! the signer floor and the stake floor. Signature validity is its own dimension
//! and `cert_conformance.rs` and `pop_conformance.rs` already hold it.

use lux_consensus::cert::{
    CertError, NodeId, QuorumCert, Registration, StakeSource, ValidatorSet, Vote, VoteVerifier,
};
use lux_consensus::finality::{Finality, Position};
use serde_json::Value;

fn corpus() -> Value {
    let path = concat!(env!("CARGO_MANIFEST_DIR"), "/../../conformance/corpus.json");
    let raw = std::fs::read_to_string(path).unwrap_or_else(|e| {
        panic!("read {path}: {e} — regenerate with `go test ./conformance -update`")
    });
    serde_json::from_str(&raw).expect("corpus.json does not parse")
}

fn node(s: &str) -> NodeId {
    let bytes = hex::decode(s).unwrap_or_else(|e| panic!("node id {s}: {e}"));
    assert_eq!(bytes.len(), 20, "node id is {} bytes, want 20", bytes.len());
    let mut out = [0u8; 20];
    out.copy_from_slice(&bytes);
    out
}

fn decimal(v: &Value, key: &str) -> u64 {
    v[key]
        .as_str()
        .unwrap_or_else(|| panic!("{key} is not a decimal string"))
        .parse()
        .unwrap_or_else(|e| panic!("{key}: {e}"))
}

/// One seat of a case's set: who, how much, and whether it holds a key.
///
/// A seat the corpus marks `keyless` is a member the chain carries and no
/// verifier will ever accept a signature from. It is in the row so a reader can
/// see what the case is about, and it is in NO floor's denominator.
#[derive(Clone)]
struct Seat {
    id: NodeId,
    weight: u64,
    keyless: bool,
}

/// The validator set a case is weighed against, read straight from the row.
#[derive(Clone)]
struct Set {
    seats: Vec<Seat>,
}

impl Set {
    fn find(&self, node: &NodeId) -> Option<&Seat> {
        self.seats.iter().find(|s| &s.id == node)
    }

    /// What the set CARRIES: every seat, keyed or not. Not a floor — it is here
    /// so the keyless case can state the number it would have been measured
    /// against had the denominator been the membership roll.
    fn carried(&self) -> u64 {
        self.seats.iter().map(|s| s.weight).sum()
    }
}

impl StakeSource for Set {
    fn weight(&self, node: &NodeId, _: u64) -> u64 {
        // An id outside the set carries no stake — an unknown voter must never
        // be able to inflate a tally — and neither does a seat that holds no key,
        // whose vote no verifier would accept.
        self.find(node)
            .filter(|s| !s.keyless)
            .map_or(0, |s| s.weight)
    }

    fn signer_stake(&self, _: u64) -> u64 {
        self.seats
            .iter()
            .filter(|s| !s.keyless)
            .map(|s| s.weight)
            .sum()
    }

    fn signer_count(&self, _: u64) -> i64 {
        self.seats.iter().filter(|s| !s.keyless).count() as i64
    }
}

/// Resolves every vote FROM A SEAT THAT HOLDS A KEY as correctly signed, so the
/// weighted half is the only thing a case can turn on. The Go oracle recorded
/// these verdicts under exactly this assumption — including that a keyless seat
/// has no signature to resolve, which is why it can never be a voter.
struct Trust(Set);

impl VoteVerifier for Trust {
    fn verify_vote(&self, node: &NodeId, _: &[u8], _: &[u8], _: u64) -> bool {
        self.0.find(node).is_some_and(|s| !s.keyless)
    }
}

/// Names the clause a refusal came from, in the corpus's vocabulary.
///
/// Rust names the clause more finely than Go does: Go returns one
/// `ErrQCBelowThreshold` for both an unresolved set and a short signer count,
/// and one `ErrQCStakeBelowMajority` for both a zero total and a short tally.
/// The mapping is where that equivalence is written down, so a real disagreement
/// about the DECISION cannot hide inside a difference of vocabulary.
fn refusal(rung: &str, err: &CertError) -> &'static str {
    match err {
        // Go folds all three into ErrQCBelowThreshold: an unresolved set, a
        // signing set below the minimum Byzantine committee, and a short signer
        // count are one refusal class there and three variants here.
        CertError::UnresolvedSet { .. }
        | CertError::MinCommittee { .. }
        | CertError::SignerFloor { .. }
        | CertError::BelowThreshold { .. } => "belowThreshold",
        CertError::StakeBelowMajority { .. } => "stakeBelowMajority",
        CertError::StakeBelowSupermajority { .. } => "stakeBelowSupermajority",
        // Go folds a zero total into the rung's own stake refusal.
        CertError::StakeZero { .. } => {
            if rung == "nova" {
                "stakeBelowMajority"
            } else {
                "stakeBelowSupermajority"
            }
        }
        CertError::ThresholdNotDerived { .. } => "thresholdNotDerived",
        CertError::ZeroWeight => "zeroWeight",
        CertError::WeightOverflow => "weightOverflow",
        other => panic!("a verdict the corpus cannot name: {other}"),
    }
}

/// Every frozen finality verdict, decided again here.
#[test]
fn the_weighted_decision_is_the_one_go_made() {
    let c = corpus();
    let epoch = decimal(&c["verdict"], "epoch");
    let cases = c["verdict"]["finality"]
        .as_array()
        .expect("corpus has no verdict.finality section");

    for case in cases {
        let name = case["name"].as_str().expect("case has no name");
        let rung = case["rung"].as_str().expect("case has no rung");

        let tier = match rung {
            "nova" => Finality::Nova,
            "quasar" => Finality::Quasar,
            other => panic!("{name}: a certificate attests nova or quasar, not {other}"),
        };

        let seats: Vec<Seat> = case["set"]
            .as_array()
            .unwrap_or_else(|| panic!("{name}: no set"))
            .iter()
            .map(|s| Seat {
                id: node(s["nodeID"].as_str().expect("seat has no nodeID")),
                weight: decimal(s, "weight"),
                // Absent means keyed, which is what every seat but the keyless
                // vector's is.
                keyless: s["keyless"].as_bool().unwrap_or(false),
            })
            .collect();
        let set = Set { seats };

        // The set the corpus states must be the set it says it states — and the
        // recorded total is the SIGNER stake, so a keyless seat's weight must be
        // absent from it. On a set with no keyless seat the two readings coincide.
        assert_eq!(
            set.signer_stake(epoch),
            decimal(case, "total"),
            "{name}: the recorded total is not the sum of the seats that can sign"
        );
        assert!(
            set.carried() >= set.signer_stake(epoch),
            "{name}: the signer stake exceeds what the set carries"
        );

        let votes: Vec<Vote> = case["signers"]
            .as_array()
            .unwrap_or_else(|| panic!("{name}: no signers"))
            .iter()
            .map(|s| Vote {
                node_id: node(s.as_str().expect("signer is not a node id")),
                accept: true,
                signature: vec![0x01],
            })
            .collect();

        let threshold = case["threshold"].as_u64().expect("case has no threshold") as u32;

        // The decision does not depend on the position, which the corpus states
        // outright — so the default one is as good as any and this harness needs
        // to carry no position material at all.
        //
        // Assembled at the vote count and then stamped with what the row's
        // certificate DECLARES, because the two are different questions. A row
        // below the floor its set derives is a certificate no honest assembler can
        // build and one every verifier must still refuse, so stating it means
        // building it the way an adversary would. The ordering and dedup clauses
        // still run.
        let mut cert = QuorumCert::assemble(tier, Position::default(), votes.len() as u32, &votes)
            .unwrap_or_else(|e| panic!("{name}: assemble: {e}"));
        cert.threshold = threshold;

        let decided = cert.verify_weighted(&Trust(set.clone()), &set, epoch);
        let accept = case["accept"].as_bool().expect("case has no accept");
        let want = case["refusal"].as_str().expect("case has no refusal");

        match decided {
            Ok(()) => assert!(
                accept,
                "{name}: this crate accepted a certificate Go refused with {want}"
            ),
            Err(e) => {
                assert!(
                    !accept,
                    "{name}: this crate refused ({e}) a certificate Go accepted"
                );
                assert_eq!(
                    refusal(rung, &e),
                    want,
                    "{name}: refused on the wrong clause ({e})"
                );
            }
        }
    }

    // A section that quietly lost its cases would let this test pass over an
    // empty list, which is the failure mode the whole file exists to close.
    assert_eq!(
        cases.len(),
        18,
        "expected 18 frozen finality verdicts, checked {}",
        cases.len()
    );
}

/// The admission clauses this crate's door reaches.
///
/// The corpus records which door produced each verdict, because the standard has
/// two and they do not enforce the same clauses. This crate ports `Register`, so
/// it answers for those rows and states how many it answered — a runner that
/// silently matched zero rows would report PASS while checking nothing.
#[test]
fn the_admission_decision_is_the_one_go_made() {
    let c = corpus();
    let cases = c["verdict"]["admission"]
        .as_array()
        .expect("corpus has no verdict.admission section");

    let mut checked = 0;
    for case in cases {
        if case["door"].as_str() != Some("Register") {
            continue;
        }
        let name = case["name"].as_str().expect("case has no name");

        let registrations: Vec<_> = case["weights"]
            .as_array()
            .unwrap_or_else(|| panic!("{name}: no weights"))
            .iter()
            .enumerate()
            .map(|(i, w)| {
                let mut id = [0u8; 20];
                id[16..].copy_from_slice(&((i as u32) + 1).to_be_bytes());
                Registration {
                    node: id,
                    // Present and the right width, but not a point: the weight
                    // clauses are reached before any key is read, and a real key
                    // here would be pinning possession — which pop_conformance
                    // already pins, once, on its own.
                    public_key: vec![0xAB; 48],
                    proof: vec![0xAB; 96],
                    weight: w
                        .as_str()
                        .expect("weight is not a decimal")
                        .parse()
                        .unwrap(),
                }
            })
            .collect();

        let decided = ValidatorSet::register(registrations);
        let admitted = case["admitted"].as_bool().expect("case has no admitted");
        let want = case["refusal"].as_str().expect("case has no refusal");

        match decided {
            Ok(_) => assert!(
                admitted,
                "{name}: this crate admitted a set Go refused with {want}"
            ),
            Err(e) => {
                assert!(
                    !admitted,
                    "{name}: this crate refused ({e}) a set Go admitted"
                );
                assert_eq!(
                    refusal("", &e),
                    want,
                    "{name}: refused on the wrong clause ({e})"
                );
            }
        }
        checked += 1;
    }

    assert_eq!(
        checked, 1,
        "expected 1 row at this crate's door, checked {checked}"
    );
}

/// The weight clause both of Go's doors enforce, checked at this crate's door.
///
/// Go has two doors and this crate has one. `Register` admits a fresh set and
/// demands possession of every key; `FlattenValidatorSet` reads a set the chain
/// already admitted, has no proof to check, and forgives a key it cannot decode.
/// `ValidatorSet::insert` is the door here, and it demands possession — so most
/// of what the corpus records at the already-admitted door is not answerable
/// from this side, and the harness above skips it.
///
/// This row is the exception, for a precise reason: both doors refuse a keyed
/// seat carrying no stake BEFORE any key material is read — Go checks it after
/// the key is known to be present and before it is decoded, and `insert` checks
/// it after `NoKey` and before `pop::verify`. A shaped key is therefore enough
/// to reach the clause on either side, and the verdict Go froze is one this
/// crate can be held to rather than one it merely skips.
///
/// The clause itself is the phantom signer: a seat that can sign and holds no
/// stake raises the count of distinct signers a floor is read against and adds
/// nothing to the weight the same certificate is weighed by.
#[test]
fn a_keyed_seat_with_no_stake_is_refused_at_either_door() {
    let c = corpus();
    let cases = c["verdict"]["admission"]
        .as_array()
        .expect("corpus has no verdict.admission section");

    let mut checked = 0;
    for case in cases {
        if case["door"].as_str() != Some("FlattenValidatorSet")
            || case["refusal"].as_str() != Some("zeroWeight")
        {
            continue;
        }
        let name = case["name"].as_str().expect("case has no name");

        // Go refused the set; the row must say so, or this test would be
        // holding the crate to an expectation the corpus does not state.
        assert!(
            !case["admitted"].as_bool().expect("case has no admitted"),
            "{name}: a zeroWeight refusal that admitted the set"
        );

        let weights: Vec<u64> = case["weights"]
            .as_array()
            .unwrap_or_else(|| panic!("{name}: no weights"))
            .iter()
            .map(|w| w.as_str().expect("weight is not a decimal").parse().unwrap())
            .collect();
        assert!(
            weights.contains(&0),
            "{name}: a zeroWeight row with no weightless seat"
        );

        // Every weightless seat is refused on the weight clause, whichever
        // position it holds — the seat, not the ordering, is what the clause is
        // about. A shaped key reaches it: the clause runs before the key is read.
        for (i, w) in weights.iter().enumerate() {
            if *w != 0 {
                continue;
            }
            let mut id = [0u8; 20];
            id[16..].copy_from_slice(&((i as u32) + 1).to_be_bytes());

            let mut set = ValidatorSet::new();
            match set.insert(id, *w, &[0xAB; 48], &[0xAB; 96]) {
                Ok(()) => panic!("{name}: seat {} carries no stake and was admitted", i + 1),
                Err(e) => assert_eq!(
                    refusal("", &e),
                    "zeroWeight",
                    "{name}: seat {} refused on the wrong clause ({e})",
                    i + 1
                ),
            }
        }
        checked += 1;
    }

    assert_eq!(
        checked, 1,
        "expected 1 weightless-seat row at the already-admitted door, checked {checked}"
    );
}
