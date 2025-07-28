// Copyright (C) 2024-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/spf13/cobra"
	zmq "github.com/pebbe/zmq4"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/factories"
	"github.com/luxfi/consensus/poll"
	"github.com/luxfi/ids"
)

// BenchNode represents a validator in the benchmark
type BenchNode struct {
	ID        ids.ID
	PublicKey string
	Consensus poll.Unary
	Choice    ids.ID
	Finalized bool
}

// BenchMessage for network communication
type BenchMessage struct {
	Type       string            `json:"type"`
	NodeID     string            `json:"node_id,omitempty"`
	PublicKey  string            `json:"public_key,omitempty"`
	Cores      int               `json:"cores,omitempty"`
	Memory     int64             `json:"memory,omitempty"`
	Validators []ValidatorInfo   `json:"validators,omitempty"`
	Genesis    *BenchGenesis     `json:"genesis,omitempty"`
	Round      int               `json:"round,omitempty"`
	Votes      map[string]string `json:"votes,omitempty"`
	Stats      *BenchStats       `json:"stats,omitempty"`
}

// ValidatorInfo for genesis creation
type ValidatorInfo struct {
	NodeID    string `json:"node_id"`
	PublicKey string `json:"public_key"`
	Stake     uint64 `json:"stake"`
}

// BenchGenesis for benchmark network
type BenchGenesis struct {
	NetworkID  uint32          `json:"network_id"`
	Validators []ValidatorInfo `json:"validators"`
	Params     config.Parameters `json:"params"`
	StartTime  time.Time       `json:"start_time"`
}

// BenchStats for performance tracking
type BenchStats struct {
	Rounds          int           `json:"rounds"`
	FinalizedNodes  int           `json:"finalized_nodes"`
	Duration        time.Duration `json:"duration"`
	MessagesPerSec  float64       `json:"messages_per_sec"`
	RoundsPerSec    float64       `json:"rounds_per_sec"`
	ConsensusChoice string        `json:"consensus_choice"`
}

func benchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bench [coordinator-address] [total-nodes]",
		Short: "Run distributed consensus benchmark",
		Long: `Run a distributed consensus benchmark across multiple machines.
Each machine will auto-detect cores and run multiple validators.

Example:
  # On coordinator machine:
  consensus bench :5555 100

  # On worker machines:
  consensus bench 192.168.1.10:5555 100
`,
		Args: cobra.ExactArgs(2),
		RunE: runBench,
	}

	cmd.Flags().Int("validators-per-core", 10, "Number of validators to run per CPU core")
	cmd.Flags().Int("rounds", 100, "Maximum consensus rounds")
	cmd.Flags().Duration("timeout", 5*time.Minute, "Benchmark timeout")
	cmd.Flags().String("factory", "confidence", "Consensus factory: confidence, threshold")
	cmd.Flags().Bool("verbose", false, "Enable verbose logging")

	return cmd
}

func runBench(cmd *cobra.Command, args []string) error {
	address := args[0]
	totalNodes := 0
	fmt.Sscanf(args[1], "%d", &totalNodes)

	validatorsPerCore, _ := cmd.Flags().GetInt("validators-per-core")
	rounds, _ := cmd.Flags().GetInt("rounds")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	factory, _ := cmd.Flags().GetString("factory")
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Detect system resources
	cores := runtime.NumCPU()
	memory := getSystemMemory()
	localValidators := cores * validatorsPerCore

	fmt.Printf("=== Consensus Benchmark ===\n")
	fmt.Printf("Address: %s\n", address)
	fmt.Printf("Total nodes: %d\n", totalNodes)
	fmt.Printf("Local cores: %d\n", cores)
	fmt.Printf("Memory: %d GB\n", memory/(1024*1024*1024))
	fmt.Printf("Local validators: %d (%d per core)\n", localValidators, validatorsPerCore)
	fmt.Printf("Max rounds: %d\n", rounds)
	fmt.Printf("Factory: %s\n", factory)

	// Check if we're coordinator or worker
	if isCoordinator(address) {
		return runBenchCoordinator(address, totalNodes, localValidators, rounds, timeout, factory, verbose)
	} else {
		return runBenchWorker(address, totalNodes, localValidators, rounds, timeout, factory, verbose)
	}
}

func runBenchCoordinator(address string, totalNodes, localValidators, maxRounds int, timeout time.Duration, factoryType string, verbose bool) error {
	fmt.Println("\n=== Running as COORDINATOR ===")

	// Create ZMQ socket
	ctx, _ := zmq.NewContext()
	socket, err := ctx.NewSocket(zmq.ROUTER)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	defer socket.Close()

	if err := socket.Bind("tcp://" + address); err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}

	fmt.Printf("Listening on %s...\n", address)

	// Collect worker information
	workers := make(map[string]*WorkerInfo)
	validators := make([]ValidatorInfo, 0, totalNodes)
	
	// Generate local validators first
	fmt.Printf("\nGenerating %d local validators...\n", localValidators)
	localNodes := generateValidators(localValidators)
	for _, node := range localNodes {
		validators = append(validators, ValidatorInfo{
			NodeID:    node.ID.String(),
			PublicKey: node.PublicKey,
			Stake:     2000, // Default stake
		})
	}

	// Add self as worker
	workers["coordinator"] = &WorkerInfo{
		Identity: "coordinator",
		Cores:    runtime.NumCPU(),
		Memory:   getSystemMemory(),
		NumNodes: localValidators,
	}

	// Wait for workers
	fmt.Printf("\nWaiting for workers to provide %d validators total...\n", totalNodes)
	collectedValidators := localValidators
	
	timeoutChan := time.After(30 * time.Second)
	for collectedValidators < totalNodes {
		select {
		case <-timeoutChan:
			if collectedValidators == localValidators {
				fmt.Println("No workers connected, running local benchmark only")
				totalNodes = localValidators
			} else {
				fmt.Printf("Timeout: proceeding with %d validators\n", collectedValidators)
				totalNodes = collectedValidators
			}
			break

		default:
			socket.SetRcvtimeo(100 * time.Millisecond)
			msg, err := socket.RecvMessage(0)
			if err != nil {
				continue
			}

			if len(msg) < 2 {
				continue
			}

			identity := msg[0]
			data := msg[1]

			var benchMsg BenchMessage
			if err := json.Unmarshal([]byte(data), &benchMsg); err != nil {
				continue
			}

			if benchMsg.Type == "register" {
				workers[identity] = &WorkerInfo{
					Identity: identity,
					Cores:    benchMsg.Cores,
					Memory:   benchMsg.Memory,
					NumNodes: len(benchMsg.Validators),
				}

				validators = append(validators, benchMsg.Validators...)
				collectedValidators += len(benchMsg.Validators)

				fmt.Printf("Worker registered: %d cores, %d validators (total: %d/%d)\n",
					benchMsg.Cores, len(benchMsg.Validators), collectedValidators, totalNodes)
			}
		}
	}

	// Create genesis
	genesis := &BenchGenesis{
		NetworkID:  99999, // Benchmark network ID
		Validators: validators[:totalNodes],
		Params:     config.DefaultParameters,
		StartTime:  time.Now(),
	}

	// Adjust parameters for network size
	if totalNodes > 50 {
		genesis.Params.K = 21
		genesis.Params.AlphaPreference = 13
		genesis.Params.AlphaConfidence = 18
	} else if totalNodes > 20 {
		genesis.Params.K = 11
		genesis.Params.AlphaPreference = 7
		genesis.Params.AlphaConfidence = 9
	}

	fmt.Printf("\n=== Genesis Configuration ===\n")
	fmt.Printf("Network ID: %d\n", genesis.NetworkID)
	fmt.Printf("Validators: %d\n", len(genesis.Validators))
	fmt.Printf("K: %d\n", genesis.Params.K)
	fmt.Printf("Start time: %s\n", genesis.StartTime.Format(time.RFC3339))

	// Broadcast genesis to workers
	genesisMsg := BenchMessage{
		Type:    "genesis",
		Genesis: genesis,
	}
	genesisData, _ := json.Marshal(genesisMsg)

	for identity := range workers {
		if identity != "coordinator" {
			socket.SendMessage(identity, genesisData)
		}
	}

	// Initialize local nodes with genesis
	fmt.Printf("\nInitializing consensus for %d local validators...\n", localValidators)
	if err := initializeConsensus(localNodes, genesis, factoryType); err != nil {
		return err
	}

	// Run benchmark
	fmt.Println("\n=== Starting Benchmark ===")
	stats, err := runBenchmarkRounds(socket, workers, localNodes, genesis, maxRounds, timeout, verbose)
	if err != nil {
		return err
	}

	// Display results
	displayBenchmarkResults(stats, len(genesis.Validators))

	// Send shutdown
	shutdownMsg := BenchMessage{Type: "shutdown", Stats: stats}
	shutdownData, _ := json.Marshal(shutdownMsg)
	for identity := range workers {
		if identity != "coordinator" {
			socket.SendMessage(identity, shutdownData)
		}
	}

	return nil
}

func runBenchWorker(address string, totalNodes, localValidators, maxRounds int, timeout time.Duration, factoryType string, verbose bool) error {
	fmt.Println("\n=== Running as WORKER ===")

	// Generate validators
	fmt.Printf("Generating %d validators...\n", localValidators)
	localNodes := generateValidators(localValidators)
	
	validatorInfos := make([]ValidatorInfo, len(localNodes))
	for i, node := range localNodes {
		validatorInfos[i] = ValidatorInfo{
			NodeID:    node.ID.String(),
			PublicKey: node.PublicKey,
			Stake:     2000,
		}
	}

	// Connect to coordinator
	ctx, _ := zmq.NewContext()
	socket, err := ctx.NewSocket(zmq.DEALER)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	defer socket.Close()

	// Set identity
	hostname, _ := getHostname()
	identity := fmt.Sprintf("worker-%s-%d", hostname, time.Now().Unix())
	socket.SetIdentity(identity)

	fmt.Printf("Connecting to coordinator at %s...\n", address)
	if err := socket.Connect("tcp://" + address); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Register with coordinator
	registerMsg := BenchMessage{
		Type:       "register",
		Cores:      runtime.NumCPU(),
		Memory:     getSystemMemory(),
		Validators: validatorInfos,
	}
	registerData, _ := json.Marshal(registerMsg)
	socket.SendMessage(registerData)

	fmt.Println("Registered with coordinator, waiting for genesis...")

	// Wait for genesis
	msg, err := socket.RecvMessage(0)
	if err != nil {
		return fmt.Errorf("failed to receive genesis: %w", err)
	}

	var genesisMsg BenchMessage
	if err := json.Unmarshal([]byte(msg[0]), &genesisMsg); err != nil {
		return fmt.Errorf("failed to parse genesis: %w", err)
	}

	if genesisMsg.Type != "genesis" || genesisMsg.Genesis == nil {
		return fmt.Errorf("invalid genesis message")
	}

	genesis := genesisMsg.Genesis
	fmt.Printf("\nReceived genesis with %d validators\n", len(genesis.Validators))

	// Initialize consensus
	if err := initializeConsensus(localNodes, genesis, factoryType); err != nil {
		return err
	}

	// Participate in benchmark
	fmt.Println("\nParticipating in benchmark...")
	return participateInBenchmark(socket, localNodes, genesis, verbose)
}

func generateValidators(count int) []*BenchNode {
	nodes := make([]*BenchNode, count)
	
	for i := 0; i < count; i++ {
		// Generate key pair
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			panic(err)
		}

		// Create node ID from public key
		nodeID := ids.ID{}
		copy(nodeID[:], pubKey[:32])

		nodes[i] = &BenchNode{
			ID:        nodeID,
			PublicKey: hex.EncodeToString(pubKey),
		}
	}

	return nodes
}

func initializeConsensus(nodes []*BenchNode, genesis *BenchGenesis, factoryType string) error {
	// Select factory
	var factory poll.Factory
	switch factoryType {
	case "confidence":
		factory = factories.ConfidenceFactory
	case "threshold":
		factory = factories.FlatFactory
	default:
		factory = poll.DefaultFactory
	}

	// Convert parameters
	pollParams := poll.ConvertConfigParams(genesis.Params)

	// Initialize consensus for each node
	// Split nodes into two groups with different initial choices
	choice0 := ids.GenerateTestID()
	choice1 := ids.GenerateTestID()

	for i, node := range nodes {
		node.Consensus = factory.NewUnary(pollParams)
		if i%2 == 0 {
			node.Choice = choice0
		} else {
			node.Choice = choice1
		}
		node.Finalized = false
	}

	return nil
}

func runBenchmarkRounds(socket *zmq.Socket, workers map[string]*WorkerInfo, localNodes []*BenchNode, genesis *BenchGenesis, maxRounds int, timeout time.Duration, verbose bool) (*BenchStats, error) {
	startTime := time.Now()
	totalMessages := 0
	finalizedNodes := 0

	// Track choices globally
	nodeChoices := make(map[string]ids.ID)
	for _, node := range localNodes {
		nodeChoices[node.ID.String()] = node.Choice
	}

	for round := 0; round < maxRounds; round++ {
		roundStart := time.Now()

		// Check if consensus reached
		if finalizedNodes >= len(genesis.Validators)*9/10 { // 90% finalized
			fmt.Printf("\nConsensus reached after %d rounds\n", round)
			break
		}

		// Broadcast round start
		roundMsg := BenchMessage{
			Type:  "round_start",
			Round: round,
		}
		roundData, _ := json.Marshal(roundMsg)

		for identity := range workers {
			if identity != "coordinator" {
				socket.SendMessage(identity, roundData)
			}
		}

		// Process local nodes
		localVotes := processLocalRound(localNodes, nodeChoices, genesis.Params.K)
		totalMessages += len(localNodes) * genesis.Params.K

		// Collect votes from workers
		workerVotes := make(map[string]map[string]string)
		workerVotes["coordinator"] = localVotes

		// Wait for worker votes with timeout
		workerTimeout := time.After(100 * time.Millisecond)
		receivedWorkers := 1 // coordinator

		for receivedWorkers < len(workers) {
			select {
			case <-workerTimeout:
				if verbose {
					fmt.Printf("Round %d: timeout waiting for workers\n", round)
				}
				goto ProcessVotes

			default:
				socket.SetRcvtimeo(10 * time.Millisecond)
				msg, err := socket.RecvMessage(0)
				if err != nil {
					continue
				}

				if len(msg) < 2 {
					continue
				}

				identity := msg[0]
				data := msg[1]

				var voteMsg BenchMessage
				if err := json.Unmarshal([]byte(data), &voteMsg); err != nil {
					continue
				}

				if voteMsg.Type == "votes" && voteMsg.Round == round {
					workerVotes[identity] = voteMsg.Votes
					receivedWorkers++
					
					// Update global choices
					for nodeID, choice := range voteMsg.Votes {
						if id, err := ids.FromString(choice); err == nil {
							nodeChoices[nodeID] = id
						}
					}
				}
			}
		}

	ProcessVotes:
		// Distribute aggregated votes back to workers
		allVotes := make(map[string]string)
		for _, votes := range workerVotes {
			for k, v := range votes {
				allVotes[k] = v
			}
		}

		votesMsg := BenchMessage{
			Type:  "round_votes",
			Round: round,
			Votes: allVotes,
		}
		votesData, _ := json.Marshal(votesMsg)

		for identity := range workers {
			if identity != "coordinator" {
				socket.SendMessage(identity, votesData)
			}
		}

		// Apply votes to local nodes
		applyVotes(localNodes, allVotes, genesis.Params.K)

		// Count finalized
		localFinalized := 0
		for _, node := range localNodes {
			if node.Finalized {
				localFinalized++
			}
		}

		// Collect finalization status from workers
		// (simplified - in full implementation would track this properly)
		finalizedNodes = localFinalized * len(workers)

		// Progress update
		if verbose || round%10 == 0 {
			roundDuration := time.Since(roundStart)
			fmt.Printf("Round %d: %d/%d finalized, duration: %v\n",
				round, finalizedNodes, len(genesis.Validators), roundDuration)
		}

		// Check timeout
		if time.Since(startTime) > timeout {
			fmt.Println("Benchmark timeout reached")
			break
		}
	}

	duration := time.Since(startTime)

	// Determine consensus choice
	choiceCounts := make(map[ids.ID]int)
	for _, choice := range nodeChoices {
		choiceCounts[choice]++
	}

	var consensusChoice ids.ID
	maxCount := 0
	for choice, count := range choiceCounts {
		if count > maxCount {
			consensusChoice = choice
			maxCount = count
		}
	}

	stats := &BenchStats{
		Rounds:          len(nodeChoices),
		FinalizedNodes:  finalizedNodes,
		Duration:        duration,
		MessagesPerSec:  float64(totalMessages) / duration.Seconds(),
		RoundsPerSec:    float64(len(nodeChoices)) / duration.Seconds(),
		ConsensusChoice: consensusChoice.String(),
	}

	return stats, nil
}

func processLocalRound(nodes []*BenchNode, globalChoices map[string]ids.ID, k int) map[string]string {
	votes := make(map[string]string)

	for _, node := range nodes {
		if !node.Finalized {
			// Sample K nodes from global choices
			sampled := sampleNodes(globalChoices, k)
			node.Consensus.RecordPoll(sampled)
			
			// Update choice
			node.Choice = node.Consensus.Preference()
			node.Finalized = node.Consensus.Finalized()
		}
		
		votes[node.ID.String()] = node.Choice.String()
	}

	return votes
}

func applyVotes(nodes []*BenchNode, allVotes map[string]string, k int) {
	// Convert vote strings to IDs
	voteIDs := make(map[string]ids.ID)
	for nodeID, choice := range allVotes {
		if id, err := ids.FromString(choice); err == nil {
			voteIDs[nodeID] = id
		}
	}

	for _, node := range nodes {
		if !node.Finalized {
			// Sample K nodes
			sampled := sampleNodes(voteIDs, k)
			node.Consensus.RecordPoll(sampled)
			
			// Update state
			node.Choice = node.Consensus.Preference()
			node.Finalized = node.Consensus.Finalized()
		}
	}
}

func sampleNodes(choices map[string]ids.ID, k int) []ids.ID {
	sampled := make([]ids.ID, 0, k)
	
	// Simple sampling - in production would use proper random sampling
	for _, choice := range choices {
		sampled = append(sampled, choice)
		if len(sampled) >= k {
			break
		}
	}

	// Pad with repeats if necessary
	for len(sampled) < k && len(sampled) > 0 {
		sampled = append(sampled, sampled[0])
	}

	return sampled
}

func participateInBenchmark(socket *zmq.Socket, localNodes []*BenchNode, genesis *BenchGenesis, verbose bool) error {
	nodeChoices := make(map[string]ids.ID)
	
	for {
		msg, err := socket.RecvMessage(0)
		if err != nil {
			continue
		}

		var benchMsg BenchMessage
		if err := json.Unmarshal([]byte(msg[0]), &benchMsg); err != nil {
			continue
		}

		switch benchMsg.Type {
		case "round_start":
			// Process round
			votes := processLocalRound(localNodes, nodeChoices, genesis.Params.K)
			
			// Send votes
			voteMsg := BenchMessage{
				Type:  "votes",
				Round: benchMsg.Round,
				Votes: votes,
			}
			voteData, _ := json.Marshal(voteMsg)
			socket.SendMessage(voteData)

		case "round_votes":
			// Apply global votes
			applyVotes(localNodes, benchMsg.Votes, genesis.Params.K)
			
			// Update choices
			for nodeID, choice := range benchMsg.Votes {
				if id, err := ids.FromString(choice); err == nil {
					nodeChoices[nodeID] = id
				}
			}

		case "shutdown":
			if benchMsg.Stats != nil && verbose {
				fmt.Printf("\nBenchmark complete: %d rounds, duration: %v\n",
					benchMsg.Stats.Rounds, benchMsg.Stats.Duration)
			}
			return nil
		}
	}
}

func displayBenchmarkResults(stats *BenchStats, totalValidators int) {
	fmt.Println("\n=== Benchmark Results ===")
	fmt.Printf("Total validators: %d\n", totalValidators)
	fmt.Printf("Total rounds: %d\n", stats.Rounds)
	fmt.Printf("Duration: %v\n", stats.Duration)
	fmt.Printf("Finalized nodes: %d (%.1f%%)\n", 
		stats.FinalizedNodes, float64(stats.FinalizedNodes)/float64(totalValidators)*100)
	fmt.Printf("Rounds/second: %.2f\n", stats.RoundsPerSec)
	fmt.Printf("Messages/second: %.2f\n", stats.MessagesPerSec)
	fmt.Printf("Consensus choice: %s\n", stats.ConsensusChoice)
	
	// Performance metrics
	fmt.Println("\n=== Performance Metrics ===")
	msPerRound := stats.Duration.Milliseconds() / int64(stats.Rounds)
	fmt.Printf("Average round time: %dms\n", msPerRound)
	fmt.Printf("Throughput: %.0f decisions/sec\n", 1000.0/float64(msPerRound))
}

// Helper types and functions

type WorkerInfo struct {
	Identity string
	Cores    int
	Memory   int64
	NumNodes int
}

func isCoordinator(address string) bool {
	// If address starts with : it's a bind address (coordinator)
	return address[0] == ':'
}

func getSystemMemory() int64 {
	// Simplified - would use actual system calls
	return 16 * 1024 * 1024 * 1024 // 16GB default
}

func getHostname() (string, error) {
	hostname, err := net.LookupAddr("127.0.0.1")
	if err != nil || len(hostname) == 0 {
		return "unknown", nil
	}
	return hostname[0], nil
}