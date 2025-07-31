// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/luxfi/consensus/config"
	"log/slog"
)

var logger = slog.Default().With("module", "checker")

func main() {
	// Command line flags
	network := flag.String("network", "mainnet", "Network type: mainnet, testnet, or local")
	k := flag.Int("k", 0, "Sample size (0 to use network default)")
	alphaPref := flag.Int("alpha-pref", 0, "Alpha preference threshold (0 to use network default)")
	alphaConf := flag.Int("alpha-conf", 0, "Alpha confidence threshold (0 to use network default)")
	beta := flag.Int("beta", 0, "Beta rounds (0 to use network default)")
	totalNodes := flag.Int("nodes", 0, "Total number of nodes in network")
	byzantineNodes := flag.Int("byzantine", 0, "Number of Byzantine nodes")
	showAnalysis := flag.Bool("analyze", true, "Show detailed analysis")
	flag.Parse()

	// Load base configuration
	var cfg *config.Config
	switch *network {
	case "mainnet":
		cfg = &config.MainnetConfig
	case "testnet":
		cfg = &config.TestnetConfig
	case "local":
		cfg = &config.LocalConfig
	default:
		logger.Error("Invalid network type", "network", *network)
		os.Exit(1)
	}

	// Apply overrides
	if *k > 0 {
		cfg.K = *k
	}
	if *alphaPref > 0 {
		cfg.AlphaPreference = *alphaPref
	}
	if *alphaConf > 0 {
		cfg.AlphaConfidence = *alphaConf
	}
	if *beta > 0 {
		cfg.Beta = *beta
	}
	if *totalNodes > 0 {
		cfg.TotalNodes = *totalNodes
	}

	// Validate configuration
	validator := config.NewValidator()
	result := validator.ValidateDetailed(cfg)
	
	fmt.Printf("\n=== Consensus Parameter Check for %s ===\n", *network)
	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("  K (sample size):        %d\n", cfg.K)
	fmt.Printf("  Alpha Preference:       %d\n", cfg.AlphaPreference)
	fmt.Printf("  Alpha Confidence:       %d\n", cfg.AlphaConfidence)
	fmt.Printf("  Beta (rounds):          %d\n", cfg.Beta)
	fmt.Printf("  Concurrent Reprisms:     %d\n", cfg.ConcurrentReprisms)
	fmt.Printf("  Max Processing Time:    %s\n", cfg.MaxItemProcessingTime)
	fmt.Printf("  Min Round Interval:     %s\n", cfg.MinRoundInterval)
	
	if cfg.TotalNodes > 0 {
		fmt.Printf("  Total Nodes:            %d\n", cfg.TotalNodes)
		fmt.Printf("  Sampling Ratio:         %.1f%%\n", float64(cfg.K)/float64(cfg.TotalNodes)*100)
	}

	// Show validation results
	if !result.Valid {
		fmt.Printf("\n❌ Configuration Errors:\n")
		for _, err := range result.Errors {
			fmt.Printf("   - %s: %s (current: %v)\n", err.Field, err.Constraint, err.Value)
			fmt.Printf("     Suggestion: %s\n", err.Suggestion)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n⚠️  Configuration Warnings:\n")
		for _, warn := range result.Warnings {
			fmt.Printf("   - %s: %s (current: %v)\n", warn.Field, warn.Constraint, warn.Value)
			fmt.Printf("     Suggestion: %s\n", warn.Suggestion)
		}
	}

	// Security analysis
	if *showAnalysis {
		fmt.Printf("\n=== Security Analysis ===\n")
		
		// Calculate failure probability
		epsilon := calculateFailureProbability(cfg.K, cfg.AlphaConfidence, *byzantineNodes)
		fmt.Printf("\nFailure Probability (ε):\n")
		fmt.Printf("  With %d Byzantine nodes: %.2e\n", *byzantineNodes, epsilon)
		
		// Safety cutoff
		safetyCutoff := calculateSafetyCutoff(cfg.Beta, epsilon)
		fmt.Printf("\nSafety Cutoff:\n")
		fmt.Printf("  Probability of incorrect finalization: %.2e\n", safetyCutoff)
		fmt.Printf("  Expected false positives per billion decisions: %.2f\n", safetyCutoff*1e9)
		
		// Byzantine tolerance
		maxByzantine := cfg.K - cfg.AlphaConfidence
		byzantinePercent := float64(maxByzantine) / float64(cfg.K) * 100
		fmt.Printf("\nByzantine Tolerance:\n")
		fmt.Printf("  Maximum Byzantine nodes in sample: %d (%.1f%%)\n", maxByzantine, byzantinePercent)
		
		if cfg.TotalNodes > 0 {
			networkByzantine := int(float64(cfg.TotalNodes) * float64(maxByzantine) / float64(cfg.K))
			fmt.Printf("  Maximum Byzantine nodes in network: %d (%.1f%%)\n", 
				networkByzantine, float64(networkByzantine)/float64(cfg.TotalNodes)*100)
		}
		
		// Performance metrics
		fmt.Printf("\n=== Performance Analysis ===\n")
		
		// Finality time
		if cfg.NetworkLatency > 0 {
			expectedFinality := time.Duration(cfg.Beta) * cfg.NetworkLatency
			fmt.Printf("\nExpected Finality Time:\n")
			fmt.Printf("  With %s network latency: %s\n", cfg.NetworkLatency, expectedFinality)
		} else {
			// Estimate based on processing time
			roundTime := cfg.MaxItemProcessingTime / time.Duration(cfg.K)
			if roundTime < cfg.MinRoundInterval {
				roundTime = cfg.MinRoundInterval
			}
			expectedFinality := time.Duration(cfg.Beta) * roundTime
			fmt.Printf("\nEstimated Finality Time:\n")
			fmt.Printf("  Based on processing time: %s\n", expectedFinality)
		}
		
		// Throughput analysis
		fmt.Printf("\nThroughput Capacity:\n")
		fmt.Printf("  Optimal Processing:     %d items\n", cfg.OptimalProcessing)
		fmt.Printf("  Max Outstanding Items:  %d items\n", cfg.MaxOutstandingItems)
		
		if cfg.ConcurrentReprisms < cfg.Beta {
			fmt.Printf("  Pipelining Efficiency:  %.1f%% (%d/%d rounds)\n",
				float64(cfg.ConcurrentReprisms)/float64(cfg.Beta)*100,
				cfg.ConcurrentReprisms, cfg.Beta)
		} else {
			fmt.Printf("  Pipelining Efficiency:  100%% (full pipelining)\n")
		}
	}

	if result.Valid && len(result.Warnings) == 0 {
		fmt.Printf("\n✅ Configuration is valid and optimized!\n")
	} else if result.Valid {
		fmt.Printf("\n✅ Configuration is valid (with warnings)\n")
	} else {
		fmt.Printf("\n❌ Configuration is invalid - please fix errors\n")
		os.Exit(1)
	}
}

// calculateFailureProbability calculates the probability of a Byzantine attack succeeding
func calculateFailureProbability(k, alphaConfidence, byzantineNodes int) float64 {
	if byzantineNodes == 0 {
		return 0
	}
	
	// Simplified calculation - in reality this involves hypergeometric distribution
	// This is the probability that Byzantine nodes control >= alphaConfidence votes
	byzantineRatio := float64(byzantineNodes) / float64(k)
	threshold := float64(alphaConfidence) / float64(k)
	
	if byzantineRatio >= threshold {
		return 1.0 // Byzantine nodes already control the threshold
	}
	
	// Approximate using binomial distribution
	// P(X >= alphaConfidence) where X ~ Binomial(k, byzantineRatio)
	return binomialTailProbability(k, byzantineRatio, alphaConfidence)
}

// calculateSafetyCutoff calculates the probability of incorrect finalization
func calculateSafetyCutoff(beta int, epsilon float64) float64 {
	// Probability of beta consecutive failures
	return math.Pow(epsilon, float64(beta))
}

// binomialTailProbability calculates P(X >= k) for X ~ Binomial(n, p)
func binomialTailProbability(n int, p float64, k int) float64 {
	if k > n {
		return 0
	}
	
	// Use normal approximation for large n
	if n > 100 {
		mean := float64(n) * p
		stdDev := math.Sqrt(float64(n) * p * (1 - p))
		z := (float64(k) - 0.5 - mean) / stdDev
		return 0.5 * (1 - erf(z/math.Sqrt(2)))
	}
	
	// Direct calculation for small n
	sum := 0.0
	for i := k; i <= n; i++ {
		sum += binomialPMF(n, i, p)
	}
	return sum
}

// binomialPMF calculates the probability mass function for binomial distribution
func binomialPMF(n, k int, p float64) float64 {
	return float64(binomialCoeff(n, k)) * math.Pow(p, float64(k)) * math.Pow(1-p, float64(n-k))
}

// binomialCoeff calculates binomial coefficient "n choose k"
func binomialCoeff(n, k int) int {
	if k > n-k {
		k = n - k
	}
	result := 1
	for i := 0; i < k; i++ {
		result = result * (n - i) / (i + 1)
	}
	return result
}

// erf is the error function
func erf(x float64) float64 {
	// Approximation of error function
	a1 := 0.254829592
	a2 := -0.284496736
	a3 := 1.421413741
	a4 := -1.453152027
	a5 := 1.061405429
	p := 0.3275911
	
	sign := 1.0
	if x < 0 {
		sign = -1.0
		x = -x
	}
	
	t := 1.0 / (1.0 + p*x)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-x*x)
	
	return sign * y
}