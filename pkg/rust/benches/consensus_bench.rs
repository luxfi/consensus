// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

use criterion::{black_box, criterion_group, criterion_main, BatchSize, BenchmarkId, Criterion, Throughput};
use lux_consensus::{ConsensusEngine, LuxBlock, LuxConsensusConfig, LuxEngineType, LuxVote};
use std::ptr;
use std::time::Duration;

/// Generate a sample block with given ID and parent
fn create_block(id: u8, parent_id: u8, height: u64) -> LuxBlock {
    LuxBlock {
        id: [id; 32],
        parent_id: [parent_id; 32],
        height,
        timestamp: 1234567890 + height,
        data: ptr::null_mut(),
        data_size: 0,
    }
}

/// Generate a vote for a block
fn create_vote(voter_id: u8, block_id: [u8; 32], is_preference: bool) -> LuxVote {
    LuxVote {
        voter_id: [voter_id; 32],
        block_id,
        is_preference,
    }
}

/// Create a standard consensus configuration
fn create_config(engine_type: LuxEngineType) -> LuxConsensusConfig {
    LuxConsensusConfig {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 10240,
        max_item_processing_time_ns: 2000000000,
        engine_type,
    }
}

/// Benchmark single block addition
fn bench_single_block_add(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = create_config(LuxEngineType::DAG);

    c.bench_function("single_block_add", |b| {
        b.iter_batched(
            || ConsensusEngine::new(&config).expect("Failed to create engine"),
            |mut engine| {
                let block = create_block(1, 0, 1);
                engine.add_block(black_box(&block)).expect("Failed to add block");
            },
            BatchSize::SmallInput,
        );
    });

    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark batch block additions with different sizes
fn bench_batch_block_add(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = create_config(LuxEngineType::DAG);
    let batch_sizes = vec![100, 1000, 10000];

    let mut group = c.benchmark_group("batch_block_add");

    for size in batch_sizes {
        group.throughput(Throughput::Elements(size));
        group.bench_with_input(BenchmarkId::from_parameter(size), &size, |b, &size| {
            b.iter_batched(
                || {
                    let engine = ConsensusEngine::new(&config).expect("Failed to create engine");
                    let blocks: Vec<LuxBlock> = (0..size as u8)
                        .map(|i| create_block(i, if i == 0 { 0 } else { i - 1 }, i as u64))
                        .collect();
                    (engine, blocks)
                },
                |(mut engine, blocks)| {
                    for block in blocks.iter() {
                        engine.add_block(black_box(block)).expect("Failed to add block");
                    }
                },
                BatchSize::SmallInput,
            );
        });
    }

    group.finish();
    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark single vote processing
fn bench_single_vote(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = create_config(LuxEngineType::DAG);

    c.bench_function("single_vote_process", |b| {
        b.iter_batched(
            || {
                let mut engine = ConsensusEngine::new(&config).expect("Failed to create engine");
                let block = create_block(1, 0, 1);
                engine.add_block(&block).expect("Failed to add block");
                (engine, block.id)
            },
            |(mut engine, block_id)| {
                let vote = create_vote(1, block_id, true);
                engine.process_vote(black_box(&vote)).expect("Failed to process vote");
            },
            BatchSize::SmallInput,
        );
    });

    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark batch vote processing with different sizes
fn bench_batch_vote(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = create_config(LuxEngineType::DAG);
    let batch_sizes = vec![100, 1000, 10000];

    let mut group = c.benchmark_group("batch_vote_process");

    for size in batch_sizes {
        group.throughput(Throughput::Elements(size));
        group.bench_with_input(BenchmarkId::from_parameter(size), &size, |b, &size| {
            b.iter_batched(
                || {
                    let mut engine = ConsensusEngine::new(&config).expect("Failed to create engine");
                    let block = create_block(1, 0, 1);
                    engine.add_block(&block).expect("Failed to add block");

                    let votes: Vec<LuxVote> = (0..size as u8)
                        .map(|i| create_vote(i, block.id, i % 2 == 0))
                        .collect();
                    (engine, votes)
                },
                |(mut engine, votes)| {
                    for vote in votes.iter() {
                        engine.process_vote(black_box(vote)).expect("Failed to process vote");
                    }
                },
                BatchSize::SmallInput,
            );
        });
    }

    group.finish();
    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark finalization (acceptance checking)
fn bench_finalization(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = LuxConsensusConfig {
        k: 5,
        alpha_preference: 3,
        alpha_confidence: 3,
        beta: 5,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 10240,
        max_item_processing_time_ns: 2000000000,
        engine_type: LuxEngineType::DAG,
    };

    c.bench_function("finalization_check", |b| {
        b.iter_batched(
            || {
                let mut engine = ConsensusEngine::new(&config).expect("Failed to create engine");
                let block = create_block(1, 0, 1);
                engine.add_block(&block).expect("Failed to add block");

                // Add enough votes to trigger finalization
                for i in 0..5 {
                    let vote = create_vote(i, block.id, true);
                    engine.process_vote(&vote).expect("Failed to process vote");
                }
                (engine, block.id)
            },
            |(engine, block_id)| {
                let _ = engine.is_accepted(black_box(&block_id));
            },
            BatchSize::SmallInput,
        );
    });

    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark preference retrieval
fn bench_get_preference(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = create_config(LuxEngineType::DAG);

    c.bench_function("get_preference", |b| {
        b.iter_batched(
            || {
                let mut engine = ConsensusEngine::new(&config).expect("Failed to create engine");
                let block = create_block(1, 0, 1);
                engine.add_block(&block).expect("Failed to add block");
                engine
            },
            |engine| {
                let _ = engine.get_preference();
            },
            BatchSize::SmallInput,
        );
    });

    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark statistics retrieval
fn bench_get_stats(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = create_config(LuxEngineType::DAG);

    c.bench_function("get_stats", |b| {
        b.iter_batched(
            || {
                let mut engine = ConsensusEngine::new(&config).expect("Failed to create engine");
                // Add some activity
                for i in 0..10 {
                    let block = create_block(i, if i == 0 { 0 } else { i - 1 }, i as u64);
                    engine.add_block(&block).expect("Failed to add block");
                    let vote = create_vote(i, block.id, true);
                    engine.process_vote(&vote).expect("Failed to process vote");
                }
                engine
            },
            |engine| {
                let _ = engine.get_stats();
            },
            BatchSize::SmallInput,
        );
    });

    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark different consensus engine types
fn bench_engine_types(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let engine_types = vec![
        (LuxEngineType::Chain, "chain"),
        (LuxEngineType::DAG, "dag"),
        (LuxEngineType::PQ, "pq"),
    ];

    let mut group = c.benchmark_group("engine_types");

    for (engine_type, name) in engine_types {
        group.bench_with_input(BenchmarkId::from_parameter(name), &engine_type, |b, &engine_type| {
            let config = create_config(engine_type);
            b.iter_batched(
                || ConsensusEngine::new(&config).expect("Failed to create engine"),
                |mut engine| {
                    let block = create_block(1, 0, 1);
                    engine.add_block(black_box(&block)).expect("Failed to add block");

                    for i in 0..3 {
                        let vote = create_vote(i, block.id, true);
                        engine.process_vote(&vote).expect("Failed to process vote");
                    }

                    let _ = engine.is_accepted(&block.id);
                },
                BatchSize::SmallInput,
            );
        });
    }

    group.finish();
    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark polling operation
fn bench_poll(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = create_config(LuxEngineType::DAG);

    c.bench_function("poll_validators", |b| {
        b.iter_batched(
            || {
                let mut engine = ConsensusEngine::new(&config).expect("Failed to create engine");
                let block = create_block(1, 0, 1);
                engine.add_block(&block).expect("Failed to add block");

                let validator_ids: Vec<[u8; 32]> = (0..10)
                    .map(|i| [i; 32])
                    .collect();
                (engine, validator_ids)
            },
            |(mut engine, validator_ids)| {
                engine.poll(black_box(&validator_ids)).expect("Failed to poll");
            },
            BatchSize::SmallInput,
        );
    });

    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

/// Benchmark complete consensus flow
fn bench_complete_flow(c: &mut Criterion) {
    ConsensusEngine::init().expect("Failed to init consensus");

    let config = LuxConsensusConfig {
        k: 5,
        alpha_preference: 3,
        alpha_confidence: 3,
        beta: 5,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 10240,
        max_item_processing_time_ns: 2000000000,
        engine_type: LuxEngineType::DAG,
    };

    c.bench_function("complete_consensus_flow", |b| {
        b.iter_batched(
            || ConsensusEngine::new(&config).expect("Failed to create engine"),
            |mut engine| {
                // Add blocks
                for i in 0..5 {
                    let block = create_block(i, if i == 0 { 0 } else { i - 1 }, i as u64);
                    engine.add_block(&block).expect("Failed to add block");

                    // Process votes for each block
                    for j in 0..5 {
                        let vote = create_vote(j, block.id, true);
                        engine.process_vote(&vote).expect("Failed to process vote");
                    }

                    // Check acceptance
                    let _ = engine.is_accepted(&block.id);
                }

                // Get final preference and stats
                let _ = engine.get_preference();
                let _ = engine.get_stats();
            },
            BatchSize::SmallInput,
        );
    });

    ConsensusEngine::cleanup().expect("Failed to cleanup");
}

criterion_group! {
    name = consensus_benches;
    config = Criterion::default()
        .measurement_time(Duration::from_secs(10))
        .sample_size(100)
        .warm_up_time(Duration::from_secs(3));
    targets =
        bench_single_block_add,
        bench_batch_block_add,
        bench_single_vote,
        bench_batch_vote,
        bench_finalization,
        bench_get_preference,
        bench_get_stats,
        bench_engine_types,
        bench_poll,
        bench_complete_flow
}

criterion_main!(consensus_benches);