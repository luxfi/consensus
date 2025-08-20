// +build zmq

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/wave"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/qzmq"
)

func main() {
	var (
		nodes    = flag.Int("nodes", 10, "Number of nodes")
		batch    = flag.Int("batch", 4096, "Batch size")
		interval = flag.Duration("interval", 5*time.Millisecond, "Batch interval")
		rounds   = flag.Int("rounds", 100, "Number of rounds")
		quiet    = flag.Bool("quiet", false, "Quiet mode (minimal output)")
		addr     = flag.String("addr", "tcp://127.0.0.1:5555", "ZMQ address")
	)
	flag.Parse()

	if !*quiet {
		fmt.Printf("ZMQ Consensus Benchmark\n")
		fmt.Printf("=======================\n")
		fmt.Printf("Nodes: %d\n", *nodes)
		fmt.Printf("Batch: %d\n", *batch)
		fmt.Printf("Interval: %v\n", *interval)
		fmt.Printf("Rounds: %d\n", *rounds)
		fmt.Printf("Address: %s\n\n", *addr)
	}

	bench := &ZMQBenchmark{
		nodes:    *nodes,
		batch:    *batch,
		interval: *interval,
		rounds:   *rounds,
		quiet:    *quiet,
		addr:     *addr,
	}

	if err := bench.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Benchmark failed: %v\n", err)
		os.Exit(1)
	}
}

type ZMQBenchmark struct {
	nodes    int
	batch    int
	interval time.Duration
	rounds   int
	quiet    bool
	addr     string
}

func (b *ZMQBenchmark) Run() error {
	ctx := context.Background()
	
	// Create QZMQ sessions for each node
	sessions := make([]*qzmq.Session, b.nodes)
	for i := 0; i < b.nodes; i++ {
		session, err := qzmq.NewSession(ctx, fmt.Sprintf("node-%d", i))
		if err != nil {
			return fmt.Errorf("failed to create session %d: %w", i, err)
		}
		defer session.Close()
		sessions[i] = session
	}

	// Create consensus engines
	params := config.DefaultParams()
	engines := make([]*wave.Engine[string], b.nodes)
	for i := 0; i < b.nodes; i++ {
		engines[i] = wave.New[string](params)
	}

	// Create emitters
	nodeList := make([]string, b.nodes)
	for i := 0; i < b.nodes; i++ {
		nodeList[i] = fmt.Sprintf("node-%d", i)
	}
	emitter := photon.NewUniformEmitter(nodeList)

	// Metrics
	var (
		totalMessages int64
		totalBytes    int64
		totalDecided  int64
		startTime     = time.Now()
	)

	// Run benchmark rounds
	var wg sync.WaitGroup
	for round := 0; round < b.rounds; round++ {
		itemID := fmt.Sprintf("item-%d", round)
		
		// Initialize engines for this item
		for _, engine := range engines {
			engine.Initialize(itemID, 0)
		}

		// Simulate consensus for this item
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			
			decided := false
			for attempt := 0; attempt < 10 && !decided; attempt++ {
				// Select voters
				voters, err := emitter.Emit(ctx, params.K, uint64(r*10+attempt))
				if err != nil {
					continue
				}

				// Simulate message exchange
				msgSize := b.batch
				atomic.AddInt64(&totalMessages, int64(len(voters)))
				atomic.AddInt64(&totalBytes, int64(len(voters)*msgSize))

				// Record votes
				votes := len(voters) * 8 / 10 // 80% positive votes
				for _, engine := range engines {
					engine.RecordPoll(itemID, votes)
					if engine.IsAccepted(itemID) {
						decided = true
					}
				}
			}

			if decided {
				atomic.AddInt64(&totalDecided, 1)
			}
		}(round)

		// Control rate
		if round%10 == 0 {
			time.Sleep(b.interval)
		}
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Calculate metrics
	messagesPerSec := float64(totalMessages) / duration.Seconds()
	bytesPerSec := float64(totalBytes) / duration.Seconds()
	decisionsPerSec := float64(totalDecided) / duration.Seconds()
	latency := duration / time.Duration(b.rounds)

	// Print results
	if b.quiet {
		fmt.Printf("TPS: %.0f, Latency: %v, Decided: %d/%d\n",
			decisionsPerSec*float64(b.batch), latency, totalDecided, b.rounds)
	} else {
		fmt.Printf("\nBenchmark Results\n")
		fmt.Printf("=================\n")
		fmt.Printf("Duration: %v\n", duration)
		fmt.Printf("Messages: %d (%.0f/sec)\n", totalMessages, messagesPerSec)
		fmt.Printf("Bytes: %d (%.2f MB/sec)\n", totalBytes, bytesPerSec/1024/1024)
		fmt.Printf("Decisions: %d/%d (%.2f/sec)\n", totalDecided, b.rounds, decisionsPerSec)
		fmt.Printf("TPS: %.0f\n", decisionsPerSec*float64(b.batch))
		fmt.Printf("Latency: %v\n", latency)
	}

	return nil
}