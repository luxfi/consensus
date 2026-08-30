// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! What a node actually spends time on, measured.
//!
//! The benchmark that stood here imported `LuxBlock`, `LuxVote`,
//! `LuxConsensusConfig`, `LuxEngineType` and `ConsensusEngine` — five names this
//! crate has not exported for a long time — so `cargo bench` did not compile,
//! and the report beside it carried numbers no binary in this repository could
//! produce. It has been retracted in place since; this replaces it with the
//! paths that are on the round and certificate hot loop.
//!
//! Four groups, in the order a round runs them:
//!
//!   * `threshold` — the FPC PRF, once per round per node.
//!   * `message`   — the canonical vote message, once per vote signed and once
//!                   per certificate verified.
//!   * `wire`      — certificate encode and decode, once per gossip hop.
//!   * `verify`    — the whole finality predicate over real BLS signatures. This
//!                   is the expensive one and the one that decides.
//!
//! Three more groups exist so a number here can be divided by a number from the
//! Go or C++ leg:
//!
//!   * `sign`      — one signature, and the aggregate of n.
//!   * `matched`   — the same blst calls the other two legs make, in the same
//!                   order. `verify/signature` above is what this crate really
//!                   pays; `matched/verify` is what is comparable. The gap is a
//!                   policy this crate chose, and naming it is the point.
//!   * `round`     — the work of turning a position into an ADMITTED
//!                   certificate, split by who pays it. No leg was timing this.

use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use lux_consensus::bls::{Registry, DST};
use lux_consensus::finality::{canonical_vote_message, Cert, Finality, Keys, Position, Vote, NODE_LEN};
use lux_consensus::{derive_epoch_seed, FpcSelector};
use std::hint::black_box;

fn position() -> Position {
    Position {
        chain_id: [0x11; 32],
        height: 0x0102030405060708,
        round: 0x0A0B0C0D,
        block_id: [0x22; 32],
        parent_id: [0x33; 32],
        canonical_id: [0x44; 32],
        parent_canonical_id: [0x55; 32],
        execution_state_root: [0x66; 32],
        payload_root: [0x77; 32],
        validator_set_root: [0x88; 32],
    }
}

/// A committee of `n` validators with real keys, and a certificate they all
/// signed. Deterministic: the key material is derived from the index, so a run
/// is comparable to the run before it.
fn signed(n: usize) -> (Cert, Registry) {
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let mut registry = Registry::new();
    let mut votes = Vec::with_capacity(n);

    for i in 0..n {
        let mut ikm = [0u8; 32];
        ikm[..8].copy_from_slice(&(i as u64 + 1).to_be_bytes());
        let sk = blst::min_pk::SecretKey::key_gen(&ikm, &[]).expect("key_gen");

        let mut node = [0u8; NODE_LEN];
        node[..8].copy_from_slice(&(i as u64).to_be_bytes());
        assert!(registry.insert(node, &sk.sk_to_pk().compress()));

        votes.push(Vote {
            node,
            accept: true,
            signature: sk.sign(&message, DST, &[]).compress().to_vec(),
        });
    }

    let cert = Cert::assemble(pos, Finality::Quasar, n as u32, votes).expect("assemble");
    (cert, registry)
}

fn threshold(c: &mut Criterion) {
    let seed = derive_epoch_seed(42, &[0x11; 32], &[0x22; 32]);
    let sel = FpcSelector::new(0.5, 0.8, seed);
    let mut group = c.benchmark_group("threshold");

    group.bench_function("derive_epoch_seed", |b| {
        b.iter(|| derive_epoch_seed(black_box(42), black_box(&[0x11; 32]), black_box(&[0x22; 32])))
    });

    for k in [4usize, 21, 100] {
        group.bench_with_input(BenchmarkId::new("select", k), &k, |b, &k| {
            let mut phase = 0u64;
            b.iter(|| {
                phase = phase.wrapping_add(1);
                sel.select_threshold(black_box(phase), black_box(k))
            })
        });
    }
    group.finish();
}

fn message(c: &mut Criterion) {
    let pos = position();
    c.benchmark_group("message")
        .throughput(Throughput::Bytes(lux_consensus::VOTE_MESSAGE_LEN as u64))
        .bench_function("canonical_vote", |b| {
            b.iter(|| canonical_vote_message(black_box(&pos), black_box(true)))
        });
}

fn wire(c: &mut Criterion) {
    let mut group = c.benchmark_group("wire");
    for n in [1usize, 21, 100] {
        let (cert, _) = signed(n);
        let bytes = cert.encode();
        group.throughput(Throughput::Bytes(bytes.len() as u64));
        group.bench_with_input(BenchmarkId::new("encode", n), &cert, |b, cert| {
            b.iter(|| cert.encode())
        });
        group.bench_with_input(BenchmarkId::new("decode", n), &bytes, |b, bytes| {
            b.iter(|| Cert::decode(black_box(bytes)).expect("decode"))
        });
    }
    group.finish();
}

fn verify(c: &mut Criterion) {
    let mut group = c.benchmark_group("verify");
    group.sample_size(20);

    for n in [1usize, 4, 21, 41, 100] {
        let (cert, registry) = signed(n);
        group.throughput(Throughput::Elements(n as u64));
        group.bench_with_input(BenchmarkId::new("certificate", n), &n, |b, _| {
            b.iter(|| cert.verify(black_box(&registry)).expect("verify"))
        });
    }

    // One signature on its own, so the certificate figures above divide cleanly
    // and a change in the predicate is distinguishable from a change in the
    // pairing.
    let (cert, registry) = signed(1);
    let msg = cert.message();
    let v = &cert.votes[0];
    group.bench_function("signature", |b| {
        b.iter(|| registry.verify(black_box(&v.node), black_box(&msg), black_box(&v.signature)))
    });
    group.finish();
}


/// One signature, and one aggregate of n. The Go and C++ legs time the same two,
/// so these are what make the signing side of the table comparable.
fn sign(c: &mut Criterion) {
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let mut ikm = [0u8; 32];
    ikm[..8].copy_from_slice(&1u64.to_be_bytes());
    let sk = blst::min_pk::SecretKey::key_gen(&ikm, &[]).expect("key_gen");

    let mut group = c.benchmark_group("sign");
    group.bench_function("one", |b| {
        b.iter(|| sk.sign(black_box(&message), DST, &[]).compress())
    });

    for n in [1usize, 4, 21, 41, 100] {
        let (cert, _) = signed(n);
        let sigs: Vec<blst::min_pk::Signature> = cert
            .votes
            .iter()
            .map(|v| blst::min_pk::Signature::uncompress(&v.signature).expect("uncompress"))
            .collect();
        group.bench_with_input(BenchmarkId::new("aggregate", n), &sigs, |b, sigs| {
            b.iter(|| {
                let refs: Vec<&blst::min_pk::Signature> = sigs.iter().collect();
                blst::min_pk::AggregateSignature::aggregate(black_box(&refs), false)
                    .expect("aggregate")
            })
        });
    }
    group.finish();
}

/// The same blst calls the Go and C++ legs make, in the same order.
///
/// [`Registry::verify`] validates the PUBLIC KEY on every verification although
/// [`Registry::insert`] already validated it when the validator joined the set.
/// That is a subgroup multiplication per vote that the set boundary has already
/// paid for. `matched/verify` is the same verification without it, so the cost
/// of the choice is a measured number rather than an opinion, and so the figure
/// that is compared across languages is a figure of the same work.
///
/// The SIGNATURE group check stays on in both. It is not redundant with
/// anything: the signature arrives from the wire on every call.
fn matched(c: &mut Criterion) {
    let pos = position();
    let message = canonical_vote_message(&pos, true);
    let mut group = c.benchmark_group("matched");

    let mut ikm = [0u8; 32];
    ikm[..8].copy_from_slice(&1u64.to_be_bytes());
    let sk = blst::min_pk::SecretKey::key_gen(&ikm, &[]).expect("key_gen");
    let pk = sk.sk_to_pk();
    let compressed = sk.sign(&message, DST, &[]).compress();

    // Decompress the signature, group-check it ONCE, pair. The key is already
    // decompressed and was validated when the validator joined.
    group.bench_function("verify", |b| {
        b.iter(|| {
            let sig = blst::min_pk::Signature::uncompress(black_box(&compressed)).expect("uncompress");
            assert_eq!(
                sig.verify(true, black_box(&message), DST, &[], &pk, false),
                blst::BLST_ERROR::BLST_SUCCESS
            );
        })
    });

    // The same verification with the redundant key validation the registry does.
    group.bench_function("verify_revalidating_key", |b| {
        b.iter(|| {
            let sig = blst::min_pk::Signature::uncompress(black_box(&compressed)).expect("uncompress");
            assert_eq!(
                sig.verify(true, black_box(&message), DST, &[], &pk, true),
                blst::BLST_ERROR::BLST_SUCCESS
            );
        })
    });

    // The pairing alone — the floor every leg's verify sits on.
    let point = blst::min_pk::Signature::uncompress(&compressed).expect("uncompress");
    group.bench_function("pairing", |b| {
        b.iter(|| {
            assert_eq!(
                point.verify(false, black_box(&message), DST, &[], &pk, false),
                blst::BLST_ERROR::BLST_SUCCESS
            );
        })
    });

    // The check this crate pays twice, priced.
    group.bench_function("group_check_signature", |b| {
        b.iter(|| black_box(&point).validate(true).is_ok())
    });

    // The O(1) aggregate predicate, over keys a validator set already holds
    // decompressed. The Go and C++ legs time this; Rust had no counterpart.
    for n in [1usize, 4, 21, 41, 100] {
        let (cert, _) = signed(n);
        let sigs: Vec<blst::min_pk::Signature> = cert
            .votes
            .iter()
            .map(|v| blst::min_pk::Signature::uncompress(&v.signature).expect("uncompress"))
            .collect();
        let keys: Vec<blst::min_pk::PublicKey> = (0..n)
            .map(|i| {
                let mut ikm = [0u8; 32];
                ikm[..8].copy_from_slice(&(i as u64 + 1).to_be_bytes());
                blst::min_pk::SecretKey::key_gen(&ikm, &[]).expect("key_gen").sk_to_pk()
            })
            .collect();
        let refs: Vec<&blst::min_pk::Signature> = sigs.iter().collect();
        let aggregate = blst::min_pk::AggregateSignature::aggregate(&refs, false)
            .expect("aggregate")
            .to_signature();

        group.bench_with_input(BenchmarkId::new("fast_aggregate_verify", n), &keys, |b, keys| {
            b.iter(|| {
                let refs: Vec<&blst::min_pk::PublicKey> = keys.iter().collect();
                let sum = blst::min_pk::AggregatePublicKey::aggregate(black_box(&refs), false)
                    .expect("aggregate")
                    .to_public_key();
                assert_eq!(
                    aggregate.verify(false, &message, DST, &[], &sum, false),
                    blst::BLST_ERROR::BLST_SUCCESS
                );
            })
        });
    }
    group.finish();
}

/// A finality round: the work of turning a position into an ADMITTED
/// certificate. Split by who pays, because the three parties pay different
/// amounts and only one of them is on the critical path.
///
///   * `sign`    one validator's own cost — build the message, sign it once.
///               Independent of n: a validator does not sign n times.
///   * `collect` the assembling node — canonical order and the wire. O(n), and
///               no curve arithmetic at all.
///   * `admit`   a follower — decode the gossiped bytes and run the predicate.
///               O(n) pairings, which is finality's critical path.
fn round(c: &mut Criterion) {
    let pos = position();
    let mut group = c.benchmark_group("round");
    group.sample_size(20);

    let mut ikm = [0u8; 32];
    ikm[..8].copy_from_slice(&1u64.to_be_bytes());
    let sk = blst::min_pk::SecretKey::key_gen(&ikm, &[]).expect("key_gen");
    group.bench_function("sign", |b| {
        b.iter(|| {
            let message = canonical_vote_message(black_box(&pos), true);
            sk.sign(&message, DST, &[]).compress()
        })
    });

    for n in [4usize, 21, 41, 100] {
        let (cert, registry) = signed(n);
        let wire = cert.encode();

        // Reversed, so the assembler's sort does work rather than confirm it:
        // votes arrive in whatever order they were gossiped.
        let mut shuffled = cert.votes.clone();
        shuffled.reverse();
        group.bench_with_input(BenchmarkId::new("collect", n), &shuffled, |b, votes| {
            b.iter(|| {
                Cert::assemble(pos.clone(), Finality::Quasar, n as u32, votes.clone())
                    .expect("assemble")
                    .encode()
            })
        });

        group.throughput(Throughput::Elements(n as u64));
        group.bench_with_input(BenchmarkId::new("admit", n), &wire, |b, wire| {
            b.iter(|| {
                Cert::decode(black_box(wire))
                    .expect("decode")
                    .verify(&registry)
                    .expect("verify")
            })
        });
    }
    group.finish();
}

criterion_group!(more, sign, matched, round);

criterion_group!(benches, threshold, message, wire, verify);
criterion_main!(benches, more);
