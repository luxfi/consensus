// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! What a node actually pays to decide.
//!
//! The costs are not evenly spread. Building the signed message and picking a
//! threshold are arithmetic on fixed-width bytes; verifying a certificate is
//! elliptic-curve pairings, and it grows with the committee. Anything reported
//! about this crate's throughput that does not separate the two is measuring
//! the cheap part.

use std::hint::black_box;

use blst::min_pk::SecretKey;
use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};

use lux_consensus::cert::{pop_message, QuorumCert, ValidatorSet, Vote, DST, POP_DST};
use lux_consensus::finality::{canonical_vote_message, Finality, Id, Position};
use lux_consensus::{sha256, Engine, FpcSelector, QuasarConfig, QuasarEngine, VoteType};

fn position() -> Position {
    let f = |b: u8| [b; 32];
    Position {
        chain_id: f(7),
        height: 42,
        round: 3,
        block_id: f(11),
        parent_id: f(10),
        canonical_id: f(12),
        parent_canonical_id: f(13),
        execution_state_root: f(14),
        payload_root: f(15),
        validator_set_root: f(16),
    }
}

fn node_id(n: usize) -> Id {
    let mut id = [0u8; 32];
    id[..8].copy_from_slice(&(n as u64).to_be_bytes());
    id
}

/// A committee of `n` signers with a real certificate over `position()`.
fn certified(n: usize) -> (ValidatorSet, QuorumCert) {
    let message = canonical_vote_message(&position(), true);
    let mut set = ValidatorSet::new();
    let mut votes = Vec::with_capacity(n);

    for i in 0..n {
        let mut ikm = [0u8; 32];
        ikm[..8].copy_from_slice(&(i as u64 + 1).to_be_bytes());
        let sk = SecretKey::key_gen(&ikm, &[]).expect("key_gen");
        let id = node_id(i);
        let pop = sk
            .sign(&pop_message(&id, &sk.sk_to_pk().compress()), POP_DST, &[])
            .compress()
            .to_vec();
        set.insert(id, 100, &sk.sk_to_pk().compress(), &pop).expect("insert");
        votes.push(Vote {
            node_id: id,
            accept: true,
            signature: sk.sign(&message, DST, &[]).compress().to_vec(),
        });
    }

    let cert = QuorumCert::assemble(Finality::Quasar, position(), n as u32, &votes).expect("cert");
    (set, cert)
}

/// The signed message: 226 fixed-width bytes, no allocation beyond the buffer.
fn bench_message(c: &mut Criterion) {
    let pos = position();
    c.bench_function("canonical_vote_message", |b| {
        b.iter(|| canonical_vote_message(black_box(&pos), black_box(true)))
    });
}

/// The PRF behind every threshold, and the threshold itself.
fn bench_threshold(c: &mut Criterion) {
    let input = [0u8; 40];
    c.bench_function("sha256_40b", |b| b.iter(|| sha256(black_box(&input))));

    let s = FpcSelector::default();
    c.bench_function("fpc_select_threshold", |b| {
        b.iter(|| s.select_threshold(black_box(7), black_box(21)))
    });
}

/// Verifying a certificate, signature by signature. This is the real cost of
/// accepting a block, and it is linear in the committee.
fn bench_verify(c: &mut Criterion) {
    let mut group = c.benchmark_group("cert_verify");
    for n in [4usize, 11, 21] {
        let (set, cert) = certified(n);
        group.throughput(Throughput::Elements(n as u64));
        group.bench_with_input(BenchmarkId::from_parameter(n), &n, |b, _| {
            b.iter(|| black_box(&cert).verify(black_box(&set), 0).is_ok())
        });
    }
    group.finish();
}

/// Assembling a certificate: sort, dedup, and no cryptography at all.
fn bench_assemble(c: &mut Criterion) {
    let (_, cert) = certified(21);
    let votes = cert.votes.clone();
    c.bench_function("cert_assemble_21", |b| {
        b.iter(|| {
            QuorumCert::assemble(
                Finality::Quasar,
                position(),
                21,
                black_box(&votes),
            )
        })
    });
}

/// The probabilistic engine's ballot path, which does no cryptography — the
/// number that is easy to make look large.
fn bench_ballots(c: &mut Criterion) {
    let mut config = QuasarConfig::testnet();
    config.k = 21;
    config.beta = 1;

    c.bench_function("record_ballot_to_decision", |b| {
        b.iter_batched(
            || {
                let mut engine = QuasarEngine::new(config.clone());
                engine.start().unwrap();
                for i in 0..21 {
                    engine.add_validator(lux_consensus::NodeID::from(node_id(i)), 1);
                }
                let block = lux_consensus::Block::new(
                    lux_consensus::ID::from([11u8; 32]),
                    lux_consensus::ID::zero(),
                    1,
                    Vec::new(),
                );
                engine.add(block.clone()).unwrap();
                (engine, block)
            },
            |(mut engine, block)| {
                for i in 0..21 {
                    let vote = lux_consensus::Vote::new(
                        block.id.clone(),
                        VoteType::Preference,
                        lux_consensus::NodeID::from(node_id(i)),
                    );
                    let _ = engine.record_vote(vote);
                }
                engine.is_accepted(&block.id)
            },
            criterion::BatchSize::SmallInput,
        )
    });
}

criterion_group!(
    benches,
    bench_message,
    bench_threshold,
    bench_verify,
    bench_assemble,
    bench_ballots,
);
criterion_main!(benches);
