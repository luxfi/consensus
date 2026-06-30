// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// topological.go — the PREFERENCE LAYER, the mutable, sibling-tolerant block tree.
//
// This mirrors avalanchego snow/consensus/snowman/topological.go: the live block
// tree (blocks/tips), the build preference, and the vote/poll surface that drives
// each block's snowball instance. It is the half of avalanchego's Topological that
// stays MUTABLE — siblings coexist, votes accumulate, preference moves. The OTHER
// half of avalanchego's Topological — the committed-prefix advance (lastAcceptedID /
// acceptPreferredChild / rejectTransitively, β-driven there) — is decomplected out:
// in Lux it is CERT-driven and lives as the pure fold in ledger.go, applied by the
// shell in consensus.go. Same tree shape; finality trigger swapped from β to cert.
//
// Method mapping (ours -> avalanchego Topological):
//
//	Block / *Block               -> snowman_block.go's snowmanBlock (the tree node)
//	AddBlock                     -> Add (admit a child of a known block)
//	ProcessVote / Poll           -> RecordPoll (accumulate votes into the per-block
//	                                snowball driver; LIVENESS only — finality is the cert)
//	IsAccepted / IsRejected      -> block status (Decided)
//	GetBlock                     -> blocks[id] lookup (Processing)
//	Preference / ForcePreference -> Preference / preferredIDs (the build tail)
//	EpochHeightOf                -> (Lux-only) the block's pinned P-chain epoch height
//	ancestry / blocksAncestry    -> GetParent + children, exposed as the read-only
//	                                Ancestry the pure Finalize fold reads
//
// The Photon -> Wave -> Focus per-block driver (engine.Driver) is avalanchego's
// per-node snowball.Consensus instance — orthogonal to the tree and kept AS-IS.
package chain

import (
	"context"
	"fmt"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/ids"
)

// Block represents a block in the chain — the preference tree's node (avalanchego
// snowmanBlock). Tracks the value-chain linkage (id/parent/height), the pinned
// P-chain epoch, the per-block Photon->Wave->Focus driver, and the decided flags.
type Block struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	data      []byte

	// canonicalID / parentCanonicalID / execStateRoot / payloadRoot are the inner
	// EXECUTION commitment (the incident-1082814 canonical identity). For a
	// proposervm-wrapped block these are the inner block's identity; for a bare block
	// canonicalID == id (graceful degrade). canonicalID is what finality, the
	// per-height equivocation index, and the cert position all key on; the outer
	// id/parentID stay the transport/DAG keys. ids.Empty roots mean "not exposed".
	canonicalID       ids.ID
	parentCanonicalID ids.ID
	execStateRoot     ids.ID
	payloadRoot       ids.ID

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

// AddBlock admits a block into the preference tree (avalanchego Topological.Add). It
// is tracking-only and PERMISSIVE: any child is admitted, siblings coexist, and the
// new block becomes the sole build tip of its parent. Unknown-parent / fetch safety
// is enforced at FINALIZE (the fold's ErrAncestorNotTracked), not here — tracking is
// decomplected from finality.
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

// ProcessVote records one vote into a block's Photon->Wave->Focus driver
// (avalanchego RecordPoll, per block). Reaching the α accept count sets the LIVENESS
// flag block.accepted — the engine's DrainAccepted trigger — but NEVER advances the
// committed ledger. Finality is the cert fold's job alone.
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

// Poll conducts a consensus poll over a batch of vote responses (avalanchego
// RecordPoll). It drives each block's Wave->Focus driver; convergence + the α accept
// count sets the LIVENESS flag only. Finality (and the reorg) is the cert path.
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

// Preference returns the FINALITY preference — the cert-selected/finalized tip
// (avalanchego Topological.Preference). It stays at the finalized tip until a
// quorum cert selects a child; a mere build-tip move does NOT advance it. This is
// the finality-reporting concern and MUST NOT be conflated with the build target
// (see PreferredBuildTip) — the conformance suite pins this contract.
func (c *ChainConsensus) Preference() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// The committed finalized (certified) tip wins once finality has advanced; before
	// any cert, the recovery hint (vm.LastAccepted) is the build anchor.
	if anchor, ok := c.ledger.BuildAnchor(); ok {
		return anchor
	}
	// No certified tip and no hint: the preliminary build preference, then any tip.
	if c.preference != ids.Empty {
		return c.preference
	}
	for tip := range c.tips {
		return tip
	}
	return ids.Empty
}

// PreferredBuildTip returns the deterministic BUILD target — the deepest VERIFIED
// block extending the finalized chain (NOT merely the finalized tip). The node
// steers the VM to build its next block on THIS, so when a verified-but-unfinalized
// block exists at height H every validator builds H+1 ON TOP of it and they
// CONVERGE — instead of each building a competing sibling at height H, which splits
// the α-of-K vote so no single block ever reaches the cert and the chain HALTS the
// moment one proposer is down (the down-leader liveness bug). Sibling ties break on
// lowest block ID so every node with the same block tree picks the SAME chain.
//
// This is the DECOMPLECTED build concern (separate from the finality Preference):
// it is a BUILD hint only — finality stays governed exclusively by the α-of-K cert
// folded into c.ledger (applyCertLocked), so advancing the build target past the
// finalized tip touches no finality decision and cannot affect safety, only
// liveness — exactly as avalanchego's snowman steers the VM to its preferred
// non-finalized tip.
func (c *ChainConsensus) PreferredBuildTip() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buildTipLocked()
}

// buildTipLocked descends from the finalized tip (or, before the first finalize,
// the preliminary preference, else the lowest-ID tracked tip) through VERIFIED,
// non-rejected children — choosing the lowest-ID child at each level — and returns
// the first block with no such child: the deterministic build tip. The descent is
// bounded by the tracked-block count so a malformed tree can never spin forever.
// Caller holds c.mu.
func (c *ChainConsensus) buildTipLocked() ids.ID {
	var cur ids.ID
	anchor, hasAnchor := c.ledger.BuildAnchor()
	switch {
	case hasAnchor:
		cur = anchor
	case c.preference != ids.Empty:
		cur = c.preference
	default:
		// No finalized head and no preliminary preference yet: anchor on the
		// lowest-ID tracked tip so the choice is deterministic across nodes.
		for id := range c.tips {
			if cur == ids.Empty || id.Compare(cur) < 0 {
				cur = id
			}
		}
		if cur == ids.Empty {
			return ids.Empty
		}
	}
	anc := c.ancestry()
	for range c.blocks {
		best := ids.Empty
		for _, ch := range anc.Children(cur) {
			b, ok := c.blocks[ch]
			if !ok || b.rejected {
				continue
			}
			if best == ids.Empty || ch.Compare(best) < 0 {
				best = ch
			}
		}
		if best == ids.Empty {
			break // cur has no verified child — it is the build tip
		}
		cur = best
	}
	return cur
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

// ForcePreference reaffirms the engine's preferred tip after a VM SetPreference
// failure. It is a recovery mechanism used when SetPreference fails AFTER a block
// was accepted — without it the VM and consensus engine could disagree on the
// chain tip, causing a state-divergence death spiral. Every legitimate caller invokes
// it with the block that was JUST finalized ("SetPreference failed after Accept"), so
// the block is already the finalized tip and this is a reaffirming no-op.
//
// The committed ledger tip is advanced ONLY by the cert fold (applyCertLocked); the
// reorg is the sole authority on finalized history. So this method never moves the
// finalized tip — with the per-height ADMISSION gate gone, there is no invariant for a
// stray preference to corrupt. It only adopts blockID as the preliminary build
// preference before the FIRST finalize, and always records it as a build tip.
func (c *ChainConsensus) ForcePreference(blockID ids.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ledger.set {
		c.preference = blockID // no finalized head yet — preliminary build preference
	}
	// After the first finalize the ledger tip is authoritative and untouched here.
	c.tips[blockID] = true
}

// ancestry exposes the live block tree to the pure fold as a read-only Ancestry. The
// fold reads parent/height links and sibling children through this view ONLY; it never
// mutates the DAG. Caller holds c.mu (the view is used only within the locked fold).
func (c *ChainConsensus) ancestry() Ancestry { return blocksAncestry{blocks: c.blocks} }

// blocksAncestry is the Ancestry over c.blocks (avalanchego GetParent + the per-block
// children). Parent/Children are exact reads of the tree linkage, expressed behind the
// interface so the Finalize fold stays engine-free and unit-testable.
type blocksAncestry struct{ blocks map[ids.ID]*Block }

func (a blocksAncestry) Parent(id ids.ID) (ids.ID, uint64, ids.ID, bool) {
	b, ok := a.blocks[id]
	if !ok {
		return ids.Empty, 0, ids.Empty, false
	}
	canonical := b.canonicalID
	if canonical == ids.Empty {
		canonical = b.id // bare block: canonical == outer
	}
	return b.parentID, b.height, canonical, true
}

func (a blocksAncestry) Children(id ids.ID) []ids.ID {
	var out []ids.ID
	for cid, b := range a.blocks {
		if b.parentID == id {
			out = append(out, cid)
		}
	}
	return out
}
