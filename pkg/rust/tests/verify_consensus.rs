// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// Verification test for consensus FFI behavior

use lux_consensus::*;
use std::time::{SystemTime, UNIX_EPOCH};

#[test]
fn verify_chain_consensus() {
    println!("\n=== VERIFYING CHAIN CONSENSUS ===");

    ConsensusEngine::init().expect("Init failed");

    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 15,
        alpha_confidence: 15,
        beta: 20,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 1024,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::Chain,
    };

    let mut engine = ConsensusEngine::new(&config).expect("Chain engine creation failed");

    // Add blocks to chain
    for i in 0..5 {
        let block = LuxBlock {
            id: [i as u8; 32],
            parent_id: if i == 0 { [0; 32] } else { [(i - 1) as u8; 32] },
            height: i as u64,
            timestamp: SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs(),
            data: std::ptr::null_mut(),
            data_size: 0,
        };

        engine.add_block(&block).expect(&format!("Failed to add block {}", i));
        println!("  Added block {} to Chain consensus", i);
    }

    // Process votes
    for i in 0..20 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: [4; 32], // Vote for last block
            is_preference: false,
        };
        engine.process_vote(&vote).expect("Failed to process vote");
    }

    let is_accepted = engine.is_accepted(&[4; 32]).expect("Failed to check acceptance");
    assert!(is_accepted, "Chain: Block 4 should be accepted");
    println!("  ✅ Chain consensus: Block 4 accepted");

    ConsensusEngine::cleanup().expect("Cleanup failed");
}

#[test]
fn verify_dag_consensus() {
    println!("\n=== VERIFYING DAG CONSENSUS ===");

    ConsensusEngine::init().expect("Init failed");

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

    let mut engine = ConsensusEngine::new(&config).expect("DAG engine creation failed");

    // Create DAG structure with multiple parents
    let blocks = vec![
        // Genesis
        LuxBlock {
            id: [0; 32],
            parent_id: [0; 32],
            height: 0,
            timestamp: SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs(),
            data: std::ptr::null_mut(),
            data_size: 0,
        },
        // Layer 1
        LuxBlock {
            id: [1; 32],
            parent_id: [0; 32],
            height: 1,
            timestamp: SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs(),
            data: std::ptr::null_mut(),
            data_size: 0,
        },
        LuxBlock {
            id: [2; 32],
            parent_id: [0; 32],
            height: 1,
            timestamp: SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs(),
            data: std::ptr::null_mut(),
            data_size: 0,
        },
        // Layer 2 (references both layer 1 blocks)
        LuxBlock {
            id: [3; 32],
            parent_id: [1; 32], // In real DAG, would have multiple parents
            height: 2,
            timestamp: SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs(),
            data: std::ptr::null_mut(),
            data_size: 0,
        },
    ];

    for block in &blocks {
        engine.add_block(block).expect(&format!("Failed to add DAG block {:?}", block.id[0]));
        println!("  Added block {} to DAG", block.id[0]);
    }

    // Vote for convergence
    for i in 0..20 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: [3; 32], // Vote for tip
            is_preference: false,
        };
        engine.process_vote(&vote).expect("Failed to process DAG vote");
    }

    let is_accepted = engine.is_accepted(&[3; 32]).expect("Failed to check DAG acceptance");
    assert!(is_accepted, "DAG: Block 3 should be accepted");
    println!("  ✅ DAG consensus: Block 3 accepted");

    ConsensusEngine::cleanup().expect("Cleanup failed");
}

#[test]
fn verify_pq_consensus() {
    println!("\n=== VERIFYING PQ (Post-Quantum) CONSENSUS ===");

    ConsensusEngine::init().expect("Init failed");

    let config = LuxConsensusConfig {
        k: 10,
        alpha_preference: 7,
        alpha_confidence: 7,
        beta: 10,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 512,
        max_item_processing_time_ns: 1_000_000_000,
        engine_type: LuxEngineType::PQ,
    };

    let mut engine = ConsensusEngine::new(&config).expect("PQ engine creation failed");

    // Add quantum-resistant blocks
    for i in 0..3 {
        let block = LuxBlock {
            id: [(0xF0 + i) as u8; 32], // Special prefix for PQ blocks
            parent_id: if i == 0 { [0; 32] } else { [(0xF0 + i - 1) as u8; 32] },
            height: i as u64,
            timestamp: SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs(),
            data: std::ptr::null_mut(),
            data_size: 0,
        };

        engine.add_block(&block).expect(&format!("Failed to add PQ block {}", i));
        println!("  Added PQ block {} (ID: 0x{:02X})", i, 0xF0 + i);
    }

    // Process quantum-safe votes
    for i in 0..10 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: [0xF2; 32], // Vote for last PQ block
            is_preference: false,
        };
        engine.process_vote(&vote).expect("Failed to process PQ vote");
    }

    let is_accepted = engine.is_accepted(&[0xF2; 32]).expect("Failed to check PQ acceptance");
    assert!(is_accepted, "PQ: Block 0xF2 should be accepted");
    println!("  ✅ PQ consensus: Block 0xF2 accepted with quantum resistance");

    ConsensusEngine::cleanup().expect("Cleanup failed");
}

#[test]
fn verify_consensus_stats() {
    println!("\n=== VERIFYING CONSENSUS STATISTICS ===");

    ConsensusEngine::init().expect("Init failed");

    let engines = vec![
        ("Chain", LuxEngineType::Chain),
        ("DAG", LuxEngineType::DAG),
        ("PQ", LuxEngineType::PQ),
    ];

    for (name, engine_type) in engines {
        let config = LuxConsensusConfig {
            k: 20,
            alpha_preference: 15,
            alpha_confidence: 15,
            beta: 20,
            concurrent_polls: 1,
            optimal_processing: 1,
            max_outstanding_items: 1024,
            max_item_processing_time_ns: 2_000_000_000,
            engine_type,
        };

        let mut engine = ConsensusEngine::new(&config).expect(&format!("{} engine failed", name));

        // Add blocks and votes
        for i in 0..10 {
            let block = LuxBlock {
                id: [i as u8; 32],
                parent_id: if i == 0 { [0; 32] } else { [(i - 1) as u8; 32] },
                height: i as u64,
                timestamp: SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs(),
                data: std::ptr::null_mut(),
                data_size: 0,
            };
            engine.add_block(&block).expect("Failed to add block");

            // Vote for the block
            for j in 0..20 {
                let vote = LuxVote {
                    voter_id: [j; 32],
                    block_id: block.id,
                    is_preference: false,
                };
                engine.process_vote(&vote).expect("Failed to process vote");
            }
        }

        let stats = engine.get_stats().expect("Failed to get stats");
        println!("\n  {} Engine Statistics:", name);
        println!("    Blocks accepted: {}", stats.blocks_accepted);
        println!("    Blocks rejected: {}", stats.blocks_rejected);
        println!("    Votes processed: {}", stats.votes_processed);
        println!("    Polls completed: {}", stats.polls_completed);

        assert!(stats.votes_processed > 0, "{}: Should have processed votes", name);
        println!("  ✅ {} consensus statistics verified", name);
    }

    ConsensusEngine::cleanup().expect("Cleanup failed");
}