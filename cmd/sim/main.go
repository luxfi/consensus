package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/wave"
	"github.com/luxfi/consensus/photon"
)

func main() {
	var (
		nodes    = flag.Int("nodes", 21, "Number of nodes in simulation")
		rounds   = flag.Int("rounds", 100, "Number of consensus rounds")
		items    = flag.Int("items", 10, "Number of items to decide on")
		network  = flag.String("network", "mainnet", "Network config (mainnet, testnet, local, xchain)")
		seed     = flag.Int64("seed", 0, "Random seed (0 for time-based)")
		verbose  = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	rand.Seed(*seed)

	fmt.Printf("Lux Consensus Simulator\n")
	fmt.Printf("======================\n")
	fmt.Printf("Nodes: %d\n", *nodes)
	fmt.Printf("Rounds: %d\n", *rounds)
	fmt.Printf("Items: %d\n", *items)
	fmt.Printf("Network: %s\n", *network)
	fmt.Printf("Seed: %d\n\n", *seed)

	// Get network config
	var params config.Params
	switch *network {
	case "mainnet":
		params = config.MainnetParams()
	case "testnet":
		params = config.TestnetParams()
	case "local":
		params = config.LocalParams()
	case "xchain":
		params = config.XChainParams()
	default:
		fmt.Fprintf(os.Stderr, "Unknown network: %s\n", *network)
		os.Exit(1)
	}

	// Create node list
	nodeList := make([]string, *nodes)
	for i := 0; i < *nodes; i++ {
		nodeList[i] = fmt.Sprintf("node-%d", i)
	}

	// Run simulation
	sim := &Simulator{
		nodes:   nodeList,
		params:  params,
		verbose: *verbose,
	}

	start := time.Now()
	results := sim.Run(*items, *rounds)
	duration := time.Since(start)

	// Print results
	fmt.Printf("\nSimulation Results\n")
	fmt.Printf("==================\n")
	fmt.Printf("Total time: %v\n", duration)
	fmt.Printf("Items decided: %d/%d\n", results.Decided, *items)
	fmt.Printf("Average rounds to decision: %.2f\n", results.AvgRounds)
	fmt.Printf("Min rounds: %d\n", results.MinRounds)
	fmt.Printf("Max rounds: %d\n", results.MaxRounds)
	fmt.Printf("Throughput: %.2f decisions/sec\n", float64(results.Decided)/duration.Seconds())

	if results.Decided < *items {
		os.Exit(1)
	}
}

type Simulator struct {
	nodes   []string
	params  config.Params
	verbose bool
}

type Results struct {
	Decided   int
	AvgRounds float64
	MinRounds int
	MaxRounds int
}

func (s *Simulator) Run(items, maxRounds int) Results {
	results := Results{
		MinRounds: maxRounds,
	}

	totalRounds := 0

	for item := 0; item < items; item++ {
		itemID := fmt.Sprintf("item-%d", item)
		
		if s.verbose {
			fmt.Printf("Processing %s...\n", itemID)
		}

		// Create consensus engine for this item
		engine := wave.New[string](s.params)
		engine.Initialize(itemID, 0)

		// Create emitter for node selection
		emitter := photon.NewUniformEmitter(s.nodes)

		// Run consensus rounds
		decided := false
		for round := 1; round <= maxRounds; round++ {
			// Select K nodes to vote
			voters, err := emitter.Emit(context.Background(), s.params.K, uint64(round))
			if err != nil {
				fmt.Printf("Error selecting voters: %v\n", err)
				continue
			}

			// Simulate votes (80% vote for item, 20% against)
			votes := 0
			for _, voter := range voters {
				if rand.Float64() < 0.8 {
					votes++
				}
				_ = voter // Use voter to avoid unused variable
			}

			// Record poll result
			engine.RecordPoll(itemID, votes)

			// Check if decided
			if engine.IsAccepted(itemID) {
				decided = true
				totalRounds += round
				
				if round < results.MinRounds {
					results.MinRounds = round
				}
				if round > results.MaxRounds {
					results.MaxRounds = round
				}

				// Report performance back to emitter
				for _, voter := range voters {
					emitter.Report(voter, true)
				}

				if s.verbose {
					fmt.Printf("  Decided in %d rounds\n", round)
				}
				break
			}
		}

		if decided {
			results.Decided++
		} else if s.verbose {
			fmt.Printf("  Failed to decide after %d rounds\n", maxRounds)
		}
	}

	if results.Decided > 0 {
		results.AvgRounds = float64(totalRounds) / float64(results.Decided)
	}

	return results
}