// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// committee_scaling_test.go — the owner's "prove each rung" gate: the BFT finality
// committee scales 1→N with the LIVE validator set, and a genuine n-validator chain
// FINALIZES a sibling storm at every rung n=1..5 with the correct effective (K,α).
//
// The effective quorum is bftAlpha(n) = ⌊2n/3⌋+1 — the smallest integer STRICTLY
// greater than ⅔ of n (config.TwoThirdsStakeFloor+1), the same rational threshold the
// ⅔-by-stake cert enforces. That yields:
//
//	n=1 → 1-of-1   (single validator: its own accept IS the quorum — self-finalize is
//	               correct, there is no peer to fork against)
//	n=2 → 2-of-2   (f=0; a 1-of-2 quorum violates the fail-closed bound 2α−n>f, so BOTH
//	               must agree — safety before liveness)
//	n=3 → 3-of-3   (>⅔ of 3 is 3; 2-of-3 is exactly ⅔, not a supermajority → excluded)
//	n=4 → 3-of-4   (f=1; the classic 2f+1)
//	n=5 → 4-of-5   (f=1; the mainnet C-Chain rung)
//
// This is scenario A — a GENUINE n-validator chain (preset K=n, e.g. a small sovereign
// L1 / FeasibleParams). Scenario B — a LARGE preset (MainnetParams K=21) SHRUNK to the
// live set by effectiveCommittee/bftCommittee, with the minBFTCommittee=4 floor that
// keeps a transient low count from self-finalizing (the 1085013 guard) — is proven by
// TestViewChange_MainnetParams_5Validators_CommitteeFromStakeSource (21→5/α4) and
// TestFix1_* (the floor). Here every rung uses its own genuine committee so the floor
// is inert and the pure scaling is visible.
package chain

import (
	"fmt"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// genuineParams builds the config of a REAL n-validator chain: the sample K and
// both α gates sized to the live set (K=n, α=bftAlpha(n)). View-change is on for n>1 (the
// round machine that converges a symmetric sibling split); a single validator needs none.
func genuineParams(n int) config.Parameters {
	a := bftAlpha(n)
	p := stormParams5() // storm-tuned timing (RoundTO 1s), then resize to n
	p.K = n
	p.AlphaPreference = a
	p.AlphaConfidence = a
	// The legacy float α (Quasar-compat) must track the integer ratio, floored at the 0.66
	// minimum config.Valid() requires (same clamp FeasibleParams applies).
	p.Alpha = float64(a) / float64(n)
	if p.Alpha < 0.66 {
		p.Alpha = 0.66
	}
	if p.Alpha > 1.0 {
		p.Alpha = 1.0
	}
	p.Beta = 1
	p.BetaVirtuous = 1
	if p.BetaRogue < n {
		p.BetaRogue = n
	}
	p.ViewChange = n > 1
	if err := p.Valid(); err != nil {
		panic(fmt.Sprintf("genuineParams(%d) invalid: %v", n, err))
	}
	return p
}

func TestCommitteeScaling_1through5_EachRungFinalizes(t *testing.T) {
	type rung struct {
		n         int
		wantAlpha int
	}
	rungs := []rung{
		{1, 1}, // 1-of-1 self-finalize
		{2, 2}, // 2-of-2 (BFT-safe; 1-of-2 would violate 2α−n>f)
		{3, 3}, // 3-of-3 (>⅔ of 3)
		{4, 3}, // 3-of-4
		{5, 4}, // 4-of-5 (mainnet rung)
	}

	for _, r := range rungs {
		r := r
		t.Run(fmt.Sprintf("n=%d_expects_%d-of-%d", r.n, r.wantAlpha, r.n), func(t *testing.T) {
			// Precondition: the formula itself yields the expected quorum.
			if got := bftAlpha(r.n); got != r.wantAlpha {
				t.Fatalf("bftAlpha(%d)=%d, expected %d", r.n, got, r.wantAlpha)
			}

			net := newSimNet(t, r.n, genuineParams(r.n))

			// The effective FINALITY committee every gate (cert + view-change) sizes to must be
			// exactly (n, bftAlpha(n)) — sized from the live StakeSource, not a hardcoded preset.
			gotN, gotA := net.nodes[0].rt.Transitive.effectiveCommittee(0)
			if gotN != r.n || gotA != r.wantAlpha {
				t.Fatalf("effectiveCommittee = (K=%d, α=%d), expected (K=%d, α=%d)",
					gotN, gotA, r.n, r.wantAlpha)
			}

			// Drive a SIBLING SPLIT: every validator builds a distinct competing block on the
			// same genesis parent (the anyone-can-propose storm). Convergence to ONE finalized
			// head then turns entirely on the committee threshold under test. (n=1 builds one.)
			built := make(map[ids.ID]struct{}, r.n)
			for i := 0; i < r.n; i++ {
				blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "rung-"+itoa(r.n)+"-sib-"+itoa(i))
				built[blk.ID()] = struct{}{}
				net.build(i, blk)
			}

			// FINALIZE: all n nodes must converge on a SINGLE head at height 1 (no fork, no
			// double-finalization). stormAwaitSingleHead fails immediately on two distinct heads.
			head := stormAwaitSingleHead(t, net, 1)
			if _, ok := built[head]; !ok {
				t.Fatalf("finalized head %s is not one of the built siblings", head)
			}

			// Every up node must agree on that head (full convergence, not just a subset).
			heads := net.headsAtHeight(1)
			if len(heads) != 1 {
				t.Fatalf("n=%d: expected exactly ONE finalized head across the fleet, got %v", r.n, heads)
			}
			if c := heads[head]; c != net.upCount() {
				t.Fatalf("n=%d: head %s finalized by %d/%d nodes — not full convergence", r.n, head, c, net.upCount())
			}
		})
	}
}

// TestCommitteeScaling_N1_SelfFinalizesButN2NeedsBoth locks the two tricky low rungs the
// owner called out: n=1 self-finalizes correctly (its own accept is the 1-of-1 quorum),
// but n=2 must NOT self-finalize on one node's vote — the 2-of-2 quorum means a lone node
// stays un-final (fail-closed) until its peer agrees. This is the safety boundary between
// "genuine single validator" and "any larger set".
func TestCommitteeScaling_N1_SelfFinalizesButN2NeedsBoth(t *testing.T) {
	// n=1: the sole validator finalizes its own block with no peer.
	t.Run("n=1_self_finalizes", func(t *testing.T) {
		net := newSimNet(t, 1, genuineParams(1))
		blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "solo")
		net.build(0, blk)
		if ok := waitFor(stormTO, func() bool {
			got, ok := net.nodes[0].rt.FinalizedBlockAtHeight(1)
			return ok && got == blk.ID()
		}); !ok {
			t.Fatal("n=1: the sole validator must self-finalize its own block (1-of-1)")
		}
	})

	// n=2 with one peer DOWN: the lone up node holds α=2 unreachable (only its own vote) →
	// it must NOT finalize (2-of-2 fail-closed). This is the anti-self-finality guard at the
	// smallest multi-validator rung.
	t.Run("n=2_lone_node_never_self_finalizes", func(t *testing.T) {
		net := newSimNet(t, 2, genuineParams(2))
		net.down(1) // peer offline: only node 0 is up
		blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "n2-solo")
		net.build(0, blk)
		// Give it real time to (wrongly) self-finalize if the guard were broken.
		if finalized := waitFor(2*time.Second, func() bool {
			_, ok := net.nodes[0].rt.FinalizedBlockAtHeight(1)
			return ok
		}); finalized {
			t.Fatal("n=2: a lone up node must NOT finalize on its own vote (2-of-2 required) — " +
				"self-finalizing here would fork the chain when the peer returns")
		}
	})
}
