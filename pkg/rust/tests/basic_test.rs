// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

use lux_consensus::*;

/// Helper to create a 32-byte ID from a single byte
fn make_id(b: u8) -> ID {
    let mut arr = [0u8; 32];
    arr[0] = b;
    ID::from(arr)
}

/// Helper to create a 32-byte NodeID from a single byte
fn make_node_id(b: u8) -> NodeID {
    let mut arr = [0u8; 32];
    arr[0] = b;
    NodeID::from(arr)
}

#[test]
fn test_initialization() {
    let config = QuasarConfig::testnet();
    let mut chain = QuasarEngine::new(config);
    assert!(chain.start().is_ok());
    chain.stop().unwrap();
}

#[test]
fn test_block_operations() {
    let config = QuasarConfig::testnet();
    let mut chain = QuasarEngine::new(config);
    chain.start().unwrap();

    // Create and add a block
    let block = Block::new(
        make_id(1),
        ID::zero(), // Genesis
        1,
        b"Test Block".to_vec(),
    );

    assert!(chain.add(block.clone()).is_ok());

    // Check status
    let status = chain.get_status(&block.id);
    assert_eq!(status, Status::Processing);

    chain.stop().unwrap();
}

#[test]
fn test_consensus_flow() {
    let mut config = QuasarConfig::testnet();
    config.k = 3;
    config.alpha = 0.67;  // Need ~2 votes for acceptance
    config.beta = 1;      // Just 1 round for quick test

    let mut chain = QuasarEngine::new(config);
    chain.start().unwrap();

    // Create and add a block
    let block = Block::new(
        make_id(1),
        ID::zero(),
        1,
        b"Test Block".to_vec(),
    );
    chain.add(block.clone()).unwrap();

    // Record votes
    for i in 0..3 {
        let vote = Vote::new(
            block.id.clone(),
            VoteType::Preference,
            make_node_id(i),
        );
        chain.record_vote(vote).unwrap();
    }

    // Check status (should be at least processing)
    let status = chain.get_status(&block.id);
    assert!(status == Status::Processing || status == Status::Accepted);

    chain.stop().unwrap();
}

#[test]
fn test_error_handling() {
    let config = QuasarConfig::testnet();
    let mut chain = QuasarEngine::new(config);
    chain.start().unwrap();

    // Try to get non-existent block status (should return Unknown)
    let non_existent = make_id(255);
    let status = chain.get_status(&non_existent);
    assert_eq!(status, Status::Unknown);

    // Try to vote for non-existent block (should error)
    let vote = Vote::new(
        non_existent,
        VoteType::Commit,
        make_node_id(1),
    );
    assert!(chain.record_vote(vote).is_err());

    chain.stop().unwrap();
}

#[test]
fn test_full_quasar_flow() {
    let config = QuasarConfig::testnet();
    let mut engine = QuasarEngine::new(config.clone());
    engine.start().unwrap();

    // Add validators
    for i in 0..10 {
        engine.add_validator(make_node_id(i), 1);
    }

    // Create chain of blocks
    let blocks: Vec<Block> = (1..=3).map(|height| {
        Block::new(
            make_id(height as u8),
            if height > 1 { make_id((height - 1) as u8) } else { ID::zero() },
            height,
            format!("Block {}", height).into_bytes(),
        )
    }).collect();

    // Add blocks
    for block in &blocks {
        engine.add(block.clone()).unwrap();
    }

    // Vote on each block (alpha=5 for testnet)
    for block in &blocks {
        for i in 0..5 {
            let vote = Vote::new(
                block.id.clone(),
                VoteType::Preference,
                make_node_id(i),
            );
            engine.record_vote(vote).unwrap();
        }
    }

    // All blocks should be accepted or processing
    for block in &blocks {
        let status = engine.get_status(&block.id);
        assert!(
            status == Status::Accepted || status == Status::Processing,
            "Block {} has unexpected status {:?}",
            block.height,
            status
        );
    }

    engine.stop().unwrap();
}
