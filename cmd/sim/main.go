// Package main provides the sim CLI tool for consensus simulation
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/luxfi/consensus/config"
)

func main() {
	var (
		nodes    = flag.Int("nodes", 100, "Number of nodes in the network")
		rounds   = flag.Int("rounds", 10, "Number of consensus rounds to simulate")
		network  = flag.String("network", "mainnet", "Network configuration (mainnet, testnet, local)")
		failure  = flag.Float64("failure", 0.1, "Node failure rate (0.0-1.0)")
		latency  = flag.Duration("latency", 50*time.Millisecond, "Network latency")
		verbose  = flag.Bool("verbose", false, "Verbose output")
		help     = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	if *failure < 0 || *failure > 1 {
		fmt.Fprintf(os.Stderr, "Failure rate must be between 0.0 and 1.0\n")
		os.Exit(1)
	}

	// Get network configuration
	params := getNetworkParams(*network)
	
	fmt.Println("=== Consensus Simulation ===")
	fmt.Printf("Network:    %s\n", *network)
	fmt.Printf("Nodes:      %d\n", *nodes)
	fmt.Printf("Rounds:     %d\n", *rounds)
	fmt.Printf("Failure:    %.1f%%\n", *failure*100)
	fmt.Printf("Latency:    %s\n", *latency)
	fmt.Printf("Parameters: K=%d, Alpha=%.2f, Beta=%d\n\n", params.K, params.Alpha, params.Beta)

	// Run simulation
	results := runSimulation(*nodes, *rounds, params, *failure, *latency, *verbose)
	
	// Print results
	printResults(results, params)
}

func printHelp() {
	fmt.Println("Consensus Simulator")
	fmt.Println("\nUsage: sim [options]")
	fmt.Println("\nOptions:")
	fmt.Println("  -nodes int        Number of nodes in the network (default: 100)")
	fmt.Println("  -rounds int       Number of consensus rounds (default: 10)")
	fmt.Println("  -network string   Network configuration (default: mainnet)")
	fmt.Println("                    Options: mainnet, testnet, local")
	fmt.Println("  -failure float    Node failure rate 0.0-1.0 (default: 0.1)")
	fmt.Println("  -latency duration Network latency (default: 50ms)")
	fmt.Println("  -verbose          Verbose output")
	fmt.Println("  -help             Show this help message")
	fmt.Println("\nExamples:")
	fmt.Println("  sim                                  # Run default simulation")
	fmt.Println("  sim -nodes 1000 -rounds 100          # Large scale simulation")
	fmt.Println("  sim -failure 0.3 -latency 200ms      # High failure, slow network")
	fmt.Println("  sim -network testnet -verbose        # Testnet config with details")
}

func getNetworkParams(network string) config.Parameters {
	switch network {
	case "mainnet":
		return config.MainnetParams()
	case "testnet":
		return config.TestnetParams()
	case "local":
		return config.LocalParams()
	default:
		fmt.Fprintf(os.Stderr, "Unknown network: %s, using mainnet\n", network)
		return config.MainnetParams()
	}
}

type SimulationResult struct {
	Round           int
	VotesReceived   int
	Confidence      float64
	Decision        string
	TimeToConsensus time.Duration
	FailedNodes     int
}

func runSimulation(nodes int, rounds int, params config.Parameters, failureRate float64, latency time.Duration, verbose bool) []SimulationResult {
	results := make([]SimulationResult, 0, rounds)
	ctx := context.Background()
	
	for round := 1; round <= rounds; round++ {
		if verbose {
			fmt.Printf("Round %d: ", round)
		}
		
		start := time.Now()
		result := simulateRound(ctx, nodes, params, failureRate, latency)
		result.Round = round
		result.TimeToConsensus = time.Since(start)
		
		if verbose {
			fmt.Printf("%s (confidence: %.2f%%, time: %s)\n", 
				result.Decision, result.Confidence*100, result.TimeToConsensus)
		}
		
		results = append(results, result)
		
		// Simulate inter-round delay
		time.Sleep(latency)
	}
	
	return results
}

func simulateRound(ctx context.Context, nodes int, params config.Parameters, failureRate float64, latency time.Duration) SimulationResult {
	// Calculate failed nodes
	failedNodes := int(float64(nodes) * failureRate)
	activeNodes := nodes - failedNodes
	
	// Sample K nodes randomly
	k := params.K
	if k > activeNodes {
		k = activeNodes
	}
	
	// Simulate voting
	votes := 0
	for i := 0; i < k; i++ {
		// Simulate network latency
		time.Sleep(latency / time.Duration(k))
		
		// Random vote with Byzantine behavior
		if rand.Float64() > 0.2 { // 80% honest nodes
			votes++
		}
	}
	
	// Calculate confidence
	confidence := float64(votes) / float64(k)
	
	// Determine decision based on alpha threshold
	decision := "REJECT"
	if confidence >= params.Alpha {
		decision = "ACCEPT"
	}
	
	return SimulationResult{
		VotesReceived: votes,
		Confidence:    confidence,
		Decision:      decision,
		FailedNodes:   failedNodes,
	}
}

func printResults(results []SimulationResult, params config.Parameters) {
	fmt.Println("=== Simulation Results ===")
	
	accepts := 0
	rejects := 0
	totalTime := time.Duration(0)
	totalConfidence := 0.0
	
	for _, r := range results {
		if r.Decision == "ACCEPT" {
			accepts++
		} else {
			rejects++
		}
		totalTime += r.TimeToConsensus
		totalConfidence += r.Confidence
	}
	
	fmt.Printf("\nConsensus Decisions:\n")
	fmt.Printf("  Accepts:  %d (%.1f%%)\n", accepts, float64(accepts)/float64(len(results))*100)
	fmt.Printf("  Rejects:  %d (%.1f%%)\n", rejects, float64(rejects)/float64(len(results))*100)
	
	fmt.Printf("\nPerformance:\n")
	fmt.Printf("  Avg Time:       %s\n", totalTime/time.Duration(len(results)))
	fmt.Printf("  Avg Confidence: %.2f%%\n", totalConfidence/float64(len(results))*100)
	fmt.Printf("  Alpha Required: %.2f%%\n", params.Alpha*100)
	
	// Calculate finality probability
	finalityProb := calculateFinalityProbability(params.Alpha, params.Beta, totalConfidence/float64(len(results)))
	fmt.Printf("\nFinality:\n")
	fmt.Printf("  Probability:    %.4f%%\n", finalityProb*100)
	fmt.Printf("  Beta Rounds:    %d\n", params.Beta)
}

func calculateFinalityProbability(alpha float64, beta uint32, avgConfidence float64) float64 {
	// Simplified finality calculation
	// P(finality) = confidence^beta
	prob := 1.0
	for i := uint32(0); i < beta; i++ {
		prob *= avgConfidence
	}
	return prob
}

func init() {
	rand.Seed(time.Now().UnixNano())
}