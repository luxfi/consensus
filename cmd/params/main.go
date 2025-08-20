package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/luxfi/consensus/config"
)

func main() {
	var (
		network = flag.String("network", "mainnet", "Network configuration (mainnet, testnet, local, xchain)")
		format  = flag.String("format", "text", "Output format (text, json)")
	)
	flag.Parse()

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

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(params); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding params: %v\n", err)
			os.Exit(1)
		}
	case "text":
		fmt.Printf("Network: %s\n", *network)
		fmt.Printf("Parameters:\n")
		fmt.Printf("  K (sample size): %d\n", params.K)
		fmt.Printf("  Alpha (preference): %.2f\n", params.Alpha)
		fmt.Printf("  Beta (confidence): %d\n", params.Beta)
		fmt.Printf("  Concurrent reps: %d\n", params.ConcurrentRepolls)
		fmt.Printf("  Optimal processing: %d\n", params.OptimalProcessing)
		fmt.Printf("  Max outstanding items: %d\n", params.MaxOutstandingItems)
		fmt.Printf("  Max item processing: %d\n", params.MaxItemProcessing)
		fmt.Printf("  Mixed query num push vdr: %d\n", params.MixedQueryNumPushVdr)
		fmt.Printf("  Mixed query num push non-vdr: %d\n", params.MixedQueryNumPushNonVdr)
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %s\n", *format)
		os.Exit(1)
	}
}