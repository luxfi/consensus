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

    for n in [1usize, 4, 21] {
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

criterion_group!(benches, threshold, message, wire, verify);
criterion_main!(benches);
