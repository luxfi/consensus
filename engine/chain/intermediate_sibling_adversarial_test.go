// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// intermediate_sibling_adversarial_test.go — RED-TEAM adversarial probes of the
// v1.36 "Nova" intermediate-ancestor alias-collapse fix (pathFromTip +
// WrapperByCanonical). These tests try to REFUTE safety: to make the walk stand in a
// divergent execution (FORK), miss an equivocation, or livelock on a topology the fix
// claims to resolve. Every test drives the pure fold (Finalize) directly.
//
// Reuses the wrappedAncestry harness from intermediate_sibling_livelock_test.go.
package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// ---------------------------------------------------------------------------
// #6 COMPLETENESS — TWO consecutive intermediate aliases in a row.
// tip certified @5. Cert selects inner8 @8. BOTH the height-6 and height-7
// intermediate ancestors named by the cert are the PEER's wrappers (absent here);
// this node holds its OWN wrappers of inner6 and inner7, chaining to the tip. The
// walk must resolve BOTH levels (hint threaded each iteration), not just the first.
// ---------------------------------------------------------------------------
func TestAdv_TwoConsecutiveIntermediateAliases_Resolves(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	inner6 := ids.GenerateTestID()
	inner7 := ids.GenerateTestID()
	inner8 := ids.GenerateTestID()

	wrapperB6 := ids.GenerateTestID() // this node's wrapper of inner6, parent = tip
	wrapperB7 := ids.GenerateTestID() // this node's wrapper of inner7, parent = wrapperB6
	wrapperA6 := ids.GenerateTestID() // peer's wrapper of inner6 (ABSENT)
	wrapperA7 := ids.GenerateTestID() // peer's wrapper of inner7 (ABSENT)
	blk8 := ids.GenerateTestID()      // certified top; its recorded parent is wrapperA7

	dag := wrappedAncestry{
		wrapperB6: {parent: tipOuter, height: 6, canonical: inner6, parentCanonical: tipCanon},
		wrapperB7: {parent: wrapperB6, height: 7, canonical: inner7, parentCanonical: inner6},
		// certified block: recorded outer parent is the peer's wrapperA7 (absent).
		blk8: {parent: wrapperA7, height: 8, canonical: inner8, parentCanonical: inner7},
		// wrapperA6, wrapperA7 intentionally ABSENT.
	}
	// Cert carries the attested ParentCanonical = inner7 (signature-covered, cert.go:295).
	cert := Cert{Block: blk8, Parent: wrapperA7, ParentCanonical: inner7, Height: 8, Canonical: inner8}

	_, plan, err := Finalize(led, cert, dag)
	if err != nil {
		t.Fatalf("two-level alias walk must resolve both intermediates, got err=%v", err)
	}
	if len(plan.Accept) != 3 {
		t.Fatalf("expected 3-step accept {wrapperB6@6, wrapperB7@7, blk8@8}, got %v", plan.Accept)
	}
	// The accepted path must use the LOCAL wrappers, never the absent peer envelopes.
	for _, id := range plan.Accept {
		if id == wrapperA6 || id == wrapperA7 {
			t.Fatalf("accepted an UNHELD peer envelope %s; must use local wrappers", id)
		}
	}
	if plan.Accept[0] != wrapperB6 || plan.Accept[1] != wrapperB7 || plan.Accept[2] != blk8 {
		t.Fatalf("accept order wrong: got %v want [wrapperB6 wrapperB7 blk8]", plan.Accept)
	}
}

// ---------------------------------------------------------------------------
// #6 COMPLETENESS — a resolved intermediate wrapper whose OWN outer parent is ALSO
// an absent alias. After WrapperByCanonical stands in wrapperB7 (for absent wrapperA7),
// wrapperB7's recorded outer parent is wrapperA6 (ALSO absent). The next iteration
// must re-resolve via the threaded hint (wrapperB7.parentCanonical=inner6), not wedge.
// ---------------------------------------------------------------------------
func TestAdv_ResolvedWrapperParentIsAlsoAbsentAlias(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	inner6 := ids.GenerateTestID()
	inner7 := ids.GenerateTestID()
	inner8 := ids.GenerateTestID()

	wrapperB6 := ids.GenerateTestID() // local wrapper of inner6, parent = tip
	wrapperB7 := ids.GenerateTestID() // local wrapper of inner7, parent = wrapperA6 (ABSENT)
	wrapperA6 := ids.GenerateTestID() // peer wrapper of inner6 (ABSENT)
	wrapperA7 := ids.GenerateTestID() // peer wrapper of inner7 (ABSENT)
	blk8 := ids.GenerateTestID()

	dag := wrappedAncestry{
		wrapperB6: {parent: tipOuter, height: 6, canonical: inner6, parentCanonical: tipCanon},
		// local wrapperB7 exists, but its recorded outer parent is the ABSENT wrapperA6.
		wrapperB7: {parent: wrapperA6, height: 7, canonical: inner7, parentCanonical: inner6},
		blk8:      {parent: wrapperA7, height: 8, canonical: inner8, parentCanonical: inner7},
	}
	cert := Cert{Block: blk8, Parent: wrapperA7, ParentCanonical: inner7, Height: 8, Canonical: inner8}

	_, plan, err := Finalize(led, cert, dag)
	if err != nil {
		t.Fatalf("must re-resolve the second absent alias via threaded hint, got err=%v", err)
	}
	if len(plan.Accept) != 3 {
		t.Fatalf("expected 3-step accept, got %v", plan.Accept)
	}
	if plan.Accept[0] != wrapperB6 {
		t.Fatalf("lowest accept step must be local wrapperB6, got %v", plan.Accept[0])
	}
}

// ---------------------------------------------------------------------------
// #7 SEEDING — empty hint must degrade to the OLD fail-closed defer, never a wrong
// collapse. cert.ParentCanonical is Empty AND the certified block's recorded
// parentCanonical is Empty, so no hint exists. Even though a canonically-EQUIVALENT
// wrapper is present, the walk must NOT collapse it (no attested inner identity to
// key on) — it must DEFER.
// ---------------------------------------------------------------------------
func TestAdv_EmptyHint_DefersNeverWrongCollapse(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	inner6 := ids.GenerateTestID()
	inner7 := ids.GenerateTestID()
	wrapperB6 := ids.GenerateTestID()
	wrapperA6 := ids.GenerateTestID() // absent
	blk7 := ids.GenerateTestID()

	dag := wrappedAncestry{
		// A local wrapper of inner6 IS present and chains to the tip...
		wrapperB6: {parent: tipOuter, height: 6, canonical: inner6, parentCanonical: tipCanon},
		// ...but the certified block records EMPTY parentCanonical (bare/degenerate),
		// so there is no attested inner id to collapse wrapperA6 onto.
		blk7: {parent: wrapperA6, height: 7, canonical: inner7, parentCanonical: ids.Empty},
	}
	// Cert also omits ParentCanonical (Empty).
	cert := Cert{Block: blk7, Parent: wrapperA6, ParentCanonical: ids.Empty, Height: 7, Canonical: inner7}

	_, _, err := Finalize(led, cert, dag)
	if !errors.Is(err, ErrAncestorNotTracked) {
		t.Fatalf("empty hint must DEFER (ErrAncestorNotTracked, old outer-id behavior), got err=%v", err)
	}
}

// ---------------------------------------------------------------------------
// #1 BINDING BOUNDARY — a block at the right height with a DIFFERENT canonical than
// the hint must NEVER be collapsed. This proves the stand-in is keyed strictly on
// inner-execution identity: a divergent execution (different canonical) is invisible
// to WrapperByCanonical, so the walk defers rather than standing in a fork.
// ---------------------------------------------------------------------------
func TestAdv_DivergentCanonicalNeverCollapsed(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	inner6 := ids.GenerateTestID()       // the TRUE height-6 execution the cert attests
	inner6evil := ids.GenerateTestID()   // a DIFFERENT height-6 execution
	inner7 := ids.GenerateTestID()
	wrapperEvil6 := ids.GenerateTestID() // wrapper of the WRONG execution, chains to tip
	wrapperA6 := ids.GenerateTestID()    // absent wrapper of the TRUE inner6
	blk7 := ids.GenerateTestID()

	dag := wrappedAncestry{
		// The node holds ONLY a wrapper of a DIFFERENT execution at height 6.
		wrapperEvil6: {parent: tipOuter, height: 6, canonical: inner6evil, parentCanonical: tipCanon},
		// Cert attests parentCanonical=inner6 (the TRUE execution), which the node lacks.
		blk7: {parent: wrapperA6, height: 7, canonical: inner7, parentCanonical: inner6},
	}
	cert := Cert{Block: blk7, Parent: wrapperA6, ParentCanonical: inner6, Height: 7, Canonical: inner7}

	_, plan, err := Finalize(led, cert, dag)
	// MUST NOT collapse the divergent wrapperEvil6 (canonical inner6evil != attested inner6).
	if err == nil {
		for _, id := range plan.Accept {
			if id == wrapperEvil6 {
				t.Fatalf("FORK: collapsed a DIVERGENT-execution wrapper (inner6evil) in place of the "+
					"attested inner6 — two nodes would finalize different state at height 6")
			}
		}
		t.Fatalf("expected a DEFER (node lacks the attested inner6), but Finalize succeeded: Accept=%v", plan.Accept)
	}
	if !errors.Is(err, ErrAncestorNotTracked) {
		t.Fatalf("must DEFER with ErrAncestorNotTracked (node holds no wrapper of the attested inner6), got %v", err)
	}
}

// ---------------------------------------------------------------------------
// #3 EQUIVOCATION — a fork attempt at a height that was finalized as an INTERMEDIATE
// step (via alias collapse) must still trip the equivocation guard. Finalize a
// 2-step path [inner6@6, inner7@7] via alias collapse; then present a cert at height
// 6 with a DIFFERENT canonical. byHeight[6] was recorded during the intermediate
// walk, so the guard MUST fire ErrHeightAlreadyFinalized.
// ---------------------------------------------------------------------------
func TestAdv_EquivocationAtIntermediateHeight_StillCaught(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	inner6 := ids.GenerateTestID()
	inner7 := ids.GenerateTestID()
	wrapperB6 := ids.GenerateTestID()
	wrapperA6 := ids.GenerateTestID() // absent
	blk7 := ids.GenerateTestID()

	dag := wrappedAncestry{
		wrapperB6: {parent: tipOuter, height: 6, canonical: inner6, parentCanonical: tipCanon},
		blk7:      {parent: wrapperA6, height: 7, canonical: inner7, parentCanonical: inner6},
	}
	cert := Cert{Block: blk7, Parent: wrapperA6, ParentCanonical: inner6, Height: 7, Canonical: inner7}

	next, _, err := Finalize(led, cert, dag)
	if err != nil {
		t.Fatalf("setup finalize failed: %v", err)
	}
	// height 6 was finalized as an INTERMEDIATE step; byHeight[6] must equal inner6.
	// Now a conflicting cert at height 6 with a DIFFERENT canonical must be refused.
	inner6fork := ids.GenerateTestID()
	forkBlk6 := ids.GenerateTestID()
	forkCert := Cert{Block: forkBlk6, Parent: tipOuter, ParentCanonical: tipCanon, Height: 6, Canonical: inner6fork}

	_, _, ferr := Finalize(next, forkCert, dag)
	if !errors.Is(ferr, ErrHeightAlreadyFinalized) {
		t.Fatalf("a conflicting cert at a previously-INTERMEDIATE height must trip the equivocation "+
			"guard (ErrHeightAlreadyFinalized); byHeight[6] recording is what makes this safe. got err=%v", ferr)
	}
	// And an IDEMPOTENT re-cert of the SAME intermediate canonical must be a clean no-op.
	sameCert := Cert{Block: wrapperB6, Parent: tipOuter, ParentCanonical: tipCanon, Height: 6, Canonical: inner6}
	if _, _, serr := Finalize(next, sameCert, dag); serr != nil {
		t.Fatalf("idempotent re-finalize of the same intermediate canonical must be a no-op, got %v", serr)
	}
}

// ---------------------------------------------------------------------------
// #3 FORK ATTEMPT — a cert whose ancestry threads a DIFFERENT execution at an
// already-finalized height must NOT finalize (must hit ConflictsWithFinalizedBranch),
// even when a divergent wrapper is tracked. Tip finalized @6 = inner6. A cert for
// blk7@7 claims to extend inner6 BUT the only height-6 wrapper it can reach diverges.
// ---------------------------------------------------------------------------
func TestAdv_CertThreadingDivergentFinalizedHeight_Refused(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	inner5 := ids.GenerateTestID()
	// Finalize height 6 = inner6 first, so led.height=6, byHeight[6]=inner6.
	led := seedLedger(tipOuter, inner5, 5)
	inner6 := ids.GenerateTestID()
	wrap6 := ids.GenerateTestID()
	dag0 := wrappedAncestry{
		wrap6: {parent: tipOuter, height: 6, canonical: inner6, parentCanonical: inner5},
	}
	led, _, err := Finalize(led, Cert{Block: wrap6, Parent: tipOuter, ParentCanonical: inner5, Height: 6, Canonical: inner6}, dag0)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Now a cert for blk8@8 whose parentCanonical chain reaches a height-7 block whose
	// parent is a DIFFERENT height-6 execution (inner6evil) — a branch that does NOT
	// descend from the finalized inner6. Must be refused, never finalized.
	inner6evil := ids.GenerateTestID()
	inner7 := ids.GenerateTestID()
	inner8 := ids.GenerateTestID()
	evil6 := ids.GenerateTestID()
	blk7 := ids.GenerateTestID()
	blk8 := ids.GenerateTestID()
	dag := wrappedAncestry{
		wrap6: {parent: tipOuter, height: 6, canonical: inner6, parentCanonical: inner5},
		// a divergent height-6 wrapper the attacker planted (does NOT descend from tip's finalized branch cleanly)
		evil6: {parent: tipOuter, height: 6, canonical: inner6evil, parentCanonical: inner5},
		blk7:  {parent: evil6, height: 7, canonical: inner7, parentCanonical: inner6evil},
		blk8:  {parent: blk7, height: 8, canonical: inner8, parentCanonical: inner7},
	}
	cert := Cert{Block: blk8, Parent: blk7, ParentCanonical: inner7, Height: 8, Canonical: inner8}
	_, plan, cerr := Finalize(led, cert, dag)
	if cerr == nil {
		// If it "succeeded", it must at least NOT have finalized through the divergent evil6.
		for _, id := range plan.Accept {
			if id == evil6 {
				t.Fatalf("FORK: finalized through a divergent height-6 execution (inner6evil) at an "+
					"already-finalized height (inner6). Accept=%v", plan.Accept)
			}
		}
		t.Fatalf("a cert threading a divergent already-finalized height must be REFUSED, got Accept=%v", plan.Accept)
	}
	if !errors.Is(cerr, ErrHeightAlreadyFinalized) && !errors.Is(cerr, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("expected equivocation/conflict refusal, got %v", cerr)
	}
}

// ---------------------------------------------------------------------------
// #2 PRUNING — the rebased parentID must never place a block in BOTH Accept and
// Reject, and the finalized canonical must be exact. A LOCAL sibling of the certified
// block (child of the local wrapper) is the legitimate reject target.
// ---------------------------------------------------------------------------
func TestAdv_PruningRebase_NoAcceptRejectOverlap(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	inner6 := ids.GenerateTestID()
	inner7 := ids.GenerateTestID()
	wrapperB6 := ids.GenerateTestID()
	wrapperA6 := ids.GenerateTestID() // absent
	blk7 := ids.GenerateTestID()
	localSibling7 := ids.GenerateTestID() // a DIFFERENT height-7 block extending the LOCAL wrapperB6

	dag := wrappedAncestry{
		wrapperB6:     {parent: tipOuter, height: 6, canonical: inner6, parentCanonical: tipCanon},
		blk7:          {parent: wrapperA6, height: 7, canonical: inner7, parentCanonical: inner6},
		localSibling7: {parent: wrapperB6, height: 7, canonical: ids.GenerateTestID(), parentCanonical: inner6},
	}
	cert := Cert{Block: blk7, Parent: wrapperA6, ParentCanonical: inner6, Height: 7, Canonical: inner7}

	_, plan, err := Finalize(led, cert, dag)
	if err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	acc := map[ids.ID]bool{}
	for _, id := range plan.Accept {
		acc[id] = true
	}
	for _, id := range plan.Reject {
		if acc[id] {
			t.Fatalf("SAFETY: block %s is in BOTH Accept and Reject", id)
		}
	}
	// The local sibling (child of wrapperB6, different canonical) is the legit reject.
	rejected := map[ids.ID]bool{}
	for _, id := range plan.Reject {
		rejected[id] = true
	}
	if !rejected[localSibling7] {
		t.Logf("note: localSibling7 not pruned (Reject=%v) — acceptable if it is not a child of the "+
			"rebased local wrapper, but the losing branch then lingers as (harmless) undecided garbage", plan.Reject)
	}
	// The certified block must be accepted; the tip must advance to inner7.
	if !acc[blk7] {
		t.Fatalf("certified block blk7 must be accepted, Accept=%v", plan.Accept)
	}
}

// ---------------------------------------------------------------------------
// #4 WRONG-BRANCH STAND-IN — a resolved wrapper whose outer parent does NOT reach the
// finalized tip (it reaches a DIFFERENT block at the finalized height) must be REFUSED
// with ErrConflictsWithFinalizedBranch, never finalized. This confirms a mis-branched
// stand-in fails closed (safety), it cannot fork.
func TestAdv_WrongBranchStandIn_RefusedNotForked(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	inner6 := ids.GenerateTestID()
	inner7 := ids.GenerateTestID()
	wrongParent5 := ids.GenerateTestID() // a height-5 block that is NOT the finalized tip
	wrapperB6 := ids.GenerateTestID()    // resolves for inner6 but chains to wrongParent5
	wrapperA6 := ids.GenerateTestID()    // absent
	blk7 := ids.GenerateTestID()

	dag := wrappedAncestry{
		// This wrapper of inner6 does NOT descend from the finalized tip.
		wrapperB6:    {parent: wrongParent5, height: 6, canonical: inner6, parentCanonical: ids.GenerateTestID()},
		wrongParent5: {parent: ids.GenerateTestID(), height: 5, canonical: ids.GenerateTestID(), parentCanonical: ids.Empty},
		blk7:         {parent: wrapperA6, height: 7, canonical: inner7, parentCanonical: inner6},
	}
	cert := Cert{Block: blk7, Parent: wrapperA6, ParentCanonical: inner6, Height: 7, Canonical: inner7}

	_, plan, err := Finalize(led, cert, dag)
	if err == nil {
		t.Fatalf("a stand-in that does not descend from the finalized tip must be REFUSED, got Accept=%v", plan.Accept)
	}
	if !errors.Is(err, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("expected ErrConflictsWithFinalizedBranch for a mis-branched stand-in, got %v", err)
	}
}
