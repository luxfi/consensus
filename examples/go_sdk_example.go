// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

//go:build skip
// +build skip

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/luxfi/consensus/engine/core"
	"github.com/luxfi/ids"
)

// SimpleBlock implements the Block interface for testing
type SimpleBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp time.Time
	data      []byte
}

func (b *SimpleBlock) ID() ids.ID           { return b.id }
func (b *SimpleBlock) Parent() ids.ID       { return b.parentID }
func (b *SimpleBlock) Height() uint64       { return b.height }
func (b *SimpleBlock) Timestamp() time.Time { return b.timestamp }
func (b *SimpleBlock) Bytes() []byte        { return b.data }

func main() {
	fmt.Println("=== Lux Consensus Go SDK Example ===")
	fmt.Println()

	// Check if we should use C implementation
	useCConsensus := os.Getenv("USE_C_CONSENSUS") == "1"
	if useCConsensus {
		fmt.Println("Using C consensus implementation (CGO)")
	} else {
		fmt.Println("Using pure Go consensus implementation")
	}
	fmt.Println()

	// Create consensus factory
	factory := core.NewConsensusFactory()

	// Set up consensus parameters
	params := core.Parameters{
		K:                     20,
		AlphaPreference:       15,
		AlphaConfidence:       15,
		Beta:                  20,
		ConcurrentPolls:       1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1024,
		MaxItemProcessingTime: 2 * time.Second,
	}

	fmt.Printf("Creating consensus engine with parameters:\n")
	fmt.Printf("  K: %d\n", params.K)
	fmt.Printf("  AlphaPreference: %d\n", params.AlphaPreference)
	fmt.Printf("  AlphaConfidence: %d\n", params.AlphaConfidence)
	fmt.Printf("  Beta: %d\n", params.Beta)
	fmt.Println()

	// Create consensus engine
	consensus, err := factory.CreateConsensus(params)
	if err != nil {
		log.Fatalf("Failed to create consensus engine: %v", err)
	}

	fmt.Println("✅ Consensus engine created successfully")
	fmt.Println()

	// Create some blocks
	fmt.Println("Adding blocks to consensus...")

	// Block 1
	block1 := &SimpleBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		data:      []byte("Block 1 data"),
	}
	
	if err := consensus.Add(block1); err != nil {
		log.Fatalf("Failed to add block 1: %v", err)
	}
	fmt.Printf("  Added block 1 (height: %d)\n", block1.Height())

	// Block 2
	block2 := &SimpleBlock{
		id:        ids.GenerateTestID(),
		parentID:  block1.ID(),
		height:    2,
		timestamp: time.Now(),
		data:      []byte("Block 2 data"),
	}
	
	if err := consensus.Add(block2); err != nil {
		log.Fatalf("Failed to add block 2: %v", err)
	}
	fmt.Printf("  Added block 2 (height: %d)\n", block2.Height())

	// Block 3 (competing with block 2)
	block3 := &SimpleBlock{
		id:        ids.GenerateTestID(),
		parentID:  block1.ID(),
		height:    2,
		timestamp: time.Now(),
		data:      []byte("Block 3 data"),
	}
	
	if err := consensus.Add(block3); err != nil {
		log.Fatalf("Failed to add block 3: %v", err)
	}
	fmt.Printf("  Added block 3 (height: %d, competing with block 2)\n", block3.Height())
	fmt.Println()

	// Simulate voting
	fmt.Println("Processing votes for block 2...")
	
	for i := 0; i < 20; i++ {
		voterID := ids.GenerateTestNodeID()
		isPreference := i < 15 // First 15 are preference votes
		
		if err := consensus.ProcessVote(voterID, block2.ID(), isPreference); err != nil {
			log.Fatalf("Failed to process vote: %v", err)
		}
	}
	
	fmt.Println("  Processed 20 votes (15 preference, 5 confidence)")
	fmt.Println()

	// Check block status
	fmt.Println("Checking block status:")
	block2Accepted := consensus.IsAccepted(block2.ID())
	block3Accepted := consensus.IsAccepted(block3.ID())
	fmt.Printf("  Block 2 accepted: %v\n", block2Accepted)
	fmt.Printf("  Block 3 accepted: %v\n", block3Accepted)
	fmt.Println()

	// Get preference
	preference := consensus.GetPreference()
	fmt.Printf("Current preferred block: %s\n", preference)
	fmt.Println()

	// Poll validators
	validators := []ids.NodeID{
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
	}
	
	if err := consensus.Poll(validators); err != nil {
		log.Fatalf("Failed to poll validators: %v", err)
	}
	fmt.Printf("Polled %d validators\n", len(validators))
	fmt.Println()

	// Get statistics
	stats, err := consensus.GetStats()
	if err != nil {
		log.Printf("Warning: Failed to get stats: %v", err)
	} else {
		fmt.Println("Consensus Statistics:")
		fmt.Printf("  Blocks accepted: %d\n", stats.BlocksAccepted)
		fmt.Printf("  Blocks rejected: %d\n", stats.BlocksRejected)
		fmt.Printf("  Polls completed: %d\n", stats.PollsCompleted)
		fmt.Printf("  Votes processed: %d\n", stats.VotesProcessed)
		fmt.Printf("  Average decision time: %.2fms\n", stats.AverageDecisionTime)
		fmt.Println()
	}

	// Cleanup
	fmt.Println("Cleaning up...")
	if cgoConsensus, ok := consensus.(*core.CGOConsensus); ok {
		if err := cgoConsensus.Destroy(); err != nil {
			log.Printf("Warning: Failed to destroy consensus engine: %v", err)
		}
	}

	fmt.Println("✅ Example completed successfully!")
	fmt.Println()
	
	// Instructions for running with different implementations
	fmt.Println("To run with different implementations:")
	fmt.Println("  Pure Go:  go run go_sdk_example.go")
	fmt.Println("  C (CGO):  USE_C_CONSENSUS=1 CGO_ENABLED=1 go run go_sdk_example.go")
}