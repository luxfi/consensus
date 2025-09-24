// Package examples demonstrates integration between consensus and node packages
package examples

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/luxfi/consensus/engine/core"
	"github.com/luxfi/ids"
)

// Example block for demonstration
type ExampleBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	data      []byte
}

func (b *ExampleBlock) ID() ids.ID          { return b.id }
func (b *ExampleBlock) ParentID() ids.ID    { return b.parentID }
func (b *ExampleBlock) Height() uint64      { return b.height }
func (b *ExampleBlock) Timestamp() int64    { return b.timestamp }
func (b *ExampleBlock) Bytes() []byte       { return b.data }
func (b *ExampleBlock) Verify(context.Context) error  { return nil }
func (b *ExampleBlock) Accept(context.Context) error  { 
	fmt.Printf("Block %s accepted at height %d\n", b.id, b.height)
	return nil 
}
func (b *ExampleBlock) Reject(context.Context) error  { 
	fmt.Printf("Block %s rejected\n", b.id)
	return nil 
}

func RunNodeIntegrationExample() {
	// Configure consensus parameters for fast finality
	params := core.ConsensusParams{
		K:                     20,
		AlphaPreference:      15,
		AlphaConfidence:      15,
		Beta:                 20,
		ConcurrentPolls:      10,
		OptimalProcessing:    10,
		MaxOutstandingItems:  1000,
		MaxItemProcessingTime: 30 * time.Second,
	}

	// Create consensus engine
	consensus, err := core.NewCGOConsensus(params)
	if err != nil {
		log.Fatalf("Failed to create consensus: %v", err)
	}

	fmt.Println("✅ Consensus engine initialized")
	fmt.Printf("Parameters: K=%d, Alpha=%d, Beta=%d\n", 
		params.K, params.AlphaPreference, params.Beta)

	// Create a sample block
	block := &ExampleBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now().Unix(),
		data:      []byte("Genesis block"),
	}

	// Add block to consensus
	if err := consensus.Add(block); err != nil {
		log.Fatalf("Failed to add block: %v", err)
	}

	fmt.Printf("📦 Added block %s to consensus\n", block.ID())

	// Simulate voting from validators
	fmt.Println("🗳️  Simulating validator votes...")
	for i := 0; i < params.K; i++ {
		if err := consensus.RecordPoll(block.ID(), true); err != nil {
			log.Printf("Failed to record vote %d: %v", i, err)
			continue
		}
		
		// Check if consensus achieved
		if consensus.IsAccepted(block.ID()) {
			fmt.Printf("✅ Consensus achieved after %d votes!\n", i+1)
			break
		}
	}

	// Verify final state
	if consensus.IsAccepted(block.ID()) {
		fmt.Printf("🎉 Block %s has been accepted by consensus\n", block.ID())
		fmt.Printf("Current preference: %s\n", consensus.GetPreference())
	} else {
		fmt.Printf("❌ Block %s was not accepted\n", block.ID())
	}

	// Health check
	if err := consensus.HealthCheck(); err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Println("💚 Consensus engine health check passed")
	}

	fmt.Println("\n🏁 Node integration example completed successfully!")
}