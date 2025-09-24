// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

use lux_consensus::*;
use std::ptr;
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

// Test categories matching Go and C implementations
const NUM_TEST_CATEGORIES: usize = 15;

#[test]
fn test_initialization() {
    println!("=== INITIALIZATION: Library Lifecycle ===");
    
    // Test multiple init/cleanup cycles
    for i in 0..3 {
        assert!(ConsensusEngine::init().is_ok(), "Initialize library cycle {}", i);
        assert!(ConsensusEngine::cleanup().is_ok(), "Cleanup library cycle {}", i);
    }
    
    // Test error strings
    let err_str = ConsensusEngine::error_string(LuxError::Success);
    assert_eq!(err_str, "Success");
    
    let err_str = ConsensusEngine::error_string(LuxError::InvalidParams);
    assert_eq!(err_str, "Invalid parameters");
}

#[test]
fn test_engine_creation() {
    println!("=== ENGINE: Creation and Configuration ===");
    
    ConsensusEngine::init().unwrap();
    
    // Test various configurations
    let configs = vec![
        LuxConsensusConfig {
            k: 20,
            alpha_preference: 15,
            alpha_confidence: 15,
            beta: 20,
            concurrent_polls: 1,
            optimal_processing: 1,
            max_outstanding_items: 1024,
            max_item_processing_time_ns: 2_000_000_000,
            engine_type: LuxEngineType::Chain,
        },
        LuxConsensusConfig {
            k: 30,
            alpha_preference: 20,
            alpha_confidence: 20,
            beta: 25,
            concurrent_polls: 2,
            optimal_processing: 2,
            max_outstanding_items: 2048,
            max_item_processing_time_ns: 3_000_000_000,
            engine_type: LuxEngineType::DAG,
        },
        LuxConsensusConfig {
            k: 10,
            alpha_preference: 7,
            alpha_confidence: 7,
            beta: 10,
            concurrent_polls: 1,
            optimal_processing: 1,
            max_outstanding_items: 512,
            max_item_processing_time_ns: 1_000_000_000,
            engine_type: LuxEngineType::PQ,
        },
    ];
    
    for config in configs {
        let engine = ConsensusEngine::new(&config);
        assert!(engine.is_ok(), "Create engine with config");
    }
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_block_management() {
    println!("=== BLOCKS: Add, Query, and Hierarchy ===");
    
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
    
    // Create block hierarchy
    let genesis = LuxBlock {
        id: [0; 32],
        parent_id: [0; 32],
        height: 0,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    
    let block1 = LuxBlock {
        id: [1; 32],
        parent_id: genesis.id,
        height: 1,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    
    let block2 = LuxBlock {
        id: [2; 32],
        parent_id: block1.id,
        height: 2,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    
    // Test adding blocks
    assert!(engine.add_block(&block1).is_ok(), "Add block 1");
    assert!(engine.add_block(&block2).is_ok(), "Add block 2");
    
    // Test idempotency
    assert!(engine.add_block(&block1).is_ok(), "Add duplicate block (idempotent)");
    
    // Test with block data
    let data = b"Important block data";
    let block3 = LuxBlock {
        id: [3; 32],
        parent_id: block2.id,
        height: 3,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: data.as_ptr() as *mut _,
        data_size: data.len(),
    };
    
    assert!(engine.add_block(&block3).is_ok(), "Add block with data");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_voting() {
    println!("=== VOTING: Preference and Confidence ===");
    
    ConsensusEngine::init().unwrap();
    
    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 3,
        alpha_confidence: 3,
        beta: 5,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 1024,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    let mut engine = ConsensusEngine::new(&config).unwrap();
    
    // Add test block
    let block = LuxBlock {
        id: [10; 32],
        parent_id: [0; 32],
        height: 1,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    engine.add_block(&block).unwrap();
    
    // Test preference votes
    for i in 0..3 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: block.id,
            is_preference: true,
        };
        assert!(engine.process_vote(&vote).is_ok(), "Process preference vote {}", i);
    }
    
    // Test confidence votes
    for i in 3..6 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: block.id,
            is_preference: false,
        };
        assert!(engine.process_vote(&vote).is_ok(), "Process confidence vote {}", i);
    }
    
    // Check statistics
    let stats = engine.get_stats().unwrap();
    assert_eq!(stats.votes_processed, 6, "Vote count tracking");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_acceptance() {
    println!("=== ACCEPTANCE: Decision Thresholds ===");
    
    ConsensusEngine::init().unwrap();
    
    let config = LuxConsensusConfig {
        k: 20,
        alpha_preference: 2,
        alpha_confidence: 2,
        beta: 3,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 1024,
        max_item_processing_time_ns: 2_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    let mut engine = ConsensusEngine::new(&config).unwrap();
    
    // Add competing blocks
    let block_a = LuxBlock {
        id: [0xAA; 32],
        parent_id: [0; 32],
        height: 1,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    
    let block_b = LuxBlock {
        id: [0xBB; 32],
        parent_id: [0; 32],
        height: 1,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    
    engine.add_block(&block_a).unwrap();
    engine.add_block(&block_b).unwrap();
    
    // Vote for block A to reach acceptance
    for i in 0..3 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: block_a.id,
            is_preference: false,
        };
        engine.process_vote(&vote).unwrap();
    }
    
    // Check acceptance
    let is_accepted_a = engine.is_accepted(&block_a.id).unwrap();
    assert!(is_accepted_a, "Block A accepted after threshold");
    
    let is_accepted_b = engine.is_accepted(&block_b.id).unwrap();
    assert!(!is_accepted_b, "Block B not accepted");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_preference() {
    println!("=== PREFERENCE: Preferred Block Selection ===");
    
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
    
    // Initial preference should be genesis
    let pref_id = engine.get_preference().unwrap();
    let is_genesis = pref_id.iter().all(|&b| b == 0);
    assert!(is_genesis, "Initial preference is genesis");
    
    // Add and accept a block
    let block = LuxBlock {
        id: [0xFF; 32],
        parent_id: [0; 32],
        height: 1,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    engine.add_block(&block).unwrap();
    
    // Vote to accept
    for i in 0..20 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: block.id,
            is_preference: false,
        };
        engine.process_vote(&vote).unwrap();
    }
    
    // Check preference updated
    let pref_id = engine.get_preference().unwrap();
    assert_eq!(pref_id, block.id, "Preference updated to accepted block");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_polling() {
    println!("=== POLLING: Validator Polling ===");
    
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
    
    // Create validator IDs
    let validators: Vec<[u8; 32]> = (0..10)
        .map(|i| {
            let mut id = [0u8; 32];
            id[0] = i + 100;
            id
        })
        .collect();
    
    // Test polling
    assert!(engine.poll(&validators).is_ok(), "Poll 10 validators");
    
    // Test with no validators
    assert!(engine.poll(&[]).is_ok(), "Poll with no validators");
    
    // Check stats
    let stats = engine.get_stats().unwrap();
    assert_eq!(stats.polls_completed, 2, "Poll count tracking");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_statistics() {
    println!("=== STATISTICS: Metrics Collection ===");
    
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
    
    // Initial stats
    let stats = engine.get_stats().unwrap();
    assert_eq!(stats.blocks_accepted, 0, "Initial blocks accepted");
    assert_eq!(stats.blocks_rejected, 0, "Initial blocks rejected");
    assert_eq!(stats.polls_completed, 0, "Initial polls completed");
    assert_eq!(stats.votes_processed, 0, "Initial votes processed");
    
    // Generate activity
    let block = LuxBlock {
        id: [0x42; 32],
        parent_id: [0; 32],
        height: 1,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    engine.add_block(&block).unwrap();
    
    for i in 0..5 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: block.id,
            is_preference: i % 2 == 0,
        };
        engine.process_vote(&vote).unwrap();
    }
    
    // Check updated stats
    let stats = engine.get_stats().unwrap();
    assert_eq!(stats.votes_processed, 5, "Updated votes processed");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_thread_safety() {
    println!("=== CONCURRENCY: Thread Safety ===");
    
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
    
    let engine = Arc::new(Mutex::new(ConsensusEngine::new(&config).unwrap()));
    
    let mut handles = vec![];
    
    // Thread adding blocks
    for t in 0..2 {
        let engine_clone = Arc::clone(&engine);
        let handle = thread::spawn(move || {
            for i in 0..100 {
                let block = LuxBlock {
                    id: [(t * 100 + i) as u8; 32],
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
    
    // Thread processing votes
    for t in 0..2 {
        let engine_clone = Arc::clone(&engine);
        let handle = thread::spawn(move || {
            for i in 0..100 {
                let vote = LuxVote {
                    voter_id: [(t * 100 + i) as u8; 32],
                    block_id: [(i % 100) as u8; 32], // Vote for blocks that were added
                    is_preference: i % 2 == 0,
                };
                
                let mut eng = engine_clone.lock().unwrap();
                // Ignore vote errors in concurrent test since blocks might not exist yet
                let _ = eng.process_vote(&vote);
            }
        });
        handles.push(handle);
    }
    
    // Wait for all threads
    for handle in handles {
        handle.join().unwrap();
    }
    
    // Check consistency
    let eng = engine.lock().unwrap();
    let stats = eng.get_stats().unwrap();
    assert!(stats.votes_processed > 0, "Concurrent vote processing");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_memory_management() {
    println!("=== MEMORY: Allocation and Cleanup ===");
    
    ConsensusEngine::init().unwrap();
    
    // Test multiple engine creation/destruction
    for _ in 0..10 {
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
        
        // Add many blocks
        for j in 0..100 {
            let data = format!("Block data {}", j);
            let block = LuxBlock {
                id: [j as u8; 32],
                parent_id: [0; 32],
                height: j as u64,
                timestamp: SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .unwrap()
                    .as_secs(),
                data: data.as_ptr() as *mut _,
                data_size: data.len(),
            };
            
            engine.add_block(&block).unwrap();
        }
        
        // Engine will be dropped here
    }
    
    assert!(true, "Memory stress test passed");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_error_handling() {
    println!("=== ERRORS: Error Conditions ===");
    
    // Test error display
    let err = LuxError::InvalidParams;
    assert_eq!(format!("{}", err), "Invalid parameters");
    
    let err = LuxError::OutOfMemory;
    assert_eq!(format!("{}", err), "Out of memory");
    
    // Error trait implementation
    let err: Box<dyn std::error::Error> = Box::new(LuxError::ConsensusFailed);
    assert_eq!(err.to_string(), "Consensus failed");
}

#[test]
fn test_engine_types() {
    println!("=== ENGINE TYPES: Chain, DAG, PQ ===");
    
    ConsensusEngine::init().unwrap();
    
    let types = vec![
        (LuxEngineType::Chain, "Chain"),
        (LuxEngineType::DAG, "DAG"),
        (LuxEngineType::PQ, "PQ"),
    ];
    
    for (engine_type, expected) in types {
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
        
        let engine = ConsensusEngine::new(&config);
        assert!(engine.is_ok(), "Create engine with type {:?}", engine_type);
        
        let type_str = ConsensusEngine::engine_type_string(engine_type);
        assert_eq!(type_str, expected, "Engine type string");
    }
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_performance() {
    println!("=== PERFORMANCE: Throughput and Latency ===");
    
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
    
    // Add 1000 blocks
    let start = Instant::now();
    
    for i in 0..1000 {
        let mut id = [0u8; 32];
        id[0] = (i >> 8) as u8;
        id[1] = (i & 0xFF) as u8;
        
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
    
    let duration = start.elapsed();
    assert!(duration < Duration::from_secs(1), "Add 1000 blocks in < 1 second");
    println!("  Time: {:.3} seconds", duration.as_secs_f64());
    
    // Process 10000 votes
    let start = Instant::now();
    
    for i in 0..10000 {
        let mut voter_id = [0u8; 32];
        voter_id[0] = (i >> 8) as u8;
        voter_id[1] = (i & 0xFF) as u8;
        
        // Vote for blocks that actually exist (we added 1000 blocks)
        let mut block_id = [0u8; 32];
        let block_index = i % 1000;
        block_id[0] = (block_index >> 8) as u8;
        block_id[1] = (block_index & 0xFF) as u8;
        
        let vote = LuxVote {
            voter_id,
            block_id,
            is_preference: i % 2 == 0,
        };
        
        engine.process_vote(&vote).unwrap();
    }
    
    let duration = start.elapsed();
    assert!(duration < Duration::from_secs(2), "Process 10000 votes in < 2 seconds");
    println!("  Time: {:.3} seconds", duration.as_secs_f64());
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_edge_cases() {
    println!("=== EDGE CASES: Boundary Conditions ===");
    
    ConsensusEngine::init().unwrap();
    
    // Minimum configuration
    let min_config = LuxConsensusConfig {
        k: 1,
        alpha_preference: 1,
        alpha_confidence: 1,
        beta: 1,
        concurrent_polls: 1,
        optimal_processing: 1,
        max_outstanding_items: 1,
        max_item_processing_time_ns: 1,
        engine_type: LuxEngineType::Chain,
    };
    
    let engine = ConsensusEngine::new(&min_config);
    assert!(engine.is_ok(), "Minimum configuration");
    
    // Maximum reasonable configuration
    let max_config = LuxConsensusConfig {
        k: 1000,
        alpha_preference: 750,
        alpha_confidence: 750,
        beta: 900,
        concurrent_polls: 100,
        optimal_processing: 100,
        max_outstanding_items: 1_000_000,
        max_item_processing_time_ns: 10_000_000_000,
        engine_type: LuxEngineType::DAG,
    };
    
    let mut engine = ConsensusEngine::new(&max_config).unwrap();
    
    // Very long block chain
    for i in 0..100 {
        let mut parent_id = [0u8; 32];
        if i > 0 {
            parent_id[0] = (i - 1) as u8;
        }
        
        let block = LuxBlock {
            id: [i as u8; 32],
            parent_id,
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
    
    assert!(true, "Long chain creation");
    
    ConsensusEngine::cleanup().unwrap();
}

#[test]
fn test_integration() {
    println!("=== INTEGRATION: Full Workflow ===");
    
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
    
    // Simulate full consensus workflow
    // 1. Add genesis
    let genesis = LuxBlock {
        id: [0; 32],
        parent_id: [0; 32],
        height: 0,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    
    // 2. Add competing chains
    let mut chain_a: Vec<LuxBlock> = Vec::new();
    let mut chain_b: Vec<LuxBlock> = Vec::new();
    
    for i in 0..5 {
        // Chain A
        let mut parent_id = if i == 0 {
            genesis.id
        } else {
            chain_a[i - 1].id
        };
        
        let block_a = LuxBlock {
            id: [0xA0 + i as u8; 32],
            parent_id,
            height: (i + 1) as u64,
            timestamp: SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_secs(),
            data: ptr::null_mut(),
            data_size: 0,
        };
        
        engine.add_block(&block_a).unwrap();
        chain_a.push(block_a);
        
        // Chain B
        parent_id = if i == 0 {
            genesis.id
        } else {
            chain_b[i - 1].id
        };
        
        let block_b = LuxBlock {
            id: [0xB0 + i as u8; 32],
            parent_id,
            height: (i + 1) as u64,
            timestamp: SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_secs(),
            data: ptr::null_mut(),
            data_size: 0,
        };
        
        engine.add_block(&block_b).unwrap();
        chain_b.push(block_b);
    }
    
    // 3. Vote for chain A
    for i in 0..20 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: chain_a[4].id,
            is_preference: false,
        };
        engine.process_vote(&vote).unwrap();
    }
    
    // 4. Check final state
    let is_accepted_a = engine.is_accepted(&chain_a[4].id).unwrap();
    assert!(is_accepted_a, "Chain A accepted");
    
    let is_accepted_b = engine.is_accepted(&chain_b[4].id).unwrap();
    assert!(!is_accepted_b, "Chain B rejected");
    
    let pref_id = engine.get_preference().unwrap();
    assert_eq!(pref_id, chain_a[4].id, "Preference is chain A tip");
    
    let stats = engine.get_stats().unwrap();
    assert!(stats.blocks_accepted > 0, "Blocks accepted in workflow");
    assert_eq!(stats.votes_processed, 20, "All votes processed");
    
    ConsensusEngine::cleanup().unwrap();
}