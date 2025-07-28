// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/quantum"
	"github.com/luxfi/consensus/ringtail"
	"github.com/luxfi/ids"
)

// Example: Using the Post-Quantum (PQ) Engine
//
// This example demonstrates how the PQ engine provides dual-certificate
// finality with both BLS (classical) and Ringtail (quantum) security.

func main() {
	// 1. Configure consensus parameters
	params := config.Parameters{
		K:                     21,
		AlphaPreference:       15,
		AlphaConfidence:       18,
		Beta:                  8,
		MaxItemProcessingTime: 400 * time.Millisecond,
	}

	// 2. Create node ID and keys
	nodeID := ids.GenerateTestNodeID()
	
	// Load or generate keys
	rtKeyPair, err := ringtail.GetOrCreateKeyPair("~/.lux")
	if err != nil {
		log.Fatal(err)
	}
	
	// BLS key (placeholder)
	blsKey := []byte("bls-key-placeholder")

	// 3. Create validator set
	validators := &ExampleValidatorSet{
		validators: map[ids.NodeID]*ringtail.Validator{
			nodeID: {
				NodeID:    nodeID,
				BLSPubKey: blsKey,
				RTPubKey:  rtKeyPair.PublicKey,
				Weight:    1,
			},
		},
		quorum:    15,
		threshold: 15,
	}

	// 4. Create quantum engine
	engine := quantum.New(params, nodeID)
	
	// 5. Initialize with dual keys
	ctx := context.Background()
	if err := engine.Initialize(ctx, blsKey, rtKeyPair, validators); err != nil {
		log.Fatal(err)
	}

	// 6. Start Quasar service for RT precomputation
	network := &MockNetwork{
		shares: make(chan *ringtail.Share, 100),
		certs:  make(chan *ringtail.Certificate, 10),
	}
	
	service := ringtail.NewQuasarService(nodeID, rtKeyPair, validators, network)
	if err := service.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer service.Stop()

	// 7. Add items to consensus
	log.Println("Adding blocks to PQ consensus...")
	
	for i := 0; i < 3; i++ {
		itemID := ids.GenerateTestID()
		parentID := ids.Empty
		if i > 0 {
			parentID = engine.Decision()
		}
		
		data := []byte(fmt.Sprintf("Block %d", i))
		
		if err := engine.Add(ctx, itemID, parentID, uint64(i), data); err != nil {
			log.Fatal(err)
		}
		
		// Simulate voting
		votes := make([]ids.ID, params.K)
		for j := 0; j < params.K; j++ {
			votes[j] = itemID
		}
		
		// Record votes
		if err := engine.RecordPoll(votes); err != nil {
			log.Fatal(err)
		}
		
		// Check if finalized
		if engine.Finalized() {
			log.Printf("Block %d achieved dual-certificate finality!\n", i)
			
			// Get certificate bundle
			if bundle, ok := engine.GetCertBundle(uint64(i)); ok {
				log.Printf("  BLS signature: %x...\n", bundle.BLSAgg[:8])
				log.Printf("  RT certificate: %d bytes\n", len(bundle.RTCert))
			}
		}
		
		time.Sleep(100 * time.Millisecond)
	}

	// 8. Show engine stats
	stats := engine.Metrics()
	fmt.Println("\nPQ Engine Statistics:")
	for k, v := range stats {
		fmt.Printf("  %s: %d\n", k, v)
	}

	// 9. Show Quasar service stats
	serviceStats := service.GetStats()
	fmt.Println("\nQuasar Service Statistics:")
	for k, v := range serviceStats {
		fmt.Printf("  %s: %v\n", k, v)
	}

	fmt.Println("\nDual-certificate finality achieved! 🚀")
	fmt.Println("Both classical (BLS) and quantum (Ringtail) security active.")
}

// ExampleValidatorSet is a mock validator set.
type ExampleValidatorSet struct {
	validators map[ids.NodeID]*ringtail.Validator
	quorum     int
	threshold  int
}

func (vs *ExampleValidatorSet) GetValidator(id ids.NodeID) (*ringtail.Validator, error) {
	v, ok := vs.validators[id]
	if !ok {
		return nil, fmt.Errorf("validator not found")
	}
	return v, nil
}

func (vs *ExampleValidatorSet) GetQuorum() int {
	return vs.quorum
}

func (vs *ExampleValidatorSet) GetThreshold() int {
	return vs.threshold
}

// MockNetwork is a mock network implementation.
type MockNetwork struct {
	shares chan *ringtail.Share
	certs  chan *ringtail.Certificate
}

func (n *MockNetwork) BroadcastShare(share *ringtail.Share) error {
	select {
	case n.shares <- share:
		return nil
	default:
		return fmt.Errorf("share channel full")
	}
}

func (n *MockNetwork) SendCertificate(cert *ringtail.Certificate) error {
	select {
	case n.certs <- cert:
		return nil
	default:
		return fmt.Errorf("cert channel full")
	}
}

func (n *MockNetwork) SubscribeShares() <-chan *ringtail.Share {
	return n.shares
}

func (n *MockNetwork) SubscribeCertificates() <-chan *ringtail.Certificate {
	return n.certs
}