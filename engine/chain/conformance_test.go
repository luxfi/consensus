// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// conformance_test.go — a faithful port of avalanchego's Snowman consensus
// conformance suite (snow/consensus/snowman/consensus_test.go, the ~30
// factory-driven `runConsensusTests` funcs) onto OUR ChainConsensus API. It
// proves the just-landed sibling-tolerant finality conforms to the proven
// Snowman accept/reject/reorg semantics that avalanchego's topological.go
// implements (acceptPreferredChild + rejectTransitively).
//
// WHY a re-port instead of running ava's suite directly: ava's `Consensus`
// interface (Add / RecordPoll / Preference / Processing) BRAIDS confidence
// accumulation, accept, reject, and reorg into one RecordPoll call. Our engine
// DECOMPLECTS them:
//
//   - LIVENESS (β-confidence / α-count): ProcessVote/Poll drive a per-block
//     engine.Driver and set an `accepted` LIVENESS flag at the α accept count.
//     NOTE: the Driver's β-decision is FPC-seeded from crypto/rand (driver.go)
//     and therefore NON-deterministic — so these conformance tests NEVER assert
//     on driver.Decided(); they assert the deterministic α-count flag and the
//     cert ledger.
//   - FINALITY (safety): FinalizeBranch is the SINGLE committer of finalized
//     history. A cert SELECTS a block; FinalizeBranch walks the certified branch
//     from the finalized tip, finalizes the contiguous path (Plan.Accept,
//     ascending) and prunes the losing-sibling subtrees (Plan.Reject) — exactly
//     acceptPreferredChild + rejectTransitively.
//
// So each ava "RecordPoll that ACCEPTS X and REJECTS its sibling Y" maps to a
// FinalizeBranch(X) whose plan accepts X's path and prunes Y's subtree; the ava
// assertion (what is Accepted, what is Rejected, what is still Processing, the
// preference, the last-accepted) is preserved exactly. Where an ava scenario is
// a pure Snowball-trie internal (bit-prefix decisions) with no observable beyond
// "a block can't be accepted before its ancestor", it is mapped to the closest
// faithful ChainConsensus scenario with a comment; the safety assertion is never
// weakened. The naming follows the Lux convention (no Snowman in identifiers; it
// is named here only as the upstream MODEL being conformed to, consistent with
// sibling_reorg_test.go).
//
// API mapping cheat-sheet (ava -> ours):
//
//	Initialize(genesis)         -> first FinalizeBranch(g,0,Empty) seeds the ledger
//	Add(block)                  -> AddBlock(ctx, &Block{...})        (tracking-only)
//	RecordPoll -> ACCEPT path   -> FinalizeBranch(target,h,parent)   (cert finality)
//	RecordPoll -> α confidence  -> ProcessVote(id, accept)           (liveness only)
//	block.Status == Accepted    -> IsAccepted(id) / plan.Accept / ledger
//	block.Status == Rejected    -> retained *Block.rejected / plan.Reject
//	NumProcessing()             -> numProcessing(c)  (tracked && !decided)
//	Preference()                -> GetFinalizedTip() (== Preference() once finalized)
//	IsPreferred / tail          -> build-tips set (siblings coexist pre-cert)
//	LastAccepted()              -> GetFinalizedTip() + GetFinalizedHeight()
//	PreferenceAtHeight(h)       -> FinalizedBlockAtHeight(h)
package chain

import (
	"context"
	"errors"
	mrand "math/rand"
	"testing"

	"github.com/luxfi/ids"
)

// -----------------------------------------------------------------------------
// conformance harness — small helpers layered on the established builders
// (addTracked / trackChild / FinalizeBranch). No parallel engine: these drive
// ChainConsensus directly, which is the abstraction level of ava's `Consensus`.
// -----------------------------------------------------------------------------

// seedGenesis enters genesis as the height-0 finalized head. ChainConsensus has
// no separate Initialize(genesis); the first cert-finalize seeds the per-height
// ledger (the same pattern sibling_reorg_test.go uses).
func seedGenesis(t *testing.T, c *ChainConsensus) ids.ID {
	t.Helper()
	g := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(g, 0, ids.Empty); err != nil {
		t.Fatalf("seed genesis finalize: %v", err)
	}
	return g
}

// trackChild generates a fresh block as a child of parent at height, adds it to
// the live DAG (as the engine does for a verified block), and returns its id AND
// the *Block pointer. The pointer is retained so a test can assert .accepted /
// .rejected even AFTER the block is pruned (prune deletes it from the live map,
// but sets .rejected=true first — so IsRejected(id) would return false post-prune
// while the retained pointer still reads .rejected==true).
func trackChild(c *ChainConsensus, parent ids.ID, height uint64) (ids.ID, *Block) {
	id := ids.GenerateTestID()
	b := &Block{id: id, parentID: parent, height: height}
	_ = c.AddBlock(context.Background(), b)
	return id, b
}

// numProcessing is ava's NumProcessing(): blocks tracked in the live DAG that are
// neither accepted nor rejected. A finalized path block (accepted) and a pruned
// loser (deleted) both leave processing.
func numProcessing(c *ChainConsensus) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	n := 0
	for _, b := range c.blocks {
		if !b.accepted && !b.rejected {
			n++
		}
	}
	return n
}

// isBuildTip reports whether id is an open build tip (ava's IsPreferred for an
// undecided tail, in our decomplected model where competing tips coexist).
func isBuildTip(c *ChainConsensus, id ids.ID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tips[id]
}

func numTips(c *ChainConsensus) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.tips)
}

func idIn(set []ids.ID, id ids.ID) bool {
	for _, x := range set {
		if x == id {
			return true
		}
	}
	return false
}

// =============================================================================
// Lifecycle / tracking conformance
// =============================================================================

// TestConformance_Initialize ports InitializeTest: after init, Preference is
// genesis and nothing is processing.
func TestConformance_Initialize(t *testing.T) {
	c := NewChainConsensus(1, 1, 3)
	g := seedGenesis(t, c)

	if c.Preference() != g {
		t.Fatalf("preference must be genesis, got %s", c.Preference())
	}
	if got := numProcessing(c); got != 0 {
		t.Fatalf("nothing must be processing at init, got %d", got)
	}
	if c.GetFinalizedTip() != g {
		t.Fatalf("finalized tip must be genesis, got %s", c.GetFinalizedTip())
	}
}

// TestConformance_NumProcessing ports NumProcessingTest: 0 -> add child -> 1 ->
// the child finalizes (ava: via RecordPoll; ours: via the cert) -> 0.
func TestConformance_NumProcessing(t *testing.T) {
	c := NewChainConsensus(1, 1, 1)
	g := seedGenesis(t, c)

	if numProcessing(c) != 0 {
		t.Fatalf("genesis-only: 0 processing, got %d", numProcessing(c))
	}
	childID, _ := trackChild(c, g, 1)
	if numProcessing(c) != 1 {
		t.Fatalf("after add: 1 processing, got %d", numProcessing(c))
	}
	if _, err := c.FinalizeBranch(childID, 1, g); err != nil {
		t.Fatalf("finalize child: %v", err)
	}
	if numProcessing(c) != 0 {
		t.Fatalf("after finalize: 0 processing, got %d", numProcessing(c))
	}
	if !c.IsAccepted(childID) {
		t.Fatal("finalized child must be accepted")
	}
}

// TestConformance_AddToTail ports AddToTailTest. ADAPTED: ava asserts
// Preference()==child. Our Preference() surfaces the FINALIZED tip (cert-driven);
// the freshly added tail is the BUILD preference, tracked in the tips set. Adding
// to the tail makes the child the sole build tip and drops the (finalized) parent.
func TestConformance_AddToTail(t *testing.T) {
	c := NewChainConsensus(1, 1, 3)
	g := seedGenesis(t, c)

	childID, _ := trackChild(c, g, 1)

	if !isBuildTip(c, childID) {
		t.Fatal("the new tail must be the (sole) build tip")
	}
	if numTips(c) != 1 {
		t.Fatalf("exactly one build tip after adding to the tail, got %d", numTips(c))
	}
	if isBuildTip(c, g) {
		t.Fatal("the finalized parent must no longer be a build tip")
	}
	// Finality is undisturbed by a build-tip move: Preference stays the cert tip.
	if c.Preference() != g {
		t.Fatalf("finalized preference must stay genesis until a cert selects the child, got %s", c.Preference())
	}
}

// TestConformance_AddToNonTail ports AddToNonTailTest. ADAPTED to showcase the
// sibling TOLERANCE at the heart of the fix: ava asserts a non-tail add doesn't
// move preference; ours asserts BOTH siblings coexist as build tips and NEITHER
// is finalized (a mere sibling add disturbs no finality).
func TestConformance_AddToNonTail(t *testing.T) {
	c := NewChainConsensus(1, 1, 3)
	g := seedGenesis(t, c)

	firstID, _ := trackChild(c, g, 1)
	secondID, _ := trackChild(c, g, 1) // sibling of first under genesis

	if !isBuildTip(c, firstID) || !isBuildTip(c, secondID) {
		t.Fatal("both siblings must coexist as tracked build tips (sibling tolerance)")
	}
	if numTips(c) != 2 {
		t.Fatalf("two competing build tips, got %d", numTips(c))
	}
	if numProcessing(c) != 2 {
		t.Fatalf("both siblings processing, got %d", numProcessing(c))
	}
	// A sibling add must not finalize or reorg anything.
	if c.Preference() != g {
		t.Fatalf("finalized preference must stay genesis, got %s", c.Preference())
	}
	if h, set := c.GetFinalizedHeight(); !set || h != 0 {
		t.Fatalf("finalized height must stay 0, got (%d,%v)", h, set)
	}
}

// TestConformance_AddOnUnknownParent ports AddOnUnknownParentTest. ADAPTED across
// a layer boundary: ava's Add validates the parent and errors. Our AddBlock is
// intentionally PERMISSIVE (tracking is decomplected from finality; parent
// existence + fetch-on-unknown live at the engine layer — see
// fetch_on_unknown_vote_test.go and the errUnknownParentBlock VM gate in
// error_propagation_test.go). The unknown-parent SAFETY is enforced where it
// matters — at FINALIZE: a cert for a block whose ancestry is not tracked DEFERS
// (ErrAncestorNotTracked) and is NEVER finalized on an unproven path.
func TestConformance_AddOnUnknownParent(t *testing.T) {
	c := NewChainConsensus(1, 1, 3)
	_ = seedGenesis(t, c)

	orphanID := ids.GenerateTestID()
	unknownParent := ids.GenerateTestID()

	// AddBlock is tracking-only: it tolerates the orphan...
	if err := c.AddBlock(context.Background(), &Block{id: orphanID, parentID: unknownParent, height: 2}); err != nil {
		t.Fatalf("AddBlock is tracking-only and must tolerate an orphan: %v", err)
	}
	// ...but the orphan can NEVER be finalized: its ancestry does not reach the
	// finalized tip → fail-closed DEFER, never an accept.
	if _, err := c.FinalizeBranch(orphanID, 2, unknownParent); !errors.Is(err, ErrAncestorNotTracked) {
		t.Fatalf("a block on an untracked-ancestor path must DEFER, got %v", err)
	}
	if c.IsAccepted(orphanID) {
		t.Fatal("an orphan must never be accepted")
	}
}

// TestConformance_StatusOrProcessing_PreviouslyAccepted ports
// StatusOrProcessingPreviouslyAcceptedTest. Genesis is finalized: not processing,
// it is the finalized tip and recorded in the per-height ledger. NOTE: genesis is
// the height-0 finalized head but is not a tracked *Block (the seed path does not
// AddBlock it), so the authority for "genesis is accepted" is the LEDGER, not the
// live-DAG IsAccepted.
func TestConformance_StatusOrProcessing_PreviouslyAccepted(t *testing.T) {
	c := NewChainConsensus(1, 1, 3)
	g := seedGenesis(t, c)

	if c.IsRejected(g) {
		t.Fatal("genesis must not be rejected")
	}
	if numProcessing(c) != 0 {
		t.Fatalf("genesis must not be processing, got %d", numProcessing(c))
	}
	if c.GetFinalizedTip() != g {
		t.Fatalf("genesis is the finalized tip, got %s", c.GetFinalizedTip())
	}
	if fin, ok := c.FinalizedBlockAtHeight(0); !ok || fin != g {
		t.Fatalf("genesis recorded at height 0, got (%s,%v)", fin, ok)
	}
}

// TestConformance_StatusOrProcessing_PreviouslyRejected ports
// StatusOrProcessingPreviouslyRejectedTest. A rejected block is not processing,
// not a build tip, and not finalized. Driven via the α-of-K REJECT-vote path
// (which keeps the block queryable; the cert-prune path deletes it).
func TestConformance_StatusOrProcessing_PreviouslyRejected(t *testing.T) {
	c := NewChainConsensus(1, 1, 3) // α=1 → a single reject vote rejects
	g := seedGenesis(t, c)

	bID, _ := trackChild(c, g, 1)
	if err := c.ProcessVote(context.Background(), bID, false); err != nil {
		t.Fatalf("reject vote: %v", err)
	}
	if !c.IsRejected(bID) {
		t.Fatal("block must report rejected after an α-quorum of reject votes")
	}
	if c.IsAccepted(bID) {
		t.Fatal("a rejected block must not be accepted")
	}
	if isBuildTip(c, bID) {
		t.Fatal("a rejected block is not a build tip")
	}
	if _, ok := c.FinalizedBlockAtHeight(1); ok {
		t.Fatal("a rejected block must not be finalized at its height")
	}
}

// TestConformance_StatusOrProcessing_Unissued ports
// StatusOrProcessingUnissuedTest. A built-but-never-added block is untracked: not
// processing, not preferred, not decided.
func TestConformance_StatusOrProcessing_Unissued(t *testing.T) {
	c := NewChainConsensus(1, 1, 3)
	_ = seedGenesis(t, c)

	unissued := ids.GenerateTestID()
	if _, ok := c.GetBlock(unissued); ok {
		t.Fatal("an unissued block must not be tracked")
	}
	if c.IsAccepted(unissued) || c.IsRejected(unissued) {
		t.Fatal("an unissued block is undecided")
	}
	if isBuildTip(c, unissued) {
		t.Fatal("an unissued block is not a build tip")
	}
}

// TestConformance_StatusOrProcessing_Issued ports StatusOrProcessingIssuedTest.
// An added-but-undecided block is processing and is a preferred build tip.
func TestConformance_StatusOrProcessing_Issued(t *testing.T) {
	c := NewChainConsensus(1, 1, 3)
	g := seedGenesis(t, c)

	bID, _ := trackChild(c, g, 1)
	if _, ok := c.GetBlock(bID); !ok {
		t.Fatal("an issued block must be tracked")
	}
	if c.IsAccepted(bID) || c.IsRejected(bID) {
		t.Fatal("a freshly issued block is undecided")
	}
	if !isBuildTip(c, bID) {
		t.Fatal("an issued tail block must be a build tip")
	}
	if numProcessing(c) != 1 {
		t.Fatalf("the issued block must be processing, got %d", numProcessing(c))
	}
}

// =============================================================================
// RecordPoll conformance — accept / reject / reorg (the high-value proofs)
// =============================================================================

// TestConformance_RecordPollAcceptSingleBlock ports RecordPollAcceptSingleBlockTest:
// a single block finalizes, leaving 0 processing and advancing the tip. (ava needs
// β polls; our cert is the artifact those β rounds produce — a single
// FinalizeBranch commits it, see the package header.)
func TestConformance_RecordPollAcceptSingleBlock(t *testing.T) {
	c := NewChainConsensus(1, 1, 2)
	g := seedGenesis(t, c)

	blkID, _ := trackChild(c, g, 1)
	if numProcessing(c) != 1 {
		t.Fatalf("block processes before finality, got %d", numProcessing(c))
	}
	plan, err := c.FinalizeBranch(blkID, 1, g)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if len(plan.Accept) != 1 || plan.Accept[0] != blkID || len(plan.Reject) != 0 {
		t.Fatalf("single-block plan: accepted=%v pruned=%v", plan.Accept, plan.Reject)
	}
	if !c.IsAccepted(blkID) {
		t.Fatal("block must be accepted")
	}
	if numProcessing(c) != 0 {
		t.Fatalf("0 processing after finality, got %d", numProcessing(c))
	}
	if c.GetFinalizedTip() != blkID {
		t.Fatalf("tip advances to the block, got %s", c.GetFinalizedTip())
	}
}

// TestConformance_RecordPollAcceptAndReject ports RecordPollAcceptAndRejectTest —
// accept one block, reject its sibling. The cert SELECTS first; FinalizeBranch
// accepts first and PRUNES (rejects) the sibling second. This is the core
// accept-and-reject reorg.
func TestConformance_RecordPollAcceptAndReject(t *testing.T) {
	c := NewChainConsensus(1, 1, 2)
	g := seedGenesis(t, c)

	firstID, firstB := trackChild(c, g, 1)
	secondID, secondB := trackChild(c, g, 1)

	if numProcessing(c) != 2 {
		t.Fatalf("both siblings process before the cert, got %d", numProcessing(c))
	}

	plan, err := c.FinalizeBranch(firstID, 1, g)
	if err != nil {
		t.Fatalf("finalize first: %v", err)
	}
	if len(plan.Accept) != 1 || plan.Accept[0] != firstID {
		t.Fatalf("first must be the finalized path, got %v", plan.Accept)
	}
	if len(plan.Reject) != 1 || plan.Reject[0] != secondID {
		t.Fatalf("the sibling second must be the pruned set, got %v", plan.Reject)
	}
	if !c.IsAccepted(firstID) || !firstB.accepted {
		t.Fatal("first must be accepted")
	}
	if !secondB.rejected {
		t.Fatal("sibling second must be rejected (pruned)")
	}
	if _, ok := c.GetBlock(secondID); ok {
		t.Fatal("the pruned sibling must be removed from the live DAG")
	}
	if fin, _ := c.FinalizedBlockAtHeight(1); fin != firstID {
		t.Fatalf("height 1 must be finalized to first, got %s", fin)
	}
	if c.GetFinalizedTip() != firstID {
		t.Fatalf("tip must be first, got %s", c.GetFinalizedTip())
	}
	if numProcessing(c) != 0 {
		t.Fatalf("0 processing after accept+reject, got %d", numProcessing(c))
	}
}

// TestConformance_RecordPollSplitVoteNoChange ports RecordPollSplitVoteNoChangeTest
// — the β-confidence proof, and the regression guard for the OLD premature
// `acceptVotes>=alpha` accept. With K=2,α=2 a SPLIT poll gives each sibling 1 of 2
// votes (below α) so NEITHER decides; then we show α is the true threshold (one
// more vote sets the LIVENESS flag) and that even the α-count NEVER advances the
// finalized ledger (finality is the cert path only).
func TestConformance_RecordPollSplitVoteNoChange(t *testing.T) {
	c := NewChainConsensus(2, 2, 1) // K=2, α=2
	g := seedGenesis(t, c)

	firstID, _ := trackChild(c, g, 1)
	secondID, _ := trackChild(c, g, 1)

	// One split poll: one accept chit to each sibling. Below α=2 → no decision.
	if err := c.ProcessVote(context.Background(), firstID, true); err != nil {
		t.Fatalf("vote first: %v", err)
	}
	if err := c.ProcessVote(context.Background(), secondID, true); err != nil {
		t.Fatalf("vote second: %v", err)
	}
	if c.IsAccepted(firstID) || c.IsAccepted(secondID) {
		t.Fatal("SPLIT VOTE DECIDED below α — premature-accept regression / β-confidence violated")
	}
	if _, ok := c.FinalizedBlockAtHeight(1); ok {
		t.Fatal("a split vote must finalize nothing at height 1")
	}
	if numProcessing(c) != 2 {
		t.Fatalf("both siblings remain processing under a split, got %d", numProcessing(c))
	}

	// α is the threshold (not premature): one more accept lifts `first` to the
	// α-count LIVENESS flag — but that is STILL not finality.
	if err := c.ProcessVote(context.Background(), firstID, true); err != nil {
		t.Fatalf("second vote first: %v", err)
	}
	if !c.IsAccepted(firstID) {
		t.Fatal("α accepts must set the liveness flag")
	}
	if _, ok := c.FinalizedBlockAtHeight(1); ok {
		t.Fatal("the α-count is LIVENESS only — it must never advance finalized history")
	}
}

// TestConformance_RecordPollWhenFinalized ports RecordPollWhenFinalizedTest:
// re-finalizing the already-finalized head is an idempotent no-op.
func TestConformance_RecordPollWhenFinalized(t *testing.T) {
	c := NewChainConsensus(1, 1, 1)
	g := seedGenesis(t, c)

	plan, err := c.FinalizeBranch(g, 0, ids.Empty)
	if err != nil {
		t.Fatalf("idempotent re-finalize must be nil-error: %v", err)
	}
	if len(plan.Accept) != 0 || len(plan.Reject) != 0 {
		t.Fatalf("idempotent finalize must be an empty plan, got %+v", plan)
	}
	if numProcessing(c) != 0 {
		t.Fatalf("nothing processing, got %d", numProcessing(c))
	}
	if c.Preference() != g {
		t.Fatalf("preference stays genesis, got %s", c.Preference())
	}
}

// TestConformance_RecordPollRejectTransitively ports RecordPollRejectTransitivelyTest
// — reject a block AND all its descendants. g->{block0}, g->block1->block2. A cert
// selects block0; block1 and its descendant block2 are pruned as one subtree
// (== Plan.Reject == rejectTransitively).
func TestConformance_RecordPollRejectTransitively(t *testing.T) {
	c := NewChainConsensus(1, 1, 1)
	g := seedGenesis(t, c)

	block0, _ := trackChild(c, g, 1)
	block1, block1B := trackChild(c, g, 1)
	block2, block2B := trackChild(c, block1, 2)

	plan, err := c.FinalizeBranch(block0, 1, g)
	if err != nil {
		t.Fatalf("finalize block0: %v", err)
	}
	if !c.IsAccepted(block0) {
		t.Fatal("block0 must be accepted")
	}
	if len(plan.Reject) != 2 || !idIn(plan.Reject, block1) || !idIn(plan.Reject, block2) {
		t.Fatalf("the losing subtree {block1,block2} must be pruned, got %v", plan.Reject)
	}
	if !block1B.rejected || !block2B.rejected {
		t.Fatal("block1 and its descendant block2 must be rejected transitively")
	}
	if _, ok := c.GetBlock(block1); ok {
		t.Fatal("pruned block1 removed from live DAG")
	}
	if _, ok := c.GetBlock(block2); ok {
		t.Fatal("pruned block2 removed from live DAG")
	}
	if numProcessing(c) != 0 {
		t.Fatalf("0 processing after the transitive reject, got %d", numProcessing(c))
	}
	if c.GetFinalizedTip() != block0 {
		t.Fatalf("tip must be block0, got %s", c.GetFinalizedTip())
	}
}

// TestConformance_RecordPollTransitivelyResetConfidence ports
// RecordPollTransitivelyResetConfidenceTest. ADAPTED: ava reaches the end-state via
// Snowball confidence flapping between block2 and its sibling block3; our cert
// commits the same COMMITTED end-state directly. g->{block0,block1},
// block1->{block2,block3}. Finalize block1 (prunes block0), then block3 (prunes
// block2). End state == ava: block1,block3 accepted; block0,block2 rejected.
func TestConformance_RecordPollTransitivelyResetConfidence(t *testing.T) {
	c := NewChainConsensus(1, 1, 2)
	g := seedGenesis(t, c)

	block0, block0B := trackChild(c, g, 1)
	block1, _ := trackChild(c, g, 1)
	block2, block2B := trackChild(c, block1, 2)
	block3, _ := trackChild(c, block1, 2)

	// Cert selects block1 at height 1 → block0 (its sibling) is pruned.
	if _, err := c.FinalizeBranch(block1, 1, g); err != nil {
		t.Fatalf("finalize block1: %v", err)
	}
	if !block0B.rejected {
		t.Fatal("block0 must be pruned when block1 finalizes")
	}
	// Cert selects block3 at height 2 → block2 (its sibling) is pruned.
	if _, err := c.FinalizeBranch(block3, 2, block1); err != nil {
		t.Fatalf("finalize block3: %v", err)
	}
	if !block2B.rejected {
		t.Fatal("block2 must be pruned when block3 finalizes")
	}

	if !c.IsAccepted(block1) || !c.IsAccepted(block3) {
		t.Fatal("block1 and block3 must be accepted")
	}
	if c.IsAccepted(block0) || c.IsAccepted(block2) {
		t.Fatal("the losing blocks block0,block2 must never be accepted")
	}
	if fin, _ := c.FinalizedBlockAtHeight(1); fin != block1 {
		t.Fatalf("height 1 finalized to block1, got %s", fin)
	}
	if fin, _ := c.FinalizedBlockAtHeight(2); fin != block3 {
		t.Fatalf("height 2 finalized to block3, got %s", fin)
	}
}

// TestConformance_RecordPollInvalidVote ports RecordPollInvalidVoteTest: a vote for
// an unknown block id is harmless — ProcessVote errors and mutates nothing, Poll
// skips it, and the valid block still finalizes.
func TestConformance_RecordPollInvalidVote(t *testing.T) {
	c := NewChainConsensus(1, 1, 2)
	g := seedGenesis(t, c)

	bID, _ := trackChild(c, g, 1)
	unknown := ids.GenerateTestID()

	if err := c.ProcessVote(context.Background(), unknown, true); err == nil {
		t.Fatal("a vote for an unknown block must error")
	}
	// Poll tolerates an unknown id in the response map (skips it).
	if err := c.Poll(context.Background(), map[ids.ID]int{unknown: 3}); err != nil {
		t.Fatalf("Poll must tolerate unknown ids: %v", err)
	}
	// The valid block is unaffected and finalizes normally.
	if _, err := c.FinalizeBranch(bID, 1, g); err != nil {
		t.Fatalf("valid block must finalize: %v", err)
	}
	if !c.IsAccepted(bID) {
		t.Fatal("the valid block must be accepted")
	}
	if c.GetFinalizedTip() != bID {
		t.Fatalf("tip must be the valid block, got %s", c.GetFinalizedTip())
	}
}

// TestConformance_RecordPollTransitiveVoting ports RecordPollTransitiveVotingTest.
// g->block0->{block1->block2, block3->block4}. First finalize the common ancestor
// block0; then a cert for block2 (a 2-step jump along block1->block2) finalizes
// block1,block2 and prunes the competing block3->block4 subtree. End state: block0,
// block1,block2 accepted; block3,block4 rejected.
func TestConformance_RecordPollTransitiveVoting(t *testing.T) {
	c := NewChainConsensus(3, 3, 1)
	g := seedGenesis(t, c)

	block0, _ := trackChild(c, g, 1)
	block1, _ := trackChild(c, block0, 2)
	block2, _ := trackChild(c, block1, 3)
	block3, block3B := trackChild(c, block0, 2)
	block4, block4B := trackChild(c, block3, 3)

	// Finalize the common ancestor (no siblings to prune at height 1).
	if _, err := c.FinalizeBranch(block0, 1, g); err != nil {
		t.Fatalf("finalize block0: %v", err)
	}
	// Cert for block2: path block1->block2 finalizes in one call; the competing
	// block3->block4 subtree (sibling of block1 under block0) is pruned.
	plan, err := c.FinalizeBranch(block2, 3, block1)
	if err != nil {
		t.Fatalf("finalize block2: %v", err)
	}
	if len(plan.Accept) != 2 || plan.Accept[0] != block1 || plan.Accept[1] != block2 {
		t.Fatalf("path must be {block1,block2} ascending, got %v", plan.Accept)
	}
	if len(plan.Reject) != 2 || !idIn(plan.Reject, block3) || !idIn(plan.Reject, block4) {
		t.Fatalf("losing subtree {block3,block4} must be pruned, got %v", plan.Reject)
	}
	for _, id := range []ids.ID{block0, block1, block2} {
		if !c.IsAccepted(id) {
			t.Fatalf("%s must be accepted", id)
		}
	}
	if !block3B.rejected || !block4B.rejected {
		t.Fatal("block3,block4 must be rejected transitively")
	}
	if numProcessing(c) != 0 {
		t.Fatalf("0 processing after the reorg, got %d", numProcessing(c))
	}
	if c.GetFinalizedTip() != block2 {
		t.Fatalf("tip must be block2, got %s", c.GetFinalizedTip())
	}
}

// TestConformance_RecordPollDivergedVoting ports
// RecordPollDivergedVotingWithNoConflictingBitTest. ADAPTED: ava's test is a
// Snowball binary-trie internal (votes over already-rejected bits are dropped) —
// its only observable safety property is "a block whose ancestor is effectively
// rejected can never be accepted." In our model: a block descending from a LOSING
// branch is refused (ErrConflictsWithFinalizedBranch) and never accepted.
func TestConformance_RecordPollDivergedVoting(t *testing.T) {
	c := NewChainConsensus(1, 1, 2)
	g := seedGenesis(t, c)

	a1, _ := trackChild(c, g, 1)
	// The a-branch wins height 1.
	if _, err := c.FinalizeBranch(a1, 1, g); err != nil {
		t.Fatalf("finalize a1: %v", err)
	}
	// The losing branch appears AFTER: b1 (sibling of a1) and its child b2.
	b1, _ := trackChild(c, g, 1)
	b2, _ := trackChild(c, b1, 2)

	// b2 can never be accepted: its ancestry reaches b1, a losing sibling at the
	// already-finalized height 1.
	if _, err := c.FinalizeBranch(b2, 2, b1); !errors.Is(err, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("a block on a losing branch must be refused as a conflict, got %v", err)
	}
	if c.IsAccepted(b2) {
		t.Fatal("b2 (ancestor effectively rejected) must never be accepted")
	}
}

// TestConformance_RecordPollChangePreferredChain ports
// RecordPollChangePreferredChainTest — the sibling-tolerance CORE. Two sibling
// chains a1->a2 and b1->b2 coexist; a cert SELECTS one branch, finalizing it and
// pruning the other. ADAPTED: ava drives this with pre-finality Snowball preference
// that can flap back and forth before β; our finality is CERT-DRIVEN and
// IRREVERSIBLE (the safety property). The faithful invariant is therefore: the
// engine can finalize EITHER sibling branch (no per-height deadlock) and the prune
// is exact. We prove BOTH selections in independent instances (symmetry).
func TestConformance_RecordPollChangePreferredChain(t *testing.T) {
	selectBranch := func(t *testing.T, finalizeA bool) {
		t.Helper()
		c := NewChainConsensus(1, 1, 10)
		g := seedGenesis(t, c)

		a1, a1B := trackChild(c, g, 1)
		a2, a2B := trackChild(c, a1, 2)
		b1, b1B := trackChild(c, g, 1)
		b2, b2B := trackChild(c, b1, 2)

		// Both sibling-chain tips coexist before any cert (sibling tolerance).
		if !isBuildTip(c, a2) || !isBuildTip(c, b2) {
			t.Fatal("both sibling-chain tips must coexist as build tips")
		}

		if finalizeA {
			plan, err := c.FinalizeBranch(a2, 2, a1)
			if err != nil {
				t.Fatalf("finalize a-chain: %v", err)
			}
			if len(plan.Accept) != 2 || plan.Accept[0] != a1 || plan.Accept[1] != a2 {
				t.Fatalf("a-chain path must be {a1,a2} ascending, got %v", plan.Accept)
			}
			if len(plan.Reject) != 2 || !idIn(plan.Reject, b1) || !idIn(plan.Reject, b2) {
				t.Fatalf("b-chain must be pruned, got %v", plan.Reject)
			}
			if !a1B.accepted || !a2B.accepted {
				t.Fatal("a-chain must be accepted")
			}
			if !b1B.rejected || !b2B.rejected {
				t.Fatal("b-chain must be rejected")
			}
			if c.GetFinalizedTip() != a2 {
				t.Fatalf("tip must be a2, got %s", c.GetFinalizedTip())
			}
		} else {
			plan, err := c.FinalizeBranch(b2, 2, b1)
			if err != nil {
				t.Fatalf("finalize b-chain: %v", err)
			}
			if len(plan.Accept) != 2 || plan.Accept[0] != b1 || plan.Accept[1] != b2 {
				t.Fatalf("b-chain path must be {b1,b2} ascending, got %v", plan.Accept)
			}
			if len(plan.Reject) != 2 || !idIn(plan.Reject, a1) || !idIn(plan.Reject, a2) {
				t.Fatalf("a-chain must be pruned, got %v", plan.Reject)
			}
			if !b1B.accepted || !b2B.accepted {
				t.Fatal("b-chain must be accepted")
			}
			if !a1B.rejected || !a2B.rejected {
				t.Fatal("a-chain must be rejected")
			}
			if c.GetFinalizedTip() != b2 {
				t.Fatalf("tip must be b2, got %s", c.GetFinalizedTip())
			}
		}
	}

	t.Run("cert_selects_a_chain", func(t *testing.T) { selectBranch(t, true) })
	t.Run("cert_selects_b_chain", func(t *testing.T) { selectBranch(t, false) })
}

// TestConformance_LastAccepted ports LastAcceptedTest. GetFinalizedTip() +
// GetFinalizedHeight() ARE LastAccepted(). g->block0->block1->block2 with a
// block1Conflict sibling of block1. The tip advances exactly as each block
// finalizes, the conflict is pruned when block1 wins.
func TestConformance_LastAccepted(t *testing.T) {
	c := NewChainConsensus(1, 1, 2)
	g := seedGenesis(t, c)

	// LastAccepted == genesis initially.
	if c.GetFinalizedTip() != g {
		t.Fatalf("initial LastAccepted is genesis, got %s", c.GetFinalizedTip())
	}
	if h, set := c.GetFinalizedHeight(); !set || h != 0 {
		t.Fatalf("initial height 0, got (%d,%v)", h, set)
	}

	block0, _ := trackChild(c, g, 1)
	block1, _ := trackChild(c, block0, 2)
	block1Conflict, conflictB := trackChild(c, block0, 2) // sibling of block1
	block2, _ := trackChild(c, block1, 3)

	// Adding blocks does not advance LastAccepted.
	if c.GetFinalizedTip() != g {
		t.Fatalf("adds must not advance LastAccepted, got %s", c.GetFinalizedTip())
	}

	if _, err := c.FinalizeBranch(block0, 1, g); err != nil {
		t.Fatalf("finalize block0: %v", err)
	}
	if c.GetFinalizedTip() != block0 {
		t.Fatalf("LastAccepted -> block0, got %s", c.GetFinalizedTip())
	}
	if h, _ := c.GetFinalizedHeight(); h != 1 {
		t.Fatalf("height -> 1, got %d", h)
	}

	if _, err := c.FinalizeBranch(block1, 2, block0); err != nil {
		t.Fatalf("finalize block1: %v", err)
	}
	if c.GetFinalizedTip() != block1 {
		t.Fatalf("LastAccepted -> block1, got %s", c.GetFinalizedTip())
	}
	if h, _ := c.GetFinalizedHeight(); h != 2 {
		t.Fatalf("height -> 2, got %d", h)
	}
	if !conflictB.rejected {
		t.Fatal("block1Conflict must be pruned when block1 finalizes")
	}
	if _, ok := c.GetBlock(block1Conflict); ok {
		t.Fatal("block1Conflict removed from live DAG")
	}

	if _, err := c.FinalizeBranch(block2, 3, block1); err != nil {
		t.Fatalf("finalize block2: %v", err)
	}
	if c.GetFinalizedTip() != block2 {
		t.Fatalf("LastAccepted -> block2, got %s", c.GetFinalizedTip())
	}
	if h, _ := c.GetFinalizedHeight(); h != 3 {
		t.Fatalf("height -> 3, got %d", h)
	}
}

// =============================================================================
// Accept/Reject plan conformance (ava's Error-on-Accept/Reject probes)
// =============================================================================
//
// ava forces block.AcceptV/RejectV = errTest and asserts RecordPoll surfaces the
// error — its INDIRECT way of proving Accept/Reject is invoked on a specific
// block. Our FinalizeBranch returns the Accept/Reject sets EXPLICITLY in the plan,
// so we assert the exact designated set (a STRONGER, direct assertion). The engine
// applies these via applyBranchFinalization (VM.Accept ascending, VM.Reject the
// pruned subtree); VM-side Accept/Reject ERROR propagation is an engine concern,
// already proven in error_propagation_test.go.

// TestConformance_ErrorOnAccept ports ErrorOnAcceptTest: the finalized block is the
// Accept target.
func TestConformance_ErrorOnAccept(t *testing.T) {
	c := NewChainConsensus(1, 1, 1)
	g := seedGenesis(t, c)

	block0, block0B := trackChild(c, g, 1)
	plan, err := c.FinalizeBranch(block0, 1, g)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if len(plan.Accept) != 1 || plan.Accept[0] != block0 {
		t.Fatalf("block0 must be the Accept target, got %v", plan.Accept)
	}
	if !c.IsAccepted(block0) || !block0B.accepted {
		t.Fatal("block0 must be accepted")
	}
}

// TestConformance_ErrorOnRejectSibling ports ErrorOnRejectSiblingTest: accepting
// block0 designates its sibling block1 for Reject (the Pruned set).
func TestConformance_ErrorOnRejectSibling(t *testing.T) {
	c := NewChainConsensus(1, 1, 1)
	g := seedGenesis(t, c)

	block0, _ := trackChild(c, g, 1)
	block1, block1B := trackChild(c, g, 1)

	plan, err := c.FinalizeBranch(block0, 1, g)
	if err != nil {
		t.Fatalf("finalize block0: %v", err)
	}
	if len(plan.Reject) != 1 || plan.Reject[0] != block1 {
		t.Fatalf("the sibling block1 must be the Reject set, got %v", plan.Reject)
	}
	if !block1B.rejected {
		t.Fatal("block1 must be rejected")
	}
}

// TestConformance_ErrorOnTransitiveRejection ports ErrorOnTransitiveRejectionTest:
// accepting block0 designates the sibling block1 AND its descendant block2 for
// Reject (the transitive Pruned subtree).
func TestConformance_ErrorOnTransitiveRejection(t *testing.T) {
	c := NewChainConsensus(1, 1, 1)
	g := seedGenesis(t, c)

	block0, _ := trackChild(c, g, 1)
	block1, block1B := trackChild(c, g, 1)
	block2, block2B := trackChild(c, block1, 2)

	plan, err := c.FinalizeBranch(block0, 1, g)
	if err != nil {
		t.Fatalf("finalize block0: %v", err)
	}
	if len(plan.Reject) != 2 || !idIn(plan.Reject, block1) || !idIn(plan.Reject, block2) {
		t.Fatalf("the transitive Reject set {block1,block2}, got %v", plan.Reject)
	}
	if !block1B.rejected || !block2B.rejected {
		t.Fatal("block1 and block2 must be rejected transitively")
	}
}

// TestConformance_ErrorOnAddDecidedBlock ports ErrorOnAddDecidedBlockTest. ADAPTED:
// ava's Add(genesis) errors because genesis is already decided. Our analogues for
// "cannot re-introduce / re-decide a decided block": AddBlock rejects a DUPLICATE
// id, and an already-finalized height cannot be re-decided to a DIFFERENT block
// (equivocation, ErrHeightAlreadyFinalized).
func TestConformance_ErrorOnAddDecidedBlock(t *testing.T) {
	c := NewChainConsensus(1, 1, 1)
	g := seedGenesis(t, c)

	block0, _ := trackChild(c, g, 1)
	// Re-adding a tracked block id errors.
	if err := c.AddBlock(context.Background(), &Block{id: block0, parentID: g, height: 1}); err == nil {
		t.Fatal("re-adding a tracked block must error")
	}
	// Finalize it, then attempt to re-decide the height with a DIFFERENT block.
	if _, err := c.FinalizeBranch(block0, 1, g); err != nil {
		t.Fatalf("finalize block0: %v", err)
	}
	if _, err := c.FinalizeBranch(ids.GenerateTestID(), 1, g); !errors.Is(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("re-deciding a finalized height must be refused as equivocation, got %v", err)
	}
}

// =============================================================================
// Parameterized / regression conformance
// =============================================================================

// TestConformance_RecordPollWithDefaultParameters ports RecordPollWithDefaultParameters.
// ADAPTED: ava shows NumProcessing -> 0 after β successful polls under
// DefaultParameters. Our deterministic conformance: two conflicting blocks process
// until the cert decision, which drops processing to 0 (winner accepted, loser
// pruned). (The β round COUNT is a cert-production property, external to
// ChainConsensus — see the package header.)
func TestConformance_RecordPollWithDefaultParameters(t *testing.T) {
	c := NewChainConsensus(20, 15, 20) // default-shaped: K=20, α=15, β=20
	g := seedGenesis(t, c)

	blk1, _ := trackChild(c, g, 1)
	blk2, _ := trackChild(c, g, 1)
	if numProcessing(c) != 2 {
		t.Fatalf("both conflicting blocks process until a decision, got %d", numProcessing(c))
	}
	if _, err := c.FinalizeBranch(blk1, 1, g); err != nil {
		t.Fatalf("finalize blk1: %v", err)
	}
	if numProcessing(c) != 0 {
		t.Fatalf("0 processing after the cert decision, got %d", numProcessing(c))
	}
	if !c.IsAccepted(blk1) {
		t.Fatal("blk1 must be accepted")
	}
	if _, ok := c.GetBlock(blk2); ok {
		t.Fatal("the losing conflict blk2 must be pruned")
	}
}

// TestConformance_RecordPollRegressionIndegree ports
// RecordPollRegressionCalculateInDegreeIndegreeCalculation. ADAPTED: ava's regression
// is that transitive votes for a descendant accept the WHOLE ancestor chain in one
// poll without an indegree miscalculation. Our analogue: a cert for blk3 finalizes
// the entire path blk1,blk2,blk3 in ONE FinalizeBranch (multi-step), all accepted.
func TestConformance_RecordPollRegressionIndegree(t *testing.T) {
	c := NewChainConsensus(3, 2, 1)
	g := seedGenesis(t, c)

	blk1, _ := trackChild(c, g, 1)
	blk2, _ := trackChild(c, blk1, 2)
	blk3, _ := trackChild(c, blk2, 3)

	plan, err := c.FinalizeBranch(blk3, 3, blk2)
	if err != nil {
		t.Fatalf("finalize blk3: %v", err)
	}
	if len(plan.Accept) != 3 || plan.Accept[0] != blk1 || plan.Accept[1] != blk2 || plan.Accept[2] != blk3 {
		t.Fatalf("the whole path must finalize ascending {blk1,blk2,blk3}, got %v", plan.Accept)
	}
	for _, id := range []ids.ID{blk1, blk2, blk3} {
		if !c.IsAccepted(id) {
			t.Fatalf("%s must be accepted", id)
		}
	}
	if numProcessing(c) != 0 {
		t.Fatalf("0 processing after the path finalize, got %d", numProcessing(c))
	}
}

// TestConformance_RandomizedConsistency ports RandomizedConsistencyTest — the
// fuzz/consistency safety proof. ADAPTED to be DETERMINISTIC (explicit seed, no
// time/unseeded randomness): a randomized sequence of sibling forks + cert
// selections, asserting the core Snowman SAFETY invariants never break under any
// interleaving:
//
//	(1) at most ONE block is finalized per height (no fork);
//	(2) finalized history is a single CONTIGUOUS chain 0..H;
//	(3) every losing sibling subtree is pruned (rejected, removed) — never accepted;
//	(4) no block is ever both accepted and rejected;
//	(5) the FinalizeBranch plan exactly matches the expected accept path and prune set.
func TestConformance_RandomizedConsistency(t *testing.T) {
	const (
		seed       = int64(0xC0FFEE)
		iterations = 64
	)
	rng := mrand.New(mrand.NewSource(seed)) // deterministic, explicit seed

	c := NewChainConsensus(20, 15, 20)
	g := seedGenesis(t, c)

	type rec struct {
		id ids.ID
		b  *Block
	}
	var created []rec
	expected := map[uint64]ids.ID{0: g} // the finalized block per height (the ledger)
	tip := g
	var tipH uint64

	for it := 0; it < iterations; it++ {
		h := tipH + 1
		sibN := 2 + rng.Intn(3) // 2..4 competing siblings off the current tip
		winner := rng.Intn(sibN)

		var winnerID, target, parentOfTarget ids.ID
		var newH uint64 = h
		var acceptedPath []ids.ID
		prunedExpect := map[ids.ID]bool{}

		for s := 0; s < sibN; s++ {
			sid, sb := trackChild(c, tip, h)
			created = append(created, rec{sid, sb})
			giveGrandchild := rng.Intn(2) == 0
			var gid ids.ID
			if giveGrandchild {
				var gb *Block
				gid, gb = trackChild(c, sid, h+1)
				created = append(created, rec{gid, gb})
			}
			if s == winner {
				winnerID = sid
				acceptedPath = append(acceptedPath, sid)
				if giveGrandchild {
					// Always target the winner's deepest block so the winner's
					// whole subtree is on the finalized path (no leftovers).
					target = gid
					parentOfTarget = sid
					newH = h + 1
					acceptedPath = append(acceptedPath, gid)
				} else {
					target = sid
					parentOfTarget = tip
				}
			} else {
				prunedExpect[sid] = true
				if giveGrandchild {
					prunedExpect[gid] = true
				}
			}
		}

		plan, err := c.FinalizeBranch(target, newH, parentOfTarget)
		if err != nil {
			t.Fatalf("iter %d: finalize: %v", it, err)
		}
		// (5) plan.Accept == the winner path, ascending and exact.
		if len(plan.Accept) != len(acceptedPath) {
			t.Fatalf("iter %d: accepted size %d want %d", it, len(plan.Accept), len(acceptedPath))
		}
		for i := range acceptedPath {
			if plan.Accept[i] != acceptedPath[i] {
				t.Fatalf("iter %d: accepted[%d]=%s want %s", it, i, plan.Accept[i], acceptedPath[i])
			}
		}
		// (5) plan.Reject == the loser set, exact (order-independent: prune walks a map).
		if len(plan.Reject) != len(prunedExpect) {
			t.Fatalf("iter %d: pruned size %d want %d", it, len(plan.Reject), len(prunedExpect))
		}
		for _, id := range plan.Reject {
			if !prunedExpect[id] {
				t.Fatalf("iter %d: unexpected prune %s", it, id)
			}
		}
		// (3) by construction every block this round is decided (winner-path
		// accepted or loser-subtree pruned) → nothing left processing.
		if got := numProcessing(c); got != 0 {
			t.Fatalf("iter %d: %d blocks left processing", it, got)
		}
		// (1) exactly one finalized block at the new height(s); advance the ledger.
		expected[h] = winnerID
		if newH == h+1 {
			expected[h+1] = target
		}

		tip = target
		tipH = newH
	}

	// (2) the finalized ledger is a single contiguous chain 0..tipH, one per height.
	if c.GetFinalizedTip() != tip {
		t.Fatalf("final tip %s want %s", c.GetFinalizedTip(), tip)
	}
	if h, set := c.GetFinalizedHeight(); !set || h != tipH {
		t.Fatalf("final height (%d,%v) want %d", h, set, tipH)
	}
	for hh := uint64(0); hh <= tipH; hh++ {
		fin, ok := c.FinalizedBlockAtHeight(hh)
		if !ok || fin != expected[hh] {
			t.Fatalf("ledger height %d: got (%s,%v) want %s", hh, fin, ok, expected[hh])
		}
	}
	// (4) no block is both accepted and rejected; every accepted block IS the
	// unique finalized block at its height (consistency/agreement).
	for _, r := range created {
		if r.b.accepted && r.b.rejected {
			t.Fatalf("block %s is both accepted and rejected", r.id)
		}
		if r.b.accepted {
			if fin, ok := c.FinalizedBlockAtHeight(r.b.height); !ok || fin != r.id {
				t.Fatalf("accepted block %s is not the finalized block at its height %d", r.id, r.b.height)
			}
		}
	}
}
