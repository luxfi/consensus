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

// finalizedEntry is the CERTIFIED record at one finalized height: the canonical
// execution commitment (the authoritative finality identity) and the outer
// envelope id (transport, retained for serving/diagnostics). Equivocation and
// idempotency are decided on `canonical` ONLY — the envelope is non-authoritative.
type finalizedEntry struct {
	canonical ids.ID // inner execution commitment — THE finalized identity at this height
	envelope  ids.ID // outer/proposervm id — transport cache key (non-authoritative)
}

// FinalityLedger is the committed, append-only prefix of finalized history — an
// immutable VALUE. It is never mutated in place; Finalize returns a NEW one.
//
// THE CERTIFIED FRONTIER vs THE RECOVERY HINT (the incident-1082814 durable rule).
// The ledger separates two notions that the pre-fix code fatally conflated:
//
//   - The CERTIFIED frontier (tip/canonical/height/set + byHeight) advances ONLY
//     by folding a verified quorum cert (or the bootstrap frontier-trust path).
//     byHeight indexes the CANONICAL commitment finalized at each height; a second
//     cert for a DIFFERENT canonical id at an already-certified height is the ONLY
//     thing that is equivocation. Two outer envelopes wrapping the SAME canonical
//     block are duplicates, never a fork.
//   - The recovery HINT (hint/hintHeight/hasHint) is seeded from vm.LastAccepted on
//     boot/import. It is NON-AUTHORITATIVE: it never registers as finalized height,
//     never enters byHeight, and can NEVER trigger equivocation. It is a build
//     anchor only — "where to build next" until a real cert arrives. A cert at the
//     hint's height with a different canonical id simply seeds certified history;
//     the wrong local guess is silently superseded by network truth.
//
// All fields are unexported and read-only after construction; the projections
// (CertifiedTip/Height/At/BuildAnchor) are the only outside view.
type FinalityLedger struct {
	// Certified frontier — advanced ONLY by a verified QC fold (or bootstrap
	// frontier-trust). `tip` is the OUTER envelope id of the certified head (the
	// join point the ancestry walk seeks in the transport DAG); `canonical` is its
	// canonical commitment.
	tip       ids.ID
	canonical ids.ID
	height    uint64
	set       bool // false until the first CERT (or bootstrap) finalizes
	byHeight  map[uint64]finalizedEntry

	// Recovery hint — from vm.LastAccepted. Non-authoritative build anchor only.
	hint       ids.ID // outer id to build on until a cert finalizes
	hintHeight uint64
	hasHint    bool
}

// Tip is the OUTER envelope id of the certified head (ids.Empty before the first
// cert). This is the true finalized tip — backed by a QC. The recovery hint is NOT
// returned here (use BuildAnchor for the build view).
func (l FinalityLedger) Tip() ids.ID { return l.tip }

// CanonicalTip is the canonical execution commitment of the certified head
// (ids.Empty before the first cert).
func (l FinalityLedger) CanonicalTip() ids.ID { return l.canonical }

// Height returns the CERTIFIED finalized height and whether any cert has
// finalized yet. The recovery hint does NOT count: a hint-only ledger returns
// (0,false), so the finality height gate and equivocation index see no finalized
// height until a real cert exists.
func (l FinalityLedger) Height() (uint64, bool) { return l.height, l.set }

// At returns the CANONICAL commitment finalized at height, if a CERTIFIED entry
// exists there (equivocation evidence lookup). Hints are never returned.
func (l FinalityLedger) At(height uint64) (ids.ID, bool) {
	e, ok := l.byHeight[height]
	if !ok {
		return ids.Empty, false
	}
	return e.canonical, true
}

// EnvelopeAt returns the outer transport id finalized at height (for serving /
// diagnostics), if a certified entry exists there.
func (l FinalityLedger) EnvelopeAt(height uint64) (ids.ID, bool) {
	e, ok := l.byHeight[height]
	if !ok {
		return ids.Empty, false
	}
	return e.envelope, true
}

// BuildAnchor returns the outer id the VM should build/prefer on, and whether any
// anchor exists. It is the HIGHER of {certified tip, recovery hint}: the certified
// tip normally, but a forward recovery hint (vm.LastAccepted above the certified
// height — e.g. a state-sync import) wins so the VM builds where the node actually
// has state. This is a BUILD concern (transport), strictly decoupled from the
// finality Height() — advancing the build anchor past the certified tip touches no
// finality decision (a hint can never finalize), so it affects only liveness.
func (l FinalityLedger) BuildAnchor() (ids.ID, bool) {
	switch {
	case l.set && l.hasHint:
		if l.hintHeight > l.height {
			return l.hint, true
		}
		return l.tip, true
	case l.set:
		return l.tip, true
	case l.hasHint:
		return l.hint, true
	default:
		return ids.Empty, false
	}
}

// Ancestry is the READ-ONLY view of the preference DAG the fold needs. The
// preference layer (topological.go) implements it over the live block tree.
// Finalize NEVER mutates the DAG — it reads ancestry to prove the certified path and
// to collect the losing-sibling subtrees to prune.
type Ancestry interface {
	// Parent returns id's OUTER parent, id's OWN height, and id's CANONICAL execution
	// commitment; ok is false if id is untracked. The canonical id is what the
	// per-height equivocation index records for an intermediate catch-up-path block.
	Parent(id ids.ID) (parent ids.ID, height uint64, canonical ids.ID, ok bool)
	// Children returns the ids of every tracked block whose parent is id.
	Children(id ids.ID) []ids.ID
}

// Cert is the minimal finality subject — the block a quorum certificate selects,
// decoupled from the wire VerifiedQuorumCert. Finality is "fold a Cert into the
// ledger".
//
// Block/Parent are the OUTER transport ids (used to walk the transport DAG and as
// the VM accept target); Canonical is the inner execution commitment — the
// AUTHORITATIVE finality identity the fold keys equivocation/idempotency on. For a
// non-wrapped block Canonical == Block. Parent is ids.Empty only for the genesis /
// first finalize.
type Cert struct {
	Block     ids.ID
	Parent    ids.ID
	Height    uint64
	Canonical ids.ID
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
// It enforces:
//
//	(a) ONE CANONICAL commitment finalized per height. Keyed on the inner execution
//	    commitment (cert.Canonical), NOT the outer envelope: the SAME canonical id
//	    already finalized here is an idempotent no-op regardless of which envelope the
//	    cert names (a duplicate alias — the incident-1082814 case); a DIFFERENT
//	    canonical id at an already-finalized height is equivocation →
//	    ErrHeightAlreadyFinalized.
//	(b) the certified block must DESCEND from the finalized tip via a tracked,
//	    contiguous ancestry: a non-tip ancestor at/below the finalized height →
//	    ErrConflictsWithFinalizedBranch; an untracked ancestor → ErrAncestorNotTracked
//	    (DEFER, behind); a height gap / malformed linkage → ErrNonMonotonicFinalizedHeight.
func Finalize(led FinalityLedger, cert Cert, dag Ancestry) (FinalityLedger, Plan, error) {
	// The AUTHORITATIVE finality identity is the canonical commitment, never the
	// outer envelope. A cert that omits it (canonical == Empty) is degenerate; fall
	// back to the outer id so a non-wrapped chain (outer == canonical) is unchanged.
	canonical := cert.Canonical
	if canonical == ids.Empty {
		canonical = cert.Block
	}

	// (a) idempotent / equivocation at the target height — keyed on the CANONICAL
	// commitment. byHeight holds CERTIFIED entries only (hints never enter it), so a
	// hit here is always a prior QC-backed finalization.
	if existing, ok := led.byHeight[cert.Height]; ok {
		if existing.canonical == canonical {
			// SAME inner block already certified here — a no-op regardless of which
			// outer envelope this cert names (duplicate alias, NOT a fork).
			return led, Plan{}, nil
		}
		// A DIFFERENT canonical commitment is already CERTIFIED at this height: two
		// valid certs select different execution blocks at one height — the real fork.
		return led, Plan{}, fmt.Errorf("%w: height %d already finalized canonical %s (envelope %s), refused canonical %s (envelope %s)",
			ErrHeightAlreadyFinalized, cert.Height, existing.canonical, existing.envelope, canonical, cert.Block)
	}

	// First CERT finalize seeds certified history — no prior certified tip to
	// extend or reorg. The recovery hint (if any) is non-authoritative and is simply
	// superseded here: a cert at the hint's height, even for a different canonical id
	// than the hint guessed, finalizes cleanly (the hint never blocks a cert).
	if !led.set {
		return seedLedger(cert.Block, canonical, cert.Height), Plan{Accept: []ids.ID{cert.Block}}, nil
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
	// the path ascending, Reject every losing-sibling subtree along it. byHeight
	// records the CANONICAL commitment of each step (the authoritative finality id).
	next := led.clone()
	var plan Plan
	for _, s := range path {
		plan.Reject = append(plan.Reject, losingSubtrees(s.id, s.parentID, dag)...)
		next.byHeight[s.height] = finalizedEntry{canonical: s.canonical, envelope: s.id}
		next.tip = s.id
		next.canonical = s.canonical
		next.height = s.height
		plan.Accept = append(plan.Accept, s.id)
	}
	next.pruneBelowWindow() // keep byHeight (and the next clone) O(window), not O(chain height)
	return next, plan, nil
}

// seedLedger constructs the first CERTIFIED ledger value from the seed (outer
// envelope id, canonical commitment, height). Clears any recovery hint — certified
// history now dominates the build anchor.
func seedLedger(envelope, canonical ids.ID, height uint64) FinalityLedger {
	return FinalityLedger{
		tip:       envelope,
		canonical: canonical,
		height:    height,
		set:       true,
		byHeight:  map[uint64]finalizedEntry{height: {canonical: canonical, envelope: envelope}},
	}
}

// withHint returns a COPY of the ledger with the recovery-hint fields set to
// (envelope, height), PRESERVING any certified frontier. The hint is
// NON-AUTHORITATIVE: it sets no certified state (Height stays (0,false) until a
// cert; byHeight is untouched), so equivocation can never fire from it. It is a
// build anchor only. A hint must never wipe a QC-backed frontier — hence the copy
// rather than a fresh value.
func (l FinalityLedger) withHint(envelope ids.ID, height uint64) FinalityLedger {
	next := l.clone()
	next.hint = envelope
	next.hintHeight = height
	next.hasHint = true
	return next
}

// clone returns a deep copy of the ledger (a fresh byHeight map) so the fold never
// mutates the receiver value's map. One copy per Finalize. byHeight is bounded to
// equivocationWindow entries (pruneBelowWindow), so this copy is O(window) — constant
// cost at any chain height — never O(chain height).
func (l FinalityLedger) clone() FinalityLedger {
	bh := make(map[uint64]finalizedEntry, len(l.byHeight)+1)
	for h, e := range l.byHeight {
		bh[h] = e
	}
	return FinalityLedger{
		tip: l.tip, canonical: l.canonical, height: l.height, set: l.set, byHeight: bh,
		hint: l.hint, hintHeight: l.hintHeight, hasHint: l.hasHint,
	}
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
	id        ids.ID
	height    uint64
	parentID  ids.ID
	canonical ids.ID // canonical commitment of this step (recorded in byHeight)
}

// pathFromTip returns the contiguous ancestry finalizedTip → target in ASCENDING
// height order, by walking target's parent links through the DAG. Errors distinguish
// the three non-extending cases (conflict / behind / gap). Caller guarantees
// cert.Height > led.height and led.set. The top step's canonical comes from the
// cert; intermediate steps' canonical come from the tracked DAG.
func pathFromTip(led FinalityLedger, cert Cert, dag Ancestry) ([]step, error) {
	topCanonical := cert.Canonical
	if topCanonical == ids.Empty {
		topCanonical = cert.Block
	}
	steps := []step{{id: cert.Block, height: cert.Height, parentID: cert.Parent, canonical: topCanonical}}
	cur := cert.Parent
	childHeight := cert.Height
	for cur != led.tip {
		parent, curHeight, curCanonical, ok := dag.Parent(cur)
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
		stepCanonical := curCanonical
		if stepCanonical == ids.Empty {
			stepCanonical = cur
		}
		steps = append(steps, step{id: cur, height: curHeight, parentID: parent, canonical: stepCanonical})
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
