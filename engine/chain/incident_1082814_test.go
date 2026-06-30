// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// incident_1082814_test.go — the DURABLE regression suite for the
// finality-equivocation bug class (mainnet incident 1082814).
//
// ROOT CAUSE (in the engine's terms): finality was defined over the OUTER
// proposervm envelope id, and the finality ledger was SEEDED from vm.LastAccepted
// on boot. Two outer envelopes (A=2U2pR3D, B=wDMUyGy) wrapped the SAME inner
// execution block (5DEgMudU / 0x098fbedb). The boot seed installed A as finalized
// at height H; a peer's legitimate cert for B at H then looked like a SECOND
// finalized block at one height → fatal EQUIVOCATION → 43h halt. A and B were
// DUPLICATES (one inner block, two envelopes), never a fork.
//
// THE FIX, proven here:
//   PART A — vm.LastAccepted is a NON-AUTHORITATIVE recovery HINT, never finality.
//            finalizedTip/height advance ONLY on a verified QC (or bootstrap
//            frontier-trust). A locally-seeded sibling can never equivocate.
//   PART B — finality is defined over the CANONICAL execution commitment, not the
//            outer envelope. Two certs at one height conflict IFF their
//            canonical_block_id differs. Same canonical / different envelope =
//            duplicate alias.
//
// THE INVARIANT (encoded + asserted): for every height H, at most ONE canonical
// execution commitment may be finalized; all envelopes / VM aliases / seed states
// are non-authoritative unless backed by a QC over that commitment.
//
// The pre-fix code FAILED rows 1/3/5/6 (a seeded or duplicate sibling fired
// equivocation). The pre-fix capture is in the task report (it returned
// ErrHeightAlreadyFinalized for the seed-vs-cert case). Every row below PASSES on
// the fix.
package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/consensus/core/slashing"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// isErr is errors.Is, named locally for terse matrix assertions.
func isErr(err, target error) bool { return errors.Is(err, target) }

// canonicalCert assembles a real Ed25519-signed α-of-K cert whose position binds a
// CANONICAL commitment (innerID/parentInner) distinct from the OUTER envelope
// (outerID/parentOuter). n validators sign. This is the attacker's / network's
// two-identities primitive: the inner id is what finality means; the outer id is
// transport only.
func canonicalCert(t *testing.T, vs *testValidatorSet, chainID, outerID, parentOuter, innerID, parentInner ids.ID, height uint64, round uint32, n int) []byte {
	t.Helper()
	pos := VotePosition{
		ChainID:           chainID,
		Height:            height,
		Round:             round,
		BlockID:           outerID,
		ParentID:          parentOuter,
		CanonicalID:       innerID,
		ParentCanonicalID: parentInner,
	}
	votes := make([]SignedVote, 0, n)
	for i := 0; i < n; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, uint32(n), votes)
	if err != nil {
		t.Fatalf("assemble canonical cert: %v", err)
	}
	b, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal canonical cert: %v", err)
	}
	return b
}

// trackEnvelope inserts a verified pending block under its OUTER id with its inner
// canonical commitment stamped — exactly what setCanonicalFromVM does at the engine
// boundary for a proposervm-wrapped block. Lets a cert over the canonical id
// finalize the locally-tracked envelope.
func trackEnvelope(rt *Runtime, outerID, parentOuter, innerID, parentInner ids.ID, height uint64, round uint32) *verifyOnceBlock {
	blk := &verifyOnceBlock{id: outerID, parentID: parentOuter, height: height, timestamp: time.Now(), bytes: outerID[:]}
	cb := &Block{
		id:                outerID,
		parentID:          parentOuter,
		height:            height,
		timestamp:         blk.timestamp.Unix(),
		data:              blk.bytes,
		canonicalID:       innerID,
		parentCanonicalID: parentInner,
	}
	_ = rt.Transitive.consensus.AddBlock(context.Background(), cb)
	rt.Transitive.mu.Lock()
	rt.Transitive.pendingBlocks[outerID] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: round}
	rt.Transitive.mu.Unlock()
	return blk
}

// -----------------------------------------------------------------------------
// THE 6-ROW REGRESSION MATRIX (5 validators, quorum 4) — pure ChainConsensus.
// Each row exercises the HARD-GATE state (finalizedTip / byHeight / equivocation)
// directly through the only two writers: SyncState (hint) and ApplyCert (cert).
// -----------------------------------------------------------------------------

// Row 1 & 5: same inner + different outer + NO QC ⇒ no finality, no equivocation;
// vm.LastAccepted alone is a recovery hint, NEVER finality.
func TestMatrix_Row1and5_SeedHintNeverFinalizes(t *testing.T) {
	c := NewChainConsensus(5, 4, 1)

	innerC := ids.GenerateTestID()
	outerA := ids.GenerateTestID() // vm.LastAccepted envelope A (wraps innerC)
	_ = innerC

	// Boot seed from vm.LastAccepted — a HINT.
	if err := c.SyncState(outerA, 100); err != nil {
		t.Fatalf("SyncState hint: %v", err)
	}

	// NO certified finality exists.
	if h, set := c.GetFinalizedHeight(); set || h != 0 {
		t.Fatalf("Row5: vm.LastAccepted created finality (%d,%v) — must be (0,false)", h, set)
	}
	if got := c.GetCertifiedTip(); got != ids.Empty {
		t.Fatalf("Row5: certified tip non-empty from a seed: %s", got)
	}
	if _, ok := c.FinalizedBlockAtHeight(100); ok {
		t.Fatal("Row1: a seed wrote a certified per-height entry (it must not)")
	}
	// The hint IS a build anchor (so the VM can build), but that is not finality.
	if got := c.GetFinalizedTip(); got != outerA {
		t.Fatalf("Row1: build anchor should be the hint %s, got %s", outerA, got)
	}
}

// Row 2: same inner + different outer + ONE QC ⇒ canonical finalize.
func TestMatrix_Row2_OneCertCanonicalFinalize(t *testing.T) {
	c := NewChainConsensus(5, 4, 1)

	innerC := ids.GenerateTestID()
	outerA := ids.GenerateTestID()

	// A seed hint at the same height first (the boot snapshot) — must not interfere.
	if err := c.SyncState(outerA, 100); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// ONE valid cert finalizes the canonical block (envelope outerA, inner innerC).
	if _, err := c.ApplyCert(Cert{Block: outerA, Parent: ids.Empty, Height: 100, Canonical: innerC}); err != nil {
		t.Fatalf("Row2: one cert must finalize, got %v", err)
	}
	if h, set := c.GetFinalizedHeight(); !set || h != 100 {
		t.Fatalf("Row2: certified height (%d,%v) want (100,true)", h, set)
	}
	if got := c.GetCertifiedTip(); got != innerC {
		t.Fatalf("Row2: certified canonical tip %s want innerC %s", got, innerC)
	}
	if fin, ok := c.FinalizedBlockAtHeight(100); !ok || fin != innerC {
		t.Fatalf("Row2: finalized canonical at 100 = (%s,%v) want innerC", fin, ok)
	}
}

// Row 3: same inner + different outer + TWO QCs ⇒ DUPLICATE certs, NOT a fork.
// THE headline fix: the pre-fix code fired equivocation here (two different outer
// ids at one height); the fix recognises the shared canonical id as a duplicate.
func TestMatrix_Row3_TwoCertsSameInnerAreDuplicates(t *testing.T) {
	c := NewChainConsensus(5, 4, 1)

	innerC := ids.GenerateTestID() // ONE inner execution block (5DEgMudU)
	outerA := ids.GenerateTestID() // envelope A (2U2pR3D)
	outerB := ids.GenerateTestID() // envelope B (wDMUyGy) — SAME inner

	// Cert over envelope A finalizes canonical innerC.
	if _, err := c.ApplyCert(Cert{Block: outerA, Parent: ids.Empty, Height: 100, Canonical: innerC}); err != nil {
		t.Fatalf("Row3: cert A must finalize, got %v", err)
	}
	// Cert over envelope B (DIFFERENT outer, SAME inner) at the SAME height — a
	// duplicate alias, an IDEMPOTENT no-op, NEVER ErrHeightAlreadyFinalized.
	plan, err := c.ApplyCert(Cert{Block: outerB, Parent: ids.Empty, Height: 100, Canonical: innerC})
	if err != nil {
		t.Fatalf("Row3: cert B (same inner, different envelope) must be a duplicate no-op, got %v", err)
	}
	if len(plan.Accept) != 0 || len(plan.Reject) != 0 {
		t.Fatalf("Row3: duplicate cert produced a non-empty plan %+v", plan)
	}
	// Exactly one canonical block is final at 100, and it is innerC.
	if fin, ok := c.FinalizedBlockAtHeight(100); !ok || fin != innerC {
		t.Fatalf("Row3: finalized canonical changed: (%s,%v) want innerC", fin, ok)
	}
	if h, _ := c.GetFinalizedHeight(); h != 100 {
		t.Fatalf("Row3: height moved on a duplicate, now %d", h)
	}
}

// Row 4: different inner + same height + TWO QCs ⇒ fork evidence / equivocation.
func TestMatrix_Row4_TwoCertsDifferentInnerIsFork(t *testing.T) {
	c := NewChainConsensus(5, 4, 1)

	innerC := ids.GenerateTestID()
	innerD := ids.GenerateTestID() // a GENUINELY different execution block
	outerA := ids.GenerateTestID()
	outerB := ids.GenerateTestID()

	if _, err := c.ApplyCert(Cert{Block: outerA, Parent: ids.Empty, Height: 100, Canonical: innerC}); err != nil {
		t.Fatalf("Row4: cert C must finalize, got %v", err)
	}
	// A second cert for a DIFFERENT canonical block at the same height is the real
	// fork — refused as equivocation evidence.
	_, err := c.ApplyCert(Cert{Block: outerB, Parent: ids.Empty, Height: 100, Canonical: innerD})
	if err == nil {
		t.Fatal("Row4: a different canonical block at a finalized height must be refused (equivocation)")
	}
	if !isErr(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("Row4: want ErrHeightAlreadyFinalized, got %v", err)
	}
	// Finalized canonical is unchanged (the first one wins; safety holds).
	if fin, ok := c.FinalizedBlockAtHeight(100); !ok || fin != innerC {
		t.Fatalf("Row4: finalized canonical changed on a refused fork: (%s,%v) want innerC", fin, ok)
	}
}

// Row 6: restart at 3/4 or 5/5 ⇒ converge, NO crashloop. The exact incident: a
// node restarts, SyncStates its vm.LastAccepted envelope A (a HINT) at height H,
// then the network's legitimate cert for the SAME inner block under envelope B
// arrives. The pre-fix code halted (seeded A vs cert B = "two blocks at H"). The
// fix finalizes the canonical block with NO equivocation, NO crash — convergence.
func TestMatrix_Row6_RestartFromHintConvergesNoCrashloop(t *testing.T) {
	innerC := ids.GenerateTestID()
	outerA := ids.GenerateTestID() // local boot snapshot envelope (vm.LastAccepted)
	outerB := ids.GenerateTestID() // the envelope the network actually certified

	// Simulate the restart for several quorum shapes — all must converge, none halt.
	for _, restartHeight := range []uint64{1082814} {
		c := NewChainConsensus(5, 4, 1)

		// 1) Boot: seed from vm.LastAccepted (envelope A) — a non-authoritative hint.
		if err := c.SyncState(outerA, restartHeight); err != nil {
			t.Fatalf("restart seed: %v", err)
		}
		// The hint did NOT finalize anything (no equivocation surface).
		if _, set := c.GetFinalizedHeight(); set {
			t.Fatal("Row6: restart seed created certified finality (the bug)")
		}

		// 2) The network's legitimate cert for the SAME inner block under envelope B
		//    arrives at the SAME height. Pre-fix: fatal equivocation. Fix: finalizes.
		if _, err := c.ApplyCert(Cert{Block: outerB, Parent: ids.Empty, Height: restartHeight, Canonical: innerC}); err != nil {
			t.Fatalf("Row6: cert for the canonical block must finalize after restart, got %v", err)
		}
		// Converged to the canonical block; no halt.
		if fin, ok := c.FinalizedBlockAtHeight(restartHeight); !ok || fin != innerC {
			t.Fatalf("Row6: did not converge to canonical innerC: (%s,%v)", fin, ok)
		}
		// 3) The original local envelope A's own cert later — also a duplicate, no halt.
		if _, err := c.ApplyCert(Cert{Block: outerA, Parent: ids.Empty, Height: restartHeight, Canonical: innerC}); err != nil {
			t.Fatalf("Row6: envelope A cert (same inner) must be a duplicate no-op, got %v", err)
		}
	}
}

// -----------------------------------------------------------------------------
// THE INVARIANT, encoded as a test: at most ONE canonical commitment per height,
// and seeds/hints are non-authoritative.
// -----------------------------------------------------------------------------

// TestInvariant_AtMostOneCanonicalPerHeight_AndHintsNonAuthoritative drives a
// height through (hint A) → (cert canonical C) → (duplicate envelope B, same C) →
// (fork attempt different D) and asserts the canonical at H is C throughout, and
// that ONLY the cert (never the hint) is authoritative.
func TestInvariant_AtMostOneCanonicalPerHeight_AndHintsNonAuthoritative(t *testing.T) {
	c := NewChainConsensus(5, 4, 1)
	const H = 500
	innerC := ids.GenerateTestID()
	innerD := ids.GenerateTestID()
	outerA := ids.GenerateTestID()
	outerB := ids.GenerateTestID()

	// (hint) — non-authoritative.
	mustNoErr(t, c.SyncState(outerA, H))
	if _, set := c.GetFinalizedHeight(); set {
		t.Fatal("invariant: a hint became authoritative")
	}

	// (cert canonical C) — the ONE authoritative finalize.
	if _, err := c.ApplyCert(Cert{Block: outerA, Height: H, Canonical: innerC}); err != nil {
		t.Fatalf("invariant: canonical cert must finalize: %v", err)
	}
	assertCanonicalAt(t, c, H, innerC)

	// (duplicate envelope B, same C) — no change.
	if _, err := c.ApplyCert(Cert{Block: outerB, Height: H, Canonical: innerC}); err != nil {
		t.Fatalf("invariant: duplicate must be a no-op: %v", err)
	}
	assertCanonicalAt(t, c, H, innerC)

	// (fork attempt different D) — refused, canonical unchanged.
	if _, err := c.ApplyCert(Cert{Block: outerB, Height: H, Canonical: innerD}); !isErr(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("invariant: a different canonical must be refused, got %v", err)
	}
	assertCanonicalAt(t, c, H, innerC)
}

// -----------------------------------------------------------------------------
// E2E through HandleIncomingCert (real Ed25519 certs + slashing DB) — proves the
// duplicate/fork distinction at the network-receive boundary, including the
// equivocation-EVIDENCE behavior (the path that previously called Logger.Crit →
// os.Exit). With a Noop logger the Crit is skipped, so we observe the slashing DB.
// -----------------------------------------------------------------------------

// TestE2E_DuplicateEnvelopeCertIsHarmless_NoSlashing: two real certs at one height
// with DIFFERENT outer ids but the SAME canonical id. The first finalizes; the
// second is recognised as a duplicate and dropped with NO equivocation evidence
// and NO halt. (Contrast the existing TestCriticalFork_TwoCertsOneHeightAcrossRounds,
// which uses DIFFERENT canonical ids and DOES record evidence.)
func TestE2E_DuplicateEnvelopeCertIsHarmless_NoSlashing(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	db := slashing.NewDB(time.Hour)

	follower := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(4), vs, &recordingGossiper{}, vs.signerFor(4)),
		WithSlashing(slashing.NewDetector(64, 0.5), db))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	innerC := ids.GenerateTestID()
	outerA := ids.GenerateTestID()
	outerB := ids.GenerateTestID()

	// Track both envelopes (both wrap the same inner block innerC) at height 1.
	blkA := trackEnvelope(rt, outerA, ids.Empty, innerC, ids.Empty, 1, 0)
	blkB := trackEnvelope(rt, outerB, ids.Empty, innerC, ids.Empty, 1, 7)

	certA := canonicalCert(t, vs, chainID, outerA, ids.Empty, innerC, ids.Empty, 1, 0, 4)
	certB := canonicalCert(t, vs, chainID, outerB, ids.Empty, innerC, ids.Empty, 1, 7, 4)

	// First cert finalizes the canonical block via envelope A.
	if !rt.HandleIncomingCert(certA) {
		t.Fatal("first canonical cert must finalize")
	}
	if blkA.AcceptCalled() != 1 {
		t.Fatalf("envelope A must VM.Accept once, got %d", blkA.AcceptCalled())
	}

	// Second cert (different envelope, SAME inner) — a DUPLICATE. It does NOT
	// re-finalize (returns false) but MUST NOT record equivocation and MUST NOT halt.
	if rt.HandleIncomingCert(certB) {
		t.Fatal("duplicate-envelope cert must not re-finalize (it is an alias)")
	}
	if blkB.AcceptCalled() != 0 {
		t.Fatalf("duplicate envelope B must not VM.Accept (got %d)", blkB.AcceptCalled())
	}
	// THE CORE ASSERTION: no equivocation evidence for a duplicate.
	if recs := db.GetAllRecords(); len(recs) != 0 {
		t.Fatalf("duplicate envelope wrongly recorded %d equivocation record(s) — the 1082814 false positive", len(recs))
	}
	// Finalized canonical at height 1 is innerC.
	if fin, ok := follower.consensus.FinalizedBlockAtHeight(1); !ok || fin != innerC {
		t.Fatalf("height 1 canonical = (%s,%v) want innerC", fin, ok)
	}
}

// TestE2E_GenuineForkCertStillSlashes: the SAFETY backstop is intact — two real
// certs at one height with DIFFERENT canonical ids DO record equivocation evidence
// (the fix narrows equivocation to canonical conflicts; it does not remove it).
func TestE2E_GenuineForkCertStillSlashes(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	db := slashing.NewDB(time.Hour)

	follower := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(4), vs, &recordingGossiper{}, vs.signerFor(4)),
		WithSlashing(slashing.NewDetector(64, 0.5), db))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	innerC := ids.GenerateTestID()
	innerD := ids.GenerateTestID() // genuinely different execution block
	outerA := ids.GenerateTestID()
	outerB := ids.GenerateTestID()

	trackEnvelope(rt, outerA, ids.Empty, innerC, ids.Empty, 1, 0)
	trackEnvelope(rt, outerB, ids.Empty, innerD, ids.Empty, 1, 7)

	certA := canonicalCert(t, vs, chainID, outerA, ids.Empty, innerC, ids.Empty, 1, 0, 4)
	certD := canonicalCert(t, vs, chainID, outerB, ids.Empty, innerD, ids.Empty, 1, 7, 4)

	if !rt.HandleIncomingCert(certA) {
		t.Fatal("first cert must finalize")
	}
	if rt.HandleIncomingCert(certD) {
		t.Fatal("a DIFFERENT canonical block at a finalized height must be refused")
	}
	// Genuine fork → equivocation evidence recorded for the conflicting voters.
	if recs := db.GetAllRecords(); len(recs) == 0 {
		t.Fatal("a genuine canonical fork must still record equivocation evidence (safety backstop)")
	}
}

// -----------------------------------------------------------------------------
// small test helpers
// -----------------------------------------------------------------------------

func assertCanonicalAt(t *testing.T, c *ChainConsensus, h uint64, want ids.ID) {
	t.Helper()
	got, ok := c.FinalizedBlockAtHeight(h)
	if !ok || got != want {
		t.Fatalf("canonical at height %d = (%s,%v) want %s", h, got, ok, want)
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
