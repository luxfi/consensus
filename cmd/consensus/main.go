package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/prism"
	"github.com/luxfi/consensus/core/wave"
	"github.com/luxfi/consensus/types"
)

// mockTransport simulates network communication
type mockTransport struct {
	votes []wave.VoteMsg[string]
}

func (m *mockTransport) RequestVotes(ctx context.Context, peers []types.NodeID, item string) (<-chan wave.VoteMsg[string], error) {
	ch := make(chan wave.VoteMsg[string], len(m.votes))
	for _, v := range m.votes {
		ch <- v
	}
	close(ch)
	return ch, nil
}

func main() {
	var (
		nodes   = flag.Int("nodes", 21, "number of nodes in consensus")
		rounds  = flag.Int("rounds", 10, "number of consensus rounds")
		verbose = flag.Bool("v", false, "verbose output")
	)
	flag.Parse()

	fmt.Printf("Lux Consensus - Wave-first Architecture\n")
	fmt.Printf("Nodes: %d, Rounds: %d\n\n", *nodes, *rounds)

	// Create configuration based on node count
	cfg := config.DefaultParams()
	cfg.K = *nodes
	
	// Adjust parameters based on network size
	if *nodes <= 5 {
		cfg.Alpha = 0.6
		cfg.Beta = 3
		cfg.RoundTO = 100 * time.Millisecond
	} else if *nodes <= 11 {
		cfg.Alpha = 0.7
		cfg.Beta = 6
		cfg.RoundTO = 200 * time.Millisecond
	} else {
		cfg.Alpha = 0.8
		cfg.Beta = 15
		cfg.RoundTO = 250 * time.Millisecond
	}

	// Create peer list
	peers := make([]types.NodeID, *nodes)
	for i := 0; i < *nodes; i++ {
		peers[i] = types.NodeID(fmt.Sprintf("node-%d", i))
	}

	// Create sampler and consensus
	sel := prism.New(peers, prism.DefaultOptions())
	
	// Simulate different voting scenarios
	scenarios := []struct {
		name   string
		votes  []wave.VoteMsg[string]
		expect string
	}{
		{
			name:   "Strong Accept",
			votes:  generateVotes("item1", peers, 0.8, true),
			expect: "accept",
		},
		{
			name:   "Strong Reject", 
			votes:  generateVotes("item2", peers, 0.2, true),
			expect: "reject",
		},
		{
			name:   "Conflicting",
			votes:  generateVotes("item3", peers, 0.5, true),
			expect: "undecided",
		},
	}

	for _, scenario := range scenarios {
		fmt.Printf("Scenario: %s\n", scenario.name)
		
		tx := &mockTransport{votes: scenario.votes}
		w := wave.New[string](cfg, sel, tx)
		ctx := context.Background()

		// Run consensus rounds
		item := scenario.votes[0].Item
		for round := 0; round < *rounds; round++ {
			w.Tick(ctx, item)
			
			if st, ok := w.State(item); ok {
				if *verbose {
					fmt.Printf("  Round %d: prefer=%v conf=%d decided=%v",
						round+1, st.Step.Prefer, st.Step.Conf, st.Decided)
					if st.Decided {
						fmt.Printf(" result=%v", st.Result)
					}
					fmt.Println()
				}
				
				if st.Decided {
					result := "unknown"
					if st.Result == types.DecideAccept {
						result = "accept"
					} else if st.Result == types.DecideReject {
						result = "reject"
					}
					fmt.Printf("  Decision: %s (rounds=%d)\n", result, round+1)
					break
				}
			}
			
			// Small delay between rounds
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Println()
	}

	if *verbose {
		log.Printf("Consensus simulation complete")
	}
}

func generateVotes(item string, peers []types.NodeID, yesRatio float64, randomize bool) []wave.VoteMsg[string] {
	votes := make([]wave.VoteMsg[string], len(peers))
	yesCount := int(float64(len(peers)) * yesRatio)
	
	for i := range peers {
		votes[i] = wave.VoteMsg[string]{
			Item:   item,
			Prefer: i < yesCount,
			From:   peers[i],
		}
	}
	
	return votes
}