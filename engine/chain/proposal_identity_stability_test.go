// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

// Proposal-identity stability — the 8-gate verification matrix for the ProposalKey
// refactor (value vs place, one canonical path).
//
// DESIGN (Hickey/Pike decomplection): "The outer block ID is a transport ALIAS.
// ProposalKey is consensus IDENTITY. There is exactly ONE own proposal per ProposalKey."
// ProposerVM re-mints the outer envelope ID on every rebuild at the SAME (parent,height)
// off a stale parent — proven live on mainnet 2026-07-03 (luxd-0 rebuilt 1082880 with a
// changing blkID every ~16s, heartbeat PAUSED + clean mempool, so it is NOT
// gas-escalator/mempool-specific). Keying build-dedup on that churning PLACE scattered
// our own votes across sibling candidates so none reached α=4 → no cert → frozen. The
// refactor keys reuse on the stable ProposalKey{parent,height} via the pendingOwnProposals
// index (one reuse decision, one register/drop pair, zero heuristic scans).
//
// Each gate is a real test. WITHOUT the fix (revert the ProposalKey reuse check in
// buildBlocksLocked) the churn gates fail (siblings grow, no cert); WITH it one candidate
// reaches α and the height decides. Gates 2 (different parent), 3 (prune-on-decide) and 6
// (no-double-finalize) are the safety gates RED re-verifies.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// -----------------------------------------------------------------------------
// churnVM — the proposervm re-wrap model at the heart of the mainnet 1082880 freeze.
// BuildBlock returns a block with a FRESH envelope ID on every call, all bearing the
// SAME inner (parentID, height, body) — the identity churn the fix must collapse.
// -----------------------------------------------------------------------------

type churnVM struct {
	mu     sync.Mutex
	parent ids.ID
	height uint64
	body   string
	minted []ids.ID
	last   ids.ID
}

func (m *churnVM) BuildBlock(context.Context) (block.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	blk := &verifyOnceBlock{
		id:        ids.GenerateTestID(), // the re-wrap: a NEW envelope ID every tick
		parentID:  m.parent,
		height:    m.height,
		timestamp: time.Now(),
		bytes:     []byte(m.body),
	}
	m.minted = append(m.minted, blk.id)
	return blk, nil
}

func (m *churnVM) mintedIDs() []ids.ID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ids.ID(nil), m.minted...)
}

func (m *churnVM) advance(parent ids.ID, height uint64) {
	m.mu.Lock()
	m.parent, m.height, m.minted, m.last = parent, height, nil, parent
	m.mu.Unlock()
}

func (m *churnVM) GetBlock(context.Context, ids.ID) (block.Block, error) {
	return nil, errVerifiedAlready
}
func (m *churnVM) ParseBlock(_ context.Context, b []byte) (block.Block, error) {
	return &verifyOnceBlock{bytes: b}, nil
}
func (m *churnVM) LastAccepted(context.Context) (ids.ID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last, nil
}
func (m *churnVM) SetPreference(context.Context, ids.ID) error { return nil }

// -----------------------------------------------------------------------------
// Test-local helpers (race-safe under t.mu; the engine is Started).
// -----------------------------------------------------------------------------

// ownUndecidedAt reads the pendingOwnProposals IDENTITY index directly — the single
// source of truth for "do I already hold my own undecided proposal at this ProposalKey".
func ownUndecidedAt(e *Transitive, parent ids.ID, height uint64) *PendingBlock {
	e.mu.Lock()
	defer e.mu.Unlock()
	pb := e.pendingOwnProposals[ProposalKey{ParentID: parent, Height: height}]
	if pb != nil && pb.Decided {
		return nil
	}
	return pb
}

// ownUndecidedCountAt counts OWN, still-UNDECIDED candidates at (parent,height) in the
// TRANSPORT map — the observable "sibling" count. Post-fix this is 1 no matter how many
// times the VM re-wrapped the height; pre-fix (revert the ProposalKey reuse check) it
// grows one per build tick. This is the direct structural signature of the fix.
func ownUndecidedCountAt(e *Transitive, parent ids.ID, height uint64) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := 0
	for _, pb := range e.pendingBlocks {
		if pb == nil || pb.ConsensusBlock == nil {
			continue
		}
		if pb.IsOwnProposal && !pb.Decided &&
			pb.ConsensusBlock.height == height && pb.ConsensusBlock.parentID == parent {
			n++
		}
	}
	return n
}

func isBlockTracked(e *Transitive, id ids.ID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.pendingBlocks[id]
	return ok
}

// posForTracked reconstructs the engine's canonical vote position for a tracked block —
// the ed25519 analogue of signedVoteForEngine.
func posForTracked(e *Transitive, blockID ids.ID) VotePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()
	pb := e.pendingBlocks[blockID]
	if pb == nil {
		return VotePosition{ChainID: e.chainID, BlockID: blockID}
	}
	return e.blockPositionLocked(pb, blockID)
}

// insertPending seeds a verified pending block directly, mirroring registerOwnProposalLocked:
// an OWN undecided block populates BOTH indices (transport + identity); a remote block or a
// decided one populates ONLY the transport map. No self-vote is recorded (the specific test
// drives votes itself), so it never double-binds node 0's per-height signature.
func insertPending(e *Transitive, blk *verifyOnceBlock, round uint32, own, decided bool) *PendingBlock {
	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix(), data: blk.bytes}
	_ = e.consensus.AddBlock(context.Background(), cb)
	pb := &PendingBlock{
		ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(),
		Round: round, Decided: decided, IsOwnProposal: own,
	}
	e.mu.Lock()
	e.pendingBlocks[blk.id] = pb
	if own && !decided {
		e.pendingOwnProposals[e.proposalKeyOf(cb)] = pb
	}
	e.mu.Unlock()
	return pb
}

// trackOwnProposalPK is trackProposal (records the proposer's own signed self-vote so a
// cert can assemble) PLUS registration in the pendingOwnProposals identity index — the
// full state buildBlocksLocked establishes for an own proposal.
func trackOwnProposalPK(e *Transitive, chainID ids.ID, blk *verifyOnceBlock, round uint32) VotePosition {
	pos := trackProposal(e, chainID, blk, round)
	e.mu.Lock()
	if pb, ok := e.pendingBlocks[blk.id]; ok {
		e.pendingOwnProposals[e.proposalKeyOf(pb.ConsensusBlock)] = pb
	}
	e.mu.Unlock()
	return pos
}

// =============================================================================
// Gate 1 — Churn repro: same parent+height, new envelope IDs → ONE identity, one
// vote target, no sibling churn; it reaches α and decides.
// =============================================================================

func TestReproposalChurn_StableCandidateReachesAlpha(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod()           // K=5, α=4 (production zero-margin quorum)
	params.RoundTO = 30 * time.Second // park the background re-poll ticker: only the BUILD path acts
	e, _ := newQuorumEngine(t, params, vs, 0, &recordingGossiper{})
	cp := newReSolicitProbe()
	e.SetProposer(cp)

	parent := ids.GenerateTestID() // the stale 1082879 parent
	const H = uint64(1082880)
	vm := &churnVM{parent: parent, height: H, body: "churn-1082880", last: parent}
	e.SetVM(vm)

	// Each build tick re-wraps (parent,H) under a FRESH envelope ID — the proposervm
	// behavior off a stale parent that froze mainnet 1082880.
	const ticks = 6
	for i := 0; i < ticks; i++ {
		if err := e.Notify(context.Background(), Message{Type: PendingTxs}); err != nil {
			t.Fatalf("Notify #%d: %v", i, err)
		}
	}
	minted := vm.mintedIDs()
	if len(minted) != ticks {
		t.Fatalf("expected %d re-wrapped envelope IDs, got %d", ticks, len(minted))
	}

	// POST-FIX: exactly ONE own candidate at (parent,H) despite `ticks` re-wraps.
	// PRE-FIX (revert the pendingOwnProposals reuse check in buildBlocksLocked): `ticks`
	// siblings — the vote scatter that starves α (the 1082880 no-cert freeze).
	if n := ownUndecidedCountAt(e, parent, H); n != 1 {
		t.Fatalf("IDENTITY CHURN: expected exactly 1 stable own candidate at (parent,H), got %d — "+
			"each re-wrap added a sibling, scattering votes so none reaches α", n)
	}
	// The identity index keys the FIRST candidate; later re-wraps collapsed onto it.
	stable := minted[0]
	if got := ownUndecidedAt(e, parent, H); got == nil || got.ConsensusBlock.id != stable {
		t.Fatalf("pendingOwnProposals[ProposalKey] must key the FIRST candidate %s, got %v", stable, got)
	}
	for i := 1; i < ticks; i++ {
		if isBlockTracked(e, minted[i]) {
			t.Fatalf("re-wrap #%d (%s) must be dropped, not tracked as a sibling", i, minted[i])
		}
		if cp.requestCount(minted[i]) != 0 {
			t.Fatalf("re-wrap #%d must never be solicited (only the stable candidate is)", i)
		}
	}
	if got := cp.requestCount(stable); got < ticks-1 {
		t.Fatalf("stable candidate must be re-solicited on rebuilds: got %d want >= %d", got, ticks-1)
	}

	// LIVENESS: the ONE stable candidate reaches α=4 and DECIDES. The K>1 build path does
	// not self-bind, so 4 peer signatures are exactly α.
	pos := posForTracked(e, stable)
	for i := 1; i <= 4; i++ {
		e.ReceiveVote(vs.signedVote(i, pos))
	}
	if !waitFor(3*time.Second, func() bool { return e.IsAccepted(stable) }) {
		t.Fatal("the stable candidate must reach α=4 and decide")
	}
}

// =============================================================================
// Gate 2 — Same height, DIFFERENT parent → new ProposalKey, NO collapse (highest safety).
// =============================================================================

func TestProposalStability_DifferentParentNotCollapsed(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

	P := ids.GenerateTestID()
	P2 := ids.GenerateTestID()
	const H = uint64(1082880)

	blk := newTestBlock(H, P, "own-at-P")
	pb := insertPending(e, blk, 0, true, false)

	if got := ownUndecidedAt(e, P, H); got != pb {
		t.Fatalf("(P,H) must return our own undecided candidate, got %v", got)
	}
	// DIFFERENT parent = DIFFERENT ProposalKey → must NOT collapse (would drop a genuine
	// new-parent proposal — a safety break / fork risk).
	if got := ownUndecidedAt(e, P2, H); got != nil {
		t.Fatalf("SAFETY VIOLATION: (P2,H) collapsed onto the (P,H) candidate %v", got)
	}
	// DIFFERENT height = DIFFERENT ProposalKey.
	if got := ownUndecidedAt(e, P, H+1); got != nil {
		t.Fatalf("SAFETY VIOLATION: (P,H+1) collapsed onto the (P,H) candidate %v", got)
	}
}

// =============================================================================
// Gate 3 — Decided own proposal is PRUNED from the identity index (never reused).
// =============================================================================

func TestProposalStability_DecidedCandidateNotReused(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})
	P := ids.GenerateTestID()
	const H = uint64(7)

	blk := newTestBlock(H, P, "to-finalize")
	pos := trackOwnProposalPK(e, chainID, blk, 0) // registers in pendingOwnProposals + self-vote
	if ownUndecidedAt(e, P, H) == nil {
		t.Fatal("setup: candidate must be in the identity index")
	}

	// Finalize through the real cert path.
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))
	e.ReceiveVote(vs.signedVote(3, pos)) // self+1+2+3 = 4 = α → finalizes
	if !waitFor(3*time.Second, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("setup: block must finalize with its α-of-K quorum")
	}

	// PRUNE-ON-DECIDE: dropOwnProposalLocked removed it from the identity index, so a later
	// build at (P,H) can never reuse a decided proposal.
	if got := ownUndecidedAt(e, P, H); got != nil {
		t.Fatalf("a decided proposal must be pruned from pendingOwnProposals, got %v", got)
	}
}

// =============================================================================
// Gate 4 — Remote block populates the transport map but NOT the identity index.
// =============================================================================

func TestProposalStability_RemoteNotCollapsed(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})
	P := ids.GenerateTestID()
	const H = uint64(42)

	rblk := newTestBlock(H, P, "remote")
	insertPending(e, rblk, 0, false, false) // remote: transport map only
	if !isBlockTracked(e, rblk.id) {
		t.Fatal("remote block must be tracked in the transport map (exact-ID lookup)")
	}
	if got := ownUndecidedAt(e, P, H); got != nil {
		t.Fatalf("a REMOTE block must NOT populate the own-proposal identity index, got %v", got)
	}
	// Our OWN block at the same slot DOES populate it.
	oblk := newTestBlock(H, P, "own")
	own := insertPending(e, oblk, 0, true, false)
	if got := ownUndecidedAt(e, P, H); got != own {
		t.Fatalf("our own undecided candidate at (P,H) must be in the identity index, got %v", got)
	}
}

// =============================================================================
// Gate 5 — View/slot replacement allowed (stability never wedges).
// =============================================================================

func TestProposalStability_ViewChangeAllowsReplacement(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})
	P := ids.GenerateTestID()
	const H = uint64(9)

	c1 := newTestBlock(H, P, "c1")
	pb1 := insertPending(e, c1, 0, true, false)
	if got := ownUndecidedAt(e, P, H); got != pb1 {
		t.Fatalf("C1 must be held stable in the identity index, got %v", got)
	}

	// A bare round advance (same block, higher round) is NOT a wedge: ProposalKey is
	// round-agnostic and the round rides the vote position, so the SAME candidate is
	// re-solicited in the higher round — the liveness we want.
	pb1.Round = 3
	if got := ownUndecidedAt(e, P, H); got != pb1 {
		t.Fatalf("a bare round advance must keep the SAME candidate, got %v", got)
	}

	// A view/slot change that ABANDONS the candidate (reject → dropOwnProposalLocked)
	// frees the key so the new view can propose fresh.
	e.mu.Lock()
	e.dropOwnProposalLocked(pb1)
	delete(e.pendingBlocks, c1.id)
	e.mu.Unlock()
	if got := ownUndecidedAt(e, P, H); got != nil {
		t.Fatalf("after abandonment the key must be free for replacement, got %v", got)
	}
	c2 := newTestBlock(H, P, "c2")
	pb2 := insertPending(e, c2, 0, true, false)
	if got := ownUndecidedAt(e, P, H); got != pb2 {
		t.Fatalf("a replacement candidate must be adoptable after invalidation, got %v", got)
	}
}

// =============================================================================
// Gate 6 — No double-finalize (safety): sibling votes never merge, ≤1 decides per height.
// =============================================================================

func TestProposalStability_NoDoubleFinalize(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})
	P := ids.GenerateTestID()
	const H = uint64(1082880)

	// Two sibling candidates at the SAME (P,H) — the pre-fix scatter the ProposalKey index
	// prevents. Even if siblings somehow coexist, the honest vote-once discipline
	// (reserveSlotForSign) + the α-of-K cert guarantee at most ONE finalizes; the fix adds
	// no vote and lowers no threshold, so it cannot create a second cert.
	aBlk := newTestBlock(H, P, "sibling-A")
	posA := trackProposal(e, chainID, aBlk, 0) // A: self-vote recorded (node 0 bound to A@H)

	bBlk := newTestBlock(H, P, "sibling-B")
	insertPending(e, bBlk, 0, true, false) // B: no self-vote (node 0 already bound to A@H)
	posB := posForTracked(e, bBlk.id)

	// Honest validators sign A first — each binds to A's canonical at height H.
	e.ReceiveVote(vs.signedVote(1, posA))
	e.ReceiveVote(vs.signedVote(2, posA))
	e.ReceiveVote(vs.signedVote(3, posA)) // self(A)+1+2+3 = 4 = α → A decides
	if !waitFor(3*time.Second, func() bool { return e.IsAccepted(aBlk.id) }) {
		t.Fatal("sibling A must finalize with its α-of-K quorum")
	}

	// Now try to finalize B. Nodes 1,2,3 already bound to A REFUSE to sign B → unsigned,
	// uncounted. A's signed votes are BlockID=A and can never count for B. Only node 4
	// (uncommitted) signs B → 1 vote, far below α=4.
	e.ReceiveVote(vs.signedVote(1, posB)) // refused (unsigned — won't verify)
	e.ReceiveVote(vs.signedVote(2, posB)) // refused
	e.ReceiveVote(vs.signedVote(3, posB)) // refused
	e.ReceiveVote(vs.signedVote(4, posB)) // 1 real vote
	if waitFor(1*time.Second, func() bool { return e.IsAccepted(bBlk.id) }) {
		t.Fatal("DOUBLE-FINALIZE: sibling B decided at a height already decided by A")
	}
	if !e.IsAccepted(aBlk.id) {
		t.Fatal("A must remain the sole accepted block at height H")
	}
}

// =============================================================================
// Gate 7 — n-matrix quorum unchanged: α = ⌊2k/3⌋+1; finalize at α, not at α-1.
// =============================================================================

func TestProposalStability_QuorumMatrixUnchanged(t *testing.T) {
	rows := []struct{ k, alpha int }{
		{1, 1}, {2, 2}, {3, 3}, {4, 3}, {5, 4}, {6, 5},
	}
	for _, r := range rows {
		if got := bftAlpha(r.k); got != r.alpha {
			t.Fatalf("quorum formula drift: bftAlpha(%d)=%d want %d", r.k, got, r.alpha)
		}
	}
	for _, r := range rows {
		if r.k == 1 {
			continue // K==1 finalizes inline (single_node tests); the count matrix starts at 2
		}
		r := r
		t.Run(fmt.Sprintf("K%d_alpha%d", r.k, r.alpha), func(t *testing.T) {
			// α-1 votes must NOT finalize.
			vsNeg := newTestValidatorSet(r.k)
			eNeg, cidNeg := newQuorumEngine(t, matrixParams(r.k, r.alpha), vsNeg, 0, &recordingGossiper{})
			bNeg := newTestBlock(1, ids.Empty, "matrix-neg")
			posNeg := trackProposal(eNeg, cidNeg, bNeg, 0) // self = 1
			for i := 1; i <= r.alpha-2; i++ {              // self + (α-2) = α-1
				eNeg.ReceiveVote(vsNeg.signedVote(i, posNeg))
			}
			if waitFor(500*time.Millisecond, func() bool { return eNeg.IsAccepted(bNeg.id) }) {
				t.Fatalf("K=%d: finalized on α-1=%d votes — quorum weakened", r.k, r.alpha-1)
			}
			// EXACTLY α votes must finalize.
			vsPos := newTestValidatorSet(r.k)
			ePos, cidPos := newQuorumEngine(t, matrixParams(r.k, r.alpha), vsPos, 0, &recordingGossiper{})
			bPos := newTestBlock(1, ids.Empty, "matrix-pos")
			posPos := trackProposal(ePos, cidPos, bPos, 0) // self = 1
			for i := 1; i <= r.alpha-1; i++ {              // self + (α-1) = α
				ePos.ReceiveVote(vsPos.signedVote(i, posPos))
			}
			if !waitFor(3*time.Second, func() bool { return ePos.IsAccepted(bPos.id) }) {
				t.Fatalf("K=%d: did NOT finalize on exactly α=%d votes", r.k, r.alpha)
			}
		})
	}
}

// =============================================================================
// Gate 8 — Mainnet repro model: envelope churn each tick off a stale parent → one stable
// blkID → α → decides → the NEXT height continues (sustained, not "one block then hang").
// =============================================================================

func TestProposalStability_MainnetReproModel(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod()
	params.RoundTO = 30 * time.Second
	e, _ := newQuorumEngine(t, params, vs, 0, &recordingGossiper{})
	e.SetProposer(newReSolicitProbe())

	parent := ids.GenerateTestID()
	vm := &churnVM{parent: parent, height: 1082880, body: "mainnet-model", last: parent}
	e.SetVM(vm)

	decideOneHeight := func(parent ids.ID, height uint64) ids.ID {
		t.Helper()
		vm.advance(parent, height)
		for i := 0; i < 5; i++ { // envelope churn each build tick
			if err := e.Notify(context.Background(), Message{Type: PendingTxs}); err != nil {
				t.Fatalf("h%d Notify #%d: %v", height, i, err)
			}
		}
		if n := ownUndecidedCountAt(e, parent, height); n != 1 {
			t.Fatalf("h%d: expected 1 stable candidate under churn, got %d (sibling scatter)", height, n)
		}
		stable := vm.mintedIDs()[0]
		pos := posForTracked(e, stable)
		for i := 1; i <= 4; i++ {
			e.ReceiveVote(vs.signedVote(i, pos))
		}
		if !waitFor(3*time.Second, func() bool { return e.IsAccepted(stable) }) {
			t.Fatalf("h%d: stable candidate did not reach α and decide", height)
		}
		return stable
	}

	head := decideOneHeight(parent, 1082880)
	head = decideOneHeight(head, 1082881)
	_ = decideOneHeight(head, 1082882) // sustained
}
