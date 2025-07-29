// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/quasar"
	"github.com/luxfi/ids"
)

var (
	profile   = flag.String("profile", "local", "Consensus profile: local, testnet, mainnet")
	batchSize = flag.Int("batch", 4096, "Batch size for proposals")
	minRound  = flag.Duration("min-round", 5*time.Millisecond, "Minimum round interval")
	rounds    = flag.Int("rounds", 1000, "Number of rounds to run")
)

// SimpleBenchmark runs a local consensus benchmark without networking
type SimpleBenchmark struct {
	nodeID  ids.NodeID
	engine  *quasar.Engine
	params  config.Parameters
	
	// Metrics
	consensusRounds atomic.Int64
	txProcessed     atomic.Int64
	startTime       time.Time
}

func main() {
	flag.Parse()
	
	// Create node ID
	nodeID := ids.GenerateTestNodeID()
	log.Printf("🚀 Starting simple benchmark node %s", nodeID.String())
	
	// Get consensus parameters
	params := getConsensusParams(*profile)
	params.BatchSize = *batchSize
	params.MinRoundInterval = *minRound
	
	// Create benchmark
	bench := &SimpleBenchmark{
		nodeID:    nodeID,
		engine:    quasar.New(params, nodeID),
		params:    params,
		startTime: time.Now(),
	}
	
	// Handle shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-sigCh
		log.Println("📤 Shutting down...")
		cancel()
	}()
	
	// Run benchmark
	bench.Run(ctx, *rounds)
	
	// Print results
	bench.PrintStats()
}

func (b *SimpleBenchmark) Run(ctx context.Context, rounds int) {
	log.Printf("Running %d consensus rounds...", rounds)
	
	ticker := time.NewTicker(b.params.MinRoundInterval)
	defer ticker.Stop()
	
	for i := 0; i < rounds; i++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.runRound()
		}
	}
}

func (b *SimpleBenchmark) runRound() {
	b.consensusRounds.Add(1)
	
	// Simulate adding items
	itemID := ids.GenerateTestID()
	parentID := ids.Empty
	height := uint64(b.consensusRounds.Load())
	data := []byte(fmt.Sprintf("block-%d", height))
	
	// Add item to consensus
	if err := b.engine.Add(context.Background(), itemID, parentID, height, data); err != nil {
		log.Printf("Error adding item: %v", err)
		return
	}
	
	// Simulate polling
	votes := generateVotes(b.params.K, itemID)
	if err := b.engine.RecordPoll(context.Background(), votes); err != nil {
		log.Printf("Error recording poll: %v", err)
		return
	}
	
	// Track processed transactions
	b.txProcessed.Add(int64(b.params.BatchSize))
}

func (b *SimpleBenchmark) PrintStats() {
	elapsed := time.Since(b.startTime)
	tps := float64(b.txProcessed.Load()) / elapsed.Seconds()
	
	fmt.Printf("\n📊 Benchmark Results:\n")
	fmt.Printf("   Duration: %v\n", elapsed)
	fmt.Printf("   Consensus rounds: %d\n", b.consensusRounds.Load())
	fmt.Printf("   Transactions processed: %d\n", b.txProcessed.Load())
	fmt.Printf("   TPS: %.2f\n", tps)
	fmt.Printf("   Avg round time: %.2fms\n", float64(elapsed.Milliseconds())/float64(b.consensusRounds.Load()))
}

func getConsensusParams(profile string) config.Parameters {
	params, err := config.GetPresetParameters(profile)
	if err != nil {
		log.Printf("Unknown profile %s, using local", profile)
		params, _ = config.GetPresetParameters("local")
	}
	return params
}

func generateVotes(k int, preference ids.ID) map[ids.NodeID]ids.ID {
	votes := make(map[ids.NodeID]ids.ID)
	
	// Generate k votes, majority for the preference
	for i := 0; i < k; i++ {
		nodeID := ids.GenerateTestNodeID()
		if i < (k*2/3) {
			votes[nodeID] = preference
		} else {
			votes[nodeID] = ids.GenerateTestID()
		}
	}
	
	return votes
}