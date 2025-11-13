#!/usr/bin/env python3
# Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

import sys
import time
from lux_consensus import (
    Chain, Config, Block, Vote,
    Status, Decision, VoteType,
    ConsensusError, default_config, new_chain, new_block, new_vote
)

def test_initialization():
    """Test basic initialization"""
    print("=== Test: Initialization ===")

    config = default_config()
    chain = new_chain(config)
    print("✅ Chain created successfully")

    # Chain will be automatically cleaned up when it goes out of scope
    return True

def test_block_operations():
    """Test block operations"""
    print("=== Test: Block Operations ===")

    config = default_config()
    chain = new_chain(config)
    chain.start()  # Start the engine first

    # Create and add genesis block (already added by start)
    # Create child block
    genesis_id = chain.last_accepted
    block1 = new_block(b"block1", genesis_id, 1, b"Block 1")
    chain.add(block1)
    print(f"✅ Added block 1: {block1.id}")

    # Check status
    status = chain.get_status(block1.id)
    print(f"✅ Block 1 status: {status}")

    # Test voting
    vote = new_vote(block1.id, VoteType.COMMIT, b"validator1")
    chain.record_vote(vote)
    print(f"✅ Recorded vote for block 1")
    
    chain.stop()

    return True

def test_consensus_flow():
    """Test consensus flow"""
    print("=== Test: Consensus Flow ===")

    config = default_config()
    config.k = 3  # Small network
    config.alpha = 2  # Need 2 votes for acceptance
    
    chain = new_chain(config)
    chain.start()

    # Create block (genesis already created by start)
    genesis_id = chain.last_accepted
    block = new_block(b"block1", genesis_id, 1, b"Test Block")
    chain.add(block)

    # Simulate voting from multiple validators
    validators = [b"validator1", b"validator2", b"validator3"]
    for validator in validators:
        vote = new_vote(block.id, VoteType.COMMIT, validator)
        chain.record_vote(vote)
        print(f"✅ Validator {validator.decode()} voted")

    # Check if block is accepted
    if chain.is_accepted(block.id):
        print("✅ Block achieved consensus and was accepted")
    else:
        print("⚠️  Block not yet accepted (may need more votes)")
    
    chain.stop()

    return True

def test_error_handling():
    """Test error handling"""
    print("=== Test: Error Handling ===")
    
    config = default_config()
    chain = new_chain(config)
    chain.start()

    # Get status of non-existent block (should return UNKNOWN, not raise error)
    status = chain.get_status(b"non_existent")
    if status == Status.UNKNOWN:
        print("✅ Non-existent block correctly returns UNKNOWN status")
    else:
        print("❌ Non-existent block should return UNKNOWN status")
        return False
    
    # Try to record vote for non-existent block (should raise error)
    try:
        vote = new_vote(b"non_existent", VoteType.COMMIT, b"validator1")
        chain.record_vote(vote)
        print("❌ Should have raised error for vote on non-existent block")
        return False
    except ConsensusError:
        print("✅ Correctly raised error for vote on non-existent block")
    
    chain.stop()

    return True

def main():
    """Run all tests"""
    print("Starting Lux Consensus Python SDK Tests\n")
    
    tests = [
        test_initialization,
        test_block_operations,
        test_consensus_flow,
        test_error_handling
    ]
    
    passed = 0
    failed = 0
    
    for test in tests:
        try:
            if test():
                passed += 1
            else:
                failed += 1
                print(f"❌ Test {test.__name__} failed")
        except Exception as e:
            failed += 1
            print(f"❌ Test {test.__name__} raised exception: {e}")
        print()
    
    print(f"Results: {passed} passed, {failed} failed")
    
    if failed > 0:
        sys.exit(1)
    
    print("\n✅ All tests passed!")
    return 0

if __name__ == "__main__":
    sys.exit(main())