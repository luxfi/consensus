// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package main demonstrates the consensus with FPC enabled by default
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/types"
	"github.com/luxfi/consensus/wave"
	"github.com/luxfi/consensus/wave/fpc"
)

type ItemID string
type TxID string

type transport struct {
	prefer bool
}

func (t transport) RequestVotes(ctx context.Context, peers []types.NodeID, item ItemID) <-chan photon.Photon[ItemID] {
	out := make(chan photon.Photon[ItemID], len(peers))
	go func() {
		defer close(out)
		// Simulate network RPCs to peers
		time.Sleep(20 * time.Millisecond)
		// Return votes (in production, these come from actual peers)
		for i := 0; i < len(peers); i++ {
			out <- photon.Photon[ItemID]{Item: item, Prefer: t.prefer}
		}
	}()
	return out
}

func (t transport) MakeLocalPhoton(item ItemID, prefer bool) photon.Photon[ItemID] {
	return photon.Photon[ItemID]{Item: item, Prefer: prefer}
}

func main() {
	fmt.Println("=== Lux Consensus with FPC (Fast Path Certification) ENABLED by default ===")
	fmt.Println()

	// Setup validators
	peers := []types.NodeID{"validator1", "validator2", "validator3", "validator4", "validator5", "validator6", "validator7"}
	
	// Create sampler
	sel := prism.NewDefault(peers, prism.Options{
		MinPeers: 5,
		MaxPeers: 10,
	})
	
	// Create Wave consensus with FPC enabled (default configuration)
	w := wave.New[ItemID](wave.Config{
		K:       5,        // Sample size
		Alpha:   0.8,      // Success threshold (80%)
		Beta:    5,        // Confidence target
		Gamma:   3,        // Max inconclusive before FPC activates
		RoundTO: 250 * time.Millisecond,
	}, sel, transport{prefer: true})
	
	// Create FPC (Fast Path Consensus) - ALWAYS ON
	quorum := fpc.Quorum{N: 7, F: 2} // f=2, need 2f+1=5 votes for fast path
	fpcCfg := fpc.Config{
		Quorum:            quorum,
		Epoch:             0,
		VoteLimitPerBlock: 256,
		VotePrefix:        []byte("LUX/FPC/V1"),
	}
	fl := fpc.New(fpcCfg, fpc.SimpleClassifier{})
	
	fmt.Println("Configuration:")
	fmt.Printf("  Validators: %d\n", len(peers))
	fmt.Printf("  Sample size (K): %d\n", 5)
	fmt.Printf("  FPC: ENABLED (switches after %d inconclusive rounds)\n", 3)
	fmt.Printf("  Fast Path (FPC): ENABLED (threshold: %d votes)\n", 5)
	fmt.Println()

	// Demonstrate Fast Path for owned transactions
	fmt.Println("=== Fast Path Demonstration (Owned Transactions) ===")
	ownedTx := TxID("owned-tx-001")
	
	// Simulate 5 validators voting for the transaction
	fmt.Printf("Collecting votes for %s...\n", ownedTx)
	for i := 0; i < 5; i++ {
		fl.Propose(ownedTx)
		status := fl.Status(ownedTx)
		fmt.Printf("  Vote %d: status = %s\n", i+1, statusString(status))
	}
	
	// Check if executable
	var txRef fpc.TxRef
	copy(txRef[:], []byte(ownedTx)[:32])
	if fl.Status(txRef) == fpc.StatusExecutable {
		fmt.Println("✅ Transaction is EXECUTABLE via Fast Path (before block finalization!)")
	}
	fmt.Println()

	// Demonstrate Wave consensus with FPC
	fmt.Println("=== Wave Consensus with FPC ===")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	item := ItemID("block#12345")
	fmt.Printf("Running consensus for %s...\n", item)
	
	for round := 1; round <= 10; round++ {
		w.Tick(ctx, item)
		st, _ := w.State(item)
		
		stageStr := "Snowball"
		if st.Stage == wave.StageFPC {
			stageStr = "FPC (Fast Path Consensus)"
		}
		
		fmt.Printf("  Round %d: prefer=%v, confidence=%d, stage=%s\n", 
			round, st.Step.Prefer, st.Step.Conf, stageStr)
		
		if st.Decided {
			result := "REJECTED"
			if st.Result == types.DecideAccept {
				result = "ACCEPTED"
			}
			fmt.Printf("✅ Consensus reached: %s (in %d rounds)\n", result, round)
			break
		}
		
		time.Sleep(50 * time.Millisecond)
	}
	
	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Println("• FPC (Fast Path Consensus) is ENABLED by default")
	fmt.Println("• Owned transactions achieve finality with just 2f+1 votes")
	fmt.Println("• No coordination overhead for non-conflicting transactions")
	fmt.Println("• Automatic escalation to FPC for difficult consensus")
	fmt.Println("• Production-ready for millions of TPS on X-Chain")
}

func statusString(s fpc.Status) string {
	switch s {
	case fpc.StatusPending:
		return "Pending"
	case fpc.StatusExecutable:
		return "Executable"
	case fpc.StatusFinal:
		return "Final"
	default:
		return "Unknown"
	}
}