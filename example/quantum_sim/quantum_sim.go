// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/quorum"
	"github.com/luxfi/ids"
)

// Simulate a simple consensus scenario
func SimulateConsensus() {
	// Use testnet parameters
	params := config.TestnetParameters
	
	// Create two choices
	red := ids.GenerateTestID()
	blue := ids.GenerateTestID()
	
	// Create a binary threshold instance starting with red preference
	bt := quorum.NewBinaryThreshold()
	
	// Initialize with parameters
	paramsValue := quorum.BinaryThresholdParameters{
		AlphaPreference: params.AlphaPreference,
		AlphaConfidence: params.AlphaConfidence,
		Beta:            params.Beta,
	}
	bt.Initialize(&paramsValue, red)
	
	// Simulate rounds of voting
	rng := rand.New(rand.NewSource(42))
	
	fmt.Println("Starting Binary Threshold consensus simulation")
	fmt.Printf("Parameters: K=%d, αp=%d, αc=%d, β=%d\n", 
		params.K, params.AlphaPreference, params.AlphaConfidence, params.Beta)
	fmt.Printf("Choices: Red=%s, Blue=%s\n\n", red, blue)
	
	round := 0
	consecutiveSuccesses := 0
	currentPreference := red
	
	for consecutiveSuccesses < params.Beta && round < 50 {
		round++
		
		// Simulate votes from K validators
		votes := make(map[ids.ID]int)
		
		// Simulate different scenarios:
		// - 60% prefer blue after round 5
		// - Some Byzantine behavior (20%)
		for i := 0; i < params.K; i++ {
			r := rng.Float64()
			
			if round < 5 {
				// Initially split 50-50
				if r < 0.5 {
					votes[red]++
				} else {
					votes[blue]++
				}
			} else {
				// After round 5, blue becomes more popular
				if r < 0.2 {
					// Byzantine - vote randomly
					if rng.Float64() < 0.5 {
						votes[red]++
					} else {
						votes[blue]++
					}
				} else if r < 0.6 {
					votes[blue]++
				} else {
					votes[red]++
				}
			}
		}
		
		// Determine which choice won the poll
		winner := red
		if votes[blue] > votes[red] {
			winner = blue
		}
		
		// Update preference if needed
		if winner != currentPreference && votes[winner] >= params.AlphaPreference {
			currentPreference = winner
			consecutiveSuccesses = 0
		}
		
		// Check if we meet confidence threshold
		if votes[currentPreference] >= params.AlphaConfidence {
			consecutiveSuccesses++
			bt.RecordPoll(votes[currentPreference])
		} else {
			consecutiveSuccesses = 0
			bt.RecordPoll(0)
		}
		
		fmt.Printf("Round %2d: Red=%2d, Blue=%2d | Preference=%s | Consecutive=%d/%d\n",
			round, votes[red], votes[blue], 
			currentPreference, consecutiveSuccesses, params.Beta)
	}
	
	if consecutiveSuccesses >= params.Beta {
		fmt.Printf("\n✅ Consensus reached! Final choice: %s\n", currentPreference)
	} else {
		fmt.Printf("\n❌ No consensus after %d rounds\n", round)
	}
}

// Simulate network partitions
func SimulatePartition() {
	fmt.Println("\n\nSimulating Network Partition Scenario")
	fmt.Println("=====================================")
	
	params := config.LocalParameters
	
	// Create choices
	choiceA := ids.GenerateTestID()
	choiceB := ids.GenerateTestID()
	
	// Create two groups of validators
	group1 := quorum.NewBinaryThreshold()
	paramsValue := quorum.BinaryThresholdParameters{
		AlphaPreference: params.AlphaPreference,
		AlphaConfidence: params.AlphaConfidence,
		Beta:            params.Beta,
	}
	group1.Initialize(&paramsValue, choiceA)
	
	group2 := quorum.NewBinaryThreshold()
	group2.Initialize(&paramsValue, choiceB)
	
	fmt.Printf("Group 1 starts preferring A, Group 2 starts preferring B\n")
	fmt.Printf("K=%d validators total, split into two partitions\n\n", params.K)
	
	// Simulate each group voting internally
	for round := 1; round <= 10; round++ {
		// Group 1 mostly votes for A
		votesG1 := map[ids.ID]int{
			choiceA: params.K * 4 / 5,
			choiceB: params.K * 1 / 5,
		}
		
		// Group 2 mostly votes for B  
		votesG2 := map[ids.ID]int{
			choiceA: params.K * 1 / 5,
			choiceB: params.K * 4 / 5,
		}
		
		// Record polls for each group
		if votesG1[choiceA] >= params.AlphaConfidence {
			group1.RecordPoll(votesG1[choiceA])
		}
		if votesG2[choiceB] >= params.AlphaConfidence {
			group2.RecordPoll(votesG2[choiceB])
		}
		
		fmt.Printf("Round %2d: Group1 sees A=%d,B=%d | Group2 sees A=%d,B=%d\n",
			round, votesG1[choiceA], votesG1[choiceB], votesG2[choiceA], votesG2[choiceB])
	}
	
	fmt.Println("\n⚡ Network partition heals at round 11")
	
	// After partition heals, both groups see mixed votes
	for round := 11; round <= 20; round++ {
		// Now both groups see similar mixed votes (slight B preference)
		votes := map[ids.ID]int{
			choiceA: params.K * 2 / 5,
			choiceB: params.K * 3 / 5,
		}
		
		if votes[choiceB] >= params.AlphaConfidence {
			group1.RecordPoll(votes[choiceB])
			group2.RecordPoll(votes[choiceB])
		}
		
		fmt.Printf("Round %2d: Both groups see A=%d, B=%d\n",
			round, votes[choiceA], votes[choiceB])
		
		if group1.Finalized() && group2.Finalized() {
			fmt.Printf("\n✅ Both groups converged to same choice: %s\n", choiceB)
			break
		}
	}
}

// Simulate Byzantine validators
func SimulateByzantine() {
	fmt.Println("\n\nSimulating Byzantine Validators")
	fmt.Println("===============================")
	
	params := config.LocalParameters
	
	// Create choices
	honest := ids.GenerateTestID()
	byzantine := ids.GenerateTestID()
	
	totalValidators := 100
	byzantineCount := 20 // 20% Byzantine
	
	fmt.Printf("Total validators: %d (Byzantine: %d)\n", totalValidators, byzantineCount)
	fmt.Printf("Safety threshold requires >2/3 honest = %d\n\n", totalValidators*2/3)
	
	bt := quorum.NewBinaryThreshold()
	paramsValue := quorum.BinaryThresholdParameters{
		AlphaPreference: params.AlphaPreference,
		AlphaConfidence: params.AlphaConfidence,
		Beta:            params.Beta,
	}
	bt.Initialize(&paramsValue, honest)
	
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	round := 0
	consecutiveSuccesses := 0
	
	for consecutiveSuccesses < params.Beta && round < 50 {
		round++
		
		// Honest validators vote for honest choice
		honestVotes := totalValidators - byzantineCount
		
		// Byzantine validators try to disrupt
		byzantineVotesForHonest := 0
		byzantineVotesForByzantine := byzantineCount
		
		// Sometimes Byzantine validators vote honestly to build false confidence
		if rng.Float64() < 0.3 {
			byzantineVotesForHonest = byzantineCount / 2
			byzantineVotesForByzantine = byzantineCount / 2
		}
		
		votes := map[ids.ID]int{
			honest:    honestVotes + byzantineVotesForHonest,
			byzantine: byzantineVotesForByzantine,
		}
		
		// Sample K validators  
		sampledHonest := 0
		sampledByzantine := 0
		
		for i := 0; i < params.K; i++ {
			if rng.Float64() < float64(votes[honest])/float64(totalValidators) {
				sampledHonest++
			} else {
				sampledByzantine++
			}
		}
		
		fmt.Printf("Round %2d: Sample H=%2d, B=%2d | ", round, sampledHonest, sampledByzantine)
		
		if sampledHonest >= params.AlphaConfidence {
			consecutiveSuccesses++
			bt.RecordPoll(sampledHonest)
			fmt.Printf("✓ Progress %d/%d\n", consecutiveSuccesses, params.Beta)
		} else {
			consecutiveSuccesses = 0
			bt.RecordPoll(0)
			fmt.Printf("✗ Reset\n")
		}
	}
	
	if consecutiveSuccesses >= params.Beta {
		fmt.Printf("\n✅ Consensus successful despite Byzantine validators!\n")
	} else {
		fmt.Printf("\n❌ Byzantine validators prevented consensus\n")
	}
}

func main() {
	fmt.Println("🚀 Lux Consensus Quantum Simulation")
	fmt.Println("===================================")
	
	// Run different simulation scenarios
	SimulateConsensus()
	SimulatePartition()
	SimulateByzantine()
	
	fmt.Println("\n✨ Simulation complete!")
}