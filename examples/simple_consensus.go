// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/luxfi/consensus" // Single clean import!
)

func main() {
	// Create engine with default config
	cfg := consensus.DefaultConfig()
	chain := consensus.NewChain(cfg)

	// Start the engine
	ctx := context.Background()
	if err := chain.Start(ctx); err != nil {
		panic(err)
	}
	defer chain.Stop()

	// Create a new block
	block := &consensus.Block{
		ID:       consensus.ID{1, 2, 3},
		ParentID: consensus.GenesisID,
		Height:   1,
		Time:     time.Now(),
		Payload:  []byte("Hello, Lux Consensus!"),
	}

	// Add the block
	if err := chain.Add(ctx, block); err != nil {
		panic(fmt.Sprintf("Failed to add block: %v", err))
	}
	fmt.Printf("Added block %x at height %d\n", block.ID, block.Height)

	// Simulate votes from validators
	validators := []consensus.NodeID{
		{1}, {2}, {3}, {4}, {5},
		{6}, {7}, {8}, {9}, {10},
		{11}, {12}, {13}, {14}, {15},
		{16}, {17}, {18}, {19}, {20},
	}

	// Vote on the block
	for i, validator := range validators {
		vote := consensus.NewVote(
			block.ID,
			consensus.VotePreference,
			validator,
		)
		vote.Signature = []byte(fmt.Sprintf("sig-%d", i))

		if err := chain.RecordVote(ctx, vote); err != nil {
			panic(fmt.Sprintf("Failed to record vote: %v", err))
		}
	}

	// Check if the block is accepted
	if chain.IsAccepted(block.ID) {
		fmt.Println("Block accepted! ✅")
		fmt.Printf("Status: %v\n", chain.GetStatus(block.ID))
	} else {
		fmt.Println("Block not yet accepted")
		fmt.Printf("Status: %v\n", chain.GetStatus(block.ID))
	}

	// Create another block using helper
	block2 := consensus.NewBlock(
		consensus.ID{4, 5, 6},
		block.ID,
		2,
		[]byte("Second block"),
	)
	block2.Time = time.Now()

	// Add and vote on the second block
	if err := chain.Add(ctx, block2); err != nil {
		panic(fmt.Sprintf("Failed to add block 2: %v", err))
	}

	// Vote with quorum
	for i := 0; i < cfg.Alpha; i++ {
		vote := consensus.NewVote(
			block2.ID,
			consensus.VoteCommit,
			validators[i],
		)
		vote.Signature = []byte(fmt.Sprintf("sig2-%d", i))

		if err := chain.RecordVote(ctx, vote); err != nil {
			panic(fmt.Sprintf("Failed to record vote: %v", err))
		}
	}

	// Both blocks should be accepted
	fmt.Printf("\nConsensus Results:\n")
	fmt.Printf("Block 1 accepted: %v\n", chain.IsAccepted(block.ID))
	fmt.Printf("Block 2 accepted: %v\n", chain.IsAccepted(block2.ID))
}

// Example of using QuickStart for even simpler initialization
func quickStartExample() {
	ctx := context.Background()

	// One-liner to start consensus
	chain, err := consensus.QuickStart(ctx)
	if err != nil {
		panic(err)
	}
	defer chain.Stop()

	// Ready to use!
	block := consensus.NewBlock(
		consensus.ID{1, 2, 3},
		consensus.GenesisID,
		1,
		[]byte("Quick start block"),
	)

	if err := chain.Add(ctx, block); err != nil {
		panic(err)
	}

	fmt.Println("QuickStart example complete!")
}
