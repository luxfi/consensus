// Copyright (C) 2024-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/spf13/cobra"
)

func runChecker(cmd *cobra.Command, args []string) error {
	// Get parameters from flags or use defaults
	params := config.DefaultParameters

	// You can add flags to override default parameters
	k, _ := cmd.Flags().GetInt("k")
	if k > 0 {
		params.K = k
	}

	alphaPreference, _ := cmd.Flags().GetInt("alpha-preference")
	if alphaPreference > 0 {
		params.AlphaPreference = alphaPreference
	}

	alphaConfidence, _ := cmd.Flags().GetInt("alpha-confidence")
	if alphaConfidence > 0 {
		params.AlphaConfidence = alphaConfidence
	}

	beta, _ := cmd.Flags().GetInt("beta")
	if beta > 0 {
		params.Beta = uint32(beta)
	}

	// Validate parameters
	if err := params.Valid(); err != nil {
		return fmt.Errorf("invalid parameters: %w", err)
	}

	// Display parameter analysis
	fmt.Println("=== Lux Consensus Parameter Analysis ===")
	fmt.Printf("\nParameters:\n")
	fmt.Printf("  K (Sample Size):        %d\n", params.K)
	fmt.Printf("  Alpha Preference:       %d (%.1f%%)\n", params.AlphaPreference, float64(params.AlphaPreference)/float64(params.K)*100)
	fmt.Printf("  Alpha Confidence:       %d (%.1f%%)\n", params.AlphaConfidence, float64(params.AlphaConfidence)/float64(params.K)*100)
	fmt.Printf("  Beta (Confidence):      %d\n", params.Beta)
	fmt.Printf("  Min Round Interval:     %v\n", params.MinRoundInterval)
	fmt.Printf("  Processing Time:        %v\n", params.MaxItemProcessingTime)

	// Safety analysis
	fmt.Println("\n=== Safety Analysis ===")

	// Byzantine fault tolerance
	byzantineThreshold := (params.K - 1) / 3
	fmt.Printf("Byzantine Fault Tolerance: %d nodes (%.1f%%)\n",
		byzantineThreshold, float64(byzantineThreshold)/float64(params.K)*100)

	// Check if alpha thresholds are safe
	if params.AlphaPreference <= params.K/2 {
		fmt.Printf("⚠️  WARNING: Alpha Preference (%d) should be > K/2 (%d) for safety\n",
			params.AlphaPreference, params.K/2)
	} else {
		fmt.Printf("✓ Alpha Preference is safely above K/2\n")
	}

	if params.AlphaConfidence <= params.K/2 {
		fmt.Printf("⚠️  WARNING: Alpha Confidence (%d) should be > K/2 (%d) for safety\n",
			params.AlphaConfidence, params.K/2)
	} else {
		fmt.Printf("✓ Alpha Confidence is safely above K/2\n")
	}

	// Performance characteristics
	fmt.Println("\n=== Performance Characteristics ===")

	// Expected rounds to finalization (simplified model)
	successProb := float64(params.AlphaConfidence) / float64(params.K)
	expectedRounds := float64(params.Beta) / successProb
	fmt.Printf("Expected rounds to finalization: %.1f\n", expectedRounds)

	// Time to finalization
	roundTime := params.MaxItemProcessingTime
	if params.MinRoundInterval > roundTime {
		roundTime = params.MinRoundInterval
	}
	expectedTime := time.Duration(float64(roundTime) * expectedRounds)
	fmt.Printf("Expected time to finalization: %v\n", expectedTime)

	// Network recommendations
	fmt.Println("\n=== Network Recommendations ===")

	// Minimum network size
	minNetworkSize := params.K * 3 // Rule of thumb: 3x sample size
	fmt.Printf("Minimum recommended network size: %d nodes\n", minNetworkSize)

	// Optimal network size
	optimalNetworkSize := params.K * 10 // Rule of thumb: 10x sample size
	fmt.Printf("Optimal network size: %d+ nodes\n", optimalNetworkSize)

	return nil
}

func init() {
	// Add flags to check command
	checkCmd := checkCmd()
	checkCmd.Flags().Int("k", 0, "Sample size (K parameter)")
	checkCmd.Flags().Int("alpha-preference", 0, "Alpha preference threshold")
	checkCmd.Flags().Int("alpha-confidence", 0, "Alpha confidence threshold")
	checkCmd.Flags().Int("beta", 0, "Beta (consecutive success threshold)")
}
