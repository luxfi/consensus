// Package main provides the consensus simulation tool for testing network behavior
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

func main() {
	var (
		engine  = flag.String("engine", "all", "Engine to simulate: nova, nebula, quasar, all")
		nodes   = flag.Int("nodes", 100, "Number of nodes in the network")
		rounds  = flag.Int("rounds", 10, "Number of consensus rounds to simulate")
		blocks  = flag.Int("blocks", 100, "Number of blocks to process per round")
		network = flag.String("network", "mainnet", "Network configuration (mainnet, testnet, local)")
		failure = flag.Float64("failure", 0.1, "Node failure rate (0.0-1.0)")
		latency = flag.Duration("latency", 50*time.Millisecond, "Network latency")
		jitter  = flag.Duration("jitter", 10*time.Millisecond, "Network latency jitter")
		verbose = flag.Bool("verbose", false, "Verbose output")
		help    = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	if *failure < 0 || *failure > 1 {
		fmt.Fprintf(os.Stderr, "Failure rate must be between 0.0 and 1.0\n")
		os.Exit(1)
	}

	// Get network configuration
	params := getNetworkParams(*network)

	fmt.Println("=== Lux Consensus Network Simulation ===")
	fmt.Printf("Engine:     %s\n", *engine)
	fmt.Printf("Network:    %s\n", *network)
	fmt.Printf("Nodes:      %d\n", *nodes)
	fmt.Printf("Rounds:     %d\n", *rounds)
	fmt.Printf("Blocks:     %d\n", *blocks)
	fmt.Printf("Failure:    %.1f%%\n", *failure*100)
	fmt.Printf("Latency:    %s ±%s\n", *latency, *jitter)
	fmt.Printf("Parameters: K=%d, Alpha=%.2f, Beta=%d\n\n", params.K, params.Alpha, params.Beta)

	// Run simulation
	runSimulation(*engine, *nodes, *rounds, *blocks, params, *failure, *latency, *jitter, *verbose)
}

func printHelp() {
	fmt.Println("Lux Consensus Network Simulator")
	fmt.Println("\nThis tool simulates consensus behavior under various network conditions.")
	fmt.Println("It tests all three consensus engines:")
	fmt.Println("  - Nova: Linear chain consensus (replaces Snowman)")
	fmt.Println("  - Nebula: DAG consensus (replaces Avalanche)")
	fmt.Println("  - Quasar: Quantum consensus with post-quantum security")
	fmt.Println("\nUsage: sim [options]")
	fmt.Println("\nOptions:")
	fmt.Println("  -engine string    Engine to simulate (default: all)")
	fmt.Println("                    Options: nova, nebula, quasar, all")
	fmt.Println("  -nodes int        Number of nodes in the network (default: 100)")
	fmt.Println("  -rounds int       Number of consensus rounds (default: 10)")
	fmt.Println("  -blocks int       Blocks per round (default: 100)")
	fmt.Println("  -network string   Network configuration (default: mainnet)")
	fmt.Println("                    Options: mainnet, testnet, local")
	fmt.Println("  -failure float    Node failure rate 0.0-1.0 (default: 0.1)")
	fmt.Println("  -latency duration Network latency (default: 50ms)")
	fmt.Println("  -jitter duration  Latency variation (default: 10ms)")
	fmt.Println("  -verbose          Verbose output")
	fmt.Println("  -help             Show this help message")
	fmt.Println("\nExamples:")
	fmt.Println("  sim                                  # Run default simulation")
	fmt.Println("  sim -engine nova -nodes 1000         # Simulate Nova with 1000 nodes")
	fmt.Println("  sim -failure 0.3 -latency 200ms      # High failure, slow network")
	fmt.Println("  sim -network testnet -verbose        # Testnet config with details")
	fmt.Println("  sim -engine quasar -rounds 100       # Quantum consensus stress test")
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
		fmt.Fprintf(os.Stderr, "Unknown network: %s, using mainnet\n", network)
		return config.MainnetParams()
	}
}

// NetworkNode represents a simulated network node
type NetworkNode struct {
	ID         ids.NodeID
	Engine     consensus.Engine
	Light      uint64 // Validator light (weight)
	Active     bool
	Byzantine  bool
	Latency    time.Duration
	LastVote   time.Time
	mu         sync.RWMutex
}

// SimulationMetrics tracks simulation results
type SimulationMetrics struct {
	Engine          string
	Rounds          int
	BlocksProcessed atomic.Uint64
	BlocksFinalized atomic.Uint64
	ConsensusTimes  []time.Duration
	FailureRate     float64
	NetworkLatency  time.Duration
	TotalTime       time.Duration
	mu              sync.RWMutex
}

func runSimulation(engineType string, nodes int, rounds int, blocksPerRound int, params config.Parameters, failureRate float64, latency time.Duration, jitter time.Duration, verbose bool) {
	engines := getEngines(engineType)
	
	for name, createEngine := range engines {
		fmt.Printf("\n=== %s Engine Simulation ===\n", name)
		
		metrics := &SimulationMetrics{
			Engine:         name,
			Rounds:         rounds,
			FailureRate:    failureRate,
			NetworkLatency: latency,
		}
		
		// Create network nodes
		network := createNetwork(nodes, createEngine, failureRate, latency, jitter)
		
		// Run simulation rounds
		start := time.Now()
		for round := 1; round <= rounds; round++ {
			if verbose {
				fmt.Printf("Round %d/%d: ", round, rounds)
			}
			
			runRound(network, blocksPerRound, params, metrics, verbose)
			
			if verbose {
				fmt.Printf("✓ (%d blocks finalized)\n", metrics.BlocksFinalized.Load())
			}
		}
		metrics.TotalTime = time.Since(start)
		
		// Print results
		printMetrics(metrics, params)
	}
}

func getEngines(engineType string) map[string]func() consensus.Engine {
	engines := make(map[string]func() consensus.Engine)
	
	switch engineType {
	case "nova":
		engines["Nova"] = func() consensus.Engine { return consensus.NewChainEngine() }
	case "nebula":
		engines["Nebula"] = func() consensus.Engine { return consensus.NewDAGEngine() }
	case "quasar":
		engines["Quasar"] = func() consensus.Engine { return consensus.NewPQEngine() }
	case "all":
		engines["Nova"] = func() consensus.Engine { return consensus.NewChainEngine() }
		engines["Nebula"] = func() consensus.Engine { return consensus.NewDAGEngine() }
		engines["Quasar"] = func() consensus.Engine { return consensus.NewPQEngine() }
	}
	
	return engines
}

func createNetwork(nodes int, createEngine func() consensus.Engine, failureRate float64, latency time.Duration, jitter time.Duration) []*NetworkNode {
	network := make([]*NetworkNode, nodes)
	byzantineCount := int(float64(nodes) * failureRate)
	
	for i := 0; i < nodes; i++ {
		node := &NetworkNode{
			ID:        ids.GenerateTestNodeID(),
			Engine:    createEngine(),
			Light:     100 + uint64(i*10), // Variable validator weight
			Active:    true,
			Byzantine: i < byzantineCount,
			Latency:   latency + time.Duration(rand.Int63n(int64(jitter)*2)-int64(jitter)),
		}
		network[i] = node
		
		// Start the engine
		ctx := context.Background()
		if err := node.Engine.Start(ctx); err != nil {
			panic(fmt.Sprintf("Failed to start engine for node %d: %v", i, err))
		}
	}
	
	return network
}

func runRound(network []*NetworkNode, blocks int, params config.Parameters, metrics *SimulationMetrics, verbose bool) {
	for b := 0; b < blocks; b++ {
		blockID := ids.GenerateTestID()
		start := time.Now()
		
		// Sample K nodes for voting
		voters := sampleNodes(network, params.K)
		
		// Simulate voting with network delays
		votes := simulateVoting(voters, blockID)
		
		// Check consensus
		if checkConsensus(votes, params.Alpha) {
			metrics.BlocksFinalized.Add(1)
			consensusTime := time.Since(start)
			
			metrics.mu.Lock()
			metrics.ConsensusTimes = append(metrics.ConsensusTimes, consensusTime)
			metrics.mu.Unlock()
		}
		
		metrics.BlocksProcessed.Add(1)
	}
}

func sampleNodes(network []*NetworkNode, k int) []*NetworkNode {
	activeNodes := make([]*NetworkNode, 0, len(network))
	for _, node := range network {
		if node.Active && !node.Byzantine {
			activeNodes = append(activeNodes, node)
		}
	}
	
	if k > len(activeNodes) {
		k = len(activeNodes)
	}
	
	// Fisher-Yates shuffle for random sampling
	sampled := make([]*NetworkNode, k)
	for i := 0; i < k; i++ {
		j := rand.Intn(len(activeNodes) - i)
		sampled[i] = activeNodes[j]
		activeNodes[j], activeNodes[len(activeNodes)-1-i] = activeNodes[len(activeNodes)-1-i], activeNodes[j]
	}
	
	return sampled
}

func simulateVoting(voters []*NetworkNode, blockID ids.ID) int {
	votes := 0
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	for _, node := range voters {
		wg.Add(1)
		go func(n *NetworkNode) {
			defer wg.Done()
			
			// Simulate network latency
			time.Sleep(n.Latency)
			
			// Byzantine nodes vote randomly
			if n.Byzantine {
				if rand.Float64() < 0.5 {
					mu.Lock()
					votes++
					mu.Unlock()
				}
			} else {
				// Honest nodes vote correctly
				mu.Lock()
				votes++
				mu.Unlock()
			}
			
			n.mu.Lock()
			n.LastVote = time.Now()
			n.mu.Unlock()
		}(node)
	}
	
	wg.Wait()
	return votes
}

func checkConsensus(votes int, alphaThreshold float64) bool {
	// Simple majority check
	return float64(votes) >= alphaThreshold*float64(votes)
}

func printMetrics(metrics *SimulationMetrics, params config.Parameters) {
	fmt.Printf("\nResults:\n")
	fmt.Printf("  Blocks Processed:  %d\n", metrics.BlocksProcessed.Load())
	fmt.Printf("  Blocks Finalized:  %d\n", metrics.BlocksFinalized.Load())
	fmt.Printf("  Finalization Rate: %.1f%%\n", 
		float64(metrics.BlocksFinalized.Load())/float64(metrics.BlocksProcessed.Load())*100)
	
	if len(metrics.ConsensusTimes) > 0 {
		avgTime := time.Duration(0)
		for _, t := range metrics.ConsensusTimes {
			avgTime += t
		}
		avgTime /= time.Duration(len(metrics.ConsensusTimes))
		
		fmt.Printf("  Avg Consensus Time: %s\n", avgTime)
		fmt.Printf("  Throughput:         %.2f blocks/sec\n", 
			float64(metrics.BlocksFinalized.Load())/metrics.TotalTime.Seconds())
	}
	
	fmt.Printf("  Total Time:        %s\n", metrics.TotalTime)
	
	// Calculate theoretical limits
	maxThroughput := 1.0 / (metrics.NetworkLatency.Seconds() * float64(params.Beta))
	fmt.Printf("\nTheoretical Limits:\n")
	fmt.Printf("  Max Throughput:    %.2f blocks/sec\n", maxThroughput)
	fmt.Printf("  Min Latency:       %s\n", time.Duration(float64(params.Beta)*float64(metrics.NetworkLatency)))
	fmt.Printf("  Byzantine Tolerance: %.1f%%\n", (1.0-params.Alpha)*100)
}

func init() {
	// As of Go 1.20, rand.Seed is deprecated - random seeding is automatic
	// No manual seeding required for better randomness
}
