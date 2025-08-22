// Package main provides the checker CLI tool for consensus health checking
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
		engine  = flag.String("engine", "all", "Engine to check (chain, dag, pq, all)")
		timeout = flag.Duration("timeout", 5*time.Second, "Health check timeout")
		verbose = flag.Bool("verbose", false, "Verbose output")
		help    = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	exitCode := 0

	switch *engine {
	case "chain":
		if !checkEngine(ctx, "chain", consensus.NewChainEngine(), *verbose) {
			exitCode = 1
		}
	case "dag":
		if !checkEngine(ctx, "dag", consensus.NewDAGEngine(), *verbose) {
			exitCode = 1
		}
	case "pq":
		if !checkEngine(ctx, "pq", consensus.NewPQEngine(), *verbose) {
			exitCode = 1
		}
	case "all":
		if !checkEngine(ctx, "chain", consensus.NewChainEngine(), *verbose) {
			exitCode = 1
		}
		if !checkEngine(ctx, "dag", consensus.NewDAGEngine(), *verbose) {
			exitCode = 1
		}
		if !checkEngine(ctx, "pq", consensus.NewPQEngine(), *verbose) {
			exitCode = 1
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown engine: %s\n", *engine)
		os.Exit(1)
	}

	// Check configurations
	if !checkConfigurations(*verbose) {
		exitCode = 1
	}

	if exitCode == 0 {
		fmt.Println("\n✓ All health checks passed")
	} else {
		fmt.Println("\n✗ Some health checks failed")
	}

	os.Exit(exitCode)
}

func printHelp() {
	fmt.Println("Consensus Health Checker")
	fmt.Println("\nUsage: checker [options]")
	fmt.Println("\nOptions:")
	fmt.Println("  -engine string    Engine to check (default: all)")
	fmt.Println("                    Options: chain, dag, pq, all")
	fmt.Println("  -timeout duration Health check timeout (default: 5s)")
	fmt.Println("  -verbose          Verbose output")
	fmt.Println("  -help             Show this help message")
	fmt.Println("\nExamples:")
	fmt.Println("  checker                    # Check all engines")
	fmt.Println("  checker -engine chain      # Check only chain engine")
	fmt.Println("  checker -verbose           # Verbose output for debugging")
}

func checkEngine(ctx context.Context, name string, engine consensus.Engine, verbose bool) bool {
	fmt.Printf("Checking %s engine... ", name)
	
	// Start engine
	if err := engine.Start(ctx, 1); err != nil {
		fmt.Printf("✗ Failed to start: %v\n", err)
		return false
	}
	defer engine.Stop(ctx)
	
	// Check bootstrapped
	if !engine.IsBootstrapped() {
		fmt.Println("✗ Not bootstrapped")
		return false
	}
	
	// Perform health check
	health, err := engine.HealthCheck(ctx)
	if err != nil {
		fmt.Printf("✗ Health check failed: %v\n", err)
		return false
	}
	
	fmt.Println("✓")
	
	if verbose {
		fmt.Printf("  Health data: %+v\n", health)
	}
	
	return true
}

func checkConfigurations(verbose bool) bool {
	fmt.Println("\nChecking configurations...")
	
	configs := map[string]config.Parameters{
		"mainnet": config.MainnetParams(),
		"testnet": config.TestnetParams(),
		"local":   config.LocalParams(),
		"xchain":  config.XChainParams(),
	}
	
	allValid := true
	
	for name, cfg := range configs {
		fmt.Printf("  %s: ", name)
		if err := cfg.Valid(); err != nil {
			fmt.Printf("✗ %v\n", err)
			allValid = false
		} else {
			fmt.Println("✓")
			if verbose {
				fmt.Printf("    K=%d, Alpha=%.2f, Beta=%d, BlockTime=%s\n",
					cfg.K, cfg.Alpha, cfg.Beta, cfg.BlockTime)
			}
		}
	}
	
	return allValid
}