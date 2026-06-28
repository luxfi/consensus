// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/ids"
)

// The cert-FOLD finalization errors (ErrHeightAlreadyFinalized,
// ErrNonMonotonicFinalizedHeight, ErrConflictsWithFinalizedBranch,
// ErrAncestorNotTracked) live with the pure Finalize fold in ledger.go — they are
// the fold's own vocabulary. The errors below are the engine SHELL's: SyncState (an
// out-of-band import reconcile that bypasses the fold by design) and the receive-side
// epoch gate. Decomplected: fold errors with the fold, shell errors with the shell.
var (
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

	// ledger is the committed finality VALUE — the append-only prefix of finalized
	// history (ledger.go), the one source of truth for which block is final at each
	// height. It is only ever REPLACED WHOLE, never poked field-by-field, so there is
	// no markFinalized to call. Exactly two paths replace it: the cert fold
	// (applyCertLocked, the production path) and SyncState (the out-of-band import
	// reconcile). A single non-branching finalized chain is the OUTPUT of the cert's
	// reorg — pruning the losing sibling subtree — never an admission refusal.
	ledger FinalityLedger

	// preference is the preliminary BUILD tip used BEFORE the first finalize — the
	// DECOMPLECTED preference concern (avalanchego keeps `preference` separate from the
	// committed `lastAcceptedID`). Once the ledger is set the finalized tip wins and
	// this is unused; ForcePreference seeds it, Preference() reads it only pre-finalize.
	preference ids.ID
}

// NewChainConsensus creates a real consensus engine
func NewChainConsensus(k, alpha, beta int) *ChainConsensus {
	return &ChainConsensus{
		k:      k,
		alpha:  alpha,
		beta:   beta,
		blocks: make(map[ids.ID]*Block),
		tips:   make(map[ids.ID]bool),
	}
}

// ApplyCert is THE canonical finalize: fold a Cert into the committed ledger and return
// the plan the engine applies to the VM. It is the production finality path
// (engine.acceptWithCertCore calls it) — finality advances by replacing the ledger with
// the fold's result, never by a field poke, so there is no markFinalized to call.
//
// cert.Parent may be ids.Empty only for the genesis / first finalize. On a safety
// violation — equivocation (ErrHeightAlreadyFinalized), a losing/conflicting branch
// (ErrConflictsWithFinalizedBranch), a height gap (ErrNonMonotonicFinalizedHeight), or
// a not-yet-tracked ancestor (ErrAncestorNotTracked, a behind-node DEFER) — NOTHING is
// applied and the error propagates. Takes c.mu.
func (c *ChainConsensus) ApplyCert(cert Cert) (Plan, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.applyCertLocked(cert)
}

// FinalizeBranch is the back-compat wrapper for call sites that hold a
// (target, height, parent) triple rather than a Cert (bootstrap/catch-up accept and the
// safety tests). It builds the Cert and folds it — behavior identical to ApplyCert.
// parentID may be ids.Empty only for the genesis / first finalize.
func (c *ChainConsensus) FinalizeBranch(target ids.ID, height uint64, parentID ids.ID) (Plan, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.applyCertLocked(Cert{Block: target, Parent: parentID, Height: height})
}

// applyCertLocked is the engine SHELL around the pure fold: fold the cert into a NEW
// ledger value, replace the ledger with it, then apply the plan's DAG effects. Caller
// holds c.mu. This is the production finality writer; the only other ledger replacement
// is SyncState (the out-of-band import reconcile). Neither pokes a finality field.
func (c *ChainConsensus) applyCertLocked(cert Cert) (Plan, error) {
	led, plan, err := Finalize(c.ledger, cert, c.ancestry())
	if err != nil {
		return Plan{}, err
	}
	c.ledger = led    // THE ONLY way finality advances — one value assignment after a pure fold
	c.applyPlan(plan) // DAG-side effects only (accepted/rejected/tips); never finality
	return plan, nil
}

// applyPlan applies the fold's plan to the live DAG: mark the Accept path accepted and
// drop it from the build tips; mark the Reject (losing-sibling) subtrees rejected and
// remove them from the live DAG/tips — avalanchego acceptPreferredChild + rejectTransitively
// on the DAG side. It NEVER touches c.ledger; finality already advanced in the fold.
// Caller holds c.mu.
func (c *ChainConsensus) applyPlan(plan Plan) {
	for _, id := range plan.Accept {
		if b, ok := c.blocks[id]; ok {
			b.accepted = true
		}
		delete(c.tips, id) // a finalized block is no longer an open build tip
	}
	for _, id := range plan.Reject {
		if b, ok := c.blocks[id]; ok {
			b.rejected = true
		}
		delete(c.blocks, id)
		delete(c.tips, id)
	}
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
		"finalized_tip": c.ledger.tip.String(),
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
	if c.ledger.set && lastAcceptedID != ids.Empty && height < c.ledger.height {
		return fmt.Errorf("%w: import height %d < finalized height %d (block %s)",
			ErrSyncStateRegression, height, c.ledger.height, lastAcceptedID)
	}
	// Equal-height re-import must agree on the block (a different block at the
	// already-finalized height is equivocation, not a reconcile).
	if c.ledger.set && lastAcceptedID != ids.Empty && height == c.ledger.height {
		if existing, ok := c.ledger.At(height); ok && existing != lastAcceptedID {
			return fmt.Errorf("%w: import height %d already finalized %s, refused %s",
				ErrHeightAlreadyFinalized, height, existing, lastAcceptedID)
		}
	}

	// Seed a NEW ledger value at the synced head so the next finalize (the first block
	// built on the import) satisfies the monotonic-height + descends-from-tip invariants
	// against the imported head. A whole-value assignment — not a field poke.
	if lastAcceptedID != ids.Empty {
		c.ledger = seedLedger(lastAcceptedID, height)
	} else {
		// genesis/teardown reset (empty head at height 0): reset finality to a CLEAN
		// genesis VALUE — Empty tip, height 0, unset, no per-height record. LOW-2: the
		// prior code cleared ONLY the tip and left height/byHeight/set stale, which
		// desynced the tip from the height — every future cert then wedged in pathFromTip
		// (it seeks the now-Empty tip and never finds it). A whole-value reset cannot
		// desync: the next cert simply seeds finality afresh (first-finalize).
		c.ledger = FinalityLedger{}
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

// GetFinalizedTip returns the current finalized tip block ID.
// This is useful for debugging and health checks.
func (c *ChainConsensus) GetFinalizedTip() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ledger.tip
}

// GetFinalizedHeight returns the current finalized height and whether any block
// has been finalized yet. The engine uses this to gate an incoming cert on
// height (reject a cert at or below the last-finalized height — MED-5) BEFORE
// running the cert through the per-height guard.
func (c *ChainConsensus) GetFinalizedHeight() (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ledger.Height()
}

// FinalizedBlockAtHeight returns the block finalized at the given height, if
// any. Used to produce equivocation evidence: when a second cert is refused at
// an already-finalized height, the caller reports BOTH the finalized block and
// the refused one.
func (c *ChainConsensus) FinalizedBlockAtHeight(height uint64) (ids.ID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ledger.At(height)
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
