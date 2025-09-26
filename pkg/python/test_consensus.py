#!/usr/bin/env python3
# Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
# See the file LICENSE for licensing terms.

import sys
import time
from lux_consensus import (
    ConsensusEngine, ConsensusConfig, Block, Vote, 
    EngineType, ConsensusError, engine_type_string
)

def test_initialization():
    """Test basic initialization"""
    print("=== Test: Initialization ===")
    
    config = ConsensusConfig(
        k=20,
        alpha_preference=15,
        alpha_confidence=15,
        beta=20,
        engine_type=EngineType.DAG
    )
    
    engine = ConsensusEngine(config)
    print("✅ Engine created successfully")
    
    # Engine will be automatically cleaned up when it goes out of scope
    return True

def test_block_operations():
    """Test block operations"""
    print("\n=== Test: Block Operations ===")
    
    config = ConsensusConfig()
    engine = ConsensusEngine(config)
    
    # Create blocks
    block1 = Block(
        block_id=b'\x01' * 32,
        parent_id=b'\x00' * 32,  # Genesis parent
        height=1,
        timestamp=int(time.time()),
        data=b"Block 1 data"
    )
    
    block2 = Block(
        block_id=b'\x02' * 32,
        parent_id=b'\x01' * 32,
        height=2
    )
    
    # Add blocks
    engine.add_block(block1)
    print(f"  Added block 1 (height: {block1.height})")
    
    engine.add_block(block2)
    print(f"  Added block 2 (height: {block2.height})")
    
    # Check acceptance
    is_accepted = engine.is_accepted(block1.id)
    print(f"  Block 1 accepted: {is_accepted}")
    
    print("✅ Block operations successful")
    return True

def test_voting():
    """Test voting functionality"""
    print("\n=== Test: Voting ===")
    
    config = ConsensusConfig(
        alpha_preference=2,
        alpha_confidence=2,
        beta=3
    )
    engine = ConsensusEngine(config)
    
    # Add a block
    block = Block(
        block_id=b'\x03' * 32,
        parent_id=b'\x00' * 32,
        height=1
    )
    engine.add_block(block)
    
    # Cast votes
    for i in range(3):
        vote = Vote(
            voter_id=bytes([i]) * 32,
            block_id=block.id,
            is_preference=False
        )
        engine.process_vote(vote)
    
    print(f"  Processed 3 votes for block")
    
    # Get stats
    stats = engine.get_stats()
    print(f"  Votes processed: {stats.votes_processed}")
    assert stats.votes_processed == 3
    
    print("✅ Voting successful")
    return True

def test_preference():
    """Test preference tracking"""
    print("\n=== Test: Preference ===")
    
    config = ConsensusConfig()
    engine = ConsensusEngine(config)
    
    # Get initial preference (should be genesis)
    pref_id = engine.get_preference()
    is_genesis = all(b == 0 for b in pref_id)
    print(f"  Initial preference is genesis: {is_genesis}")
    assert is_genesis
    
    print("✅ Preference tracking successful")
    return True

def test_polling():
    """Test validator polling"""
    print("\n=== Test: Polling ===")
    
    config = ConsensusConfig()
    engine = ConsensusEngine(config)
    
    # Create validator IDs
    validators = [
        bytes([10]) * 32,
        bytes([11]) * 32,
        bytes([12]) * 32,
        bytes([13]) * 32,
        bytes([14]) * 32,
    ]
    
    # Poll validators
    engine.poll(validators)
    print(f"  Polled {len(validators)} validators")
    
    # Check stats
    stats = engine.get_stats()
    print(f"  Polls completed: {stats.polls_completed}")
    assert stats.polls_completed == 1
    
    print("✅ Polling successful")
    return True

def test_statistics():
    """Test statistics collection"""
    print("\n=== Test: Statistics ===")
    
    config = ConsensusConfig()
    engine = ConsensusEngine(config)
    
    # Add some activity
    block = Block(
        block_id=b'\x04' * 32,
        parent_id=b'\x00' * 32,
        height=1
    )
    engine.add_block(block)
    
    vote = Vote(
        voter_id=b'\x05' * 32,
        block_id=block.id,
        is_preference=True
    )
    engine.process_vote(vote)
    
    # Get stats
    stats = engine.get_stats()
    print(f"  Stats: {stats}")
    
    assert stats.votes_processed == 1
    
    print("✅ Statistics collection successful")
    return True

def test_error_handling():
    """Test error handling"""
    print("\n=== Test: Error Handling ===")
    
    try:
        # Try to create block with invalid ID length
        block = Block(
            block_id=b'\x01' * 16,  # Wrong length
            parent_id=b'\x00' * 32,
            height=1
        )
        assert False, "Should have raised ValueError"
    except ValueError as e:
        print(f"  Caught expected error: {e}")
    
    print("✅ Error handling successful")
    return True

def test_engine_types():
    """Test different engine types"""
    print("\n=== Test: Engine Types ===")
    
    for engine_type in [EngineType.CHAIN, EngineType.DAG, EngineType.PQ]:
        config = ConsensusConfig(engine_type=engine_type)
        engine = ConsensusEngine(config)
        type_str = engine_type_string(engine_type)
        print(f"  Created {type_str} engine")
    
    print("✅ Engine types successful")
    return True

def main():
    """Run all tests"""
    print("=== Lux Consensus Python Tests ===")
    print("===================================")
    
    tests = [
        test_initialization,
        test_block_operations,
        test_voting,
        test_preference,
        test_polling,
        test_statistics,
        test_error_handling,
        test_engine_types,
    ]
    
    passed = 0
    failed = 0
    
    for test in tests:
        try:
            if test():
                passed += 1
            else:
                failed += 1
                print(f"❌ {test.__name__} failed")
        except Exception as e:
            failed += 1
            print(f"❌ {test.__name__} failed with exception: {e}")
    
    print("\n===================================")
    print(f"SUMMARY: {passed} passed, {failed} failed")
    
    if failed == 0:
        print("🎉 ALL TESTS PASSED!")
        return 0
    else:
        print("❌ SOME TESTS FAILED!")
        return 1

if __name__ == "__main__":
    sys.exit(main())