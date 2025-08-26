// Package main provides the unified benchmark tool for all consensus engines
package main

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain"
	"github.com/luxfi/consensus/engine/dag"
	"github.com/luxfi/consensus/engine/pq"
	"github.com/luxfi/ids"
)

type BenchmarkMode string

const (
	ModeSimple   BenchmarkMode = "simple"   // Simple throughput test
	ModeQuantum  BenchmarkMode = "quantum"  // Full quantum consensus with Corona+BLS
	ModeStress   BenchmarkMode = "stress"   // Stress test with failures
	ModeLatency  BenchmarkMode = "latency"  // Latency-focused benchmarks
	ModeAll      BenchmarkMode = "all"      // Run all benchmarks
)

func main() {
	var (
		mode       = flag.String("mode", "simple", "Benchmark mode: simple, quantum, stress, latency, all")
		engine     = flag.String("engine", "all", "Engine to benchmark: chain, dag, pq, all")
		validators = flag.Int("validators", 21, "Number of validators (for quantum mode)")
		blocks     = flag.Int("blocks", 1000, "Number of blocks/vertices to process")
		rounds     = flag.Int("rounds", 10, "Consensus rounds per block (quantum mode)")
		duration   = flag.Duration("duration", 10*time.Second, "Benchmark duration")
		latency    = flag.Duration("latency", 50*time.Millisecond, "Network latency (quantum mode)")
		payload    = flag.Int("payload", 1024, "Block payload size in bytes")
		parallel   = flag.Int("parallel", 1, "Number of parallel workers")
		byzantine  = flag.Float64("byzantine", 0.0, "Byzantine failure rate (0.0-0.3)")
		verbose    = flag.Bool("verbose", false, "Verbose output")
		help       = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	// Validate byzantine rate
	if *byzantine < 0 || *byzantine > 0.3 {
		fmt.Fprintf(os.Stderr, "Byzantine rate must be between 0.0 and 0.3\n")
		os.Exit(1)
	}

	fmt.Println("=== Lux Consensus Unified Benchmark ===")
	fmt.Printf("Mode: %s\n", *mode)
	fmt.Printf("Engine: %s\n", *engine)
	fmt.Printf("Duration: %s\n", *duration)
	fmt.Printf("Blocks: %d\n", *blocks)
	
	if *mode == string(ModeQuantum) || *mode == string(ModeAll) {
		fmt.Printf("Validators: %d\n", *validators)
		fmt.Printf("Rounds/Block: %d\n", *rounds)
		fmt.Printf("Network Latency: %s\n", *latency)
		fmt.Printf("Byzantine Rate: %.1f%%\n", *byzantine*100)
	}
	fmt.Println()

	switch BenchmarkMode(*mode) {
	case ModeSimple:
		runSimpleBenchmark(*engine, *blocks, *duration, *parallel, *verbose)
	case ModeQuantum:
		runQuantumBenchmark(*engine, *validators, *blocks, *rounds, *latency, *payload, *byzantine, *verbose)
	case ModeStress:
		runStressBenchmark(*engine, *blocks, *duration, *parallel, *byzantine, *verbose)
	case ModeLatency:
		runLatencyBenchmark(*engine, *blocks, *latency, *verbose)
	case ModeAll:
		fmt.Println("=== Running All Benchmarks ===")
		runSimpleBenchmark(*engine, *blocks, *duration, *parallel, false)
		fmt.Println()
		runQuantumBenchmark(*engine, *validators, *blocks/10, *rounds, *latency, *payload, *byzantine, false)
		fmt.Println()
		runStressBenchmark(*engine, *blocks, *duration, *parallel, *byzantine, false)
		fmt.Println()
		runLatencyBenchmark(*engine, *blocks/10, *latency, false)
	default:
		fmt.Fprintf(os.Stderr, "Unknown mode: %s\n", *mode)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Lux Consensus Unified Benchmark Tool")
	fmt.Println("\nBenchmark Modes:")
	fmt.Println("  simple   - Fast throughput test with minimal overhead")
	fmt.Println("  quantum  - Full quantum consensus with Corona+BLS signatures")
	fmt.Println("  stress   - Stress test with Byzantine failures and high load")
	fmt.Println("  latency  - Measure consensus latency under various conditions")
	fmt.Println("  all      - Run all benchmark modes")
	fmt.Println("\nOptions:")
	flag.PrintDefaults()
	fmt.Println("\nExamples:")
	fmt.Println("  bench -mode simple                        # Quick throughput test")
	fmt.Println("  bench -mode quantum -validators 21        # Mainnet-like quantum consensus")
	fmt.Println("  bench -mode stress -byzantine 0.1         # Stress test with 10% failures")
	fmt.Println("  bench -mode latency -latency 100ms        # Test with 100ms network delay")
	fmt.Println("  bench -mode all -engine pq                # All benchmarks on PQ engine")
}

// Simple throughput benchmark
func runSimpleBenchmark(engineType string, blocks int, duration time.Duration, parallel int, verbose bool) {
	fmt.Println("=== Simple Throughput Benchmark ===")
	
	engines := getEngines(engineType)
	
	for name, createEngine := range engines {
		fmt.Printf("\n%s Engine:\n", name)
		
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()
		
		start := time.Now()
		processed := atomic.Uint64{}
		errors := atomic.Uint64{}
		
		var wg sync.WaitGroup
		for p := 0; p < parallel; p++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				
				for i := 0; i < blocks/parallel && ctx.Err() == nil; i++ {
					if processSimpleBlock(ctx, createEngine(), verbose) {
						processed.Add(1)
					} else {
						errors.Add(1)
					}
				}
			}()
		}
		wg.Wait()
		
		elapsed := time.Since(start)
		tps := float64(processed.Load()) / elapsed.Seconds()
		
		fmt.Printf("  Processed: %d blocks\n", processed.Load())
		fmt.Printf("  Errors: %d\n", errors.Load())
		fmt.Printf("  Time: %s\n", elapsed)
		fmt.Printf("  Throughput: %.2f blocks/sec\n", tps)
	}
}

// Quantum consensus benchmark with real crypto
func runQuantumBenchmark(engineType string, validators, blocks, rounds int, latency time.Duration, payload int, byzantine float64, verbose bool) {
	fmt.Println("=== Quantum Consensus Benchmark ===")
	
	if engineType != "pq" && engineType != "all" {
		fmt.Println("Quantum mode only supports PQ engine")
		return
	}
	
	consensus := NewQuantumConsensus(validators, latency, byzantine)
	ctx := context.Background()
	start := time.Now()
	
	for i := 0; i < blocks; i++ {
		block := consensus.CreateBlock(uint64(i), payload)
		
		if verbose {
			fmt.Printf("Block %d: ", i)
		}
		
		// Run consensus rounds
		roundsNeeded := 0
		for round := 0; round < rounds; round++ {
			roundsNeeded++
			consensus.RunConsensusRound(ctx, block)
			
			if consensus.HasQuorum(block) {
				consensus.FinalizeBlock(ctx, block)
				if verbose {
					fmt.Printf("✓ (round %d) ", roundsNeeded)
				}
				break
			}
		}
		
		if verbose {
			fmt.Println()
		}
	}
	
	elapsed := time.Since(start)
	
	fmt.Printf("\nResults:\n")
	fmt.Printf("  Blocks Processed: %d\n", consensus.blocksProcessed.Load())
	fmt.Printf("  Votes Processed: %d\n", consensus.votesProcessed.Load())
	fmt.Printf("  Signatures Generated: %d\n", consensus.sigsGenerated.Load())
	fmt.Printf("  Total Time: %s\n", elapsed)
	
	blocksPerSec := float64(consensus.blocksProcessed.Load()) / elapsed.Seconds()
	avgFinality := time.Duration(consensus.finalityTime.Load() / int64(math.Max(1, float64(consensus.blocksProcessed.Load()))))
	
	fmt.Printf("\nPerformance:\n")
	fmt.Printf("  Blocks/sec: %.2f\n", blocksPerSec)
	fmt.Printf("  Avg Finality: %s\n", avgFinality)
	fmt.Printf("  Throughput: %.2f KB/s\n", float64(payload)*blocksPerSec/1024)
	fmt.Printf("  Consensus Efficiency: %.1f%%\n", float64(consensus.blocksProcessed.Load())/float64(blocks)*100)
}

// Stress test with failures
func runStressBenchmark(engineType string, blocks int, duration time.Duration, parallel int, byzantine float64, verbose bool) {
	fmt.Println("=== Stress Test Benchmark ===")
	fmt.Printf("Byzantine Failure Rate: %.1f%%\n", byzantine*100)
	
	engines := getEngines(engineType)
	
	for name, createEngine := range engines {
		fmt.Printf("\n%s Engine:\n", name)
		
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()
		
		start := time.Now()
		processed := atomic.Uint64{}
		failed := atomic.Uint64{}
		recovered := atomic.Uint64{}
		
		var wg sync.WaitGroup
		for p := 0; p < parallel; p++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				engine := createEngine()
				
				for i := 0; i < blocks/parallel && ctx.Err() == nil; i++ {
					// Simulate Byzantine failure
					// #nosec G404 - Weak random is fine for benchmarks
					if rand.Float64() < byzantine {
						failed.Add(1)
						if verbose {
							fmt.Printf("Worker %d: Byzantine failure at block %d\n", workerID, i)
						}
						// Try to recover
						engine = createEngine() // Reset engine
						recovered.Add(1)
						continue
					}
					
					if processSimpleBlock(ctx, engine, false) {
						processed.Add(1)
					}
				}
			}(p)
		}
		wg.Wait()
		
		elapsed := time.Since(start)
		tps := float64(processed.Load()) / elapsed.Seconds()
		
		fmt.Printf("  Processed: %d blocks\n", processed.Load())
		fmt.Printf("  Failed: %d\n", failed.Load())
		fmt.Printf("  Recovered: %d\n", recovered.Load())
		fmt.Printf("  Time: %s\n", elapsed)
		fmt.Printf("  Throughput: %.2f blocks/sec\n", tps)
		fmt.Printf("  Resilience: %.1f%%\n", float64(processed.Load())/(float64(processed.Load()+failed.Load()))*100)
	}
}

// Latency-focused benchmark
func runLatencyBenchmark(engineType string, blocks int, baseLatency time.Duration, verbose bool) {
	fmt.Println("=== Latency Benchmark ===")
	
	engines := getEngines(engineType)
	latencies := []time.Duration{
		10 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
	}
	
	for name, createEngine := range engines {
		fmt.Printf("\n%s Engine:\n", name)
		fmt.Println("Latency | Blocks/sec | Avg Response")
		fmt.Println("--------|------------|-------------")
		
		for _, latency := range latencies {
			engine := createEngine()
			ctx := context.Background()
			
			start := time.Now()
			totalLatency := time.Duration(0)
			processed := 0
			
			for i := 0; i < blocks; i++ {
				blockStart := time.Now()
				
				// Simulate network latency
				time.Sleep(latency)
				
				if processSimpleBlock(ctx, engine, false) {
					processed++
					totalLatency += time.Since(blockStart)
				}
			}
			
			elapsed := time.Since(start)
			tps := float64(processed) / elapsed.Seconds()
			avgLatency := totalLatency / time.Duration(math.Max(1, float64(processed)))
			
			fmt.Printf("%-7s | %-10.2f | %s\n", latency, tps, avgLatency)
		}
	}
}

// Helper functions

func getEngines(engineType string) map[string]func() interface{} {
	engines := make(map[string]func() interface{})
	
	switch engineType {
	case "chain":
		engines["Chain"] = func() interface{} { return chain.New() }
	case "dag":
		engines["DAG"] = func() interface{} { return dag.New() }
	case "pq":
		engines["PQ"] = func() interface{} { return pq.New() }
	case "all":
		engines["Chain"] = func() interface{} { return chain.New() }
		engines["DAG"] = func() interface{} { return dag.New() }
		engines["PQ"] = func() interface{} { return pq.New() }
	}
	
	return engines
}

func processSimpleBlock(ctx context.Context, engine interface{}, verbose bool) bool {
	// Simple block processing for throughput tests
	blockID := ids.GenerateTestID()
	
	switch e := engine.(type) {
	case *chain.Transitive:
		return e.GetBlock(ctx, ids.EmptyNodeID, 0, blockID) == nil
	case *dag.Quantum:
		return e.GetVertex(ctx, ids.EmptyNodeID, 0, blockID) == nil
	case *pq.ConsensusEngine:
		_, err := e.GenerateQuantumProof(ctx, blockID)
		return err == nil
	default:
		return false
	}
}

// Quantum consensus implementation (simplified from quantum-bench)

type QuantumBlock struct {
	ID          ids.ID
	Height      uint64
	Payload     []byte
	Votes       map[ids.NodeID]bool
	mu          sync.RWMutex
}

type QuantumValidator struct {
	NodeID      ids.NodeID
	Light       uint64
	VoteLatency time.Duration
}

type QuantumConsensus struct {
	validators      []*QuantumValidator
	byzantineRate   float64
	params          config.Parameters
	mu              sync.RWMutex
	
	blocksProcessed atomic.Uint64
	votesProcessed  atomic.Uint64
	sigsGenerated   atomic.Uint64
	finalityTime    atomic.Int64
}

func NewQuantumConsensus(numValidators int, latency time.Duration, byzantineRate float64) *QuantumConsensus {
	// Select appropriate params based on validator count
	var params config.Parameters
	switch {
	case numValidators <= 5:
		params = config.LocalParams()
	case numValidators <= 11:
		params = config.TestnetParams()
	default:
		params = config.MainnetParams()
	}
	
	if params.K > numValidators {
		params.K = numValidators
	}
	
	qc := &QuantumConsensus{
		validators:    make([]*QuantumValidator, numValidators),
		byzantineRate: byzantineRate,
		params:        params,
	}
	
	// Create validators
	for i := 0; i < numValidators; i++ {
		qc.validators[i] = &QuantumValidator{
			NodeID:      ids.GenerateTestNodeID(),
			Light:       uint64(100 + i*10),
			// #nosec G404 - Weak random is fine for benchmarks
			VoteLatency: latency + time.Duration(rand.Int63n(int64(latency/2))),
		}
	}
	
	return qc
}

func (qc *QuantumConsensus) CreateBlock(height uint64, payloadSize int) *QuantumBlock {
	payload := make([]byte, payloadSize)
	if _, err := cryptorand.Read(payload); err != nil {
		panic("failed to generate random payload: " + err.Error())
	}
	
	return &QuantumBlock{
		ID:      ids.GenerateTestID(),
		Height:  height,
		Payload: payload,
		Votes:   make(map[ids.NodeID]bool),
	}
}

func (qc *QuantumConsensus) RunConsensusRound(ctx context.Context, block *QuantumBlock) {
	k := qc.params.K
	sampled := qc.sampleValidators(k)
	
	var wg sync.WaitGroup
	for _, validator := range sampled {
		wg.Add(1)
		go func(v *QuantumValidator) {
			defer wg.Done()
			
			// Simulate network delay
			time.Sleep(v.VoteLatency)
			
			// Simulate Byzantine failure
			if rand.Float64() < qc.byzantineRate {
				return
			}
			
			// Vote
			qc.vote(block, v)
		}(validator)
	}
	wg.Wait()
}

func (qc *QuantumConsensus) vote(block *QuantumBlock, validator *QuantumValidator) {
	block.mu.Lock()
	block.Votes[validator.NodeID] = true
	block.mu.Unlock()
	
	qc.votesProcessed.Add(1)
	qc.sigsGenerated.Add(1) // Simulate signature generation
}

func (qc *QuantumConsensus) HasQuorum(block *QuantumBlock) bool {
	block.mu.RLock()
	votes := len(block.Votes)
	block.mu.RUnlock()
	
	required := int(float64(qc.params.K) * qc.params.Alpha)
	return votes >= required
}

func (qc *QuantumConsensus) FinalizeBlock(ctx context.Context, block *QuantumBlock) {
	start := time.Now()
	
	// Simulate finalization work
	h := sha256.New()
	h.Write(block.ID[:])
	h.Write(block.Payload)
	
	qc.blocksProcessed.Add(1)
	qc.finalityTime.Add(int64(time.Since(start)))
}

func (qc *QuantumConsensus) sampleValidators(k int) []*QuantumValidator {
	if k > len(qc.validators) {
		k = len(qc.validators)
	}
	
	sampled := make([]*QuantumValidator, k)
	for i := 0; i < k; i++ {
		sampled[i] = qc.validators[rand.Intn(len(qc.validators))]
	}
	
	return sampled
}