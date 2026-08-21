// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// topological.go — the preference layer, the mutable, sibling-tolerant block tree.
//
// This is the topological half of a linear-chain engine: the live block tree
// (blocks/tips), the build preference, and the vote/poll surface that drives each
// block's confidence counter. It is the half that stays mutable — siblings coexist,
// votes accumulate, preference moves. The other half — the committed-prefix advance
// (lastAcceptedID / acceptPreferredChild / rejectTransitively, β-driven in a
// classical engine) — is decomplected out: here it is cert-driven and lives as the
// pure fold in ledger.go, applied by the shell in consensus.go. Same tree shape;
// finality trigger swapped from β to cert.
//
// Method roles:
//
//	Block / *Block               -> the tree-node block
//	AddBlock                     -> admit a child of a known block
//	ProcessVote / Poll           -> accumulate votes into the per-block confidence
//	                                driver; liveness only — finality is the cert
//	IsAccepted / IsRejected      -> block status (decided)
//	GetBlock                     -> blocks[id] lookup (processing)
//	Preference / ForcePreference -> the build tail
//	EpochHeightOf                -> the block's pinned P-chain epoch height
//	ancestry / blocksAncestry    -> parent + children, exposed as the read-only
//	                                Ancestry the pure Finalize fold reads
//
// The Photon -> Wave -> Focus per-block driver (engine.Driver) is the per-node
// confidence instance — orthogonal to the tree and untouched by it.
package chain

import (
	"slices"
	"context"
	"fmt"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/ids"
)

// Block represents a block in the chain — the preference tree's node. Tracks the
// value-chain linkage (id/parent/height), the pinned P-chain epoch, the per-block
// Photon->Wave->Focus driver, and the decided flags.
type Block struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	data      []byte

	// canonicalID / parentCanonicalID / execStateRoot / payloadRoot are the inner
	// execution commitment — the block's canonical identity. For a proposervm-wrapped
	// block these are the inner block's identity; for a bare block canonicalID == id
	// (graceful degrade). canonicalID is what finality, the per-height equivocation
	// index, and the cert position all key on; the outer id/parentID stay the
	// transport/DAG keys. ids.Empty roots mean "not exposed".
	canonicalID       ids.ID
	parentCanonicalID ids.ID
	execStateRoot     ids.ID
	payloadRoot       ids.ID

	// pChainHeight is the P-chain epoch height the block's weighted validator set is
	// pinned to: the set-root commitment, the ⅔-by-stake tally, and the per-voter
	// pubkey resolution all read the height-indexed validators.State at this height,
	// never the value-chain `height`. The two are different quantities.
	// platformvm.GetValidatorSet interprets its argument as a P-chain height and
	// returns errUnfinalizedHeight once that argument exceeds the current P-chain
	// height, and the value-chain height races ahead of the P-chain height on a busy
	// chain. Passing `height` there therefore resolves an empty validator set, and no
	// tally over an empty set can ever reach ⅔, so finality stops for good.
	//
	// Source: a proposervm signed block carries its PChainHeight
	// (block.SignedBlock.PChainHeight); pChainHeightOf reads it off the VM block at
	// the engine boundary. When the block does not expose one (the VM is not
	// proposervm-wrapped at the engine boundary, which is the case for the current
	// in-process chain stack), this is 0 → the set is read at P-chain height 0, the
	// genesis validator set. That is non-empty, identical on every node (everyone
	// agrees on genesis), and ≤ the current P-chain height, so finality stays live and
	// consistent — and exact for any chain whose validator set is unchanged since
	// genesis. Pinning post-genesis staking-change epochs instead needs the proposervm
	// to deliver its PChainHeight to the engine's block; the mechanism here is covered
	// by TestPChainEpochFinality_RealWiring, which feeds a block that carries one.
	pChainHeight uint64

	// Consensus state - Photon -> Wave -> Focus finality
	driver   *engine.Driver
	accepted bool
	rejected bool

	// acceptVoters / rejectVoters are the tally. α means "α distinct validators
	// agreed"; a plain counter any single voter can advance by repeating itself
	// does not mean that — without the voter's identity this layer cannot tell one
	// validator answering four times from four validators answering once, and one
	// node alone carries a block past α. That takes no malice: polls are
	// re-solicited and peers do answer twice.
	//
	// They are sets, and acceptVotes()/rejectVotes() derive from them, so the
	// count cannot drift from the identities it claims to summarize.
	acceptVoters map[ids.NodeID]struct{}
	rejectVoters map[ids.NodeID]struct{}
}

// acceptVotes is the number of distinct validators that accepted — the α predicate.
func (b *Block) acceptVotes() int { return len(b.acceptVoters) }

// rejectVotes is the number of distinct validators that rejected.
func (b *Block) rejectVotes() int { return len(b.rejectVoters) }

// AddBlock admits a block into the preference tree. It is tracking-only and
// permissive: any child is admitted, siblings coexist, and the new block becomes the
// sole build tip of its parent. Unknown-parent and fetch safety are enforced at
// finalize (the fold's ErrAncestorNotTracked), not here — tracking is decomplected
// from finality.
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

// ProcessVote records one vote into a block's Photon->Wave->Focus driver (per
// block). Reaching the α accept count sets the liveness flag block.accepted — the
// engine's DrainAccepted trigger — and never advances the committed ledger. Finality
// is the cert fold's job alone.
func (c *ChainConsensus) ProcessVote(ctx context.Context, blockID ids.ID, voter ids.NodeID, accept bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	block, exists := c.blocks[blockID]
	if !exists {
		return fmt.Errorf("block not found: %s", blockID)
	}

	if block.driver == nil {
		return fmt.Errorf("block not initialized for consensus")
	}

	// Track both accept and reject votes — one per validator. A repeat from a
	// voter already counted is not an error and not evidence of anything (polls
	// are re-solicited), it simply must not move the tally a second time. The
	// driver is likewise only advanced on a voter's first accept, so confidence
	// cannot be self-assembled either.
	if accept {
		if _, counted := block.acceptVoters[voter]; counted {
			return nil
		}
		if block.acceptVoters == nil {
			block.acceptVoters = make(map[ids.NodeID]struct{})
		}
		block.acceptVoters[voter] = struct{}{}

		// Confidence advances per successful poll, not per vote. β is the number of
		// consecutive polls that each reached α — that is what makes a decision worth
		// βα agreements. Recording a poll on every arriving vote collapses that to β
		// agreements total: at K=5, α=4, β=2 the driver decides after three distinct
		// accepts, so VM.Accept runs on a block the α=4 quorum never carried and two
		// of five validators settle it, far under the threshold the parameters name.
		//
		// Testing the α predicate first restores the floor: no confidence accrues,
		// and so nothing can be decided, until α distinct validators have accepted.
		if len(block.acceptVoters) >= c.alpha {
			block.driver.RecordVote(blockID)
		}
	} else {
		if _, counted := block.rejectVoters[voter]; counted {
			return nil
		}
		if block.rejectVoters == nil {
			block.rejectVoters = make(map[ids.NodeID]struct{})
		}
		block.rejectVoters[voter] = struct{}{}
	}

	// Liveness only (decomplected from finality). Reaching the α accept count marks
	// the block worth a finalize attempt — block.accepted is the engine's
	// DrainAccepted trigger — but it does not advance the per-height ledger. Finality
	// is committed only by the cert-driven FinalizeBranch (the α-of-K signed witness),
	// which also performs the sibling reorg. A sibling reaching α-count here is
	// harmless: the cert decides which branch finalizes, and the loser is pruned.
	if block.acceptVotes() >= c.alpha && !block.accepted {
		block.accepted = true
	}

	// Check if rejection quorum is reached (reject votes >= alpha)
	if block.rejectVotes() >= c.alpha {
		block.rejected = true
		// Remove from tips since this block is rejected
		delete(c.tips, blockID)
	}

	return nil
}

// Poll conducts a consensus poll over a batch of vote responses. It drives each
// block's Wave->Focus driver; convergence plus the α accept count sets the liveness
// flag only. Finality (and the reorg) is the cert path.
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
		if block.rejectVotes() >= c.alpha {
			block.rejected = true
			delete(c.tips, blockID)
			continue
		}

		// Only consider acceptance if we have enough accept votes
		// This prevents premature acceptance with insufficient quorum
		if block.acceptVotes() < c.alpha {
			continue
		}

		if block.driver != nil {
			blockResponses := map[ids.ID]int{blockID: votes}
			shouldContinue := block.driver.Poll(blockResponses)
			decided := block.driver.Decided()

			// Focus convergence plus the α accept count is liveness: the block is
			// worth a finalize attempt. Finality (and the reorg) belongs to the cert
			// path (FinalizeBranch), not the count path — so the count neither
			// advances the ledger nor branches it.
			if !shouldContinue && decided && block.acceptVotes() >= c.alpha {
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

// Preference returns the finality preference — the cert-selected finalized tip. It
// stays there until a quorum cert selects a child; a build-tip move does not advance
// it. This is the finality-reporting concern, distinct from the build target (see
// PreferredBuildTip) — the conformance suite pins this contract.
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

// PreferredBuildTip returns the deterministic build target — the deepest verified
// block extending the finalized chain, which is not in general the finalized tip.
// The node steers the VM to build its next block on this, so when a verified but
// unfinalized block exists at height H every validator builds H+1 on top of it and
// they converge. Building on the finalized tip instead gives each proposer a
// competing sibling at height H, which splits the α-of-K vote, so no single block
// reaches a cert and the chain stops the moment one proposer is down. Sibling ties
// break on lowest block ID, so every node holding the same tree picks the same chain.
//
// This is the build concern, decomplected from the finality Preference: a hint only.
// Finality is governed exclusively by the α-of-K cert folded into c.ledger
// (applyCertLocked), so advancing the build target past the finalized tip decides
// nothing and can affect liveness but not safety — the same way a linear-chain
// engine steers the VM to its preferred non-finalized tip.
func (c *ChainConsensus) PreferredBuildTip() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buildTipLocked()
}

// buildTipLocked descends from the finalized tip (or, before the first finalize,
// the preliminary preference, else the lowest-ID tracked tip) through verified,
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

// EpochHeightOf returns the P-chain epoch height recorded for a tracked block
// (the height its weighted validator set, set-root, and ⅔-stake tally are pinned
// to — Block.pChainHeight), and whether the block is tracked at all. It is the
// single authoritative read of "what epoch did we record for this block", used by
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

// DropEpochRegressedChildren removes every tracked child of parentID whose recorded
// P-chain epoch sits BELOW parentID's, returning their ids so the engine can evict
// them from its own pending indices too. It is the arrival-ORDER arm of the
// receive-side monotonicity bound EpochHeightOf feeds: a child gossiped before its
// parent is admitted as an orphan with no recorded parent epoch to regress against,
// and this re-reads the bound the moment the parent makes it answerable — so no
// TRACKED block keeps an epoch below its TRACKED parent's, whichever order the sender
// chose to deliver them in. A dropped child, re-gossiped, is refused outright by the
// arrival gate, its parent now being tracked. Removing only the direct children
// restores the bound: a grandchild's parent is now untracked, so it is an orphan
// again with nothing to regress against.
func (c *ChainConsensus) DropEpochRegressedChildren(parentID ids.ID) []ids.ID {
	c.mu.Lock()
	defer c.mu.Unlock()

	parent, ok := c.blocks[parentID]
	if !ok {
		return nil
	}
	var dropped []ids.ID
	for id, b := range c.blocks {
		if b.parentID == parentID && b.pChainHeight < parent.pChainHeight {
			dropped = append(dropped, id)
		}
	}
	for _, id := range dropped {
		delete(c.blocks, id)
		delete(c.tips, id)
	}
	return dropped
}

// ForcePreference reaffirms the engine's preferred tip after a VM SetPreference
// failure. It exists for the case where SetPreference fails after a block was
// accepted: left alone, the VM and the engine hold different chain tips and each
// subsequent block widens the divergence. Every legitimate caller passes the block
// that was just finalized, so the block is already the finalized tip and this is a
// reaffirming no-op.
//
// The committed ledger tip advances only through the cert fold (applyCertLocked);
// the reorg is the sole authority on finalized history. So this method never moves
// the finalized tip, and nothing here keys on preference, so a stray preference has
// no invariant to corrupt. It adopts blockID as the preliminary build preference
// before the first finalize, and always records it as a build tip.
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
// fold reads parent/height links and sibling children through this view alone; it never
// mutates the DAG. Caller holds c.mu (the view is used only within the locked fold).
func (c *ChainConsensus) ancestry() Ancestry {
	return blocksAncestry{blocks: c.blocks, recall: c.recall}
}

// blocksAncestry is the Ancestry over c.blocks (parent lookup + the per-block
// children). Parent/Children are exact reads of the tree linkage, expressed behind the
// interface so the Finalize fold stays engine-free and unit-testable.
type blocksAncestry struct {
	blocks map[ids.ID]*Block
	recall func(ids.ID) (*Block, bool)
}

// at resolves a block for the walk: from the live tree first, and failing that from
// the VM's own accepted chain.
//
// The live tree holds what this process has seen; the finalized tip it walks up from
// is durable. The two disagree after any restart — the node comes back knowing it
// finalized through height H and remembering nothing above it, while its VM still
// holds every block it ever accepted. Without the second lookup the walk cannot cross
// that boundary, so a valid cert for a block above H is refused for a missing ancestor
// the node is in fact holding, and the gap only widens with the next restart.
//
// Reading it back from the VM is the stronger source, not a weaker one: the VM's
// chain is what this node actually executed and committed, where the live tree is
// populated from gossip. Nothing here decides finality — the cert has already cleared
// the ⅔-by-stake predicate on its own — so this only supplies the parent links that
// establish the path, and a block the VM does not hold still misses, preserving the
// fail-closed defer.
func (a blocksAncestry) at(id ids.ID) (*Block, bool) {
	if b, ok := a.blocks[id]; ok {
		return b, true
	}
	if a.recall == nil {
		return nil, false
	}
	return a.recall(id)
}

func (a blocksAncestry) Parent(id ids.ID) (ids.ID, uint64, ids.ID, ids.ID, bool) {
	b, ok := a.at(id)
	if !ok {
		return ids.Empty, 0, ids.Empty, ids.Empty, false
	}
	// canonicalRep/parentCanonicalRep fall back to the outer id for a bare (non-wrapped)
	// block, so a chain with no inner/outer split is unchanged.
	return b.parentID, b.height, b.canonicalRep(), b.parentCanonicalRep(), true
}

// WrapperByCanonical resolves an inner execution commitment at a height to a
// locally-tracked outer wrapper of it — any alias — so the finality walk can collapse a
// sibling wrapper in place of the exact envelope a cert named (the intermediate ancestor
// the node holds under a different proposervm wrapper). It prefers an already-accepted
// wrapper when several tracked aliases exist, so the walk lands on the block the VM
// committed. A miss (ok=false) means the node holds NO wrapper of that inner block,
// preserving the fail-closed behind-node defer (ErrAncestorNotTracked). This is a rare
// defer-path O(n) scan over the live block set, matching pendingByCanonicalLocked.
func (a blocksAncestry) WrapperByCanonical(canonical ids.ID, height uint64) (ids.ID, bool) {
	// This ranges the live block map, so without a deterministic choice among
	// equal candidates it returns whichever wrapper Go's map iteration reached
	// first — a different one per node. Finalize is then not a function of its
	// inputs: two nodes holding identical blocks accept different envelopes at one
	// height, hand the VM opposite Accept/Reject for the same pair, and answer a
	// bootstrapping peer's outer-envelope check differently. A committed wrapper
	// still wins outright; among uncommitted aliases the lowest id wins, so every
	// node lands on the same block.
	var hit ids.ID
	found := false
	for id, b := range a.blocks {
		if b.height != height || b.canonicalRep() != canonical {
			continue
		}
		if b.accepted {
			return id, true // the wrapper the VM actually committed — prefer it
		}
		if !found || id.Compare(hit) < 0 {
			hit, found = id, true
		}
	}
	return hit, found
}

func (a blocksAncestry) Children(id ids.ID) []ids.ID {
	// Sorted, because a map range yields a different order per node and a caller
	// that acts on the first child would then act on a different one on each.
	var out []ids.ID
	for cid, b := range a.blocks {
		if b.parentID == id {
			out = append(out, cid)
		}
	}
	slices.SortFunc(out, func(x, y ids.ID) int { return x.Compare(y) })
	return out
}
