package main

import (
	"context"
	"fmt"
	"github.com/luxfi/consensus"
	"github.com/luxfi/ids"
)

// run a deterministic scenario: 3 validators, `accept` of them vote Preference.
func run(accept int) bool {
	cfg := consensus.DefaultConfig()
	cfg.K = 3
	cfg.Alpha = 2 // quorum: 2 of 3
	chain := consensus.NewChain(cfg)
	ctx := context.Background()
	_ = chain.Start(ctx)
	var bid ids.ID
	bid[0] = 42
	_ = chain.Add(ctx, consensus.NewBlock(bid, consensus.GenesisID, 1, []byte("conf")))
	for round := 0; round < 8; round++ {
		for i := 0; i < accept; i++ {
			var nid ids.NodeID
			nid[0] = byte(i + 1)
			_ = chain.RecordVote(ctx, consensus.NewVote(bid, consensus.VotePreference, nid))
		}
	}
	return chain.IsAccepted(bid)
}

func main() {
	for _, v := range []int{3, 2, 1, 0} {
		fmt.Printf("GO   accept=%d/3 -> accepted=%t\n", v, run(v))
	}
}
