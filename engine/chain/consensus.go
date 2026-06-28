// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/ids"
)

// The finalization safety invariants. A cert-driven finalize routes through
// FinalizeBranch, which enforces these in ONE place. Two of them remain genuine
// safety properties Avalanche's pure Snowman does not need but α-of-K (external,
// attacker-chosen-Round) certs do: a SECOND valid cert for a DIFFERENT block at an
// already-decided height is equivocation evidence, never a silent second VM.Accept;
// and a cert for a block that does NOT descend from the finalized frontier conflicts
// with finalized history and is refused. The THIRD property of pure Snowman — that
// finalized history is a single non-branching chain — is now achieved the avalanchego
// way: NOT by pre-emptively refusing siblings at admission, but by REORG (prune the
// losing sibling subtree when the cert selects the winner). Siblings coexist BEFORE
// the cert decides; the reorg is the recovery.
var (
	// ErrHeightAlreadyFinalized is returned when a DIFFERENT block is already
	// finalized at the target height. This is the fork the per-height guard
	// exists to stop: two valid α-certs at the same height across different
	// rounds. The first finalizes; the second is rejected and is equivocation
	// evidence (two distinct blocks signed-final at one height). KEPT — this is a
	// real α-of-K-cert safety property pure Snowman does not need.
	ErrHeightAlreadyFinalized = errors.New("chain: a different block is already finalized at this height (equivocation: two finalized blocks at one height)")

	// ErrNonMonotonicFinalizedHeight is returned when a finalize is attempted at a
	// height at or below the current finalized height with a different block, or
	// when the cert-selected branch is not CONTIGUOUS with the frontier (a height
	// gap). Finality only ever moves forward, one height at a time, along a tracked
	// ancestry chain.
	ErrNonMonotonicFinalizedHeight = errors.New("chain: finalized height must strictly increase by contiguous steps (cannot re-finalize an old/equal height, nor jump a height gap)")

	// ErrConflictsWithFinalizedBranch is returned when a cert-selected block does
	// NOT descend from the current finalized tip — its ancestry reaches a block at
	// or below the finalized height that is NOT the finalized tip (a losing sibling
	// branch, or an already-pruned one). Under <⅓ Byzantine this can only happen for
	// a block on a branch the network did not finalize; finalizing it would branch
	// finalized history, so it is refused. (Was ErrParentNotFinalizedTip — but it is
	// no longer an ADMISSION refusal of any non-tip parent: siblings are admitted and
	// resolved by the cert's reorg; only a block proven to be on a LOSING branch is
	// refused here.)
	ErrConflictsWithFinalizedBranch = errors.New("chain: cert-selected block does not extend the finalized frontier (it descends from a losing/pruned sibling branch)")

	// ErrAncestorNotTracked is returned when FinalizeBranch cannot prove the path
	// from the finalized tip up to the cert-selected block because an ANCESTOR on
	// that path is not in the live DAG. This is a DEFER, not a conflict: the node is
	// behind and must fetch the missing ancestors (the live cert path tracks them;
	// bootstrap/catch-up feed oldest-first so the walk is single-step). The caller
	// re-applies once the gap is filled — it must NEVER finalize on this error.
	ErrAncestorNotTracked = errors.New("chain: cannot finalize — an ancestor between the finalized tip and the cert-selected block is not tracked (behind; fetch and retry)")

	// ErrSyncStateRegression is returned by SyncState when an import
	// (admin_importChain / state-sync reconcile) tries to seed the finalized
	// head at a height BELOW the height already finalized locally. SyncState
	// bypasses FinalizeBranch by design (an import is an out-of-band
	// reconcile, not an α-of-K finalize), so the monotonic invariant the finalize
	// path enforces must be re-asserted here explicitly: a backward
	// import must NOT silently regress local finalized height (which would let a
	// shorter imported chain un-finalize blocks the node already finalized).
	ErrSyncStateRegression = errors.New("chain: SyncState refused — import height is below the already-finalized height (would regress finalized history)")

	// ErrSyncStateEmptyWithHeight is returned by SyncState when an EMPTY import
	// head (lastAcceptedID == ids.Empty) is paired with a POSITIVE height (INFO-6).
	// An empty head is the genesis/teardown reset, which is meaningful only at
	// height 0; an empty head at height>0 is contradictory. If allowed it would set
	// finalizedTip=Empty while LEAVING finalizedHeight/finalizedByHeight stale (the
	// non-empty seed branch is skipped) and PRUNE blocks below the positive height —
	// the exact finalizedTip-vs-finalizedHeight desync ForcePreference was hardened
	// against. SyncState refuses it fail-closed before mutating any state.
	ErrSyncStateEmptyWithHeight = errors.New("chain: SyncState refused — empty import head paired with a non-zero height (an empty reset is genesis/teardown at height 0; this would desync finalizedTip from finalizedHeight and prune live blocks)")

	// ErrEpochRegression is returned by the receive-side epoch gate when a
	// gossiped child block's stamped P-CHAIN epoch height is BELOW its parent's
	// recorded epoch height. The build side stamps H = max(currentH, parentH)
	// (monotone), but that is PROPOSER-ONLY: a Byzantine proposer can skip it and
	// stamp a stale H_old — a past P-chain epoch where its since-departed coalition
	// held ≥⅔ — then sign with leaked old keys valid at H_old. A follower that
	// adopted H_old blindly would resolve the validator set at the stale epoch and
	// finalize a FRESH block against a set the CURRENT set never approved (a safety
	// break: live equivocation, posterior corruption). Re-asserting monotonicity
	// against the parent's recorded epoch on the RECEIVE side rejects that block
	// before it is ever tracked or voted, so a chain's epoch can only move forward —
	// reducing safety to current-set BFT with NO weak-subjectivity assumption.
	ErrEpochRegression = errors.New("chain: gossiped block refused — its P-chain epoch height regresses below its parent's recorded epoch (a Byzantine proposer cannot pin a fresh block to a stale validator set)")
)

// Block represents a block in the chain
type Block struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	data      []byte

	// pChainHeight is the P-CHAIN epoch height the block's weighted validator set
	// is pinned to (MEDIUM-1 / CRITICAL-1): the set-root commitment, the ⅔-by-stake
	// tally, AND the per-voter pubkey resolution are ALL read from the height-
	// indexed validators.State at THIS height — never the value-chain `height`.
	// They differ fundamentally: platformvm.GetValidatorSet interprets its argument
	// as a P-CHAIN height and returns errUnfinalizedHeight when the current P-chain
	// height < the argument, and the value-chain height races far ahead of the
	// P-chain height on a busy chain — feeding `height` there yields an empty set
	// and stalls finality FOREVER (the mainnet-bricking bug this fixes).
	//
	// Source: a proposervm signed block carries its PChainHeight
	// (block.SignedBlock.PChainHeight); pChainHeightOf reads it off the VM block at
	// the engine boundary. When the block does NOT expose one (the VM is not
	// proposervm-wrapped at the engine boundary, which is the case for the current
	// in-process chain stack), this is 0 → the set is read at P-chain height 0, the
	// GENESIS validator set. That is non-empty, identical on every node (everyone
	// agrees on genesis), and ≤ the current P-chain height, so finality is LIVE and
	// consistent — and EXACT for any chain whose validator set is unchanged since
	// genesis. The refinement that pins post-genesis STAKING-CHANGE epochs requires
	// the proposervm to deliver its PChainHeight to the engine's block (the
	// remaining (b2) wiring); the mechanism here is proven by
	// TestPChainEpochFinality_RealWiring, which feeds a block that DOES carry it.
	pChainHeight uint64

	// Consensus state - Photon -> Wave -> Focus finality
	driver   *engine.Driver
	accepted bool
	rejected bool

	// Vote tracking for rejection support
	acceptVotes int
	rejectVotes int
}

// ChainConsensus implements real Lux consensus for linear chains using Photon → Wave → Focus
type ChainConsensus struct {
	mu sync.RWMutex

	// Parameters
	k     int // Sample size
	alpha int // Quorum size
	beta  int // Decision threshold

	// State
	blocks map[ids.ID]*Block
	tips   map[ids.ID]bool // Current chain tips

	// Consensus tracking
	bootstrapped bool
	finalizedTip ids.ID

	// Per-height finalization ledger — the source of truth for "which block is
	// finalized at height H". finalizedTip/finalizedHeight name the head of
	// finalized history; finalizedByHeight indexes the full set so a re-finalize of
	// any past height with a different block is caught. This is a POST-finalization
	// RECORD, not a pre-emptive admission gate: it is advanced ONLY by FinalizeBranch
	// (the cert-driven finalize), and the single-non-branching-chain property is kept
	// by the REORG that FinalizeBranch performs (prune the losing sibling subtree),
	// never by refusing to track siblings. Maps avalanchego topological.go's
	// lastAcceptedID/lastAcceptedHeight (the committed lower bound), with the
	// per-height index added for α-of-K equivocation detection.
	finalizedHeight    uint64
	finalizedHeightSet bool // false until the first block is finalized
	finalizedByHeight  map[uint64]ids.ID
}

// BranchFinalization is the plan a cert-driven finalize produces and the engine
// applies to the VM. It mirrors avalanchego topological.go's accept/reject split:
//
//   - Accepted is the path from the OLD finalized tip up to the cert-selected block,
//     in ASCENDING height order — the blocks to VM.Accept (one per height the cert
//     advanced finality by; usually exactly one, more on a catch-up jump). This is
//     acceptPreferredChild applied along a path.
//   - Pruned is every block on a LOSING sibling subtree (a sibling of a path block,
//     plus all its descendants) — the blocks to VM.Reject and drop. This is
//     rejectTransitively.
//
// The consensus ledger has ALREADY been advanced when this value is returned; the
// engine owns only the VM-side effects (Accept/Reject/SetPreference).
type BranchFinalization struct {
	Accepted []ids.ID // path blocks finalized this call, ascending height
	Pruned   []ids.ID // losing-sibling subtrees (sibling + descendants), any order
}

// NewChainConsensus creates a real consensus engine
func NewChainConsensus(k, alpha, beta int) *ChainConsensus {
	return &ChainConsensus{
		k:                 k,
		alpha:             alpha,
		beta:              beta,
		blocks:            make(map[ids.ID]*Block),
		tips:              make(map[ids.ID]bool),
		finalizedByHeight: make(map[uint64]ids.ID),
	}
}

// FinalizeBranch commits the cert-selected block `target` (at `height`, parent
// `parentID`) and EVERY block on the path from the current finalized tip up to it,
// then returns the plan the engine applies to the VM: the path to Accept and the
// losing-sibling subtrees to prune. It is the SINGLE place finalized history is
// advanced.
//
// This REPLACES the old per-height admission gate. The old code refused any cert
// whose block's parent != finalizedTip (invariant (c)) — which deadlocked a fresh
// net the moment production explored a sibling, because a refusal cannot reorg. The
// new model is avalanchego's: siblings are TRACKED (AddBlock admits any child of a
// known block); when a cert SELECTS one, we walk the certified branch, finalize it,
// and PRUNE the losers. The single-non-branching-chain property is the OUTPUT of the
// reorg, not a precondition refused at admission.
//
// It enforces (mapped to the safety invariants α-of-K certs require):
//
//	(a) ONE finalized block per height. Same block already finalized here → idempotent
//	    no-op. DIFFERENT block already finalized at a decided height →
//	    ErrHeightAlreadyFinalized (equivocation evidence). [KEPT from the old gate.]
//	(b) the certified block must DESCEND from the finalized tip via a tracked,
//	    contiguous ancestry. A block whose ancestry reaches a non-tip block at/below
//	    the finalized height is on a losing/pruned branch → ErrConflictsWithFinalizedBranch.
//	    A height gap on the path → ErrNonMonotonicFinalizedHeight. An untracked
//	    ancestor → ErrAncestorNotTracked (DEFER: behind, fetch and retry).
//
// On success the per-height ledger is advanced to `target` and the path blocks are
// marked accepted in the consensus DAG; the losing subtrees are marked rejected and
// removed from the live DAG/tips. The engine applies the returned VM effects. Takes
// c.mu. `parentID` may be ids.Empty only for the genesis/first finalize.
func (c *ChainConsensus) FinalizeBranch(target ids.ID, height uint64, parentID ids.ID) (BranchFinalization, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finalizeBranchLocked(target, height, parentID)
}

// finalizeBranchLocked is FinalizeBranch's body. Caller holds c.mu.
func (c *ChainConsensus) finalizeBranchLocked(target ids.ID, height uint64, parentID ids.ID) (BranchFinalization, error) {
	// (a) idempotent / equivocation at the target height.
	if existing, ok := c.finalizedByHeight[height]; ok {
		if existing == target {
			return BranchFinalization{}, nil // this exact block already finalized here
		}
		return BranchFinalization{}, fmt.Errorf("%w: height %d already finalized %s, refused %s",
			ErrHeightAlreadyFinalized, height, existing, target)
	}

	// First finalize seeds the ledger — there is no prior tip to extend or reorg.
	if !c.finalizedHeightSet {
		c.recordFinalizedLocked(target, height)
		return BranchFinalization{Accepted: []ids.ID{target}}, nil
	}

	// Below/at the frontier with no record for this height → stale or non-monotonic.
	if height <= c.finalizedHeight {
		return BranchFinalization{}, fmt.Errorf("%w: refused height %d at finalized height %d (block %s)",
			ErrNonMonotonicFinalizedHeight, height, c.finalizedHeight, target)
	}

	// Walk the certified branch finalizedTip → target. This REPLACES invariant (c):
	// instead of refusing parent != tip, prove the ancestry reaches the tip and
	// finalize EVERY block on it (a cert may certify a descendant several heights
	// above the tip — a catch-up jump — so the path is 1..k blocks).
	path, err := c.pathFromTipLocked(target, height, parentID)
	if err != nil {
		return BranchFinalization{}, err
	}

	// Commit the path (ascending) and collect the losing-sibling subtrees to prune.
	var fin BranchFinalization
	for _, step := range path {
		fin.Pruned = append(fin.Pruned, c.collectLosingSubtreesLocked(step.id, step.parentID)...)
		c.recordFinalizedLocked(step.id, step.height)
		if b, ok := c.blocks[step.id]; ok {
			b.accepted = true
		}
		fin.Accepted = append(fin.Accepted, step.id)
	}

	// Reject + remove the pruned subtrees from the live DAG/tips (rejectTransitively).
	for _, id := range fin.Pruned {
		if b, ok := c.blocks[id]; ok {
			b.rejected = true
		}
		delete(c.blocks, id)
		delete(c.tips, id)
	}
	return fin, nil
}

// recordFinalizedLocked advances the per-height ledger head to (id, height). The
// ONLY writer of finalizedTip/finalizedHeight/finalizedByHeight on the finalize path.
func (c *ChainConsensus) recordFinalizedLocked(id ids.ID, height uint64) {
	c.finalizedByHeight[height] = id
	c.finalizedTip = id
	c.finalizedHeight = height
	c.finalizedHeightSet = true
	delete(c.tips, id) // a finalized block is no longer an open build tip
}

// finalizeStep is one block on the path from the finalized tip to the cert target.
type finalizeStep struct {
	id       ids.ID
	height   uint64
	parentID ids.ID
}

// pathFromTipLocked returns the contiguous ancestry path finalizedTip → target in
// ASCENDING height order, by walking target's parent links through the tracked DAG.
// Errors distinguish the three non-extending cases:
//   - ErrConflictsWithFinalizedBranch: an ancestor reaches the finalized height (or
//     below) at a block that is NOT the finalized tip — target is on a losing branch.
//   - ErrAncestorNotTracked: an ancestor is missing from the DAG — DEFER (behind).
//   - ErrNonMonotonicFinalizedHeight: the path is not contiguous (a height gap), or
//     a parent height does not strictly decrease (malformed linkage).
//
// Caller guarantees height > c.finalizedHeight and c.finalizedHeightSet. Caller holds c.mu.
func (c *ChainConsensus) pathFromTipLocked(target ids.ID, height uint64, parentID ids.ID) ([]finalizeStep, error) {
	steps := []finalizeStep{{id: target, height: height, parentID: parentID}}
	cur := parentID
	childHeight := height
	for cur != c.finalizedTip {
		pb, ok := c.blocks[cur]
		if !ok {
			// The path to the frontier is not fully tracked — the node is behind.
			// Fail-closed DEFER: never finalize on a path we cannot prove.
			return nil, fmt.Errorf("%w: ancestor %s of %s missing", ErrAncestorNotTracked, cur, target)
		}
		// Heights must strictly decrease as we walk toward the tip; a parent at/above
		// its child's height is malformed linkage.
		if pb.height >= childHeight {
			return nil, fmt.Errorf("%w: ancestor %s height %d not below child height %d",
				ErrNonMonotonicFinalizedHeight, pb.id, pb.height, childHeight)
		}
		// Reached the finalized height (or below) at a block that is not the tip →
		// target descends from a branch the network did not finalize.
		if pb.height <= c.finalizedHeight {
			return nil, fmt.Errorf("%w: %s ancestry reaches %s (height %d) not finalized tip %s",
				ErrConflictsWithFinalizedBranch, target, pb.id, pb.height, c.finalizedTip)
		}
		steps = append(steps, finalizeStep{id: pb.id, height: pb.height, parentID: pb.parentID})
		cur = pb.parentID
		childHeight = pb.height
	}

	// Reverse to ascending height and assert contiguity with the frontier: the first
	// (lowest) step must be exactly finalizedHeight+1 and each step exactly +1. A
	// gap means a height was skipped (defense-in-depth: a valid block's height is its
	// parent's +1, so an honest path always passes; a malformed cert/linkage fails).
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	for i := range steps {
		want := c.finalizedHeight + 1 + uint64(i)
		if steps[i].height != want {
			return nil, fmt.Errorf("%w: path height %d at position %d, want %d (gap)",
				ErrNonMonotonicFinalizedHeight, steps[i].height, i, want)
		}
	}
	return steps, nil
}

// collectLosingSubtreesLocked returns every tracked block on a LOSING sibling subtree
// of `keepID`: the other children of `parentID` (siblings of keepID) plus all their
// descendants. This is avalanchego rejectTransitively's reachable set. Caller holds c.mu.
func (c *ChainConsensus) collectLosingSubtreesLocked(keepID ids.ID, parentID ids.ID) []ids.ID {
	// Seed the frontier with direct siblings (children of parentID other than keepID).
	var queue []ids.ID
	for id, b := range c.blocks {
		if b.parentID == parentID && id != keepID {
			queue = append(queue, id)
		}
	}
	if len(queue) == 0 {
		return nil
	}
	// BFS the descendants of each losing sibling.
	out := make([]ids.ID, 0, len(queue))
	seen := make(map[ids.ID]bool, len(queue))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		for cid, cb := range c.blocks {
			if cb.parentID == id && !seen[cid] {
				queue = append(queue, cid)
			}
		}
	}
	return out
}

// AddBlock adds a block to consensus
func (c *ChainConsensus) AddBlock(ctx context.Context, block *Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if block already exists
	if _, exists := c.blocks[block.id]; exists {
		return fmt.Errorf("block already exists: %s", block.id)
	}

	// Initialize Lux consensus for this block using Photon → Wave → Focus
	block.driver = engine.NewLuxConsensus(c.k, c.alpha, c.beta)

	// Add to blocks map
	c.blocks[block.id] = block

	// Update tips
	if block.parentID != ids.Empty {
		// Remove parent from tips (no longer a tip)
		delete(c.tips, block.parentID)
	}
	c.tips[block.id] = true

	return nil
}

// ProcessVote processes a vote for a block
func (c *ChainConsensus) ProcessVote(ctx context.Context, blockID ids.ID, accept bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	block, exists := c.blocks[blockID]
	if !exists {
		return fmt.Errorf("block not found: %s", blockID)
	}

	if block.driver == nil {
		return fmt.Errorf("block not initialized for consensus")
	}

	// Track both accept and reject votes
	if accept {
		block.acceptVotes++
		block.driver.RecordVote(blockID)
	} else {
		block.rejectVotes++
	}

	// LIVENESS only (decomplected from finality). Reaching the α accept count marks
	// the block worth a finalize ATTEMPT — block.accepted is the engine's
	// DrainAccepted trigger — but it does NOT advance the per-height ledger. Finality
	// is committed ONLY by the cert-driven FinalizeBranch (the α-of-K SIGNED witness),
	// which also performs the sibling reorg. A sibling reaching α-count here is
	// harmless: the cert decides which branch finalizes, and the loser is pruned.
	if block.acceptVotes >= c.alpha && !block.accepted {
		block.accepted = true
	}

	// Check if rejection quorum is reached (reject votes >= alpha)
	if block.rejectVotes >= c.alpha {
		block.rejected = true
		// Remove from tips since this block is rejected
		delete(c.tips, blockID)
	}

	return nil
}

// Poll conducts a consensus poll
func (c *ChainConsensus) Poll(ctx context.Context, responses map[ids.ID]int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Poll each block's Lux consensus instance using Wave → Focus protocols
	for blockID, votes := range responses {
		block, exists := c.blocks[blockID]
		if !exists {
			continue
		}

		// Skip already decided blocks
		if block.accepted || block.rejected {
			continue
		}

		// Check if rejection quorum already reached (reject votes >= alpha)
		if block.rejectVotes >= c.alpha {
			block.rejected = true
			delete(c.tips, blockID)
			continue
		}

		// Only consider acceptance if we have enough accept votes
		// This prevents premature acceptance with insufficient quorum
		if block.acceptVotes < c.alpha {
			continue
		}

		if block.driver != nil {
			blockResponses := map[ids.ID]int{blockID: votes}
			shouldContinue := block.driver.Poll(blockResponses)
			decided := block.driver.Decided()

			// Focus convergence + α accept count → LIVENESS: mark the block worth a
			// finalize attempt. Finality (and the reorg) is the cert path's job
			// (FinalizeBranch), never the count path's — so the count never advances
			// the ledger nor branches it.
			if !shouldContinue && decided && block.acceptVotes >= c.alpha {
				block.accepted = true
			}
		}
	}

	return nil
}

// IsAccepted checks if a block is accepted
func (c *ChainConsensus) IsAccepted(blockID ids.ID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	block, exists := c.blocks[blockID]
	if !exists {
		return false
	}

	return block.accepted
}

// IsRejected checks if a block is rejected
func (c *ChainConsensus) IsRejected(blockID ids.ID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	block, exists := c.blocks[blockID]
	if !exists {
		return false
	}

	return block.rejected
}

// Preference returns current preferred block
func (c *ChainConsensus) Preference() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return the finalized tip if available
	if c.finalizedTip != ids.Empty {
		return c.finalizedTip
	}

	// Otherwise return latest tip
	for tip := range c.tips {
		return tip
	}

	return ids.Empty
}

// GetBlock returns a block by ID
func (c *ChainConsensus) GetBlock(blockID ids.ID) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	block, exists := c.blocks[blockID]
	return block, exists
}

// EpochHeightOf returns the P-CHAIN epoch height recorded for a tracked block
// (the height its weighted validator set, set-root, and ⅔-stake tally are pinned
// to — Block.pChainHeight), and whether the block is tracked at all. It is the
// SINGLE authoritative read of "what epoch did we record for this block", used by
// the receive-side monotonicity gate to reject a child whose stamped epoch
// regresses below its parent's recorded epoch (the far-past attack: a Byzantine
// proposer stamps a stale H where its old coalition held ≥⅔). A miss (false) means
// the parent is not yet tracked — the caller treats that fail-closed.
func (c *ChainConsensus) EpochHeightOf(blockID ids.ID) (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	block, exists := c.blocks[blockID]
	if !exists {
		return 0, false
	}
	return block.pChainHeight, true
}

// Stats returns consensus statistics
func (c *ChainConsensus) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	accepted := 0
	rejected := 0
	pending := 0

	for _, block := range c.blocks {
		if block.accepted {
			accepted++
		} else if block.rejected {
			rejected++
		} else {
			pending++
		}
	}

	return map[string]interface{}{
		"total_blocks":  len(c.blocks),
		"accepted":      accepted,
		"rejected":      rejected,
		"pending":       pending,
		"tips":          len(c.tips),
		"finalized_tip": c.finalizedTip.String(),
	}
}

// SyncState synchronizes consensus state with external state (e.g., after RLP import).
// This updates the finalizedTip and marks the consensus as bootstrapped so that
// new blocks can be built on top of the imported chain.
//
// This is called by the syncer after admin_importChain to reconcile consensus
// state with the EVM state database.
//
// MONOTONIC GUARD (defence-in-depth): SyncState bypasses FinalizeBranch by
// design — an import is an out-of-band reconcile with the VM's last-accepted
// head, NOT an α-of-K finalize, so it legitimately seeds finalizedTip/Height
// directly. But "bypasses the finalize path" must NOT mean "bypasses the
// monotonic invariant": a backward import (height below the already-finalized
// height) is refused with ErrSyncStateRegression and leaves all finalized state
// untouched. Without this guard a shorter/older imported chain could silently
// regress finalizedHeight and un-finalize blocks the node already finalized —
// re-opening the very fork window the per-height guard closes. A re-import at the
// SAME height with the SAME block is idempotent; a forward import advances. The
// only allowed move-backward is the genesis/empty reset (lastAcceptedID==Empty),
// which is valid ONLY at height 0 — an empty head at height>0 is contradictory
// and refused with ErrSyncStateEmptyWithHeight (INFO-6), since it would desync
// finalizedTip from finalizedHeight and prune live blocks.
func (c *ChainConsensus) SyncState(lastAcceptedID ids.ID, height uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Refuse an empty import head paired with a positive height (INFO-6). An empty
	// head is the genesis/teardown reset — valid only at height 0. At height>0 it
	// would assign finalizedTip=Empty while skipping the seed branch (so
	// finalizedHeight/finalizedByHeight stay stale) AND prune blocks below the
	// height, desyncing finalizedTip from finalizedHeight (the desync the per-height
	// guard and ForcePreference exist to prevent). Fail-closed BEFORE any mutation.
	// Unreachable on the live path (the sole caller, SyncStateFromVM, pairs Empty
	// with height 0), but guarded so a future caller cannot open the desync.
	if lastAcceptedID == ids.Empty && height > 0 {
		return fmt.Errorf("%w: refused empty head at height %d", ErrSyncStateEmptyWithHeight, height)
	}

	// Refuse a backward regression of finalized history. Only enforced once a
	// height is established and for a concrete (non-empty) import head; an empty
	// reset is a deliberate teardown, not a regression.
	if c.finalizedHeightSet && lastAcceptedID != ids.Empty && height < c.finalizedHeight {
		return fmt.Errorf("%w: import height %d < finalized height %d (block %s)",
			ErrSyncStateRegression, height, c.finalizedHeight, lastAcceptedID)
	}
	// Equal-height re-import must agree on the block (a different block at the
	// already-finalized height is equivocation, not a reconcile).
	if c.finalizedHeightSet && lastAcceptedID != ids.Empty && height == c.finalizedHeight {
		if existing, ok := c.finalizedByHeight[height]; ok && existing != lastAcceptedID {
			return fmt.Errorf("%w: import height %d already finalized %s, refused %s",
				ErrHeightAlreadyFinalized, height, existing, lastAcceptedID)
		}
	}

	// Update finalized tip to the synced block and seed the per-height ledger so
	// the next finalize (the first block built on the import) satisfies the
	// monotonic-height + parent==tip invariants against the imported head.
	c.finalizedTip = lastAcceptedID
	if lastAcceptedID != ids.Empty {
		c.finalizedByHeight = map[uint64]ids.ID{height: lastAcceptedID}
		c.finalizedHeight = height
		c.finalizedHeightSet = true
	}

	// Mark as bootstrapped so new blocks can be proposed
	c.bootstrapped = true

	// Update tips - the synced block is now the only tip
	c.tips = make(map[ids.ID]bool)
	if lastAcceptedID != ids.Empty {
		c.tips[lastAcceptedID] = true
	}

	// Clear any blocks below the synced height (they're now stale)
	for blockID, block := range c.blocks {
		if block.height < height {
			delete(c.blocks, blockID)
		}
	}
	return nil
}

// ForcePreference reaffirms the engine's preferred tip after a VM SetPreference
// failure. It is a recovery mechanism used when SetPreference fails AFTER a block
// was accepted — without it the VM and consensus engine could disagree on the
// chain tip, causing a state-divergence death spiral. Every legitimate caller invokes
// it with the block that was JUST finalized ("SetPreference failed after Accept"), so
// the block is already the finalized tip and this is a reaffirming no-op.
//
// finalizedTip is advanced ONLY by FinalizeBranch, atomically with
// finalizedHeight/finalizedByHeight (the cert path's reorg is the sole authority on
// the tip). So this method never moves finalizedTip off the recorded head — with the
// per-height ADMISSION gate gone, there is no invariant for a stray preference to
// corrupt. It only adopts blockID as the preliminary build preference before the
// FIRST finalize, and always records it as a build tip.
func (c *ChainConsensus) ForcePreference(blockID ids.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.finalizedHeightSet {
		c.finalizedTip = blockID // no finalized head yet — preliminary preference
	}
	// After the first finalize the tip is authoritative and untouched here.
	c.tips[blockID] = true
}

// GetFinalizedTip returns the current finalized tip block ID.
// This is useful for debugging and health checks.
func (c *ChainConsensus) GetFinalizedTip() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finalizedTip
}

// GetFinalizedHeight returns the current finalized height and whether any block
// has been finalized yet. The engine uses this to gate an incoming cert on
// height (reject a cert at or below the last-finalized height — MED-5) BEFORE
// running the cert through the per-height guard.
func (c *ChainConsensus) GetFinalizedHeight() (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finalizedHeight, c.finalizedHeightSet
}

// FinalizedBlockAtHeight returns the block finalized at the given height, if
// any. Used to produce equivocation evidence: when a second cert is refused at
// an already-finalized height, the caller reports BOTH the finalized block and
// the refused one.
func (c *ChainConsensus) FinalizedBlockAtHeight(height uint64) (ids.ID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.finalizedByHeight[height]
	return id, ok
}

// K returns the consensus sample size (the K of α-of-K). Used by the engine to
// select the single-validator (K==1) force path vs. the multi-validator
// cert-witnessed path, and by the fail-closed DEX guard.
func (c *ChainConsensus) K() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.k
}

// Alpha returns the acceptance quorum threshold (α of α-of-K). A block is
// accepted once acceptVotes >= Alpha; a QuorumCert proves exactly this many
// distinct signed accepts.
func (c *ChainConsensus) Alpha() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.alpha
}
