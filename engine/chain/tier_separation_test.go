// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// tier_separation_test.go — why a bare majority is not a certificate anything moves on.
//
// A Nova majority is ⌊n/2⌋+1. Two majorities of five share ONE validator, and one is
// not more than f=1, so a single equivocator can hold certifying majorities on two
// conflicting blocks. A block is therefore accepted, and exported, only on the ⅔
// certificate. The export surfaces — bridges, settlement, the EVM's finalized/safe
// tags, warp — subscribe through exactly one seam, the Quasar observer. These pin the
// boundary: the arithmetic that says a majority cannot be trusted, and the engine
// behaviour that neither accepts nor exports on one.
package chain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// TestTier_NovaMajorityIsNotByzantineSafe states the arithmetic plainly, as a
// standing fact rather than folklore. Two Nova quorums intersect in
// 2·NovaQuorum(n) − n validators; Byzantine safety needs that to exceed
// f = ⌊(n−1)/3⌋.
//
// The two tiers coincide only at n∈{1,2,4}, where a majority already IS ⌊2n/3⌋+1 —
// n=4 being minBFTCommittee. At the sizes this fleet runs (5, 7, 9, 21, 100) they
// separate and the overlap stops covering f: at n=5 two majorities share ONE
// validator against f=1, so a single equivocator can hold a certifying majority on
// each of two conflicting blocks. That is the whole reason no block is accepted on a
// Nova certificate.
//
// This asserts the SHORTFALL. If it ever fails, Nova has become Byzantine-safe and
// the accept rule should be re-derived rather than inherited.
func TestTier_NovaMajorityIsNotByzantineSafe(t *testing.T) {
	// Nova is never ABOVE the export floor, and two Nova majorities always meet —
	// the crash-fault guarantee it does have. Both hold at every size.
	for n := 1; n <= 256; n++ {
		if NovaQuorum(n) > 2*n/3+1 {
			t.Fatalf("n=%d: NovaQuorum=%d exceeds the ⅔ export floor %d — the accept tier must "+
				"sit at or below the export tier, never above it", n, NovaQuorum(n), 2*n/3+1)
		}
		if 2*NovaQuorum(n) <= n {
			t.Fatalf("n=%d: two Nova majorities are disjoint — even the non-equivocating "+
				"guarantee is gone", n)
		}
	}
	// At the sizes this fleet actually runs, the overlap does not cover f.
	for _, n := range []int{5, 7, 9, 21, 100} {
		overlap := 2*NovaQuorum(n) - n
		f := (n - 1) / 3
		if overlap > f {
			t.Fatalf("n=%d: Nova quorums now intersect in %d > f=%d. Nova has become "+
				"Byzantine-safe, so the reorgable-until-Quasar contract in finality.go and every "+
				"caller that relies on it should be re-derived rather than left as folklore.",
				n, overlap, f)
		}
		if NovaQuorum(n) >= 2*n/3+1 {
			t.Fatalf("n=%d: NovaQuorum=%d has reached the ⅔ export floor %d; at these sizes the "+
				"two tiers must stay distinct thresholds", n, NovaQuorum(n), 2*n/3+1)
		}
	}
}

// exportSeam records what the one export surface was told.
type exportSeam struct {
	mu     sync.Mutex
	canon  []ids.ID
	height []uint64
}

func (s *exportSeam) observe(canonical ids.ID, height uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.canon = append(s.canon, canonical)
	s.height = append(s.height, height)
}

func (s *exportSeam) fired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.height)
}

func (s *exportSeam) last() (ids.ID, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.height)
	if n == 0 {
		return ids.Empty, 0
	}
	return s.canon[n-1], s.height[n-1]
}

// TestTier_AcceptAndExportWaitForTheSupermajority walks one block across the boundary.
//
// A certificate from three of five is a bare majority, one short of ⅔: it moves nothing —
// the block is not accepted and nothing exportable sees it, since one equivocator could
// put such a majority on each of two blocks at one height. The ⅔ certificate from four of
// five then accepts the block and moves the export frontier in the same step.
func TestTier_AcceptAndExportWaitForTheSupermajority(t *testing.T) {
	const n = 5
	vs := newTestValidatorSet(n)
	chainID := ids.GenerateTestID()
	seam := &exportSeam{}

	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithStakeWeighting(vs),
		WithQuasarObserver(seam.observe))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	blk := newTestBlock(1, ids.Empty, "tier-boundary")
	trackVerifiedBlock(rt, blk, 0)
	pos := posFor(chainID, blk)

	certFrom := func(tier Finality, voters []int) []byte {
		t.Helper()
		votes := make([]SignedVote, 0, len(voters))
		for _, i := range voters {
			votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
		}
		cert, err := AssembleQuorumCert(pos, tier, uint32(SignerFloor(tier, n)), votes)
		if err != nil {
			t.Fatalf("assemble %s cert: %v", tier, err)
		}
		b, err := cert.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}

	// THE BARE MAJORITY — a well-formed Nova certificate over three of five.
	majority := []int{0, 1, 2}
	if got, want := len(majority), NovaQuorum(n); got != want {
		t.Fatalf("test setup: %d voters is not NovaQuorum(%d)=%d", got, n, want)
	}
	if rt.HandleIncomingCert(certFrom(Nova, majority)) {
		t.Fatal("a bare-majority certificate was taken as finality")
	}
	if got := blk.AcceptCalled(); got != 0 {
		t.Fatalf("VM.Accept=%d on a bare majority: one equivocator could accept two blocks at this height", got)
	}
	if seam.fired() != 0 {
		c, h := seam.last()
		t.Fatalf("EXPORT READ A SUB-⅔ HEIGHT: the export seam fired for canonical %s at height %d on "+
			"a %d-of-%d certificate, under the ⅔ floor.", c, h, len(majority), n)
	}

	// THE SUPERMAJORITY — four of five, past ⅔ of stake and seats.
	if !rt.HandleIncomingCert(certFrom(Quasar, []int{0, 1, 2, 3})) {
		t.Fatal("a ⅔ certificate must accept the block")
	}
	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("VM.Accept=%d on the ⅔ certificate, want 1", got)
	}
	if !waitFor(2*time.Second, func() bool { return seam.fired() > 0 }) {
		t.Fatalf("EXPORT NEVER FORMED: 4 of %d — a ⅔-by-stake supermajority — certified the block "+
			"and the export seam never fired.", n)
	}
	gotCanon, gotHeight := seam.last()
	if gotHeight != 1 || gotCanon != blk.id {
		t.Fatalf("export frontier published (%s, %d), want (%s, 1)", gotCanon, gotHeight, blk.id)
	}
}

// TestTier_ExportNeverLeadsAcceptance: the export frontier is published strictly
// after the block's local accept, so no consumer can be handed a height the VM has
// not applied.
func TestTier_ExportNeverLeadsAcceptance(t *testing.T) {
	const n = 5
	vs := newTestValidatorSet(n)
	chainID := ids.GenerateTestID()

	var mu sync.Mutex
	var exportedAt []int64
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithStakeWeighting(vs),
		WithQuasarObserver(func(_ ids.ID, height uint64) {
			mu.Lock()
			exportedAt = append(exportedAt, int64(height))
			mu.Unlock()
		}))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	blk := newTestBlock(1, ids.Empty, "tier-order")
	trackVerifiedBlock(rt, blk, 0)
	pos := posFor(chainID, blk)

	// A ⅔ cert delivered in one shot: acceptance and export both become possible on
	// the same call, and the order between them is what is under test.
	votes := make([]SignedVote, 0, 4)
	for i := 0; i < 4; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, Quasar, uint32(e.consensus.Alpha()), votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !rt.HandleIncomingCert(b) {
		t.Fatal("control broke: a ⅔ cert must finalize")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("control broke: VM.Accept=%d want 1", blk.AcceptCalled())
	}

	mu.Lock()
	defer mu.Unlock()
	for _, h := range exportedAt {
		if fh, set := e.consensus.GetFinalizedHeight(); !set || uint64(h) > fh {
			t.Fatalf("export published height %d while the accepted frontier is %d (set=%v) — a "+
				"consumer would be pointed at state the VM has not applied", h, fh, set)
		}
	}
}
