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

use lux_consensus::finality::*;
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
            two_thirds_count(n),
            row["twoThirdsCount"].as_i64().unwrap(),
            "twoThirdsCount n={n}"
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
