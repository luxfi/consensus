// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// TestConsensus_CertGossipPacedWhileFinalizeKeepsFailing pins the pacing on the
// finality-cert broadcast.
//
// TryAccept assembles the cert, hands it to the gossiper, and only THEN finalizes.
// When the finalize fails — the VM refuses to apply the block, or its ancestor is
// not tracked yet — the block correctly stays pending, so the very next arriving
// vote or poll tick re-enters TryAccept and re-broadcasts the identical cert. Every
// peer answers that broadcast with a finalize attempt of its own, whose votes come
// back here: the loop feeds itself and the volume is bounded only by CPU.
//
// This is what lux-mainnet's C-Chain was doing while stuck at outer height
// 1,159,288 — 7,392 cert gossips in 60 seconds across 145 pending blocks from one
// node, ~8,280 inbound certs per node per minute, on a chain that had not advanced
// in 11 hours.
//
// A cert is deterministic evidence about one decided height; the second copy tells
// a peer nothing the first did not. So it goes out once and is then paced on the
// same schedule as the re-poll backstop.
func TestConsensus_CertGossipPacedWhileFinalizeKeepsFailing(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	blk := &trackingMockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("stuck"),
		acceptErr: errors.New("vm refuses to apply"),
	}
	trackMockBlock(rt, blk, 0)
	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blk.id, ParentID: ids.Empty}

	// Full 5-of-5 signed accepts: the quorum is unambiguously present, so the cert
	// assembles. The finalize behind it fails on VM.Accept.
	for i := 0; i < 5; i++ {
		e.ReceiveVote(vs.signedVote(i, pos))
	}

	certCount := func() int {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return len(rec.certs)
	}
	if !waitFor(2*time.Second, func() bool { return certCount() >= 1 }) {
		t.Fatal("harness never reached the quorum: no cert was assembled, so this test proves nothing")
	}
	e.mu.RLock()
	pending, retained := e.pendingBlocks[blk.id]
	decided := retained && pending.Decided
	e.mu.RUnlock()
	if !retained || decided {
		t.Fatal("harness precondition: the block must stay pending and undecided (the VM.Accept error path)")
	}

	before := certCount()

	// Every one of these stands for an arriving vote or a poll tick on a block that
	// cannot finalize.
	const retries = 50
	for i := 0; i < retries; i++ {
		_ = e.TryAccept(context.Background(), blk.id)
	}

	if extra := certCount() - before; extra != 0 {
		t.Fatalf("the same cert was re-broadcast %d times across %d finalize retries — "+
			"one stuck block turns into an unbounded gossip storm (mainnet: 51 sends per block "+
			"per minute); the broadcast must be paced, not fired on every entry", extra, retries)
	}
}
