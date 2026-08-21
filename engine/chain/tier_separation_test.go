// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// tier_separation_test.go — what makes a bare majority acceptable.
//
// Nova ignites at ⌊n/2⌋+1. Two majorities of five share ONE validator, and one is
// not more than f=1, so a single equivocator can hold certifying majorities on two
// conflicting blocks. Nova is therefore not a Byzantine guarantee and finality.go
// says so: it authorizes LOCAL execution and is reorgable until Quasar.
//
// That is a sound position only while nothing exportable reads a Nova-only height.
// The export surfaces — bridges, settlement, the EVM's finalized/safe tags, warp —
// subscribe through exactly one seam, the Quasar observer, and it fires only when a
// ⅔-by-stake certificate forms. These pin the boundary at that seam, in both
// directions: the arithmetic that says Nova cannot be trusted for export, and the
// engine behaviour that keeps export from reading it.
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
// each of two conflicting blocks. That is the whole reason Nova authorizes local
// execution only.
//
// This asserts the SHORTFALL. If it ever fails, Nova has become Byzantine-safe and
// the reorgable-until-Quasar contract should be re-derived rather than inherited.
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

// TestTier_ExportWaitsForTheSupermajority walks one block across the boundary.
//
// Three of five is a Nova majority: the block executes and the VM accepts it. It is
// NOT ⅔, so nothing exportable may see it. The fourth validator's vote arrives
// after the block is already finalized — which is the ordinary case, since the
// ⅔-th vote necessarily trails a bare-majority accept — and only then may the
// export frontier move.
func TestTier_ExportWaitsForTheSupermajority(t *testing.T) {
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

	// THE NOVA ACCEPT — a bare majority, one short of the ⅔ export floor.
	novaVoters := []int{0, 1, 2}
	if got, want := len(novaVoters), NovaQuorum(n); got != want {
		t.Fatalf("test setup: %d voters is not NovaQuorum(%d)=%d", got, n, want)
	}
	votes := make([]SignedVote, 0, len(novaVoters))
	for _, i := range novaVoters {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, Nova, uint32(NovaQuorum(n)), votes)
	if err != nil {
		t.Fatalf("assemble nova cert: %v", err)
	}
	certBytes, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !rt.HandleIncomingCert(certBytes) {
		t.Fatal("control broke: a Nova majority must drive the local accept")
	}
	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("control broke: VM.Accept=%d want 1 — Nova authorizes local execution", got)
	}

	if seam.fired() != 0 {
		c, h := seam.last()
		t.Fatalf("EXPORT READ A NOVA-ONLY HEIGHT: the export seam fired for canonical %s at height "+
			"%d on a %d-of-%d cert. That set holds %d/%d of stake, under the ⅔ floor, and Nova is "+
			"reorgable by construction — a bridge, a settlement receipt or the EVM's finalized tag "+
			"reading this height is reading a decision one equivocator can still overturn.",
			c, h, len(novaVoters), n, len(novaVoters), n)
	}

	// THE TRAILING VOTE — the fourth validator, arriving after finality. The block
	// is no longer pending, so this exercises the late-attestation route.
	sig := vs.sign(3, pos)
	voteBytes, err := encodeSignedVote(vs.nodeID(3), sig)
	if err != nil {
		t.Fatalf("encode vote: %v", err)
	}
	if !rt.HandleIncomingVote(blk.id, voteBytes) {
		t.Fatal("a valid trailing accept vote for a finalized block must be counted toward export")
	}

	if !waitFor(2*time.Second, func() bool { return seam.fired() > 0 }) {
		t.Fatalf("EXPORT NEVER FORMED: 4 of %d — a ⅔-by-stake supermajority — attested and the "+
			"export seam never fired. The trailing vote is the ordinary path to Quasar, since the "+
			"⅔-th vote arrives after the bare-majority accept by construction; a height that "+
			"cannot reach export is a height bridges and settlement can never consume.", n)
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
