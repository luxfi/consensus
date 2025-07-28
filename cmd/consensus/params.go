// Copyright (C) 2024-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/luxfi/consensus/config"
)

func runInteractiveParams(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)
	
	fmt.Println("=== Interactive Consensus Parameter Configuration ===")
	fmt.Println("Press Enter to use default values shown in brackets.")
	
	params := config.DefaultParameters
	
	// K parameter
	fmt.Printf("\nSample size K [%d]: ", params.K)
	if input := readInput(reader); input != "" {
		if k, err := strconv.Atoi(input); err == nil {
			params.K = k
		}
	}
	
	// Alpha preference
	defaultAlpha := params.K/2 + 1
	fmt.Printf("Alpha preference (> K/2 = %d) [%d]: ", params.K/2, params.AlphaPreference)
	if input := readInput(reader); input != "" {
		if alpha, err := strconv.Atoi(input); err == nil {
			params.AlphaPreference = alpha
		}
	}
	
	// Alpha confidence
	fmt.Printf("Alpha confidence (> K/2 = %d) [%d]: ", params.K/2, params.AlphaConfidence)
	if input := readInput(reader); input != "" {
		if alpha, err := strconv.Atoi(input); err == nil {
			params.AlphaConfidence = alpha
		}
	}
	
	// Beta
	fmt.Printf("Beta (consecutive successes) [%d]: ", params.Beta)
	if input := readInput(reader); input != "" {
		if beta, err := strconv.Atoi(input); err == nil {
			params.Beta = beta
		}
	}
	
	// Processing time
	fmt.Printf("Max item processing time (e.g., 10s) [%v]: ", params.MaxItemProcessingTime)
	if input := readInput(reader); input != "" {
		if duration, err := time.ParseDuration(input); err == nil {
			params.MaxItemProcessingTime = duration
		}
	}
	
	// Validate parameters
	if err := params.Valid(); err != nil {
		fmt.Printf("\n❌ Invalid parameters: %v\n", err)
		return err
	}
	
	// Display configuration
	fmt.Println("\n=== Final Configuration ===")
	displayParams(params)
	
	// Save option
	fmt.Print("\nSave configuration to file? (y/n): ")
	if input := readInput(reader); strings.ToLower(input) == "y" {
		fmt.Print("Filename [params.json]: ")
		filename := readInput(reader)
		if filename == "" {
			filename = "params.json"
		}
		
		if err := saveParams(params, filename); err != nil {
			return fmt.Errorf("failed to save parameters: %w", err)
		}
		fmt.Printf("✓ Parameters saved to %s\n", filename)
	}
	
	return nil
}

func runParamsTune(cmd *cobra.Command, args []string) error {
	// Get tuning parameters
	networkSize, _ := cmd.Flags().GetInt("network-size")
	faultTolerance, _ := cmd.Flags().GetFloat64("fault-tolerance")
	targetFinalization, _ := cmd.Flags().GetDuration("target-finalization")
	
	fmt.Printf("=== Parameter Tuning ===\n")
	fmt.Printf("Network size: %d nodes\n", networkSize)
	fmt.Printf("Fault tolerance: %.1f%%\n", faultTolerance*100)
	fmt.Printf("Target finalization: %v\n", targetFinalization)
	
	// Calculate optimal K
	// K should be sqrt(N) to log(N) for good sampling
	optimalK := int(float64(networkSize) * 0.1) // 10% of network
	if optimalK < 5 {
		optimalK = 5
	}
	if optimalK > 100 {
		optimalK = 100
	}
	
	// Calculate alpha thresholds based on fault tolerance
	// Need > 50% + fault tolerance margin
	alphaPreference := int(float64(optimalK) * (0.5 + faultTolerance + 0.1))
	alphaConfidence := int(float64(optimalK) * (0.5 + faultTolerance + 0.2))
	
	// Calculate beta based on target finalization time
	// Assuming 100ms round time
	roundTime := 100 * time.Millisecond
	targetRounds := int(targetFinalization / roundTime)
	
	// Beta should be small enough to finalize quickly but large enough for safety
	beta := targetRounds / 5
	if beta < 8 {
		beta = 8
	}
	if beta > 30 {
		beta = 30
	}
	
	params := config.Parameters{
		K:                     optimalK,
		AlphaPreference:       alphaPreference,
		AlphaConfidence:       alphaConfidence,
		Beta:                  beta,
		ConcurrentRepolls:     8,
		OptimalProcessing:     10,
		MaxOutstandingItems:   networkSize / 10,
		MaxItemProcessingTime: roundTime,
		MinRoundInterval:      roundTime,
	}
	
	// Validate
	if err := params.Valid(); err != nil {
		// Adjust if invalid
		if params.AlphaPreference <= params.K/2 {
			params.AlphaPreference = params.K/2 + 1
		}
		if params.AlphaConfidence <= params.K/2 {
			params.AlphaConfidence = params.K/2 + 2
		}
		if params.Beta >= params.K {
			params.Beta = params.K - 1
		}
	}
	
	fmt.Println("\n=== Tuned Parameters ===")
	displayParams(params)
	
	// Analysis
	fmt.Println("\n=== Expected Performance ===")
	successProb := float64(params.AlphaConfidence) / float64(params.K)
	expectedRounds := float64(params.Beta) / successProb
	expectedTime := time.Duration(float64(roundTime) * expectedRounds)
	
	fmt.Printf("Success probability per round: %.2f\n", successProb)
	fmt.Printf("Expected rounds to finalization: %.1f\n", expectedRounds)
	fmt.Printf("Expected time to finalization: %v\n", expectedTime)
	
	return nil
}

func runParamsGenerate(cmd *cobra.Command, args []string) error {
	preset, _ := cmd.Flags().GetString("preset")
	output, _ := cmd.Flags().GetString("output")
	
	var params config.Parameters
	
	switch preset {
	case "mainnet":
		params = config.Parameters{
			K:                     21,
			AlphaPreference:       13,
			AlphaConfidence:       18,
			Beta:                  8,
			ConcurrentRepolls:     8,
			OptimalProcessing:     10,
			MaxOutstandingItems:   369,
			MaxItemProcessingTime: 9630 * time.Millisecond,
			MinRoundInterval:      100 * time.Millisecond,
		}
		
	case "testnet":
		params = config.Parameters{
			K:                     11,
			AlphaPreference:       7,
			AlphaConfidence:       9,
			Beta:                  6,
			ConcurrentRepolls:     4,
			OptimalProcessing:     5,
			MaxOutstandingItems:   100,
			MaxItemProcessingTime: 6300 * time.Millisecond,
			MinRoundInterval:      50 * time.Millisecond,
		}
		
	case "local":
		params = config.Parameters{
			K:                     5,
			AlphaPreference:       3,
			AlphaConfidence:       4,
			Beta:                  3,
			ConcurrentRepolls:     2,
			OptimalProcessing:     3,
			MaxOutstandingItems:   50,
			MaxItemProcessingTime: 3690 * time.Millisecond,
			MinRoundInterval:      10 * time.Millisecond,
		}
		
	default:
		return fmt.Errorf("unknown preset: %s (available: mainnet, testnet, local)", preset)
	}
	
	fmt.Printf("=== Generated %s Parameters ===\n", preset)
	displayParams(params)
	
	if output != "" {
		if err := saveParams(params, output); err != nil {
			return fmt.Errorf("failed to save parameters: %w", err)
		}
		fmt.Printf("\n✓ Parameters saved to %s\n", output)
	}
	
	return nil
}

func displayParams(p config.Parameters) {
	fmt.Printf("K (Sample Size):        %d\n", p.K)
	fmt.Printf("Alpha Preference:       %d (%.1f%%)\n", p.AlphaPreference, float64(p.AlphaPreference)/float64(p.K)*100)
	fmt.Printf("Alpha Confidence:       %d (%.1f%%)\n", p.AlphaConfidence, float64(p.AlphaConfidence)/float64(p.K)*100)
	fmt.Printf("Beta:                   %d\n", p.Beta)
	fmt.Printf("Concurrent Repolls:     %d\n", p.ConcurrentRepolls)
	fmt.Printf("Optimal Processing:     %d\n", p.OptimalProcessing)
	fmt.Printf("Max Outstanding Items:  %d\n", p.MaxOutstandingItems)
	fmt.Printf("Max Processing Time:    %v\n", p.MaxItemProcessingTime)
	fmt.Printf("Min Round Interval:     %v\n", p.MinRoundInterval)
}

func readInput(reader *bufio.Reader) string {
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func saveParams(p config.Parameters, filename string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func init() {
	// Add flags to params commands
	tuneCmd := &cobra.Command{
		Use:   "tune",
		Short: "Tune parameters for specific network conditions",
		RunE:  runParamsTune,
	}
	tuneCmd.Flags().Int("network-size", 100, "Expected network size")
	tuneCmd.Flags().Float64("fault-tolerance", 0.2, "Desired fault tolerance (0.0-0.33)")
	tuneCmd.Flags().Duration("target-finalization", 10*time.Second, "Target finalization time")
	
	genCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate parameter configurations",
		RunE:  runParamsGenerate,
	}
	genCmd.Flags().String("preset", "mainnet", "Parameter preset: mainnet, testnet, local")
	genCmd.Flags().String("output", "", "Output file for parameters")
}