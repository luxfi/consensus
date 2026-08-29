// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! Rust against Go, on the real corpus.
//!
//! `conformance/corpus.json` is generated from the Go definitions that decide
//! finality on mainnet. This harness reads that file and holds this crate to it:
//! every signed message byte for byte, both stake floors across the full u64
//! range, every count threshold, the ladder's authorizations, and the certificate
//! wire form the corpus records.
//!
//! It is deliberately not a round-trip against ourselves. A round trip proves an
//! encoder agrees with itself; this proves it agrees with the network.

use lux_consensus::finality::{Cert, Vote as CertVote, *};
use serde_json::Value;

fn corpus() -> Value {
    let path = concat!(env!("CARGO_MANIFEST_DIR"), "/../../conformance/corpus.json");
    let raw = std::fs::read_to_string(path)
        .unwrap_or_else(|e| panic!("read {path}: {e} — regenerate with `go test ./conformance -update`"));
    serde_json::from_str(&raw).expect("corpus.json does not parse")
}

fn id(v: &Value, key: &str) -> Id {
    let s = v[key].as_str().unwrap_or_else(|| panic!("case has no {key}"));
    let bytes = hex::decode(s).unwrap_or_else(|e| panic!("{key}: {e}"));
    let mut out = EMPTY;
    assert_eq!(bytes.len(), 32, "{key} is {} bytes, want 32", bytes.len());
    out.copy_from_slice(&bytes);
    out
}

fn u64_of(v: &Value, key: &str) -> u64 {
    v[key]
        .as_str()
        .unwrap_or_else(|| panic!("{key} is not a decimal string"))
        .parse()
        .unwrap_or_else(|e| panic!("{key}: {e}"))
}

/// The 32-byte FPC seed a threshold case is taken under.
fn seed_of(v: &Value) -> [u8; 32] {
    let bytes = hex::decode(v["seed"].as_str().expect("case has no seed")).expect("seed is not hex");
    assert_eq!(bytes.len(), 32, "seed is {} bytes, want 32", bytes.len());
    let mut out = [0u8; 32];
    out.copy_from_slice(&bytes);
    out
}

/// A float recorded as its exact IEEE-754 big-endian bits. The corpus writes θ
/// this way because α is a ceiling: a decimal that agrees to seventeen places
/// and differs in the last bit picks a different vote count at the k where the
/// ceiling steps.
fn float_of(v: &Value, key: &str) -> f64 {
    let bytes = hex::decode(v[key].as_str().unwrap_or_else(|| panic!("case has no {key}")))
        .unwrap_or_else(|e| panic!("{key}: {e}"));
    assert_eq!(bytes.len(), 8, "{key} is {} bytes, want 8", bytes.len());
    let mut out = [0u8; 8];
    out.copy_from_slice(&bytes);
    f64::from_be_bytes(out)
}

/// Every signed message in the corpus, byte for byte. This is the check that
/// would have caught the C++ implementation signing 70 bytes under a v1 tag
/// while the network signs 226 under v2.
#[test]
fn signed_messages_match_go() {
    let c = corpus();

    assert_eq!(
        c["version"].as_u64().unwrap() as u16,
        QUORUM_CERT_VERSION,
        "certificate version disagrees with Go"
    );
    assert_eq!(
        c["vote"]["length"].as_u64().unwrap() as usize,
        VOTE_MESSAGE_LEN,
        "message length disagrees with Go"
    );
    assert_eq!(
        c["vote"]["tag"].as_str().unwrap().as_bytes(),
        VOTE_TAG,
        "domain tag disagrees with Go"
    );
    assert_eq!(c["vote"]["qcType"].as_u64().unwrap() as u8, QC_FINALITY);

    let cases = c["vote"]["cases"].as_array().expect("no vote cases");
    assert!(!cases.is_empty(), "the corpus carries no vote cases");

    for case in cases {
        let name = case["name"].as_str().unwrap();
        let pos = Position {
            chain_id: id(case, "chainID"),
            height: u64_of(case, "height"),
            round: case["round"].as_u64().unwrap() as u32,
            block_id: id(case, "blockID"),
            parent_id: id(case, "parentID"),
            canonical_id: id(case, "canonicalID"),
            parent_canonical_id: id(case, "parentCanonicalID"),
            execution_state_root: id(case, "executionStateRoot"),
            payload_root: id(case, "payloadRoot"),
            validator_set_root: id(case, "validatorSetRoot"),
        };
        let accept = case["accept"].as_bool().unwrap();
        let got = hex::encode(canonical_vote_message(&pos, accept));
        let want = case["message"].as_str().unwrap();
        assert_eq!(got, want, "vote case {name}: this crate signs different bytes than the network");
    }
}

/// Both floors, over the whole recorded range including 2^64-1 where a naive
/// `2 * total` overflows.
#[test]
fn stake_floors_match_go() {
    let c = corpus();
    let rows = c["threshold"]["stake"].as_array().expect("no stake rows");
    assert!(!rows.is_empty());

    for row in rows {
        let total = u64_of(row, "total");
        assert_eq!(
            two_thirds_stake_floor(total),
            u64_of(row, "twoThirds"),
            "two-thirds floor at total={total}"
        );
        assert_eq!(
            half_stake_floor(total),
            u64_of(row, "half"),
            "half floor at total={total}"
        );
        // The predicate is STRICTLY greater than the floor. Recomputing the
        // recorded need catches the off-by-one that finalizes on exactly two
        // thirds — the one that looks right in every test with an even total.
        assert_eq!(
            two_thirds_stake_floor(total).wrapping_add(1),
            u64_of(row, "quasarNeed"),
            "export need at total={total}"
        );
        assert_eq!(
            half_stake_floor(total).wrapping_add(1),
            u64_of(row, "novaNeed"),
            "local-execution need at total={total}"
        );
    }
}

/// The count-side thresholds, and the weighted export count.
#[test]
fn count_thresholds_match_go() {
    let c = corpus();

    for row in c["threshold"]["count"].as_array().expect("no count rows") {
        let n = row["n"].as_i64().unwrap();
        assert_eq!(nova_quorum(n), row["novaQuorum"].as_i64().unwrap(), "novaQuorum n={n}");
        assert_eq!(
            nova_signer_floor(n),
            row["novaSignerFloor"].as_i64().unwrap(),
            "novaSignerFloor n={n}"
        );
        assert_eq!(nova_beta(n), row["novaBeta"].as_i64().unwrap(), "novaBeta n={n}");
        assert_eq!(
            crash_tolerance(n),
            row["crashTolerance"].as_i64().unwrap(),
            "crashTolerance n={n}"
        );
        assert_eq!(
            equal_stake_quasar(n),
            row["equalStakeQuasar"].as_i64().unwrap(),
            "equalStakeQuasar n={n}"
        );
    }

    for row in c["threshold"]["weighted"].as_array().expect("no weighted rows") {
        let weights: Vec<u64> = row["weights"]
            .as_array()
            .unwrap()
            .iter()
            .map(|w| w.as_str().unwrap().parse().unwrap())
            .collect();
        assert_eq!(
            weighted_quasar(&weights) as i64,
            row["count"].as_i64().unwrap(),
            "weighted export count for {:?}",
            weights
        );
    }
}

/// The ladder. An implementation with one rung cannot pass this: the corpus
/// records five, and the boundary between local execution and export is the
/// whole safety argument.
#[test]
fn ladder_matches_go() {
    let c = corpus();
    let rungs = c["ladder"].as_array().expect("no ladder");

    let all = [
        Finality::Photon,
        Finality::Wave,
        Finality::Nova,
        Finality::Quasar,
        Finality::Horizon,
    ];
    assert_eq!(rungs.len(), all.len(), "the ladder has a different number of rungs");

    for (row, rung) in rungs.iter().zip(all.iter().copied()) {
        assert_eq!(row["name"].as_str().unwrap(), rung.name());
        assert_eq!(row["value"].as_u64().unwrap() as u8, rung as u8);
        assert_eq!(
            row["authorizesLocalExecution"].as_bool().unwrap(),
            rung.authorizes_local_execution(),
            "{} local execution",
            rung.name()
        );
        assert_eq!(
            row["authorizesExport"].as_bool().unwrap(),
            rung.authorizes_export(),
            "{} export",
            rung.name()
        );
        assert_eq!(
            row["authorizesIrreversibleSettlement"].as_bool().unwrap(),
            rung.authorizes_irreversible_settlement(),
            "{} irreversible settlement",
            rung.name()
        );
    }

    // Stated outright, not read from the corpus: nothing below Quasar leaves the
    // chain. If the corpus were re-blessed with this inverted, this still fails.
    assert!(!Finality::Nova.authorizes_export(), "Nova must never authorize export");
    assert!(Finality::Nova.authorizes_local_execution());
    assert!(Finality::Quasar.authorizes_export());
    assert!(!Finality::Quasar.authorizes_irreversible_settlement());
}

/// The certificate wire form the corpus records must open with the version, the
/// role and the tier this crate names — the header a gossiped certificate is
/// parsed by.
#[test]
fn certificate_header_matches_go() {
    let c = corpus();
    for case in c["cert"]["cases"].as_array().expect("no cert cases") {
        let name = case["name"].as_str().unwrap();
        let wire = hex::decode(case["wire"].as_str().unwrap()).expect("cert wire is not hex");
        assert_eq!(
            wire.len(),
            case["length"].as_u64().unwrap() as usize,
            "{name}: recorded length disagrees with the recorded bytes"
        );
        assert!(wire.len() > 4, "{name}: certificate is too short to carry a header");

        assert_eq!(
            u16::from_be_bytes([wire[0], wire[1]]),
            QUORUM_CERT_VERSION,
            "{name}: certificate version"
        );
        assert_eq!(wire[2], QC_FINALITY, "{name}: certificate role");

        let tier = match case["tier"].as_str().unwrap() {
            "nova" => Finality::Nova,
            "quasar" => Finality::Quasar,
            other => panic!("{name}: a certificate may only attest nova or quasar, not {other}"),
        };
        assert_eq!(wire[3], tier as u8, "{name}: certificate tier byte");
    }
}

/// Every per-epoch seed in the corpus, byte for byte.
///
/// The seed is the input the whole threshold rule hangs off. If it is derived
/// differently here, every θ downstream is different and the thresholds test
/// below fails for a reason that reads like a PRF bug — so the derivation is
/// checked first and on its own.
#[test]
fn epoch_seeds_match_go() {
    let c = corpus();
    let cases = c["fpc"]["seeds"].as_array().expect("no fpc seeds");
    assert!(!cases.is_empty(), "the corpus carries no epoch seeds");

    for case in cases {
        let note = case["note"].as_str().unwrap();
        let epoch = u64_of(case, "epoch");
        let chain_id = hex::decode(case["chainID"].as_str().unwrap()).expect("chainID is not hex");
        let prev = hex::decode(case["prevBlockHash"].as_str().unwrap()).expect("prevBlockHash is not hex");

        let got = hex::encode(lux_consensus::derive_epoch_seed(epoch, &chain_id, &prev));
        let want = case["seed"].as_str().unwrap();
        assert_eq!(got, want, "epoch seed ({note}): this crate derives a different seed than the network");
    }
}

/// Every recorded threshold: θ to the bit, and the vote count read off it.
///
/// This is the check that bites. A mixer stood in for SHA-256 here, under a
/// comment claiming SHA-256, and returned α=11 at phase 0 with k=20 where the
/// network requires 15 — a node that accepted on eleven votes while its peers
/// held out for fifteen. Nothing in either implementation said so, because both
/// agreed with themselves.
///
/// θ is compared as exact IEEE-754 bits. α is a ceiling, so two θ that differ in
/// the last bit are the same number to seventeen decimal places and a different
/// chain at the k where the ceiling steps.
#[test]
fn fpc_thresholds_match_go() {
    let c = corpus();
    let cases = c["fpc"]["thresholds"].as_array().expect("no fpc thresholds");
    assert!(!cases.is_empty(), "the corpus carries no thresholds");

    for case in cases {
        let note = case["note"].as_str().unwrap();
        let seed = seed_of(case);
        let theta_min = float_of(case, "thetaMin");
        let theta_max = float_of(case, "thetaMax");
        let phase = u64_of(case, "phase");
        let k = case["k"].as_u64().unwrap() as usize;

        let sel = lux_consensus::FpcSelector::new(theta_min, theta_max, seed);

        let want_theta = case["theta"].as_str().unwrap();
        let got_theta = hex::encode(sel.theta(phase).to_be_bytes());
        assert_eq!(
            got_theta, want_theta,
            "θ at phase {phase} ({note}): the PRF disagrees with the network"
        );

        let want_alpha = case["alpha"].as_u64().unwrap() as usize;
        let got_alpha = sel.select_threshold(phase, k);
        assert_eq!(
            got_alpha, want_alpha,
            "α at phase {phase}, k={k} ({note}): this crate accepts on {got_alpha} votes where the network accepts on {want_alpha}"
        );
    }
}

/// The clamp, stated outright rather than read from the corpus: a range the
/// caller gets wrong must land on the live range, never on the caller's value.
/// Re-blessing the golden does not get past this.
#[test]
fn threshold_range_clamps_to_the_live_range() {
    let seed = lux_consensus::derive_epoch_seed(1, b"chain-A", &[]);
    for (min, max) in [(0.0, 2.0), (1.0, 0.9), (-1.0, 0.8), (0.6, 0.55)] {
        let (got_min, got_max) = lux_consensus::FpcSelector::new(min, max, seed).range();
        assert!(
            got_min > 0.0 && got_min < 1.0 && got_max > got_min && got_max <= 1.0,
            "range ({min}, {max}) clamped to ({got_min}, {got_max}), which is not a usable range"
        );
    }

    // θ never leaves its range, so α never exceeds k and never falls to zero: a
    // zero threshold accepts on no votes at all.
    let sel = lux_consensus::FpcSelector::new(0.5, 0.8, seed);
    for phase in 0..512u64 {
        let theta = sel.theta(phase);
        assert!((0.5..=0.8).contains(&theta), "θ({phase}) = {theta} left its range");
        for k in [1usize, 4, 5, 11, 20, 21] {
            let alpha = sel.select_threshold(phase, k);
            assert!(alpha >= 1 && alpha <= k, "α({phase}, k={k}) = {alpha} is not a usable count");
        }
    }
}

/// The position a cert case names, taken from the vote case of that name so the
/// certificate is composed entirely out of Go's recorded values.
fn position_named(c: &Value, name: &str) -> Position {
    let case = c["vote"]["cases"]
        .as_array()
        .expect("no vote cases")
        .iter()
        .find(|v| v["name"].as_str() == Some(name))
        .unwrap_or_else(|| panic!("no vote case named {name} to take a position from"));
    Position {
        chain_id: id(case, "chainID"),
        height: u64_of(case, "height"),
        round: case["round"].as_u64().unwrap() as u32,
        block_id: id(case, "blockID"),
        parent_id: id(case, "parentID"),
        canonical_id: id(case, "canonicalID"),
        parent_canonical_id: id(case, "parentCanonicalID"),
        execution_state_root: id(case, "executionStateRoot"),
        payload_root: id(case, "payloadRoot"),
        validator_set_root: id(case, "validatorSetRoot"),
    }
}

fn tier_named(name: &str) -> Finality {
    match name {
        "nova" => Finality::Nova,
        "quasar" => Finality::Quasar,
        other => panic!("a certificate may only attest nova or quasar, not {other}"),
    }
}

fn node_of(v: &Value) -> Node {
    let bytes = hex::decode(v["nodeID"].as_str().expect("vote has no nodeID")).expect("nodeID is not hex");
    assert_eq!(bytes.len(), NODE_LEN, "node id is {} bytes, want {NODE_LEN}", bytes.len());
    let mut out = [0u8; NODE_LEN];
    out.copy_from_slice(&bytes);
    out
}

/// Every certificate in the corpus, byte for byte.
///
/// The certificate is assembled from Go's recorded values — the position from
/// the vote case it names, the tier, the threshold, and each vote's identity,
/// accept flag and signature — and never from this crate's own decode. What is
/// compared is the whole wire form, not the four header bytes an earlier check
/// looked at, which passed happily while everything after byte four was
/// unexamined.
#[test]
fn certificates_match_go() {
    let c = corpus();
    let cases = c["cert"]["cases"].as_array().expect("no cert cases");
    assert!(!cases.is_empty(), "the corpus carries no certificates");

    for case in cases {
        let name = case["name"].as_str().unwrap();
        let votes: Vec<CertVote> = case["votes"]
            .as_array()
            .expect("cert case has no votes")
            .iter()
            .map(|v| CertVote {
                node: node_of(v),
                accept: v["accept"].as_bool().unwrap(),
                signature: hex::decode(v["signature"].as_str().unwrap()).expect("signature is not hex"),
            })
            .collect();

        let cert = Cert::assemble(
            position_named(&c, case["position"].as_str().unwrap()),
            tier_named(case["tier"].as_str().unwrap()),
            case["threshold"].as_u64().unwrap() as u32,
            votes,
        )
        .unwrap_or_else(|e| panic!("{name}: this crate refuses a certificate Go assembled: {e:?}"));

        let wire = cert.encode();
        assert_eq!(
            wire.len(),
            case["length"].as_u64().unwrap() as usize,
            "{name}: certificate length"
        );
        assert_eq!(
            hex::encode(&wire),
            case["wire"].as_str().unwrap(),
            "{name}: this crate gossips different bytes than the network"
        );

        // And back. A decoder that cannot read what the network sent is as
        // broken as an encoder that writes something else, and the ordering
        // clause means the vote list read out must equal the one put in.
        let read = Cert::decode(&wire).unwrap_or_else(|e| panic!("{name}: decode: {e:?}"));
        assert_eq!(read, cert, "{name}: decode does not invert encode");
    }
}

/// The wire is strict: a byte too few and a byte too many are both refusals.
///
/// Stated outright rather than read from the corpus. A trailing byte that is
/// tolerated gives one certificate many byte strings, and a length prefix that
/// is trusted past the end of the buffer is a read the sender chose the size of.
#[test]
fn certificate_wire_is_strict() {
    let c = corpus();
    let case = &c["cert"]["cases"].as_array().unwrap()[0];
    let wire = hex::decode(case["wire"].as_str().unwrap()).unwrap();

    assert!(Cert::decode(&wire).is_ok(), "the recorded certificate must decode");

    let mut long = wire.clone();
    long.push(0x00);
    assert_eq!(Cert::decode(&long), Err(Refusal::Wire), "a trailing byte must be refused");

    for cut in 1..=8usize.min(wire.len()) {
        let short = &wire[..wire.len() - cut];
        assert_eq!(Cert::decode(short), Err(Refusal::Wire), "a truncated certificate must be refused");
    }

    assert_eq!(Cert::decode(&[]), Err(Refusal::Wire), "no bytes are not a certificate");

    // A vote count no buffer that size could hold must be refused before it is
    // used as a capacity.
    let mut lying = wire.clone();
    let count_at = CERT_HEADER_LEN - 4;
    lying[count_at..count_at + 4].copy_from_slice(&u32::MAX.to_be_bytes());
    assert_eq!(Cert::decode(&lying), Err(Refusal::Wire), "an impossible vote count must be refused");

    // A signature length that runs past the end must be refused, not trusted.
    let mut overrun = wire.clone();
    let len_at = CERT_HEADER_LEN + NODE_LEN + 1;
    overrun[len_at..len_at + 4].copy_from_slice(&(u32::MAX / 2).to_be_bytes());
    assert_eq!(Cert::decode(&overrun), Err(Refusal::Wire), "an overrunning length prefix must be refused");
}
