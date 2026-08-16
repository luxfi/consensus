// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/types"
)

// α is a count of distinct validators, so one node cannot reach it alone
// however many times it votes.
func TestOneNodeCannotReachQuorumAlone(t *testing.T) {
	cfg := types.Config{Alpha: 3}
	c := NewChain(cfg)
	ctx := context.Background()

	blk := &types.Block{ID: types.ID{1}, Height: 1}
	if err := c.Add(ctx, blk); err != nil {
		t.Fatalf("Add: %v", err)
	}

	loner := types.NodeID{9}
	for i := 0; i < cfg.Alpha+2; i++ {
		if err := c.RecordVote(ctx, &types.Vote{BlockID: blk.ID, Voter: loner}); err != nil {
			t.Fatalf("RecordVote %d: %v", i, err)
		}
	}
	if c.IsAccepted(blk.ID) {
		t.Fatalf("one node voting %d times accepted a block at α=%d", cfg.Alpha+2, cfg.Alpha)
	}

	// The control: α distinct voters is what acceptance takes.
	for i := 0; i < cfg.Alpha; i++ {
		if err := c.RecordVote(ctx, &types.Vote{BlockID: blk.ID, Voter: types.NodeID{byte(i)}}); err != nil {
			t.Fatalf("RecordVote from voter %d: %v", i, err)
		}
	}
	if !c.IsAccepted(blk.ID) {
		t.Fatal("α distinct voters did not accept the block")
	}
}
