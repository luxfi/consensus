// Package main provides the main consensus CLI tool
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/config"
)

func main() {
	var (
		engine  = flag.String("engine", "chain", "Consensus engine (chain, dag, pq)")
		network = flag.String("network", "mainnet", "Network configuration")
		action  = flag.String("action", "info", "Action to perform (info, test, health)")
		help    = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	switch *action {
	case "info":
		showInfo(*engine, *network)
	case "test":
		testEngine(*engine, *network)
	case "health":
		checkHealth(*engine)
	default:
		fmt.Fprintf(os.Stderr, "Unknown action: %s\n", *action)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Lux Consensus CLI")
	fmt.Println("\nUsage: consensus [options]")
	fmt.Println("\nOptions:")
	fmt.Println("  -engine string   Consensus engine (default: chain)")
	fmt.Println("                   Options: chain, dag, pq")
	fmt.Println("  -network string  Network configuration (default: mainnet)")
	fmt.Println("                   Options: mainnet, testnet, local")
	fmt.Println("  -action string   Action to perform (default: info)")
	fmt.Println("                   Options: info, test, health")
	fmt.Println("  -help            Show this help message")
	fmt.Println("\nExamples:")
	fmt.Println("  consensus                          # Show chain engine info")
	fmt.Println("  consensus -engine dag -action test # Test DAG engine")
	fmt.Println("  consensus -action health           # Check engine health")
}

func showInfo(engineType, network string) {
	fmt.Printf("=== Consensus Engine Info ===\n")
	fmt.Printf("Engine:  %s\n", engineType)
	fmt.Printf("Network: %s\n", network)

	params := getNetworkParams(network)
	fmt.Printf("\nParameters:\n")
	fmt.Printf("  K (sample size):        %d\n", params.K)
	fmt.Printf("  Alpha (quorum):         %.2f\n", params.Alpha)
	fmt.Printf("  Beta (decision rounds): %d\n", params.Beta)
	fmt.Printf("  Block Time:             %s\n", params.BlockTime)
	fmt.Printf("  Round Timeout:          %s\n", params.RoundTO)

	switch engineType {
	case "chain":
		fmt.Println("\nChain Engine:")
		fmt.Println("  - Linear blockchain consensus")
		fmt.Println("  - Sequential block processing")
		fmt.Println("  - Optimized for ordered transactions")
	case "dag":
		fmt.Println("\nDAG Engine:")
		fmt.Println("  - Directed Acyclic Graph consensus")
		fmt.Println("  - Parallel transaction processing")
		fmt.Println("  - High throughput optimization")
	case "pq":
		fmt.Println("\nPost-Quantum Engine:")
		fmt.Println("  - Quantum-resistant cryptography")
		fmt.Println("  - ML-DSA-65 algorithm")
		fmt.Println("  - Future-proof security")
	}
}

func testEngine(engineType, network string) {
	fmt.Printf("Testing %s engine with %s configuration...\n", engineType, network)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch engineType {
	case "chain":
		engine := consensus.NewChain(consensus.DefaultConfig())
		// Start engine
		if err := engine.Start(ctx); err != nil {
			fmt.Printf("✗ Failed to start: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = engine.Stop() }()
		fmt.Println("✓ Engine test passed")
		fmt.Printf("  Engine started successfully\n")
	case "dag":
		fmt.Println("DAG engine not yet implemented in new API")
	case "pq":
		fmt.Println("PQ engine not yet implemented in new API")
	default:
		fmt.Fprintf(os.Stderr, "Unknown engine: %s\n", engineType)
		os.Exit(1)
	}
}

func checkHealth(engineType string) {
	fmt.Printf("Checking %s engine health...\n", engineType)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	switch engineType {
	case "chain":
		engine := consensus.NewChain(consensus.DefaultConfig())
		// Start engine
		if err := engine.Start(ctx); err != nil {
			fmt.Printf("✗ Failed to start: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = engine.Stop() }()
		fmt.Println("✓ Healthy")
		fmt.Printf("  Engine started successfully\n")
	case "dag":
		fmt.Println("DAG engine not yet implemented in new API")
	case "pq":
		fmt.Println("PQ engine not yet implemented in new API")
	default:
		fmt.Fprintf(os.Stderr, "Unknown engine: %s\n", engineType)
		os.Exit(1)
	}
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
		return config.MainnetParams()
	}
}
