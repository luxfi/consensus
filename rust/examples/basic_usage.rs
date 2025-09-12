// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

use lux_consensus::{ConsensusEngine, LuxBlock, LuxConsensusConfig, LuxEngineType, LuxVote};
use std::ptr;
use std::time::{SystemTime, UNIX_EPOCH};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("=== Lux Consensus Rust Example ===\n");
    
    // Initialize the consensus library
    println!("Initializing consensus library...");
    ConsensusEngine::init()?;
    
    // Create configuration
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
    
    println!("Creating consensus engine with config:");
    println!("  k: {}", config.k);
    println!("  alpha_preference: {}", config.alpha_preference);
    println!("  alpha_confidence: {}", config.alpha_confidence);
    println!("  beta: {}", config.beta);
    println!("  engine_type: {}\n", 
        ConsensusEngine::engine_type_string(config.engine_type));
    
    // Create consensus engine
    let mut engine = ConsensusEngine::new(&config)?;
    println!("✅ Consensus engine created successfully\n");
    
    // Create and add some blocks
    println!("Adding blocks to consensus...");
    
    // Block 1
    let block1 = LuxBlock {
        id: [1; 32],
        parent_id: [0; 32], // Genesis parent
        height: 1,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)?
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    engine.add_block(&block1)?;
    println!("  Added block 1 (height: {})", block1.height);
    
    // Block 2
    let block2 = LuxBlock {
        id: [2; 32],
        parent_id: block1.id,
        height: 2,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)?
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    engine.add_block(&block2)?;
    println!("  Added block 2 (height: {})", block2.height);
    
    // Block 3 (competing with block 2)
    let block3 = LuxBlock {
        id: [3; 32],
        parent_id: block1.id,
        height: 2,
        timestamp: SystemTime::now()
            .duration_since(UNIX_EPOCH)?
            .as_secs(),
        data: ptr::null_mut(),
        data_size: 0,
    };
    engine.add_block(&block3)?;
    println!("  Added block 3 (height: {}, competing with block 2)\n", block3.height);
    
    // Simulate voting
    println!("Processing votes for block 2...");
    for i in 0..20 {
        let vote = LuxVote {
            voter_id: [i; 32],
            block_id: block2.id,
            is_preference: i < 15, // First 15 are preference votes
        };
        engine.process_vote(&vote)?;
    }
    println!("  Processed 20 votes (15 preference, 5 confidence)\n");
    
    // Check block status
    println!("Checking block status:");
    let block2_accepted = engine.is_accepted(&block2.id)?;
    let block3_accepted = engine.is_accepted(&block3.id)?;
    println!("  Block 2 accepted: {}", block2_accepted);
    println!("  Block 3 accepted: {}\n", block3_accepted);
    
    // Get preference
    let preference = engine.get_preference()?;
    println!("Current preferred block:");
    print!("  ID: ");
    for byte in &preference[..8] {
        print!("{:02x}", byte);
    }
    println!("...\n");
    
    // Poll validators
    let validators = vec![
        [10; 32],
        [11; 32],
        [12; 32],
        [13; 32],
        [14; 32],
    ];
    engine.poll(&validators)?;
    println!("Polled {} validators\n", validators.len());
    
    // Get statistics
    let stats = engine.get_stats()?;
    println!("Consensus Statistics:");
    println!("  Blocks accepted: {}", stats.blocks_accepted);
    println!("  Blocks rejected: {}", stats.blocks_rejected);
    println!("  Polls completed: {}", stats.polls_completed);
    println!("  Votes processed: {}", stats.votes_processed);
    println!("  Average decision time: {:.2}ms\n", stats.average_decision_time_ms);
    
    // Cleanup
    println!("Cleaning up...");
    drop(engine);
    ConsensusEngine::cleanup()?;
    
    println!("✅ Example completed successfully!");
    
    Ok(())
}