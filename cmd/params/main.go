// Package main provides the params CLI tool for viewing consensus network parameters
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/luxfi/consensus/config"
)

func main() {
	var (
		network = flag.String("network", "mainnet", "Network to show parameters for (mainnet, testnet, local, xchain)")
		json    = flag.Bool("json", false, "Output in JSON format")
		help    = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	var params config.Parameters
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
		fmt.Fprintln(os.Stderr, "Valid networks: mainnet, testnet, local, xchain")
		os.Exit(1)
	}

	if *json {
		printJSON(params)
	} else {
		printTable(params)
	}
}

func printHelp() {
	fmt.Println("Consensus Parameters Viewer")
	fmt.Println("\nUsage: params [options]")
	fmt.Println("\nOptions:")
	fmt.Println("  -network string   Network to show parameters for (default: mainnet)")
	fmt.Println("                    Options: mainnet, testnet, local, xchain")
	fmt.Println("  -json            Output in JSON format")
	fmt.Println("  -help            Show this help message")
	fmt.Println("\nExamples:")
	fmt.Println("  params                      # Show mainnet parameters")
	fmt.Println("  params -network testnet     # Show testnet parameters")
	fmt.Println("  params -network local -json # Show local parameters in JSON")
}

func printTable(p config.Parameters) {
	fmt.Printf("K (sample size):        %d\n", p.K)
	fmt.Printf("Alpha (quorum):         %.2f\n", p.Alpha)
	fmt.Printf("Beta (decision rounds): %d\n", p.Beta)
	fmt.Printf("Block Time:             %s\n", p.BlockTime)
	fmt.Printf("Round Timeout:          %s\n", p.RoundTO)
	
	if p.AlphaPreference > 0 {
		fmt.Printf("Alpha Preference:       %d\n", p.AlphaPreference)
	}
	if p.AlphaConfidence > 0 {
		fmt.Printf("Alpha Confidence:       %d\n", p.AlphaConfidence)
	}
	if p.BetaVirtuous > 0 {
		fmt.Printf("Beta Virtuous:          %d\n", p.BetaVirtuous)
	}
	if p.BetaRogue > 0 {
		fmt.Printf("Beta Rogue:             %d\n", p.BetaRogue)
	}
	if p.ConcurrentPolls > 0 {
		fmt.Printf("Concurrent Polls:       %d\n", p.ConcurrentPolls)
	}
	if p.ConcurrentRepolls > 0 {
		fmt.Printf("Concurrent Repolls:     %d\n", p.ConcurrentRepolls)
	}
	if p.OptimalProcessing > 0 {
		fmt.Printf("Optimal Processing:     %d\n", p.OptimalProcessing)
	}
	if p.MaxOutstandingItems > 0 {
		fmt.Printf("Max Outstanding Items:  %d\n", p.MaxOutstandingItems)
	}
	if p.Parents > 0 {
		fmt.Printf("Parents:                %d\n", p.Parents)
	}
	if p.BatchSize > 0 {
		fmt.Printf("Batch Size:             %d\n", p.BatchSize)
	}
}

func printJSON(p config.Parameters) {
	fmt.Printf(`{
  "k": %d,
  "alpha": %.2f,
  "beta": %d,
  "blockTime": "%s",
  "roundTimeout": "%s",
  "alphaPreference": %d,
  "alphaConfidence": %d,
  "betaVirtuous": %d,
  "betaRogue": %d,
  "concurrentPolls": %d,
  "concurrentRepolls": %d,
  "optimalProcessing": %d,
  "maxOutstandingItems": %d,
  "parents": %d,
  "batchSize": %d
}
`, p.K, p.Alpha, p.Beta, p.BlockTime, p.RoundTO,
		p.AlphaPreference, p.AlphaConfidence, p.BetaVirtuous, p.BetaRogue,
		p.ConcurrentPolls, p.ConcurrentRepolls, p.OptimalProcessing,
		p.MaxOutstandingItems, p.Parents, p.BatchSize)
}