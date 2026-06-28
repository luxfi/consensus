// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// ledger.go — the FUNCTIONAL CORE of finality, decomplected from the engine.
//
// FinalityLedger is the committed, append-only prefix of finalized history — an
// immutable VALUE. Finalize is a PURE FOLD of a cert into that value:
//
//	Finalize : (Ledger, Cert, DAG) -> (Ledger', Plan, error)
//
// It has no receiver, no lock, no VM, and reads the preference DAG only through the
// read-only Ancestry interface. Because the ledger is replaced as a whole value
// (never mutated in place) and there is no field-poking method, "accept =
// markFinalized(...)" is structurally unwriteable: finality advances only by folding a
// cert and replacing the ledger (applyCertLocked) — the import reconcile (SyncState) is
// the one other whole-value replacement.
//
// This mirrors avalanchego snow/consensus/snowman/topological.go, with finality's
// trigger swapped from β-consecutive-polls to a quorum CERT: the preference tree +
// poll stays mutable and sibling-tolerant (topological.go here), while the
// committed-prefix advance — β-driven in Snowman — is this pure, cert-driven fold.
package chain

import (
	"errors"
	"fmt"

	"github.com/luxfi/ids"
)

// The finality safety invariants — the vocabulary of the fold. Two are genuine
// α-of-K-cert properties pure Snowman does not need (its certs are β-witnessed, not
// external/attacker-chosen-Round): a SECOND valid cert for a DIFFERENT block at an
// already-decided height is equivocation evidence, never a silent second VM.Accept;
// and a cert for a block that does NOT descend from the finalized frontier conflicts
// with finalized history and is refused. The single-non-branching-chain property is
// achieved the avalanchego way — not by refusing siblings at admission, but by REORG
// (prune the losing sibling subtree when the cert selects the winner).
var (
	// ErrHeightAlreadyFinalized: a DIFFERENT block is already finalized at the target
	// height — two valid α-certs at one height across different rounds. The first
	// finalizes; the second is refused and IS equivocation evidence. A genuine α-of-K
	// safety property pure Snowman does not need.
	ErrHeightAlreadyFinalized = errors.New("chain: a different block is already finalized at this height (equivocation: two finalized blocks at one height)")

	// ErrNonMonotonicFinalizedHeight: a finalize at or below the frontier with a
	// different block, or a cert-selected branch not CONTIGUOUS with the frontier (a
	// height gap / malformed linkage). Finality only ever moves forward, one height at
	// a time, along a tracked ancestry chain.
	ErrNonMonotonicFinalizedHeight = errors.New("chain: finalized height must strictly increase by contiguous steps (cannot re-finalize an old/equal height, nor jump a height gap)")

	// ErrConflictsWithFinalizedBranch: a cert-selected block does NOT descend from the
	// finalized tip — its ancestry reaches a block at/below the finalized height that is
	// NOT the finalized tip (a losing/pruned sibling branch). Under <⅓ Byzantine this
	// can only happen for a branch the network did not finalize; finalizing it would
	// branch finalized history, so it is refused.
	ErrConflictsWithFinalizedBranch = errors.New("chain: cert-selected block does not extend the finalized frontier (it descends from a losing/pruned sibling branch)")

	// ErrAncestorNotTracked: the path from the finalized tip up to the cert-selected
	// block cannot be proven because an ANCESTOR on it is not in the live DAG. This is a
	// DEFER, not a conflict: the node is behind and must fetch the missing ancestors,
	// then re-apply. It must NEVER finalize on this error.
	ErrAncestorNotTracked = errors.New("chain: cannot finalize — an ancestor between the finalized tip and the cert-selected block is not tracked (behind; fetch and retry)")
)

// FinalityLedger is the committed, append-only prefix of finalized history — an
// immutable VALUE. It is never mutated in place; Finalize returns a NEW one.
//
// tip/height name the head of finalized history (avalanchego's
// lastAcceptedID/lastAcceptedHeight — the committed lower bound); byHeight indexes
// every finalized height so a re-finalize of any past height with a different block
// is caught (α-of-K equivocation evidence). All fields are unexported and read-only
// after construction: the projections (Tip/Height/At) are the only outside view.
type FinalityLedger struct {
	tip      ids.ID            // head of finalized history (committed lower bound)
	height   uint64            // height of tip
	set      bool              // false until the first block is finalized
	byHeight map[uint64]ids.ID // every finalized height -> its block (equivocation index)
}

// Tip is the head of finalized history (ids.Empty before the first finalize).
func (l FinalityLedger) Tip() ids.ID { return l.tip }

// Height returns the finalized height and whether anything is finalized yet.
func (l FinalityLedger) Height() (uint64, bool) { return l.height, l.set }

// At returns the block finalized at height, if any (equivocation evidence lookup).
func (l FinalityLedger) At(height uint64) (ids.ID, bool) {
	id, ok := l.byHeight[height]
	return id, ok
}

// Ancestry is the READ-ONLY view of the preference DAG the fold needs. The
// preference layer (topological.go) implements it over the live block tree.
// Finalize NEVER mutates the DAG — it reads ancestry to prove the certified path and
// to collect the losing-sibling subtrees to prune.
type Ancestry interface {
	// Parent returns id's parent and id's OWN height; ok is false if id is untracked.
	Parent(id ids.ID) (parent ids.ID, height uint64, ok bool)
	// Children returns the ids of every tracked block whose parent is id.
	Children(id ids.ID) []ids.ID
}

// Cert is the minimal finality subject — the block a quorum certificate selects,
// decoupled from the wire VerifiedQuorumCert. Finality is "fold a Cert into the
// ledger". Parent is ids.Empty only for the genesis / first finalize.
type Cert struct {
	Block  ids.ID
	Parent ids.ID
	Height uint64
}

// Plan is what Finalize decides and the engine applies to the VM and DAG. It mirrors
// avalanchego topological.go's accept/reject split:
//
//   - Accept: the path from the OLD finalized tip up to the certified block, in
//     ASCENDING height order — acceptPreferredChild along a path (usually one block,
//     more on a catch-up jump).
//   - Reject: every block on a LOSING sibling subtree — a sibling of a path block plus
//     all its descendants — rejectTransitively.
type Plan struct {
	Accept []ids.ID
	Reject []ids.ID
}

// Finalize is THE finality function: a pure fold of a Cert into the ledger. No
// receiver, no lock, no mutation, no VM. On ANY error the INPUT ledger is returned
// unchanged (the caller assigns nothing). On success it returns the advanced ledger
// (a fresh value) and the plan the engine applies.
//
// It enforces (ported verbatim from the prior finalizeBranchLocked):
//
//	(a) ONE finalized block per height. The same block already finalized here is an
//	    idempotent no-op; a DIFFERENT block at an already-finalized height is
//	    equivocation → ErrHeightAlreadyFinalized.
//	(b) the certified block must DESCEND from the finalized tip via a tracked,
//	    contiguous ancestry: a non-tip ancestor at/below the finalized height →
//	    ErrConflictsWithFinalizedBranch; an untracked ancestor → ErrAncestorNotTracked
//	    (DEFER, behind); a height gap / malformed linkage → ErrNonMonotonicFinalizedHeight.
func Finalize(led FinalityLedger, cert Cert, dag Ancestry) (FinalityLedger, Plan, error) {
	// (a) idempotent / equivocation at the target height.
	if existing, ok := led.byHeight[cert.Height]; ok {
		if existing == cert.Block {
			return led, Plan{}, nil // this exact block already finalized here — no-op
		}
		return led, Plan{}, fmt.Errorf("%w: height %d already finalized %s, refused %s",
			ErrHeightAlreadyFinalized, cert.Height, existing, cert.Block)
	}

	// First finalize seeds the ledger — no prior tip to extend or reorg.
	if !led.set {
		return seedLedger(cert.Block, cert.Height), Plan{Accept: []ids.ID{cert.Block}}, nil
	}

	// Below/at the frontier with no record for this height → stale or non-monotonic.
	if cert.Height <= led.height {
		return led, Plan{}, fmt.Errorf("%w: refused height %d at finalized height %d (block %s)",
			ErrNonMonotonicFinalizedHeight, cert.Height, led.height, cert.Block)
	}

	// Walk the certified branch finalizedTip → target, proving contiguous tracked
	// ancestry. A cert may certify a descendant several heights above the tip (a
	// catch-up jump), so the path is 1..k blocks.
	path, err := pathFromTip(led, cert, dag)
	if err != nil {
		return led, Plan{}, err
	}

	// Build the NEXT ledger by COPY (never mutate the input map) and the plan: Accept
	// the path ascending, Reject every losing-sibling subtree along it.
	next := led.clone()
	var plan Plan
	for _, s := range path {
		plan.Reject = append(plan.Reject, losingSubtrees(s.id, s.parentID, dag)...)
		next.byHeight[s.height] = s.id
		next.tip = s.id
		next.height = s.height
		plan.Accept = append(plan.Accept, s.id)
	}
	next.pruneBelowWindow() // keep byHeight (and the next clone) O(window), not O(chain height)
	return next, plan, nil
}

// seedLedger constructs the first ledger value from the seed (id, height).
func seedLedger(id ids.ID, height uint64) FinalityLedger {
	return FinalityLedger{
		tip:      id,
		height:   height,
		set:      true,
		byHeight: map[uint64]ids.ID{height: id},
	}
}

// clone returns a deep copy of the ledger (a fresh byHeight map) so the fold never
// mutates the receiver value's map. One copy per Finalize. byHeight is bounded to
// equivocationWindow entries (pruneBelowWindow), so this copy is O(window) — constant
// cost at any chain height — never O(chain height).
func (l FinalityLedger) clone() FinalityLedger {
	bh := make(map[uint64]ids.ID, len(l.byHeight)+1)
	for h, id := range l.byHeight {
		bh[h] = id
	}
	return FinalityLedger{tip: l.tip, height: l.height, set: l.set, byHeight: bh}
}

// equivocationWindow bounds how many heights below the finalized tip the per-height
// index is retained. Equivocation is only actionable near the tip — a fork is attempted
// at or above the last finalized height; an older height is refused outright by the
// monotonic guard (cert.Height <= led.height) without consulting the index. Bounding
// byHeight to this window keeps the ledger — and therefore clone() — O(window) rather
// than O(chain height), and bounds its memory. Same "evidence is only useful near the
// tip" rationale as engine.go's slashingRetentionHeights.
const equivocationWindow = 1024

// pruneBelowWindow drops index entries older than equivocationWindow below the tip, so
// byHeight stays O(window) as the chain grows without bound. Pure: it mutates only the
// receiver's own already-cloned map (never the caller's input ledger).
func (l *FinalityLedger) pruneBelowWindow() {
	if l.height < equivocationWindow {
		return
	}
	cutoff := l.height - equivocationWindow
	for h := range l.byHeight {
		if h < cutoff {
			delete(l.byHeight, h)
		}
	}
}

// step is one block on the certified path from the finalized tip to the cert target.
type step struct {
	id       ids.ID
	height   uint64
	parentID ids.ID
}

// pathFromTip returns the contiguous ancestry finalizedTip → target in ASCENDING
// height order, by walking target's parent links through the DAG. Errors distinguish
// the three non-extending cases (conflict / behind / gap). Caller guarantees
// cert.Height > led.height and led.set.
func pathFromTip(led FinalityLedger, cert Cert, dag Ancestry) ([]step, error) {
	steps := []step{{id: cert.Block, height: cert.Height, parentID: cert.Parent}}
	cur := cert.Parent
	childHeight := cert.Height
	for cur != led.tip {
		parent, curHeight, ok := dag.Parent(cur)
		if !ok {
			// The path to the frontier is not fully tracked — the node is behind.
			// Fail-closed DEFER: never finalize on a path we cannot prove.
			return nil, fmt.Errorf("%w: ancestor %s of %s missing", ErrAncestorNotTracked, cur, cert.Block)
		}
		// Heights must strictly decrease toward the tip; a parent at/above its child's
		// height is malformed linkage.
		if curHeight >= childHeight {
			return nil, fmt.Errorf("%w: ancestor %s height %d not below child height %d",
				ErrNonMonotonicFinalizedHeight, cur, curHeight, childHeight)
		}
		// Reaching the finalized height (or below) at a block that is not the tip →
		// target descends from a branch the network did not finalize.
		if curHeight <= led.height {
			return nil, fmt.Errorf("%w: %s ancestry reaches %s (height %d) not finalized tip %s",
				ErrConflictsWithFinalizedBranch, cert.Block, cur, curHeight, led.tip)
		}
		steps = append(steps, step{id: cur, height: curHeight, parentID: parent})
		cur = parent
		childHeight = curHeight
	}

	// Reverse to ascending height and assert contiguity with the frontier: the lowest
	// step must be exactly finalizedHeight+1 and each step exactly +1. A gap means a
	// height was skipped (an honest block's height is its parent's +1, so an honest
	// path always passes; a malformed cert/linkage fails).
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	for i := range steps {
		want := led.height + 1 + uint64(i)
		if steps[i].height != want {
			return nil, fmt.Errorf("%w: path height %d at position %d, want %d (gap)",
				ErrNonMonotonicFinalizedHeight, steps[i].height, i, want)
		}
	}
	return steps, nil
}

// losingSubtrees returns every tracked block on a LOSING sibling subtree of keepID:
// the other children of parentID (siblings of keepID) plus all their descendants.
// This is avalanchego rejectTransitively's reachable set.
func losingSubtrees(keepID, parentID ids.ID, dag Ancestry) []ids.ID {
	var queue []ids.ID
	for _, id := range dag.Children(parentID) {
		if id != keepID {
			queue = append(queue, id)
		}
	}
	if len(queue) == 0 {
		return nil
	}
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
		for _, cid := range dag.Children(id) {
			if !seen[cid] {
				queue = append(queue, cid)
			}
		}
	}
	return out
}
