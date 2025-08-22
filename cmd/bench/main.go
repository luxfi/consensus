// Package main provides the bench CLI tool for consensus benchmarking
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain"
	"github.com/luxfi/consensus/engine/dag"
	"github.com/luxfi/consensus/engine/pq"
	"github.com/luxfi/ids"
)

func main() {
	var (
		engine    = flag.String("engine", "all", "Engine to benchmark (chain, dag, pq, all)")
		network   = flag.String("network", "local", "Network configuration (mainnet, testnet, local)")
		duration  = flag.Duration("duration", 10*time.Second, "Benchmark duration")
		blocks    = flag.Int("blocks", 1000, "Number of blocks to process")
		parallel  = flag.Int("parallel", 1, "Number of parallel workers")
		useZMQ    = flag.Bool("zmq", false, "Use ZMQ transport (if available)")
		verbose   = flag.Bool("verbose", false, "Verbose output")
		help      = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	// Get network configuration
	params := getNetworkParams(*network)
	
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	fmt.Printf("Benchmarking %s engine(s) with %s configuration\n", *engine, *network)
	fmt.Printf("Duration: %s, Blocks: %d, Parallel: %d, ZMQ: %v\n\n", *duration, *blocks, *parallel, *useZMQ)

	switch *engine {
	case "chain":
		benchmarkChain(ctx, params, *blocks, *parallel, *verbose)
	case "dag":
		benchmarkDAG(ctx, params, *blocks, *parallel, *verbose)
	case "pq":
		benchmarkPQ(ctx, params, *blocks, *parallel, *verbose)
	case "all":
		benchmarkChain(ctx, params, *blocks, *parallel, *verbose)
		fmt.Println()
		benchmarkDAG(ctx, params, *blocks, *parallel, *verbose)
		fmt.Println()
		benchmarkPQ(ctx, params, *blocks, *parallel, *verbose)
	default:
		fmt.Fprintf(os.Stderr, "Unknown engine: %s\n", *engine)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Consensus Benchmark Tool")
	fmt.Println("\nUsage: bench [options]")
	fmt.Println("\nOptions:")
	fmt.Println("  -engine string    Engine to benchmark (default: all)")
	fmt.Println("                    Options: chain, dag, pq, all")
	fmt.Println("  -network string   Network configuration (default: local)")
	fmt.Println("                    Options: mainnet, testnet, local")
	fmt.Println("  -duration time    Benchmark duration (default: 10s)")
	fmt.Println("  -blocks int       Number of blocks to process (default: 1000)")
	fmt.Println("  -parallel int     Number of parallel workers (default: 1)")
	fmt.Println("  -zmq              Use ZMQ transport if available")
	fmt.Println("  -verbose          Verbose output")
	fmt.Println("  -help             Show this help message")
	fmt.Println("\nExamples:")
	fmt.Println("  bench                                  # Benchmark all engines")
	fmt.Println("  bench -engine chain -blocks 5000       # Benchmark chain engine with 5000 blocks")
	fmt.Println("  bench -engine dag -parallel 4          # Benchmark DAG with 4 workers")
	fmt.Println("  bench -network mainnet -duration 30s   # Use mainnet config for 30s")
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
		fmt.Fprintf(os.Stderr, "Unknown network: %s, using local\n", network)
		return config.LocalParams()
	}
}

func benchmarkChain(ctx context.Context, params config.Parameters, blocks int, parallel int, verbose bool) {
	fmt.Println("=== Chain Engine Benchmark ===")
	engine := chain.New()
	
	start := time.Now()
	if err := engine.Start(ctx, 1); err != nil {
		fmt.Printf("Failed to start chain engine: %v\n", err)
		return
	}
	
	processed := 0
	errors := 0
	
	for i := 0; i < blocks && ctx.Err() == nil; i++ {
		blockID := ids.GenerateTestID()
		err := engine.GetBlock(ctx, ids.EmptyNodeID, 0, blockID)
		if err != nil {
			errors++
			if verbose {
				fmt.Printf("Error processing block %d: %v\n", i, err)
			}
		} else {
			processed++
		}
		
		if verbose && i%100 == 0 {
			fmt.Printf("Processed %d blocks...\n", i)
		}
	}
	
	elapsed := time.Since(start)
	tps := float64(processed) / elapsed.Seconds()
	
	fmt.Printf("Results:\n")
	fmt.Printf("  Processed: %d blocks\n", processed)
	fmt.Printf("  Errors:    %d\n", errors)
	fmt.Printf("  Time:      %s\n", elapsed)
	fmt.Printf("  TPS:       %.2f blocks/sec\n", tps)
	
	_ = engine.Stop(ctx)
}

func benchmarkDAG(ctx context.Context, params config.Parameters, blocks int, parallel int, verbose bool) {
	fmt.Println("=== DAG Engine Benchmark ===")
	engine := dag.New()
	
	start := time.Now()
	if err := engine.Start(ctx, 1); err != nil {
		fmt.Printf("Failed to start DAG engine: %v\n", err)
		return
	}
	
	processed := 0
	errors := 0
	
	for i := 0; i < blocks && ctx.Err() == nil; i++ {
		vertexID := ids.GenerateTestID()
		err := engine.GetVertex(ctx, ids.EmptyNodeID, 0, vertexID)
		if err != nil {
			errors++
			if verbose {
				fmt.Printf("Error processing vertex %d: %v\n", i, err)
			}
		} else {
			processed++
		}
		
		if verbose && i%100 == 0 {
			fmt.Printf("Processed %d vertices...\n", i)
		}
	}
	
	elapsed := time.Since(start)
	tps := float64(processed) / elapsed.Seconds()
	
	fmt.Printf("Results:\n")
	fmt.Printf("  Processed: %d vertices\n", processed)
	fmt.Printf("  Errors:    %d\n", errors)
	fmt.Printf("  Time:      %s\n", elapsed)
	fmt.Printf("  TPS:       %.2f vertices/sec\n", tps)
	
	_ = engine.Stop(ctx)
}

func benchmarkPQ(ctx context.Context, params config.Parameters, blocks int, parallel int, verbose bool) {
	fmt.Println("=== Post-Quantum Engine Benchmark ===")
	engine := pq.New()
	
	start := time.Now()
	if err := engine.Start(ctx, 1); err != nil {
		fmt.Printf("Failed to start PQ engine: %v\n", err)
		return
	}
	
	processed := 0
	errors := 0
	proofSizes := []int{}
	
	for i := 0; i < blocks && ctx.Err() == nil; i++ {
		blockID := ids.GenerateTestID()
		proof, err := engine.GenerateQuantumProof(ctx, blockID)
		if err != nil {
			errors++
			if verbose {
				fmt.Printf("Error generating proof %d: %v\n", i, err)
			}
		} else {
			processed++
			proofSizes = append(proofSizes, len(proof))
		}
		
		if verbose && i%100 == 0 {
			fmt.Printf("Generated %d proofs...\n", i)
		}
	}
	
	elapsed := time.Since(start)
	tps := float64(processed) / elapsed.Seconds()
	
	// Calculate average proof size
	avgProofSize := 0
	if len(proofSizes) > 0 {
		sum := 0
		for _, size := range proofSizes {
			sum += size
		}
		avgProofSize = sum / len(proofSizes)
	}
	
	fmt.Printf("Results:\n")
	fmt.Printf("  Generated: %d proofs\n", processed)
	fmt.Printf("  Errors:    %d\n", errors)
	fmt.Printf("  Time:      %s\n", elapsed)
	fmt.Printf("  TPS:       %.2f proofs/sec\n", tps)
	fmt.Printf("  Avg Proof: %d bytes\n", avgProofSize)
	
	_ = engine.Stop(ctx)
}

func init() {
	// As of Go 1.20, rand.Seed is deprecated - random seeding is automatic
	// No manual seeding required for better randomness
}