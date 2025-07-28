// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/consensus/quasar"
	"github.com/luxfi/ids"
)

// IntegrationDemo shows how Nova DAG and Quasar work together
func main() {
	fmt.Println("🔗 Nova DAG + Quasar Integration Demo")
	fmt.Println("=====================================")
	fmt.Println()

	// Create Nova DAG engine
	novaParams := config.TestnetParameters
	novaEngine := engine.New(novaParams)

	// Create Quasar engine
	quasarParams := quasar.Parameters{
		K:                     11,
		AlphaPreference:       7,
		AlphaConfidence:       9,
		Beta:                  6,
		MaxItemProcessingTime: 6300 * time.Millisecond,
		QuasarTimeout:         500 * time.Millisecond,
		QuasarThreshold:       8,
	}
	nodeID := quasar.NodeID("integration-node")
	quasarEngine := quasar.NewEngine(quasarParams, nodeID)

	// Initialize Quasar
	ctx := context.Background()
	if err := quasarEngine.Initialize(ctx); err != nil {
		log.Fatal("Failed to initialize Quasar:", err)
	}

	// Set up finalization callback
	quasarEngine.SetFinalizedCallback(func(qblock quasar.QBlock) {
		fmt.Printf("\n🎯 Q-Block Finalized!\n")
		fmt.Printf("   Height: %d\n", qblock.Height)
		fmt.Printf("   Vertices: %d\n", len(qblock.VertexIDs))
		fmt.Printf("   BLS Cert: %x...\n", qblock.BLSCert[:16])
		fmt.Printf("   RT Cert: %x...\n", qblock.RTCert[:16])
		fmt.Println()
	})

	fmt.Println("📊 Configuration:")
	fmt.Printf("   Nova Parameters: K=%d, αp=%d, αc=%d, β=%d\n",
		novaParams.K, novaParams.AlphaPreference, 
		novaParams.AlphaConfidence, novaParams.Beta)
	fmt.Printf("   Quasar Threshold: %d/%d\n", 
		quasarParams.QuasarThreshold, quasarParams.K)
	fmt.Println()

	// Simulate integrated consensus
	simulateIntegratedConsensus(novaEngine, quasarEngine)

	fmt.Println("\n✨ Integration Demo Complete!")
}

func simulateIntegratedConsensus(nova *engine.Engine, quasarEng *quasar.Engine) {
	// Create initial choices for Nova DAG
	choices := make([]ids.ID, 5)
	for i := range choices {
		choices[i] = ids.GenerateTestID()
	}

	fmt.Println("🚀 Starting Integrated Consensus Simulation")
	fmt.Println()

	height := uint64(1)
	var qblockVertices []quasar.ID

	for round := 1; round <= 12; round++ {
		fmt.Printf("Round %d:\n", round)

		// Nova DAG: Vote on choices
		votes := make([]engine.Vote, 0)
		for i := 0; i < 11; i++ { // Simulate 11 validators
			vote := engine.Vote{
				NodeID:     ids.GenerateTestNodeID(),
				Preference: choices[i%len(choices)],
				Confidence: 1,
			}
			votes = append(votes, vote)
		}
		
		// Record votes in Nova engine
		nova.RecordPoll(votes)

		// Check if Nova has preference
		if nova.Preference() != ids.Empty {
			fmt.Printf("  Nova: Preference=%x, Confidence=%d\n", 
				nova.Preference(), nova.Confidence())
		}

		// Create vertices for this round
		vertex := &IntegrationVertex{
			id:        convertToQuasarID(ids.GenerateTestID()),
			height:    height,
			timestamp: time.Now(),
			novaChoice: nova.Preference(),
		}
		
		// Add vertex to Quasar
		ctx := context.Background()
		if err := quasarEng.AddVertex(ctx, vertex); err == nil {
			qblockVertices = append(qblockVertices, vertex.id)
			fmt.Printf("  Quasar: Added vertex %x\n", vertex.id[:8])
		}

		// Every 3 rounds, create a Q-block
		if round%3 == 0 && nova.Finalized() {
			fmt.Println("\n  🔐 Creating Q-Block with dual certificates...")
			
			qblock := quasar.QBlock{
				Height:    uint64(round / 3),
				VertexIDs: qblockVertices,
				QBlockID:  convertToQuasarID(ids.GenerateTestID()),
				BLSCert:   generateMockBLSCert(),
				RTCert:    generateMockRTCert(),
			}
			
			// Trigger finalization
			if callback := quasarEng.GetFinalizedCallback(); callback != nil {
				callback(qblock)
			}
			
			// Reset for next Q-block
			qblockVertices = nil
			height++
		}

		time.Sleep(200 * time.Millisecond)
	}
}

// IntegrationVertex bridges Nova and Quasar
type IntegrationVertex struct {
	id         quasar.ID
	height     uint64
	timestamp  time.Time
	novaChoice ids.ID
}

func (v *IntegrationVertex) ID() quasar.ID        { return v.id }
func (v *IntegrationVertex) Parents() []quasar.ID { return nil }
func (v *IntegrationVertex) Height() uint64       { return v.height }
func (v *IntegrationVertex) Timestamp() time.Time { return v.timestamp }
func (v *IntegrationVertex) Transactions() []quasar.ID { return nil }
func (v *IntegrationVertex) Verify(context.Context) error { return nil }
func (v *IntegrationVertex) Bytes() []byte { return v.id[:] }

// Helper functions
func convertToQuasarID(id ids.ID) quasar.ID {
	var qid quasar.ID
	copy(qid[:], id[:])
	return qid
}

func generateMockBLSCert() []byte {
	cert := make([]byte, 96)
	for i := range cert {
		cert[i] = byte(i % 256)
	}
	return cert
}

func generateMockRTCert() []byte {
	cert := make([]byte, 1024)
	for i := range cert {
		cert[i] = byte((i * 7) % 256)
	}
	return cert
}


// The GetFinalizedCallback method is already implemented in the quasar package.