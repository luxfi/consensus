// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// TestSelfHeal_AbandonedBlock_KeepsRequestingCatchup is the sibling of
// TestSelfHeal_StuckTrackedNonOwnBlock_TriggersCertCatchup: that one proves the recovery
// fires, this one proves it fires again.
//
// Abandoning a block ends two separate things, and only one of them should end. Not
// re-polling a block its voters can no longer vote on is right — peers vote near their own
// frontier, so a height they have run past will never gather a quorum however often it is
// asked. Not asking for the block again is wrong: rePollAbandoned is set once and cleared
// nowhere, so a node whose single catch-up fetch is dropped never looks at that height
// again and sits there for the life of the process. Restart changes nothing; the same
// attempts are replayed and end in the same place.
//
// For a node that far behind the certificate fetch is the only remaining door — its own
// store may already hold the block, yet consensus cannot accept it without a decision it
// can no longer vote its way to. So the fetch must be reissued while the block stays
// undecided: one per cooldown, not one ever.
func TestSelfHeal_AbandonedBlock_KeepsRequestingCatchup(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	cu := &recordingCatchup{}
	rec := &recordingGossiper{}

	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, rec, vs.signerFor(0)),
		WithCatchup(cu),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: chainID, NetworkID: ids.GenerateTestID(), Logger: log.Noop(),
		Gossiper: &certQuorumGossiper{rec: rec}, Catchup: cu,
	}}

	// Certified through 8; we hold 9 and cannot finalize it.
	tip8 := ids.GenerateTestID()
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(tip8, tip8, 8)
	e.consensus.mu.Unlock()

	blk9 := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: tip8, height: 9,
		timestamp: time.Now(), bytes: []byte("gossiped-9-stuck-no-cert"),
	}
	trackVerifiedBlock(rt, blk9, 0) // non-own => abandonable

	// Drive past the cap. This is the already-covered behaviour: one fetch.
	driveRePoll(e, maxRePollAttempts+2)
	first := cu.count()
	if first == 0 {
		t.Fatalf("precondition: abandonment must fire a cert catch-up at all, got 0")
	}

	// The block is now abandoned. Simulate the fetch having been dropped — nothing
	// decided, nothing arrived — and let the cooldown lapse. Back-dating the throttle
	// entry is how elapsed time is expressed without sleeping through catchupCooldown.
	e.mu.Lock()
	stillPending := false
	for _, pb := range e.pendingBlocks {
		if pb.ConsensusBlock != nil && pb.ConsensusBlock.height == 9 {
			stillPending = true
			if !pb.rePollAbandoned {
				e.mu.Unlock()
				t.Fatalf("precondition: block 9 must be abandoned after %d attempts", maxRePollAttempts)
			}
		}
	}
	for h := range e.certCatchupRequested {
		e.certCatchupRequested[h] = time.Now().Add(-2 * catchupCooldown)
	}
	e.mu.Unlock()
	if !stillPending {
		t.Fatal("precondition: an abandoned block must stay pending — it is recoverable, not dropped")
	}

	// Ask again. An undecided block that nothing has delivered must still be wanted.
	driveRePoll(e, 3)

	if got := cu.count(); got <= first {
		t.Fatalf("an abandoned block whose catch-up was dropped must be re-requested once the "+
			"cooldown lapses: fetches went %d -> %d. Asked for exactly once, a single dropped "+
			"fetch pins the node at this height for good, even while its peers hold the block",
			first, got)
	}
	if m, _ := cu.lastMissing(); m != blk9.id {
		t.Fatalf("the re-request must target the stuck block %s, got %s", blk9.id, m)
	}
}

// TestSelfHeal_AbandonedBlock_StopsOnceDecided is the other half of the contract: keep
// asking is not the same as ask forever. Once the height is finalized the throttle entry
// is reclaimed and no further fetch is issued, so a recovered node goes quiet instead of
// pulling a block it already has.
func TestSelfHeal_AbandonedBlock_StopsOnceDecided(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	cu := &recordingCatchup{}
	rec := &recordingGossiper{}

	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, rec, vs.signerFor(0)),
		WithCatchup(cu),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: chainID, NetworkID: ids.GenerateTestID(), Logger: log.Noop(),
		Gossiper: &certQuorumGossiper{rec: rec}, Catchup: cu,
	}}

	tip8 := ids.GenerateTestID()
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(tip8, tip8, 8)
	e.consensus.mu.Unlock()

	blk9 := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: tip8, height: 9,
		timestamp: time.Now(), bytes: []byte("gossiped-9-then-decided"),
	}
	trackVerifiedBlock(rt, blk9, 0)

	driveRePoll(e, maxRePollAttempts+2)
	before := cu.count()

	// The catch-up lands: height 9 is now certified.
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(blk9.id, blk9.id, 9)
	e.consensus.mu.Unlock()
	e.mu.Lock()
	for h := range e.certCatchupRequested {
		e.certCatchupRequested[h] = time.Now().Add(-2 * catchupCooldown)
	}
	e.mu.Unlock()

	driveRePoll(e, 5)

	if got := cu.count(); got != before {
		t.Fatalf("a finalized height must not be fetched again: %d -> %d", before, got)
	}
}
