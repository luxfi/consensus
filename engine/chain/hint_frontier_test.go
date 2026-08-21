// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// hint_frontier_test.go — the frontier rule has TWO seed doors, and only one of
// them was given the rule.
//
// Above the certified tip, pathFromTip now recognises the finalized frontier under
// a sibling envelope: reaching the finalized height at the tip's EXECUTION is
// reaching the tip. The other door is the fresh-process seed, where the ledger is
// unset and the recovery hint is the VM's applied head. That door builds its own
// floor ledger out of the hint —
//
//	floor := FinalityLedger{tip: led.hint, height: led.hintHeight, set: true}
//
// — and the hint carries only an OUTER envelope, so floor.canonical is Empty and
// decision() degrades it to the envelope id. A sibling wrapper's inner commitment
// can never equal an envelope id, so on this door the comparison is outer-vs-outer
// again, with the same asymmetry: the node that does not hold the peer's envelope
// recovers, and the node that holds it refuses.
//
// The other half of this file is the safety half of the same rule, driven with
// shapes the alias suite does not cover: the relaxation must fire for aliases only.
package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// hintSeedCase is one fresh-process fold: the ledger is unset, the hint is this
// node's wrapper of the applied head, and a cert one height above names its
// parent by the peer's envelope.
type hintSeedCase struct {
	localHead ids.ID // this node's wrapper of the height-5 execution (the hint)
	headCanon ids.ID // the height-5 execution
	peerHead  ids.ID // the network's wrapper of the SAME execution
	cert      Cert
	led       FinalityLedger
}

func newHintSeedCase() hintSeedCase {
	c := hintSeedCase{
		localHead: ids.GenerateTestID(),
		headCanon: ids.GenerateTestID(),
		peerHead:  ids.GenerateTestID(),
	}
	inner6, blk6 := ids.GenerateTestID(), ids.GenerateTestID()
	c.cert = Cert{Block: blk6, Parent: c.peerHead, ParentCanonical: c.headCanon, Height: 6, Canonical: inner6}
	c.led = FinalityLedger{}.withHint(c.localHead, 5)
	return c
}

func (c hintSeedCase) dag(alsoTrackPeerEnvelope bool) wrappedAncestry {
	d := wrappedAncestry{
		c.localHead: {parent: ids.GenerateTestID(), height: 5, canonical: c.headCanon},
		c.cert.Block: {
			parent: c.peerHead, height: 6,
			canonical: c.cert.Canonical, parentCanonical: c.headCanon,
		},
	}
	if alsoTrackPeerEnvelope {
		d[c.peerHead] = wrapperNode{parent: ids.GenerateTestID(), height: 5, canonical: c.headCanon}
	}
	return d
}

// TestHintSeed_SiblingEnvelopeIsTheSameDecision: a node that has restarted with an
// applied head at height 5 and receives a cert at 6 naming a sibling wrapper of
// that head must finalize. Holding the peer's envelope as well must not change the
// verdict — the two arms are the same question about the same execution.
func TestHintSeed_SiblingEnvelopeIsTheSameDecision(t *testing.T) {
	c := newHintSeedCase()

	if _, _, err := Finalize(c.led, c.cert, c.dag(false)); err != nil {
		t.Fatalf("control broke: with the peer's envelope ABSENT the walk substitutes the local "+
			"wrapper and the fresh-process seed advances; got %v", err)
	}

	if _, _, err := Finalize(c.led, c.cert, c.dag(true)); err != nil {
		t.Fatalf("SAME DECISION REFUSED FOR HOLDING MORE: the fresh-process seed accepts this cert "+
			"when the peer's envelope is absent and refuses it when the node also holds that "+
			"envelope — %v. The floor ledger the seed walks from is built as "+
			"{tip: hint, height: hintHeight, set: true} with no canonical, so decision() degrades "+
			"it to the envelope id and the frontier comparison is outer-vs-outer on this door. "+
			"A node whose wrapper of the applied head differs from the network's then refuses "+
			"every cert the network produces, holding byte-identical execution state.", err)
	}
}

// TestHintSeed_DivergentExecutionStillConflicts is the safety half on the same
// door: whatever relaxation the alias case needs must fire for aliases only. A
// genuinely different execution at the hint height is a fork and must be refused.
func TestHintSeed_DivergentExecutionStillConflicts(t *testing.T) {
	c := newHintSeedCase()
	forkHead := ids.GenerateTestID()
	forkCanon := ids.GenerateTestID() // a DIFFERENT execution at height 5

	dag := wrappedAncestry{
		c.localHead:  {parent: ids.GenerateTestID(), height: 5, canonical: c.headCanon},
		forkHead:     {parent: ids.GenerateTestID(), height: 5, canonical: forkCanon},
		c.cert.Block: {parent: forkHead, height: 6, canonical: c.cert.Canonical, parentCanonical: forkCanon},
	}
	cert := c.cert
	cert.Parent, cert.ParentCanonical = forkHead, forkCanon

	if _, _, err := Finalize(c.led, cert, dag); err == nil {
		t.Fatal("a divergent execution at the applied head's height must never seed the frontier")
	}
}

// TestHintSeed_BuildAnchorNeverRegresses. BuildAnchor is defined as the HIGHER of
// {certified tip, recovery hint} — the VM builds and prefers on it, and the whole
// point of keeping the hint is that a node with state at height H does not get sent
// back to build on an ancestor.
//
// A cert at or below the hint height takes the direct-seed arm, and seedLedger
// returns a ledger with no hint at all. So a REPLAYED historical cert — bytes any
// peer can serve, verifying exactly as it did when it was fresh — moves the anchor
// backwards by however far back the replay reaches.
func TestHintSeed_BuildAnchorNeverRegresses(t *testing.T) {
	head := ids.GenerateTestID() // the VM's applied head
	const headHeight = 1000
	led := FinalityLedger{}.withHint(head, headHeight)
	if anchor, ok := led.BuildAnchor(); !ok || anchor != head {
		t.Fatal("control broke: the hint must be the build anchor before any cert")
	}

	oldParent, oldBlock, oldCanon := ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID()
	dag := wrappedAncestry{oldBlock: {parent: oldParent, height: 5, canonical: oldCanon}}
	cert := Cert{Block: oldBlock, Parent: oldParent, Height: 5, Canonical: oldCanon}

	next, plan, err := Finalize(led, cert, dag)
	if err != nil {
		return // refusing the replay is a valid answer too
	}
	anchor, _ := next.BuildAnchor()
	if anchor != head {
		h, _ := next.Height()
		t.Fatalf("the build anchor regressed from the applied head (height %d) to height %d, and "+
			"the fold asks the VM to Accept %v. seedLedger drops the hint, so after a replayed "+
			"historical cert the anchor is the only thing left — BuildAnchor's own rule is the "+
			"HIGHER of {certified tip, recovery hint}, and there is no longer a hint to be higher. "+
			"Every later cert must now prove a tracked ancestry across the %d heights between.",
			headHeight, h, plan.Accept, headHeight-h)
	}
}

// --- the safety half of the frontier rule, on the CERTIFIED door ---------------

// certifiedFrontier is a node certified through height 5 under envelope tipOuter of
// execution tipCanon, with a losing sibling branch at the same height.
type certifiedFrontier struct {
	led                  FinalityLedger
	tipOuter, tipCanon   ids.ID
	loseOuter, loseCanon ids.ID
	blk6, inner6         ids.ID
}

func newCertifiedFrontier() certifiedFrontier {
	f := certifiedFrontier{
		tipOuter: ids.GenerateTestID(), tipCanon: ids.GenerateTestID(),
		loseOuter: ids.GenerateTestID(), loseCanon: ids.GenerateTestID(),
		blk6: ids.GenerateTestID(), inner6: ids.GenerateTestID(),
	}
	f.led = seedLedger(f.tipOuter, f.tipCanon, 5)
	return f
}

// TestFrontier_LosingExecutionNamedByParentCanonical: the untracked-parent arm
// resolves a LOCAL wrapper from the cert's ParentCanonical. That hint names an
// execution, and naming the losing branch's execution must not let the walk stand
// the losing wrapper in as the frontier.
func TestFrontier_LosingExecutionNamedByParentCanonical(t *testing.T) {
	f := newCertifiedFrontier()
	unheld := ids.GenerateTestID() // the cert's parent envelope, not tracked here

	dag := wrappedAncestry{
		f.tipOuter:  {parent: ids.GenerateTestID(), height: 5, canonical: f.tipCanon},
		f.loseOuter: {parent: ids.GenerateTestID(), height: 5, canonical: f.loseCanon},
		f.blk6:      {parent: unheld, height: 6, canonical: f.inner6, parentCanonical: f.loseCanon},
	}
	cert := Cert{Block: f.blk6, Parent: unheld, ParentCanonical: f.loseCanon, Height: 6, Canonical: f.inner6}

	if _, _, err := Finalize(f.led, cert, dag); !errors.Is(err, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("a cert whose parent-canonical names the LOSING execution at the finalized height "+
			"must conflict; the alias substitution may only stand in a wrapper of the SAME "+
			"execution. got %v", err)
	}
}

// TestFrontier_TrackedLosingParentIgnoresTheHint: when the cert's parent IS
// tracked the alias substitution must not fire at all — a parent we hold is
// answered by what we hold, never by what the cert would prefer us to resolve.
func TestFrontier_TrackedLosingParentIgnoresTheHint(t *testing.T) {
	f := newCertifiedFrontier()

	dag := wrappedAncestry{
		f.tipOuter:  {parent: ids.GenerateTestID(), height: 5, canonical: f.tipCanon},
		f.loseOuter: {parent: ids.GenerateTestID(), height: 5, canonical: f.loseCanon},
		f.blk6:      {parent: f.loseOuter, height: 6, canonical: f.inner6, parentCanonical: f.loseCanon},
	}
	// The cert names the losing wrapper as its parent while claiming the TIP's
	// execution as that parent's commitment — the two do not agree, and the tracked
	// block is the authority.
	cert := Cert{Block: f.blk6, Parent: f.loseOuter, ParentCanonical: f.tipCanon, Height: 6, Canonical: f.inner6}

	if _, _, err := Finalize(f.led, cert, dag); !errors.Is(err, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("a tracked parent on the losing branch must conflict regardless of what the cert "+
			"claims its canonical is; got %v", err)
	}
}

// TestFrontier_AliasAtTheWrongHeightIsNotTheFrontier: the alias lookup is keyed on
// (execution, height). A wrapper of the tip's execution recorded at some OTHER
// height must not satisfy the frontier — otherwise a replayed wrapper at any height
// stands in for the tip.
func TestFrontier_AliasAtTheWrongHeightIsNotTheFrontier(t *testing.T) {
	f := newCertifiedFrontier()
	unheld := ids.GenerateTestID()
	misfiled := ids.GenerateTestID() // a wrapper of the TIP execution, recorded at height 4

	dag := wrappedAncestry{
		f.tipOuter: {parent: ids.GenerateTestID(), height: 5, canonical: f.tipCanon},
		misfiled:   {parent: ids.GenerateTestID(), height: 4, canonical: f.tipCanon},
		f.blk6:     {parent: unheld, height: 6, canonical: f.inner6, parentCanonical: f.tipCanon},
	}
	// The cert is at height 6, so the walk looks for the parent execution at height
	// 5 and must not settle for the height-4 copy.
	cert := Cert{Block: f.blk6, Parent: unheld, ParentCanonical: f.tipCanon, Height: 6, Canonical: f.inner6}

	next, _, err := Finalize(f.led, cert, dag)
	if err != nil {
		return // resolving to the real tip and finalizing is also correct
	}
	if h, _ := next.Height(); h != 6 {
		t.Fatalf("finalized height %d after admitting a cert through a misfiled wrapper", h)
	}
	if next.Tip() == misfiled {
		t.Fatal("the walk adopted a height-4 wrapper as the height-5 frontier")
	}
}
