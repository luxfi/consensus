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

	// Check if acceptance quorum is reached (accept votes >= alpha)
	if block.acceptVotes >= c.alpha && !block.accepted {
		block.accepted = true
		c.finalizedTip = blockID
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
			// AND we have sufficient accept votes (quorum reached)
			if !shouldContinue && decided && block.acceptVotes >= c.alpha {
				block.accepted = true
				c.finalizedTip = blockID
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
func (c *ChainConsensus) SyncState(lastAcceptedID ids.ID, height uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update finalized tip to the synced block
	c.finalizedTip = lastAcceptedID

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
}

// ForcePreference forces the consensus preferred tip to the given block.
// This is a recovery mechanism used when SetPreference fails after Accept —
// without it, the VM and consensus engine disagree on the chain tip, causing
// a state divergence death spiral.
func (c *ChainConsensus) ForcePreference(blockID ids.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.finalizedTip = blockID
	// Ensure the forced block is a tip
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
	block.accepted = true
	c.finalizedTip = blockID
	return nil
}

// AcceptViaCert marks a block accepted because a verified QuorumCert proves
// α-of-K validators accepted it. This is the multi-validator finalization
// authority that replaces the deleted self-accept force path: the caller has
// ALREADY verified the cert (cert.Verify) against the chain's VoteVerifier, so
// this method records the decision the cert proves. It does not re-decide
// quorum — the cert IS the quorum proof.
//
// The block need not be locally tracked in c.blocks (a follower may finalize a
// block via cert before it ever entered local consensus tracking): when absent
// the finalized tip is still advanced so the engine's view matches the proven
// finality. Idempotent on an already-accepted tracked block.
func (c *ChainConsensus) AcceptViaCert(blockID ids.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if block, exists := c.blocks[blockID]; exists {
		if block.accepted {
			return
		}
		block.accepted = true
	}
	c.finalizedTip = blockID
}

// GetFinalizedTip returns the current finalized tip block ID.
// This is useful for debugging and health checks.
func (c *ChainConsensus) GetFinalizedTip() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finalizedTip
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
