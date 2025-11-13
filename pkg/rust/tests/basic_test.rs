// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

use lux_consensus::*;

#[test]
fn test_initialization() {
    let config = Config::default();
    let mut chain = Chain::new(config);
    assert!(chain.start().is_ok());
    chain.stop();
}

#[test]
fn test_block_operations() {
    let config = Config::default();
    let mut chain = Chain::new(config);
    chain.start().unwrap();

    // Create and add a block
    let block = Block::new(
        ID::from([1, 2, 3]),
        ID::from([0; 32]), // Genesis
        1,
        b"Test Block".to_vec(),
    );
    
    assert!(chain.add(block.clone()).is_ok());
    
    // Check status
    let status = chain.get_status(&block.id);
    assert_eq!(status, Status::Processing);
    
    chain.stop();
}

#[test]
fn test_consensus_flow() {
    let mut config = Config::default();
    config.k = 3;
    config.alpha = 2;  // Need 2 votes for acceptance
    
    let mut chain = Chain::new(config);
    chain.start().unwrap();
    
    // Create and add a block
    let block = Block::new(
        ID::from([1, 2, 3]),
        ID::from([0; 32]),
        1,
        b"Test Block".to_vec(),
    );
    chain.add(block.clone()).unwrap();
    
    // Record votes
    for i in 0..3 {
        let vote = Vote::new(
            block.id.clone(),
            VoteType::Commit,
            NodeID::from([i as u8, 0, 0]),
        );
        chain.record_vote(vote).unwrap();
    }
    
    // Check if accepted (with sufficient votes it should be)
    assert!(chain.is_accepted(&block.id));
    
    chain.stop();
}

#[test]
fn test_error_handling() {
    let config = Config::default();
    let mut chain = Chain::new(config);
    chain.start().unwrap();
    
    // Try to get non-existent block status (should return Unknown)
    let non_existent = ID::from([255, 255, 255]);
    let status = chain.get_status(&non_existent);
    assert_eq!(status, Status::Unknown);
    
    // Try to vote for non-existent block (should error)
    let vote = Vote::new(
        non_existent,
        VoteType::Commit,
        NodeID::from([1, 2, 3]),
    );
    assert!(chain.record_vote(vote).is_err());
    
    chain.stop();
}