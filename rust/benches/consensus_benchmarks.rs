// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

use criterion::{black_box, criterion_group, criterion_main, BatchSize, BenchmarkId, Criterion};
use lux_consensus::*;
use std::ptr;
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::{SystemTime, UNIX_EPOCH};

fn benchmark_engine_creation(c: &mut Criterion) {
    ConsensusEngine::init().unwrap();
    
    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 1024,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    c.bench_function("engine_creation", |b| {
        b.iter(|| {
            let engine = ConsensusEngine::new(&config).unwrap();
            black_box(engine);
        })
    });
    
    ConsensusEngine::cleanup().unwrap();
}

fn benchmark_block_operations(c: &mut Criterion) {
    ConsensusEngine::init().unwrap();
    
    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 10000,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    let mut group = c.benchmark_group("block_operations");
    
    // Single block addition
    group.bench_function("single_block_add", |b| {
        let mut engine = ConsensusEngine::new(&config).unwrap();
        let mut counter = 0u32;
        
        b.iter(|| {
            let mut id = [0u8; 32];
            id[0] = (counter & 0xFF) as u8;
            id[1] = ((counter >> 8) & 0xFF) as u8;
            counter += 1;
            
            let block = LuxBlock {
                id,
                parent_id: [0; 32],
                height: counter as u64,
                timestamp: SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .unwrap()
                    .as_secs(),
                data: ptr::null_mut(),
                data_size: 0,
            };
            
            engine.add_block(&block).unwrap();
        })
    });
    
    // Batch block operations
    for size in &[100, 1000] {
        group.bench_with_input(
            BenchmarkId::new("batch_blocks", size),
            size,
            |b, &size| {
                b.iter_batched(
                    || ConsensusEngine::new(&config).unwrap(),
                    |mut engine| {
                        for i in 0..size {
                            let mut id = [0u8; 32];
                            id[0] = (i & 0xFF) as u8;
                            id[1] = ((i >> 8) & 0xFF) as u8;
                            id[2] = ((i >> 16) & 0xFF) as u8;
                            
                            let block = LuxBlock {
                                id,
                                parent_id: [0; 32],
                                height: i as u64,
                                timestamp: SystemTime::now()
                                    .duration_since(UNIX_EPOCH)
                                    .unwrap()
                                    .as_secs(),
                                data: ptr::null_mut(),
                                data_size: 0,
                            };
                            
                            engine.add_block(&block).unwrap();
                        }
                    },
                    BatchSize::SmallInput,
                )
            },
        );
    }
    
    group.finish();
    ConsensusEngine::cleanup().unwrap();
}

fn benchmark_vote_processing(c: &mut Criterion) {
    ConsensusEngine::init().unwrap();
    
    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 1024,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    let mut group = c.benchmark_group("vote_processing");
    
    // Setup engine with blocks
    let mut engine = ConsensusEngine::new(&config).unwrap();
    for i in 0..100 {
        let block = LuxBlock {
            id: [i as u8; 32],
            parent_id: [0; 32],
            height: i,
            timestamp: SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_secs(),
            data: ptr::null_mut(),
            data_size: 0,
        };
        engine.add_block(&block).unwrap();
    }
    
    // Single vote processing
    group.bench_function("single_vote", |b| {
        let mut counter = 0u32;
        
        b.iter(|| {
            let mut voter_id = [0u8; 32];
            voter_id[0] = (counter & 0xFF) as u8;
            voter_id[1] = ((counter >> 8) & 0xFF) as u8;
            counter += 1;
            
            let vote = LuxVote {
                voter_id,
                block_id: [(counter % 100) as u8; 32],
                is_preference: counter % 2 == 0,
            };
            
            engine.process_vote(&vote).unwrap();
        })
    });
    
    // Batch vote processing
    for size in &[1000, 10000] {
        group.bench_with_input(
            BenchmarkId::new("batch_votes", size),
            size,
            |b, &size| {
                b.iter(|| {
                    for i in 0..size {
                        let mut voter_id = [0u8; 32];
                        voter_id[0] = (i & 0xFF) as u8;
                        voter_id[1] = ((i >> 8) & 0xFF) as u8;
                        voter_id[2] = ((i >> 16) & 0xFF) as u8;
                        
                        let vote = LuxVote {
                            voter_id,
                            block_id: [(i % 100) as u8; 32],
                            is_preference: i % 2 == 0,
                        };
                        
                        engine.process_vote(&vote).unwrap();
                    }
                })
            },
        );
    }
    
    group.finish();
    ConsensusEngine::cleanup().unwrap();
}

fn benchmark_query_operations(c: &mut Criterion) {
    ConsensusEngine::init().unwrap();
    
    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 1024,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    let mut engine = ConsensusEngine::new(&config).unwrap();
    
    // Add blocks
    let mut block_ids = Vec::new();
    for i in 0..1000 {
        let mut id = [0u8; 32];
        id[0] = (i & 0xFF) as u8;
        id[1] = ((i >> 8) & 0xFF) as u8;
        block_ids.push(id);
        
        let block = LuxBlock {
            id,
            parent_id: [0; 32],
            height: i as u64,
            timestamp: SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_secs(),
            data: ptr::null_mut(),
            data_size: 0,
        };
        engine.add_block(&block).unwrap();
    }
    
    let mut group = c.benchmark_group("query_operations");
    
    group.bench_function("is_accepted", |b| {
        let mut counter = 0usize;
        b.iter(|| {
            let block_id = &block_ids[counter % 1000];
            let result = engine.is_accepted(block_id).unwrap();
            counter += 1;
            black_box(result);
        })
    });
    
    group.bench_function("get_preference", |b| {
        b.iter(|| {
            let pref = engine.get_preference().unwrap();
            black_box(pref);
        })
    });
    
    group.bench_function("get_stats", |b| {
        b.iter(|| {
            let stats = engine.get_stats().unwrap();
            black_box(stats);
        })
    });
    
    group.finish();
    ConsensusEngine::cleanup().unwrap();
}

fn benchmark_concurrent_operations(c: &mut Criterion) {
    ConsensusEngine::init().unwrap();
    
    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 10000,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    let mut group = c.benchmark_group("concurrent_operations");
    
    for num_threads in &[1, 2, 4, 8] {
        group.bench_with_input(
            BenchmarkId::new("threads", num_threads),
            num_threads,
            |b, &num_threads| {
                b.iter_batched(
                    || Arc::new(Mutex::new(ConsensusEngine::new(&config).unwrap())),
                    |engine: Arc<Mutex<ConsensusEngine>>| {
                        let mut handles = vec![];
                        let operations_per_thread = 1000;
                        
                        for thread_id in 0..num_threads {
                            let engine_clone = Arc::clone(&engine);
                            let handle = thread::spawn(move || {
                                for i in 0..operations_per_thread {
                                    let mut id = [0u8; 32];
                                    id[0] = thread_id as u8;
                                    id[1] = (i & 0xFF) as u8;
                                    id[2] = ((i >> 8) & 0xFF) as u8;
                                    
                                    let block = LuxBlock {
                                        id,
                                        parent_id: [0; 32],
                                        height: i as u64,
                                        timestamp: SystemTime::now()
                                            .duration_since(UNIX_EPOCH)
                                            .unwrap()
                                            .as_secs(),
                                        data: ptr::null_mut(),
                                        data_size: 0,
                                    };
                                    
                                    let mut eng = engine_clone.lock().unwrap();
                                    eng.add_block(&block).unwrap();
                                }
                            });
                            handles.push(handle);
                        }
                        
                        for handle in handles {
                            handle.join().unwrap();
                        }
                    },
                    BatchSize::SmallInput,
                )
            },
        );
    }
    
    group.finish();
    ConsensusEngine::cleanup().unwrap();
}

fn benchmark_memory_usage(c: &mut Criterion) {
    ConsensusEngine::init().unwrap();
    
    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 100000,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    let mut group = c.benchmark_group("memory_usage");
    group.sample_size(10); // Reduce sample size for large operations
    
    for size in &[1000, 10000] {
        group.bench_with_input(
            BenchmarkId::new("blocks_with_data", size),
            size,
            |b, &size| {
                b.iter_batched(
                    || ConsensusEngine::new(&config).unwrap(),
                    |mut engine| {
                        for i in 0..size {
                            let mut id = [0u8; 32];
                            id[0] = (i & 0xFF) as u8;
                            id[1] = ((i >> 8) & 0xFF) as u8;
                            id[2] = ((i >> 16) & 0xFF) as u8;
                            id[3] = ((i >> 24) & 0xFF) as u8;
                            
                            let data = format!("Block data {}", i);
                            let block = LuxBlock {
                                id,
                                parent_id: [0; 32],
                                height: i as u64,
                                timestamp: SystemTime::now()
                                    .duration_since(UNIX_EPOCH)
                                    .unwrap()
                                    .as_secs(),
                                data: data.as_ptr() as *mut _,
                                data_size: data.len(),
                            };
                            
                            engine.add_block(&block).unwrap();
                        }
                    },
                    BatchSize::SmallInput,
                )
            },
        );
    }
    
    group.finish();
    ConsensusEngine::cleanup().unwrap();
}

criterion_group!(
    benches,
    benchmark_engine_creation,
    benchmark_block_operations,
    benchmark_vote_processing,
    benchmark_query_operations,
    benchmark_concurrent_operations,
    benchmark_memory_usage
);

criterion_main!(benches);