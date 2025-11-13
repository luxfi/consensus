// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//! Simple consensus example using the Lux Consensus Rust SDK.
//!
//! Shows how to:
//! - Create and start a consensus engine
//! - Add blocks
//! - Record votes
//! - Check consensus status

use lux_consensus::*;  // Single clean import!

fn main() -> Result<()> {
    println!("=== Simple Consensus Example ===");
    simple_example()?;
    
    println!("\n=== QuickStart Example ===");
    quick_start_example()?;
    
    println!("\n=== Advanced Example ===");
    advanced_example()?;
    
    Ok(())
}

fn simple_example() -> Result<()> {
    // Create engine with default config
    let config = Config::default();
    let mut chain = Chain::new(config.clone());
    
    // Start the engine
    chain.start()?;
    
    // Create a new block
    let block = Block::new(
        ID::from([1, 2, 3]),
        ID::new(vec![0; 32]),  // Genesis ID
        1,
        b"Hello, Lux Consensus from Rust!".to_vec(),
    );
    
    // Add the block
    chain.add(block.clone())?;
    println!("Added block {} at height {}", block.id, block.height);
    
    // Simulate votes from validators
    let validators: Vec<NodeID> = (1..=20)
        .map(|i| NodeID::from([i as u8, 0, 0]))
        .collect();
    
    // Vote on the block
    for (i, validator) in validators.iter().enumerate() {
        let vote = Vote::new(
            block.id.clone(),
            VoteType::Preference,
            validator.clone(),
        ).with_signature(format!("sig-{}", i).into_bytes());
        
        chain.record_vote(vote)?;
    }
    
    // Check if the block is accepted
    if chain.is_accepted(&block.id) {
        println!("Block accepted! ✅");
        println!("Status: {:?}", chain.get_status(&block.id));
    } else {
        println!("Block not yet accepted");
        println!("Status: {:?}", chain.get_status(&block.id));
    }
    
    // Create another block using helper
    let block2 = new_block(
        ID::from([4, 5, 6]),
        block.id.clone(),
        2,
        b"Second block from Rust".to_vec(),
    );
    
    // Add and vote on the second block
    chain.add(block2.clone())?;
    
    // Vote with quorum
    for i in 0..config.alpha {
        let vote = new_vote(
            block2.id.clone(),
            VoteType::Commit,
            validators[i].clone(),
        ).with_signature(format!("sig2-{}", i).into_bytes());
        
        chain.record_vote(vote)?;
    }
    
    // Both blocks should be accepted
    println!("\nConsensus Results:");
    println!("Block 1 accepted: {}", chain.is_accepted(&block.id));
    println!("Block 2 accepted: {}", chain.is_accepted(&block2.id));
    
    chain.stop()?;
    Ok(())
}

fn quick_start_example() -> Result<()> {
    // One-liner to start consensus
    let mut chain = quick_start()?;
    
    // Ready to use!
    let block = new_block(
        ID::from([1, 2, 3]),
        ID::new(vec![0; 32]),
        1,
        b"Quick start block from Rust".to_vec(),
    );
    
    chain.add(block)?;
    println!("QuickStart example complete!");
    
    chain.stop()?;
    Ok(())
}

fn advanced_example() -> Result<()> {
    // Custom configuration
    let config = Config {
        alpha: 15,  // Lower quorum requirement
        k: 15,
        max_outstanding: 5,
        quantum_resistant: true,
        gpu_acceleration: true,
        ..Config::default()
    };
    
    let mut chain = Chain::new(config.clone());
    chain.start()?;
    
    // Create a chain of blocks
    let mut parent = ID::new(vec![0; 32]);
    for i in 0..5 {
        let block = Block::new(
            ID::from([i as u8, i as u8, i as u8]),
            parent.clone(),
            (i + 1) as u64,
            format!("Block {}", i).into_bytes(),
        );
        
        chain.add(block.clone())?;
        
        // Vote on the block
        let validators: Vec<NodeID> = (0..20)
            .map(|v| NodeID::from([v as u8, 0, 0]))
            .collect();
            
        for v in validators.iter().take(config.alpha) {
            let vote = Vote::new(
                block.id.clone(),
                VoteType::Commit,
                v.clone(),
            );
            chain.record_vote(vote)?;
        }
        
        if chain.is_accepted(&block.id) {
            println!("Block {} accepted at height {}", i, block.height);
            parent = block.id;
        }
    }
    
    println!("\nAdvanced example complete!");
    
    chain.stop()?;
    Ok(())
}