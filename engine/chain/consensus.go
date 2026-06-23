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

// ErrForceAcceptRequiresSingleValidator is returned by ForceAccept when invoked
// on a multi-validator engine (k != 1). Self-finality without α-of-K is only
// sound when the sole validator's accept IS the quorum; in every multi-
// validator deployment finality MUST flow through the cert-witnessed α-of-K
// path. Fail-closed.
var ErrForceAcceptRequiresSingleValidator = errors.New("chain: ForceAccept is only valid for a single-validator engine (k==1); multi-validator finality requires an alpha-of-K quorum cert")

// The per-height finalization invariants. ANY attempt to finalize a block that
// violates one of these is rejected by markFinalizedLocked and surfaced to the
// caller — a second valid cert for an already-finalized height is equivocation
// evidence, never a silent second VM.Accept. These are the safety properties
// that make α-of-K finality non-forking even when Round is attacker-chosen and
// two valid α-certs exist at one height.
var (
	// ErrHeightAlreadyFinalized is returned when a DIFFERENT block is already
	// finalized at the target height. This is the fork the per-height guard
	// exists to stop: two valid α-certs at the same height across different
	// rounds. The first finalizes; the second is rejected and is equivocation
	// evidence (two distinct blocks signed-final at one height).
	ErrHeightAlreadyFinalized = errors.New("chain: a different block is already finalized at this height (equivocation: two finalized blocks at one height)")

	// ErrNonMonotonicFinalizedHeight is returned when a finalize is attempted at
	// a height at or below the current finalized height with a different block —
	// finality only ever moves forward.
	ErrNonMonotonicFinalizedHeight = errors.New("chain: finalized height must strictly increase (cannot re-finalize an old or equal height with a different block)")

	// ErrParentNotFinalizedTip is returned when a newly-finalized block's parent
	// is not the current finalized tip. Finality is a single non-branching chain:
	// every finalized block (after the first) extends the previously finalized
	// one.
	ErrParentNotFinalizedTip = errors.New("chain: newly-finalized block's parent is not the current finalized tip (would branch finalized history)")

	// ErrSyncStateRegression is returned by SyncState when an import
	// (admin_importChain / state-sync reconcile) tries to seed the finalized
	// head at a height BELOW the height already finalized locally. SyncState
	// bypasses markFinalizedLocked by design (an import is an out-of-band
	// reconcile, not an α-of-K finalize), so the monotonic invariant the per-
	// height guard enforces must be re-asserted here explicitly: a backward
	// import must NOT silently regress local finalized height (which would let a
	// shorter imported chain un-finalize blocks the node already finalized).
	ErrSyncStateRegression = errors.New("chain: SyncState refused — import height is below the already-finalized height (would regress finalized history)")
)

// Block represents a block in the chain
type Block struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	data      []byte

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

	// Per-height finalization ledger — the single source of truth for "which
	// block is finalized at height H". finalizedTip/finalizedHeight name the
	// current head of finalized history; finalizedByHeight indexes the full set
	// so a re-finalize of any past height with a different block is caught (not
	// just the current one). Together they make finality a single non-branching
	// chain: ONE block per height, height strictly increasing, parent == tip.
	finalizedHeight    uint64
	finalizedHeightSet bool // false until the first block is finalized
	finalizedByHeight  map[uint64]ids.ID
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

// markFinalizedLocked is the SINGLE place finality is committed. Every finalize
// path — local α-of-K count (ProcessVote/Poll), cert-witnessed finality
// (AcceptViaCert), and single-validator force (ForceAccept) — routes through
// here so the per-height safety invariants are enforced in exactly one place
// (decomplected: the invariant is not braided into each caller).
//
// It enforces, in order:
//
//	(a) ONE finalized block per height. If a block is already finalized at this
//	    height: same block → idempotent no-op (nil); DIFFERENT block →
//	    ErrHeightAlreadyFinalized (the fork; equivocation evidence).
//	(b) monotonic height: a finalize at or below the current finalized height
//	    with a new block → ErrNonMonotonicFinalizedHeight. (After the first
//	    finalize, only a strictly greater height is admissible.)
//	(c) parent == current finalized tip (after the first finalize): the new
//	    block extends finalized history, never branches it →
//	    ErrParentNotFinalizedTip otherwise.
//
// On success it records the block in the per-height ledger and advances
// finalizedTip/finalizedHeight. It does NOT call into the VM or set block.accepted
// — those are the caller's responsibility (the caller owns whether a tracked
// Block exists); this method owns ONLY the safety ledger. Caller holds c.mu.
//
// `parentID` may be ids.Empty for a genesis/first block with no parent.
func (c *ChainConsensus) markFinalizedLocked(blockID ids.ID, height uint64, parentID ids.ID) error {
	// (a) one block per height.
	if existing, ok := c.finalizedByHeight[height]; ok {
		if existing == blockID {
			return nil // idempotent: this exact block already finalized here
		}
		return fmt.Errorf("%w: height %d already finalized %s, refused %s",
			ErrHeightAlreadyFinalized, height, existing, blockID)
	}

	if c.finalizedHeightSet {
		// (b) monotonic + CONTIGUOUS: finality advances by exactly one height.
		// height ≤ finalized is a backward/equal fork attempt (equal-height-same-
		// block was already idempotent in (a)); height > finalized+1 would leave a
		// gap (and a valid α-cert cannot carry a height inconsistent with the
		// block's true height unless > f validators are Byzantine). Requiring the
		// strict successor makes finalized history a single CONTIGUOUS chain.
		if height != c.finalizedHeight+1 {
			return fmt.Errorf("%w: refused height %d at finalized height %d (block %s; finality advances by exactly 1)",
				ErrNonMonotonicFinalizedHeight, height, c.finalizedHeight, blockID)
		}
		// (c) parent must be the current finalized tip — no branching.
		if parentID != c.finalizedTip {
			return fmt.Errorf("%w: block %s parent %s != finalized tip %s (height %d)",
				ErrParentNotFinalizedTip, blockID, parentID, c.finalizedTip, height)
		}
	}

	c.finalizedByHeight[height] = blockID
	c.finalizedTip = blockID
	c.finalizedHeight = height
	c.finalizedHeightSet = true
	return nil
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

	// Check if acceptance quorum is reached (accept votes >= alpha). Finality is
	// committed through the single per-height guard: if this block would fork an
	// already-finalized height (or branch finalized history) the guard refuses
	// and the block is NOT marked accepted — the count alone never finalizes.
	if block.acceptVotes >= c.alpha && !block.accepted {
		if err := c.markFinalizedLocked(blockID, block.height, block.parentID); err == nil {
			block.accepted = true
		}
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

			// Check if block reached finality through Focus convergence
			// AND we have sufficient accept votes (quorum reached). Commit through
			// the single per-height guard — a block that would fork an already-
			// finalized height is refused and stays un-accepted.
			if !shouldContinue && decided && block.acceptVotes >= c.alpha {
				if err := c.markFinalizedLocked(blockID, block.height, block.parentID); err == nil {
					block.accepted = true
				}
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
// MONOTONIC GUARD (defence-in-depth): SyncState bypasses markFinalizedLocked by
// design — an import is an out-of-band reconcile with the VM's last-accepted
// head, NOT an α-of-K finalize, so it legitimately seeds finalizedTip/Height
// directly. But "bypasses the finalize path" must NOT mean "bypasses the
// monotonic invariant": a backward import (height below the already-finalized
// height) is refused with ErrSyncStateRegression and leaves all finalized state
// untouched. Without this guard a shorter/older imported chain could silently
// regress finalizedHeight and un-finalize blocks the node already finalized —
// re-opening the very fork window the per-height guard closes. A re-import at the
// SAME height with the SAME block is idempotent; a forward import advances. The
// only allowed move-backward is the genesis/empty reset (lastAcceptedID==Empty).
func (c *ChainConsensus) SyncState(lastAcceptedID ids.ID, height uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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
// chain tip, causing a state-divergence death spiral.
//
// SAFETY (decomplected from the per-height guard): finalizedTip is the per-height
// guard's source of truth for invariant (c) (a new block's parent must equal the
// finalized tip). This method therefore MUST NOT be able to move finalizedTip OFF
// the finalized head — a recovery convenience may never corrupt the safety ledger.
// Every legitimate caller invokes ForcePreference with the block that was JUST
// finalized (its precondition: "SetPreference failed after Accept"), so the block
// is already the finalized tip and this is a reaffirming no-op on finalizedTip.
//
// If invoked with a block that is NOT the current finalized head (a future
// misuse / refactor bug), it does NOT overwrite finalizedTip — doing so would
// leave finalizedTip disagreeing with finalizedHeight/finalizedByHeight and blind
// invariant (c). It still records the block as a tip (harmless: tips only seed
// block building; finality is gated by the guard, never by tips). Fail-safe.
func (c *ChainConsensus) ForcePreference(blockID ids.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Only (re)assert the finalized tip when blockID IS the finalized head — i.e.
	// the block finalized at the current finalized height (the universal caller
	// precondition). Before the first finalize (no head yet) there is nothing to
	// corrupt, so adopting blockID as the preliminary tip is safe.
	if !c.finalizedHeightSet {
		c.finalizedTip = blockID
	} else if fin, ok := c.finalizedByHeight[c.finalizedHeight]; ok && fin == blockID {
		c.finalizedTip = blockID // reaffirm — already the head, no desync
	}
	// else: blockID is not the finalized head — do NOT move finalizedTip (would
	// desync the per-height guard's invariant). Only mark it a build tip below.

	// Ensure the forced block is a tip for block-building purposes.
	c.tips[blockID] = true
}

// ForceAccept marks a block as accepted WITHOUT a quorum — it is the
// single-validator (K==1) finalization path and NOTHING ELSE.
//
// HISTORY / WHY IT IS GATED: ForceAccept used to be the proposer-self-accept
// escape hatch — the proposer force-accepted its OWN block on its lone
// self-vote when peer Chits arrived late. That was self-finality: a value
// block could finalize with NO α-of-K agreement, so a Byzantine (or merely
// equivocating) proposer could fork the chain. It is REMOVED for K>1. With
// more than one validator, finality flows ONLY through the α-of-K path
// (ProcessVote / Poll set accepted=true once acceptVotes>=alpha) and is
// witnessed by a QuorumCert (quorum_cert.go) — there is no force path.
//
// For K==1 (a single-validator network, e.g. --dev / localnet) "α-of-K" is
// "1-of-1": the sole validator's own accept IS the quorum, so ForceAccept is
// the correct, safe finalization. The guard makes the abuse impossible to
// reach in any multi-validator deployment:
//
//	returns ErrForceAcceptRequiresSingleValidator when k != 1 — fail-closed.
//
// Idempotent: subsequent calls on an already-accepted block no-op.
func (c *ChainConsensus) ForceAccept(blockID ids.ID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.k != 1 {
		return ErrForceAcceptRequiresSingleValidator
	}
	block, exists := c.blocks[blockID]
	if !exists {
		return nil
	}
	if block.accepted {
		return nil
	}
	// Even the single-validator path commits through the per-height guard: a
	// K==1 node still must not finalize two blocks at one height or branch its
	// own finalized history (e.g. a buggy double-build). The guard is cheap and
	// keeps ONE finalization path.
	if err := c.markFinalizedLocked(blockID, block.height, block.parentID); err != nil {
		return err
	}
	block.accepted = true
	return nil
}

// AcceptViaCert marks a block accepted because a verified QuorumCert proves
// α-of-K validators accepted it. This is the multi-validator finalization
// authority that replaces the deleted self-accept force path: the caller has
// ALREADY verified the cert (cert.Verify) against the chain's VoteVerifier, so
// this method records the decision the cert proves. It does not re-decide
// quorum — the cert IS the quorum proof.
//
// CRITICAL — per-height single-finalize. The cert's Round is attacker-chosen, so
// two VALID α-certs can exist for two DIFFERENT blocks at the SAME height (each
// over a different round). Both would Verify. This method commits through
// markFinalizedLocked, which finalizes the FIRST and returns
// ErrHeightAlreadyFinalized for the SECOND — the second is rejected (no
// VM.Accept) and the error is the caller's equivocation evidence. Without this
// guard the second cert forks the chain (red's PoC).
//
// The block's (height, parentID) come from the cert position the caller already
// verified — finality binds to the position the signatures cover, never to a
// locally-tracked Block (a follower may finalize via cert before the block
// enters local tracking). When the block IS tracked, it is also marked accepted.
//
// Returns nil on success (including the idempotent re-finalize of the same
// block at the same height) or the guard error naming the violated invariant.
func (c *ChainConsensus) AcceptViaCert(blockID ids.ID, height uint64, parentID ids.ID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Commit to the per-height ledger FIRST — this is the single safety gate.
	// On any violation (already-finalized height, non-monotonic, parent !=
	// tip) nothing is accepted and the error propagates to the caller.
	if err := c.markFinalizedLocked(blockID, height, parentID); err != nil {
		return err
	}

	if block, exists := c.blocks[blockID]; exists {
		block.accepted = true
	}
	return nil
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
