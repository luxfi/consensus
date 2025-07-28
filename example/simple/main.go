// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"

	"github.com/luxfi/consensus/config"
)

func main() {
	// Demonstrate that the consensus package parameters work
	fmt.Println("Lux Consensus Package Example")
	fmt.Println("=============================")
	
	// Show mainnet parameters
	fmt.Println("\nMainnet Parameters:")
	printParams(config.MainnetParameters)
	
	// Show testnet parameters
	fmt.Println("\nTestnet Parameters:")
	printParams(config.TestnetParameters)
	
	// Show local parameters
	fmt.Println("\nLocal Parameters:")
	printParams(config.LocalParameters)
	
	// Validate parameters
	fmt.Println("\nValidating Parameters...")
	for name, params := range map[string]config.Parameters{
		"Mainnet": config.MainnetParameters,
		"Testnet": config.TestnetParameters,
		"Local":   config.LocalParameters,
	} {
		if err := params.Valid(); err != nil {
			fmt.Printf("❌ %s parameters are invalid: %v\n", name, err)
		} else {
			fmt.Printf("✅ %s parameters are valid\n", name)
		}
	}
	
	// Show runtime configuration
	fmt.Println("\nRuntime Configuration:")
	if err := config.InitializeRuntime("mainnet"); err != nil {
		fmt.Printf("Failed to initialize runtime: %v\n", err)
		return
	}
	
	runtime := config.GetRuntime()
	fmt.Printf("Runtime K: %d, Beta: %d, MinRoundInterval: %v\n", 
		runtime.K, runtime.Beta, runtime.MinRoundInterval)
}

func printParams(p config.Parameters) {
	fmt.Printf("  K: %d\n", p.K)
	fmt.Printf("  AlphaPreference: %d\n", p.AlphaPreference)
	fmt.Printf("  AlphaConfidence: %d\n", p.AlphaConfidence)
	fmt.Printf("  Beta: %d\n", p.Beta)
	fmt.Printf("  MinRoundInterval: %v\n", p.MinRoundInterval)
	fmt.Printf("  MaxItemProcessingTime: %v\n", p.MaxItemProcessingTime)
}