// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/luxfi/consensus/config"
)

func main() {
	var (
		preset       = flag.String("preset", "", "Use preset configuration: mainnet, testnet, local")
		nodeCount    = flag.Int("nodes", 0, "Number of nodes in the network")
		k            = flag.Int("k", 0, "Sample size")
		alphaPref    = flag.Int("alpha-pref", 0, "Preference quorum threshold")
		alphaConf    = flag.Int("alpha-conf", 0, "Confidence quorum threshold")
		beta         = flag.Int("beta", 0, "Consecutive rounds threshold")
		concurrent   = flag.Int("concurrent", 0, "Concurrent repolls")
		optimize     = flag.String("optimize", "", "Optimize for: latency, security, throughput")
		output       = flag.String("output", "", "Output file for parameters (JSON)")
		summary      = flag.Bool("summary", false, "Show parameter summary")
		validate     = flag.String("validate", "", "Validate parameters from JSON file")
		targetTime   = flag.Duration("target-finality", 0*time.Second, "Target finality time")
		networkLat   = flag.Int("network-latency", 50, "Expected network latency in ms")
		interactive  = flag.Bool("interactive", false, "Run in interactive mode")
		guide        = flag.Bool("guide", false, "Show parameter guidance")
		safety       = flag.Bool("safety", false, "Perform safety analysis")
		totalNodes   = flag.Int("total-nodes", 0, "Total nodes for safety analysis")
		check        = flag.Bool("check", false, "Run comprehensive parameter checker")
		tune         = flag.Bool("tune", false, "Tune parameters based on network requirements")
	)

	flag.Parse()

	// Handle interactive mode
	if *interactive {
		if err := runInteractive(); err != nil {
			fmt.Fprintf(os.Stderr, "Interactive mode error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Handle parameter guide
	if *guide {
		showParameterGuide()
		return
	}

	// Handle parameter tuning mode
	if *tune {
		if err := runTuning(); err != nil {
			fmt.Fprintf(os.Stderr, "Parameter tuning error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Handle validation mode
	if *validate != "" {
		if err := validateFile(*validate); err != nil {
			fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Parameters are valid!")
		return
	}

	// Build parameters
	builder := config.NewBuilder()

	// Apply preset if specified
	if *preset != "" {
		builder = builder.FromPreset(config.NetworkType(*preset))
	}

	// Apply node count optimization
	if *nodeCount > 0 {
		builder = builder.ForNodeCount(*nodeCount)
	}

	// Apply manual parameters
	if *k > 0 {
		builder = builder.WithSampleSize(*k)
	}
	if *alphaPref > 0 || *alphaConf > 0 {
		// Use existing values if not specified
		cfg, _ := builder.Build()
		pref := *alphaPref
		if pref == 0 {
			pref = cfg.AlphaPreference
		}
		conf := *alphaConf
		if conf == 0 {
			conf = cfg.AlphaConfidence
		}
		builder = builder.WithQuorums(pref, conf)
	}
	if *beta > 0 {
		builder = builder.WithBeta(*beta)
	}
	if *concurrent > 0 {
		builder = builder.WithConcurrentRepolls(*concurrent)
	}

	// Apply target finality
	if *targetTime > 0 {
		builder = builder.WithTargetFinality(*targetTime, *networkLat)
	}

	// Apply optimization
	switch *optimize {
	case "latency":
		builder = builder.OptimizeForLatency()
	case "security":
		builder = builder.OptimizeForSecurity()
	case "throughput":
		builder = builder.OptimizeForThroughput()
	}

	// Build final parameters
	cfg, err := builder.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building parameters: %v\n", err)
		os.Exit(1)
	}
	
	// Convert to Parameters type
	params := ConfigToParameters(cfg)

	// Output results
	if *output != "" {
		data, err := ToJSON(params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		if err := ioutil.WriteFile(*output, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Parameters written to %s\n", *output)
	} else {
		// Print JSON to stdout
		data, _ := ToJSON(params)
		fmt.Println(string(data))
	}

	// Show summary if requested
	if *summary {
		fmt.Println("\n" + Summary(params))
	}

	// Perform safety analysis if requested
	if *safety {
		nodes := *totalNodes
		if nodes == 0 && *nodeCount > 0 {
			nodes = *nodeCount
		}
		if nodes == 0 {
			// Estimate from K
			nodes = params.K
		}
		
		fmt.Println("\n🛡️  Safety Analysis:")
		fmt.Println("===================")
		report := AnalyzeSafety(params, nodes)
		displaySafetyReport(report)
		
		// Check production readiness
		if err := ValidateForProduction(params, nodes); err != nil {
			fmt.Printf("\n⚠️  Not recommended for production: %v\n", err)
		} else {
			fmt.Println("\n✅ Parameters are production-ready")
		}
	}

	// Run comprehensive checker if requested
	if *check {
		nodes := *totalNodes
		if nodes == 0 && *nodeCount > 0 {
			nodes = *nodeCount
		}
		if nodes == 0 {
			// Estimate from preset
			switch *preset {
			case "mainnet":
				nodes = 21
			case "testnet":
				nodes = 11
			case "local":
				nodes = 5
			default:
				nodes = params.K
			}
		}
		
		report := RunChecker(params, nodes, *networkLat)
		fmt.Println(FormatCheckerReport(report, nodes))
	}
}

func validateFile(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	params, err := ParametersFromJSON(data)
	if err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	if err := params.Valid(); err != nil {
		return fmt.Errorf("validation: %w", err)
	}

	fmt.Println(Summary(params))
	return nil
}

func showParameterGuide() {
	fmt.Println("📚 Lux Consensus Parameter Guide")
	fmt.Println("================================")
	
	guides := GetParameterGuides()
	for _, guide := range guides {
		fmt.Printf("### %s\n", guide.Parameter)
		fmt.Printf("Description: %s\n", guide.Description)
		fmt.Printf("Formula:     %s\n", guide.Formula)
		fmt.Printf("Range:       %v to %v\n", guide.MinValue, guide.MaxValue)
		fmt.Printf("Typical:     %s\n", guide.Typical)
		fmt.Printf("Impact:      %s\n", guide.Impact)
		fmt.Printf("Trade-offs:  %s\n\n", guide.TradeOffs)
	}
	
	fmt.Println("💡 Tips for Parameter Selection:")
	fmt.Println("1. Start with a preset (mainnet, testnet, or local)")
	fmt.Println("2. Adjust based on your specific network characteristics")
	fmt.Println("3. Use -safety flag to validate your choices")
	fmt.Println("4. Use -interactive mode for guided configuration")
}

