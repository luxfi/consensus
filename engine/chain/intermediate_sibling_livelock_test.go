// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// intermediate_sibling_livelock_test.go — REGRESSION GUARD for the v1.36 "Nova"
// finality LIVELOCK under saturation (testnet C-Chain 96368, tip frozen at 235).
//
// THE LIVE FAILURE. Under sustained load a verified α-of-K cert forms for a block a
// few heights above the finalized tip (e.g. 238 over tip 235). Every receiving node
// refuses it with ErrAncestorNotTracked on ONE specific intermediate OUTER envelope
// (`3vgfhXSS`) between the tip and the certified block, the self-heal fires an
// ancestor fetch, and the ancestor NEVER lands — a permanent livelock (~1174
// refusals/min, EVM tip frozen ~2h). Safety held (no fork); the guard chose halt.
//
// THE SUB-CAUSE (this test pins it). ledger.go `pathFromTip` walks the certified
// block's ancestry to the finalized tip by RESOLVING EACH INTERMEDIATE BY ITS OUTER
// ENVELOPE ID (`dag.Parent(cur)` where `cur` is an outer id). It has NO canonical
// fallback. Under load the network tracks DIFFERENT proposervm wrappers of the SAME
// inner execution block at an intermediate height (aliases, not a fork — heavy
// blocks + any proposer flap induce this). A cert whose ancestry threads through the
// PEER's wrapper (wrapperA6) cannot be applied by a node that holds the identical
// inner block under ITS OWN wrapper (wrapperB6): the walk demands the exact outer id
// wrapperA6, which the node lacks and which no reachable peer may retain. The
// self-heal fetches wrapperA6 forever.
//
// This is the SAME nondeterminism class as the mainnet-644 sibling storm. The 644
// fix canonicalized the VOTE-PLACEMENT path (convergedWinnerAtHeightLocked /
// canonicalRep, engine.go) and the CERT'S OWN block (finalizeLocalAliasFromVerified-
// Cert / pendingByCanonicalLocked, topology.go) — but it did NOT canonicalize the
// INTERMEDIATE-ANCESTOR walk in pathFromTip. v1.36's Tendermint-braid rip did not
// touch this seam (the Nova sampler is not the decider yet; the cert still drives
// accept), so the residual outer-id ancestry resolution survives into v1.36.0.
//
// The pure fold (ledger.go Finalize) is the exact and deterministic locus of the
// bug, so these tests drive it directly — no network, no goroutines, no timing.
package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// wrapperNode is one tracked block in the read-only ancestry: its outer parent, its
// own height, its inner execution commitment (canonical), and — recorded but IGNORED
// by the current outer-id-only pathFromTip — the inner commitment of its parent. A
// canonical-aware walk consults parentCanonical to collapse sibling wrappers; the
// current fold does not, which is the bug under test.
type wrapperNode struct {
	parent          ids.ID
	height          uint64
	canonical       ids.ID
	parentCanonical ids.ID
}

// wrappedAncestry is a read-only Ancestry over an explicit wrapper map, so the pure
// fold can be driven with a partial tree that models sibling-wrapper divergence.
type wrappedAncestry map[ids.ID]wrapperNode

func (w wrappedAncestry) Parent(id ids.ID) (ids.ID, uint64, ids.ID, ids.ID, bool) {
	n, ok := w[id]
	return n.parent, n.height, n.canonical, n.parentCanonical, ok
}

func (w wrappedAncestry) Children(id ids.ID) []ids.ID {
	var out []ids.ID
	for cid, n := range w {
		if n.parent == id {
			out = append(out, cid)
		}
	}
	return out
}

func (w wrappedAncestry) WrapperByCanonical(canonical ids.ID, height uint64) (ids.ID, bool) {
	for id, n := range w {
		if n.height == height && n.canonical == canonical {
			return id, true
		}
	}
	return ids.Empty, false
}

// TestFinalize_IntermediateSiblingWrapper_Livelocks is the repro. This node is
// certified through height 5. A verified cert selects the inner execution block at
// height 7 whose recorded outer parent is wrapperA6 (a PEER's proposervm wrapper of
// the height-6 execution). This node holds the IDENTICAL height-6 execution under its
// OWN wrapper wrapperB6, which chains to the finalized tip. Finality MUST advance:
// the node possesses the whole certified inner chain, just under its own envelopes,
// and the cert is a verified α-of-K witness. It must NOT wedge waiting to fetch a
// redundant outer-envelope alias (wrapperA6) that no reachable peer may retain.
//
// On current HEAD Finalize returns ErrAncestorNotTracked{missing: wrapperA6} — the
// livelock. This test asserts the invariant (finality under load) and therefore
// FAILS on HEAD, pinning the regression until the intermediate walk is canonicalized.
func TestFinalize_IntermediateSiblingWrapper_Livelocks(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5) // certified through height 5

	// Height-6 execution, wrapped two ways — ALIASES of one inner block, not a fork.
	inner6 := ids.GenerateTestID()
	wrapperA6 := ids.GenerateTestID() // the cert's ancestry names THIS wrapper (a peer's)
	wrapperB6 := ids.GenerateTestID() // THIS node tracks THIS wrapper of the same inner6

	// Height-7 execution — the certified block.
	inner7 := ids.GenerateTestID()
	blk7 := ids.GenerateTestID()

	dag := wrappedAncestry{
		// The node's OWN height-6 wrapper, chaining to the finalized tip.
		wrapperB6: {parent: tipOuter, height: 6, canonical: inner6, parentCanonical: tipCanon},
		// The certified height-7 block; its recorded outer parent is the peer's
		// wrapperA6 (identical inner6), a canonical-equivalent sibling of wrapperB6.
		blk7: {parent: wrapperA6, height: 7, canonical: inner7, parentCanonical: inner6},
		// wrapperA6 is intentionally ABSENT: this node never tracked the peer's envelope.
	}

	cert := Cert{Block: blk7, Parent: wrapperA6, Height: 7, Canonical: inner7}

	_, plan, err := Finalize(led, cert, dag)

	if errors.Is(err, ErrAncestorNotTracked) {
		var ant *AncestorNotTracked
		_ = errors.As(err, &ant)
		t.Fatalf(`LIVELOCK REPRO (mainnet-644 sibling class in the v1.36 intermediate-ancestor walk):

Finalize refused a VERIFIED cert for height 7 with ErrAncestorNotTracked (missing=%v),
even though this node HOLDS the certified block's parent execution (inner6) under a
canonical-equivalent wrapper (wrapperB6=%s) that chains to the finalized tip.

pathFromTip (ledger.go) resolves the intermediate ancestor by OUTER id ONLY, so it
demands the exact envelope wrapperA6=%s the cert named — which this node lacks and
which no reachable peer may retain — instead of collapsing the sibling wrapper by
inner identity (the way convergedWinnerAtHeightLocked and finalizeLocalAliasFrom-
VerifiedCert already do everywhere else). The self-heal fetch then targets wrapperA6
forever; the ancestor never lands; the EVM tip freezes. err=%v`,
			ant.Missing, wrapperB6, wrapperA6, err)
	}
	if err != nil {
		t.Fatalf("unexpected error finalizing the sibling-wrapper cert: %v", err)
	}

	// Post-fix expectation: the 2-step canonical path is accepted using the node's
	// LOCAL wrappers of the certified inner chain (wrapperB6 @6, then blk7 @7).
	if len(plan.Accept) != 2 {
		t.Fatalf("expected the canonical path {wrapperB6@6, blk7@7} (2 blocks) to be accepted, got Accept=%v", plan.Accept)
	}
	if plan.Accept[len(plan.Accept)-1] != blk7 {
		t.Fatalf("the certified block %s must be the top of the accepted path, got %v", blk7, plan.Accept)
	}
	for _, id := range plan.Accept {
		if id == wrapperA6 {
			t.Fatalf("accepted the UNHELD peer envelope wrapperA6=%s; finality must accept the LOCAL wrapper the VM holds", wrapperA6)
		}
	}
}

// TestFinalize_IntermediateGenuinelyAbsent_StillDefers is the SAFETY BOUNDARY (the
// contrast that keeps the fix honest). Here the node holds NO wrapper of the height-6
// execution at all — not even a sibling. This is a genuine behind-node gap, and the
// fold MUST still fail-closed with ErrAncestorNotTracked (fetch-and-retry), never
// invent an execution it does not have. This test PASSES on HEAD and MUST keep
// passing after the fix: the fix may only collapse canonical-EQUIVALENT siblings, it
// may never accept a certified branch whose inner ancestry the node cannot prove it
// holds.
func TestFinalize_IntermediateGenuinelyAbsent_StillDefers(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	inner7 := ids.GenerateTestID()
	blk7 := ids.GenerateTestID()
	absentParent := ids.GenerateTestID() // height-6 ancestor whose inner block we do NOT hold in ANY wrapper

	// The node tracks ONLY the certified block; it holds no height-6 wrapper.
	dag := wrappedAncestry{
		blk7: {parent: absentParent, height: 7, canonical: inner7, parentCanonical: ids.GenerateTestID()},
	}
	cert := Cert{Block: blk7, Parent: absentParent, Height: 7, Canonical: inner7}

	_, _, err := Finalize(led, cert, dag)
	if !errors.Is(err, ErrAncestorNotTracked) {
		t.Fatalf("a genuinely-absent intermediate ancestor (NO local wrapper of its inner block) must "+
			"DEFER with ErrAncestorNotTracked (fail-closed, fetch-and-retry), never finalize; got err=%v", err)
	}
}
