#!/usr/bin/env python3
"""
Simple consensus example using the Lux Consensus Python SDK.

Shows how to:
- Create and start a consensus engine
- Add blocks
- Record votes
- Check consensus status

Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
"""

import lux_consensus as consensus  # Single clean import!


def main():
    """Main example demonstrating consensus operations"""
    
    # Create engine with default config
    config = consensus.default_config()
    chain = consensus.new_chain(config)
    
    # Start the engine
    chain.start()
    
    # Create a new block
    block = consensus.Block(
        id=consensus.ID(1, 2, 3),
        parent_id=consensus.GENESIS_ID,
        height=1,
        payload=b"Hello, Lux Consensus from Python!"
    )
    
    # Add the block
    chain.add(block)
    print(f"Added block {block.id} at height {block.height}")
    
    # Simulate votes from validators
    validators = [consensus.ID(i) for i in range(1, 21)]
    
    # Vote on the block
    for i, validator in enumerate(validators):
        vote = consensus.new_vote(
            block.id,
            consensus.VOTE_PREFERENCE,
            validator
        )
        vote.signature = f"sig-{i}".encode()
        
        chain.record_vote(vote)
    
    # Check if the block is accepted
    if chain.is_accepted(block.id):
        print("Block accepted! ✅")
        print(f"Status: {chain.get_status(block.id).name}")
    else:
        print("Block not yet accepted")
        print(f"Status: {chain.get_status(block.id).name}")
    
    # Create another block using helper
    block2 = consensus.new_block(
        consensus.ID(4, 5, 6),
        block.id,
        2,
        b"Second block from Python"
    )
    
    # Add and vote on the second block
    chain.add(block2)
    
    # Vote with quorum
    for i in range(config.alpha):
        vote = consensus.new_vote(
            block2.id,
            consensus.VOTE_COMMIT,
            validators[i]
        )
        vote.signature = f"sig2-{i}".encode()
        
        chain.record_vote(vote)
    
    # Both blocks should be accepted
    print("\nConsensus Results:")
    print(f"Block 1 accepted: {chain.is_accepted(block.id)}")
    print(f"Block 2 accepted: {chain.is_accepted(block2.id)}")
    
    # Stop the engine
    chain.stop()


def quick_start_example():
    """Example using the quick_start helper for even simpler initialization"""
    
    # One-liner to start consensus
    chain = consensus.quick_start()
    
    # Ready to use!
    block = consensus.new_block(
        consensus.ID(1, 2, 3),
        consensus.GENESIS_ID,
        1,
        b"Quick start block from Python"
    )
    
    chain.add(block)
    print("QuickStart example complete!")
    
    chain.stop()


def advanced_example():
    """Advanced example with custom configuration"""
    
    # Custom configuration
    config = consensus.Config(
        alpha=15,  # Lower quorum requirement
        k=15,
        max_outstanding=5,
        network_timeout=10.0,
        quantum_resistant=True,
        gpu_acceleration=True
    )
    
    chain = consensus.new_chain(config)
    chain.start()
    
    # Create a chain of blocks
    parent = consensus.GENESIS_ID
    for i in range(5):
        block = consensus.Block(
            id=consensus.ID(i, i, i),
            parent_id=parent,
            height=i + 1,
            payload=f"Block {i}".encode()
        )
        
        chain.add(block)
        
        # Vote on the block
        validators = [consensus.ID(v) for v in range(20)]
        for v in validators[:config.alpha]:
            vote = consensus.Vote(
                block_id=block.id,
                vote_type=consensus.VOTE_COMMIT,
                voter=v
            )
            chain.record_vote(vote)
        
        if chain.is_accepted(block.id):
            print(f"Block {i} accepted at height {block.height}")
            parent = block.id
    
    print(f"\nFinal chain height: {chain.height}")
    print(f"Last accepted block: {chain.last_accepted}")
    
    chain.stop()


if __name__ == "__main__":
    print("=== Simple Consensus Example ===")
    main()
    
    print("\n=== QuickStart Example ===")
    quick_start_example()
    
    print("\n=== Advanced Example ===")
    advanced_example()