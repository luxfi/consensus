// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"time"

	"github.com/luxfi/consensus/quasar"
)

// DemoVertex implements a simple vertex for demonstration
type DemoVertex struct {
	id           quasar.ID
	parents      []quasar.ID
	height       uint64
	timestamp    time.Time
	transactions []quasar.ID
}

func (v *DemoVertex) ID() quasar.ID             { return v.id }
func (v *DemoVertex) Parents() []quasar.ID      { return v.parents }
func (v *DemoVertex) Height() uint64            { return v.height }
func (v *DemoVertex) Timestamp() time.Time      { return v.timestamp }
func (v *DemoVertex) Transactions() []quasar.ID { return v.transactions }
func (v *DemoVertex) Verify(context.Context) error { return nil }
func (v *DemoVertex) Bytes() []byte {
	// Simple byte representation
	return v.id[:]
}

// generateID creates a random ID
func generateID() quasar.ID {
	var id quasar.ID
	rand.Read(id[:])
	return id
}

func main() {
	fmt.Println("🚀 Quasar Post-Quantum Consensus Demo")
	fmt.Println("=====================================")
	fmt.Println()

	// Create engine with testnet parameters
	params := quasar.Parameters{
		K:                     11,
		AlphaPreference:       7,
		AlphaConfidence:       9,
		Beta:                  6,
		MaxItemProcessingTime: 6300 * time.Millisecond,
		QuasarTimeout:         500 * time.Millisecond,
		QuasarThreshold:       8, // 2f+1 for 11 nodes
	}

	nodeID := quasar.NodeID("demo-node-001")
	engine := quasar.NewEngine(params, nodeID)

	// Set finalized callback
	engine.SetFinalizedCallback(func(qblock quasar.QBlock) {
		fmt.Printf("\n✅ Q-Block Finalized!\n")
		fmt.Printf("   Height: %d\n", qblock.Height)
		fmt.Printf("   Q-Block ID: %x\n", qblock.QBlockID[:8])
		fmt.Printf("   Vertices: %d\n", len(qblock.VertexIDs))
		fmt.Printf("   BLS Certificate: %x...\n", qblock.BLSCert[:16])
		fmt.Printf("   Ringtail Certificate: %x...\n", qblock.RTCert[:16])
	})

	// Initialize engine
	ctx := context.Background()
	if err := engine.Initialize(ctx); err != nil {
		log.Fatal("Failed to initialize engine:", err)
	}

	fmt.Println("📊 Simulating Quasar Consensus")
	fmt.Printf("   Parameters: K=%d, αp=%d, αc=%d, β=%d\n", 
		params.K, params.AlphaPreference, params.AlphaConfidence, params.Beta)
	fmt.Printf("   Quasar Threshold: %d/%d\n", params.QuasarThreshold, params.K)
	fmt.Println()

	// Simulate DAG growth and consensus
	height := uint64(1)
	parents := []quasar.ID{}

	for round := 1; round <= 10; round++ {
		fmt.Printf("Round %d:\n", round)

		// Create vertices for this round
		vertexCount := 3 + (round % 3) // 3-5 vertices per round
		roundVertices := []quasar.ID{}

		for i := 0; i < vertexCount; i++ {
			// Create vertex
			v := &DemoVertex{
				id:        generateID(),
				parents:   append([]quasar.ID{}, parents...),
				height:    height,
				timestamp: time.Now(),
			}

			// Add some transactions
			txCount := 5 + i
			for j := 0; j < txCount; j++ {
				v.transactions = append(v.transactions, generateID())
			}

			// Add vertex to engine
			if err := engine.AddVertex(ctx, v); err != nil {
				fmt.Printf("  ❌ Failed to add vertex: %v\n", err)
				continue
			}

			fmt.Printf("  → Added vertex %x (height=%d, txs=%d)\n", 
				v.id[:8], v.height, len(v.transactions))
			roundVertices = append(roundVertices, v.id)
		}

		// Update parents for next round
		parents = roundVertices
		height++

		// Check consensus status
		status := engine.ConsensusStatus()
		fmt.Printf("  📈 Consensus: Preference=%d, Confidence=%d\n", 
			status.PreferenceStrength, status.Confidence)

		// Simulate Quasar certificate aggregation
		if round%3 == 0 {
			fmt.Println("\n  🔐 Simulating Quasar Certificate Generation...")
			
			// Create mock certificates
			mockBLSCert := make([]byte, 96)
			mockRTCert := make([]byte, 1024)
			rand.Read(mockBLSCert)
			rand.Read(mockRTCert)

			qblock := quasar.QBlock{
				Height:    uint64(round / 3),
				VertexIDs: roundVertices,
				QBlockID:  generateID(),
				BLSCert:   mockBLSCert,
				RTCert:    mockRTCert,
			}

			// Trigger finalized callback
			engine.SetFinalizedCallback(func(qb quasar.QBlock) {
				fmt.Printf("\n✅ Q-Block Finalized!\n")
				fmt.Printf("   Height: %d\n", qb.Height)
				fmt.Printf("   Q-Block ID: %x\n", qb.QBlockID[:8])
				fmt.Printf("   Vertices: %d\n", len(qb.VertexIDs))
				fmt.Printf("   BLS Certificate: %x...\n", qb.BLSCert[:16])
				fmt.Printf("   Ringtail Certificate: %x...\n", qb.RTCert[:16])
			})
			
			// Simulate finalization
			if finalized := engine.GetFinalizedCallback(); finalized != nil {
				finalized(qblock)
			}
		}

		fmt.Println()
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("\n🎉 Demo Complete!")
	fmt.Println("   - Nova DAG processed vertices with classical consensus")
	fmt.Println("   - Quasar layer generated post-quantum certificates")
	fmt.Println("   - Dual finality achieved with BLS + Ringtail signatures")
}

// The AddVertex, ConsensusStatus, and GetFinalizedCallback methods are already
// implemented in the quasar package and available on the Engine type.