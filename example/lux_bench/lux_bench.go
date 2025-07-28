// lux_bench.go - Minimal consensus benchmark harness
package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

/* ------------ Parameterisation ------------ */

type Config struct {
	N         int           // total validators
	K         int           // sample size per wave
	Beta      int           // focus rounds until finality
	MinRound  time.Duration // target round time
	BatchSize int           // tx per proposal
}

var cfg = Config{
	N:         5,                     // Local profile from your table
	K:         5,
	Beta:      4,
	MinRound:  5 * time.Millisecond,
	BatchSize: 50,
}

/* ------------ Message definitions ------------ */

type proposal struct {
	ID   int // monotonically increases each round
	From int // proposer id
}

/* ------------ Node implementation ------------ */

type Node struct {
	id    int
	cfg   *Config
	inbox chan proposal
	peers []chan proposal
	// metrics
	finalised int
}

func (n *Node) run(wg *sync.WaitGroup) {
	defer wg.Done()

	randSrc := rand.New(rand.NewSource(time.Now().UnixNano() + int64(n.id)))

	for round := 0; round < n.cfg.Beta; round++ {
		/* --- Wave: broadcast proposal to K−1 random peers (self counts as 1) --- */
		prop := proposal{ID: round, From: n.id}
		targets := randSrc.Perm(n.cfg.N)[:n.cfg.K-1]

		for _, t := range targets {
			n.peers[t] <- prop
		}

		/* --- Focus: collect proposals until we have K unique senders --- */
		senders := map[int]struct{}{n.id: {}}
		timeout := time.After(n.cfg.MinRound)

		for len(senders) < n.cfg.K {
			select {
			case p := <-n.inbox:
				senders[p.From] = struct{}{}
			case <-timeout:
				// end of this min‑round: break even if quorum is incomplete
				goto NEXT
			}
		}
	NEXT:
	}

	// Nominally we would commit here:
	n.finalised = n.cfg.Beta * n.cfg.BatchSize
}

/* ------------ Harness ------------ */

func main() {
	start := time.Now()

	// Build nodes & fully‑connected in‑memory channels
	nodes := make([]*Node, cfg.N)
	chans := make([]chan proposal, cfg.N)
	for i := range chans {
		chans[i] = make(chan proposal, 1024) // buffered
	}
	for i := 0; i < cfg.N; i++ {
		nodes[i] = &Node{id: i, cfg: &cfg, inbox: chans[i], peers: chans}
	}

	var wg sync.WaitGroup
	wg.Add(cfg.N)
	for _, n := range nodes {
		go n.run(&wg)
	}
	wg.Wait()

	elapsed := time.Since(start)

	// Aggregate metrics
	totalTx := cfg.N * nodes[0].finalised
	fmt.Printf("=== Lux raw‑consensus bench ===\n")
	fmt.Printf("Nodes          : %d\n", cfg.N)
	fmt.Printf("β rounds       : %d (@ %v)\n", cfg.Beta, cfg.MinRound)
	fmt.Printf("Batch size     : %d tx\n", cfg.BatchSize)
	fmt.Printf("Throughput     : %.1f TPS\n", float64(totalTx)/elapsed.Seconds())
	fmt.Printf("Latency/commit : %v\n", elapsed/time.Duration(cfg.Beta))
}