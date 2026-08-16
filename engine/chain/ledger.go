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
// This mirrors the upstream linear-chain consensus's topological layer, with finality's
// trigger swapped from β-consecutive-polls to a quorum CERT: the preference tree +
// poll stays mutable and sibling-tolerant (topological.go here), while the
// committed-prefix advance — β-driven upstream — is this pure, cert-driven fold.
package chain

import (
	"errors"
	"fmt"

	"github.com/luxfi/ids"
)

// The finality safety invariants — the vocabulary of the fold. Two are genuine
// α-of-K-cert properties the pure upstream engine does not need (its certs are β-witnessed, not
// external/attacker-chosen-Round): a SECOND valid cert for a DIFFERENT block at an
// already-decided height is equivocation evidence, never a silent second VM.Accept;
// and a cert for a block that does NOT descend from the finalized frontier conflicts
// with finalized history and is refused. The single-non-branching-chain property is
// achieved not by refusing siblings at admission, but by REORG
// (prune the losing sibling subtree when the cert selects the winner).
var (
	// ErrHeightAlreadyFinalized: a DIFFERENT block is already finalized at the target
	// height — two valid α-certs at one height across different rounds. The first
	// finalizes; the second is refused and IS equivocation evidence. A genuine α-of-K
	// safety property the pure upstream engine does not need.
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

// AncestorNotTracked is the TYPED form of ErrAncestorNotTracked. It carries the SPECIFIC
// untracked ancestor the finalize path needs fetched, so a catch-up handler can target the
// fetch at exactly that block — the plain sentinel named the id only inside a formatted string,
// which is why the receive-side cert handler could log the defer but never issue the fetch (the
// mainnet behind-node self-heal gap). errors.Is(err, ErrAncestorNotTracked) still holds via
// Unwrap, so every existing classifier is unchanged; self-heal handlers use errors.As to recover
// Missing and issue exactly one RequestAncestors for it.
type AncestorNotTracked struct {
	Missing ids.ID // the untracked ancestor whose absence blocks finalize — the fetch target
	Target  ids.ID // the certified block whose path to the finalized tip is blocked
}

func (e *AncestorNotTracked) Error() string {
	return fmt.Sprintf("%s: ancestor %s of %s missing", ErrAncestorNotTracked.Error(), e.Missing, e.Target)
}

// Unwrap makes errors.Is(err, ErrAncestorNotTracked) hold, so the sentinel-based classifiers
// (topology.go, tryAccept, tests) are byte-for-byte unaffected by the typed carrier.
func (e *AncestorNotTracked) Unwrap() error { return ErrAncestorNotTracked }

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
	// Parent returns id's OUTER parent, id's OWN height, id's CANONICAL execution
	// commitment, and the CANONICAL commitment of id's PARENT; ok is false if id is
	// untracked. The canonical id is what the per-height equivocation index records for
	// an intermediate catch-up-path block. parentCanonical is carried so the ancestry
	// walk can collapse the NEXT sibling wrapper by inner identity (see WrapperByCanonical).
	Parent(id ids.ID) (parent ids.ID, height uint64, canonical, parentCanonical ids.ID, ok bool)
	// Children returns the ids of every tracked block whose parent is id.
	Children(id ids.ID) []ids.ID
	// WrapperByCanonical resolves an inner execution commitment at a height to ANY
	// locally-tracked OUTER wrapper of it (any alias). The ancestry walk uses it to stand
	// a local wrapper in for the exact outer envelope a cert named — collapsing sibling
	// wrappers (aliases, not forks) the way every other finality path does. It NEVER
	// invents an execution the node does not hold: a miss (ok=false) preserves the
	// fail-closed behind-node defer (ErrAncestorNotTracked, fetch and retry).
	WrapperByCanonical(canonical ids.ID, height uint64) (id ids.ID, ok bool)
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
//
// ParentCanonical is the inner commitment of Parent (from the signed
// VotePosition.ParentCanonicalID). It lets the ancestry walk collapse a
// canonical-equivalent SIBLING WRAPPER in place of the exact outer envelope Parent
// names — the fix for the intermediate-ancestor livelock (a cert whose ancestry
// threads a wrapper the node holds under a DIFFERENT envelope must still finalize).
// Empty on a bare (non-wrapped) chain or the genesis finalize; the walk then stays
// outer-id only, byte-for-byte as before.
type Cert struct {
	Block           ids.ID
	Parent          ids.ID
	ParentCanonical ids.ID
	Height          uint64
	Canonical       ids.ID
}

// Plan is what Finalize decides and the engine applies to the VM and DAG. It mirrors
// The accept/reject split:
//
//   - Accept: the path from the OLD finalized tip up to the certified block, in
//     ASCENDING height order — acceptPreferredChild along a path (usually one block,
//     more on a catch-up jump).
//   - Reject: every block on a LOSING sibling subtree — a sibling of a path block plus
//     all its descendants — rejectTransitively.
type Plan struct {
	Accept []ids.ID
	// AcceptHeights is the height of each Accept entry, in the same order. The fold knows
	// every step's height; the engine's accept loop otherwise recovers it from
	// pendingBlocks and gets 0 for a block that is no longer tracked — which is exactly a
	// gap block. Carrying it here lets the recovery index record the true height.
	AcceptHeights []uint64
	Reject        []ids.ID
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

	// First CERT on a fresh (unset) ledger seeds certified history. When a recovery hint
	// is present — the VM's applied head, imported by SyncState — the cert must extend a
	// TRACKED chain up FROM that head, the same contiguous-ancestry proof the certified
	// path below demands above the certified tip. Walking from the hint is what stops a
	// fresh ledger from seeding blind at the cert's own height and vaulting the finalized
	// height over blocks the VM never executed (the post-restart wedge): an unheld
	// ancestry fails closed (AncestorNotTracked → a behind-node DEFER, fetch the gap),
	// while a tracked run folds and EXECUTES every height from the applied head up. With
	// no hint (a genuine genesis, no applied head to anchor to) or a cert at/below the
	// hint (the applied head's own first cert, or a straggler), seed the cert directly —
	// even for a different canonical id than the hint guessed, the hint never blocks it.
	// At or below the hint is the safe direction: the certified frontier sits under what
	// the VM has already applied, which is where it belongs. The dangerous direction is
	// above, and that is the walk this arm exists for.
	if !led.set {
		if led.hasHint && cert.Height > led.hintHeight {
			floor := FinalityLedger{tip: led.hint, height: led.hintHeight, set: true}
			path, err := pathFromTip(floor, cert, dag)
			if err != nil {
				return led, Plan{}, err
			}
			next, plan := foldPath(FinalityLedger{set: true, byHeight: map[uint64]finalizedEntry{}}, path, dag)
			return next, plan, nil
		}
		return seedLedger(cert.Block, canonical, cert.Height), Plan{Accept: []ids.ID{cert.Block}, AcceptHeights: []uint64{cert.Height}}, nil
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
	// Fold the walked path into a COPY of the ledger (never mutate the input map).
	next, plan := foldPath(led.clone(), path, dag)
	return next, plan, nil
}

// foldPath folds a walked, ascending-height path into `next` — the ONE fold body shared
// by both seed shapes: a clone of the live ledger when extending certified history above
// the certified tip, or a fresh set ledger when seeding from a recovery hint (walking up
// from the VM's applied head). It Accepts the path ascending, Rejects every losing-sibling
// subtree along it, and records each step's CANONICAL commitment (the authoritative
// finality id) in byHeight, bounded to the equivocation window on return.
func foldPath(next FinalityLedger, path []step, dag Ancestry) (FinalityLedger, Plan) {
	var plan Plan
	for _, s := range path {
		plan.Reject = append(plan.Reject, losingSubtrees(s.id, s.parentID, dag)...)
		next.byHeight[s.height] = finalizedEntry{canonical: s.canonical, envelope: s.id}
		next.tip = s.id
		next.canonical = s.canonical
		next.height = s.height
		plan.Accept = append(plan.Accept, s.id)
		plan.AcceptHeights = append(plan.AcceptHeights, s.height)
	}
	next.pruneBelowWindow() // keep byHeight O(window), not O(chain height)
	return next, plan
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
	// parentCanonHint is the INNER commitment the NEXT ancestor must equal, carried down
	// the walk so a canonical-equivalent SIBLING WRAPPER can stand in for the exact outer
	// envelope the cert named (the intermediate-ancestor livelock fix; mainnet-644 sibling
	// class). Seeded from the cert's ParentCanonicalID, else from the certified block's own
	// recorded parent-canonical. Empty (a bare, non-wrapped chain) ⇒ the walk stays
	// outer-id only, byte-for-byte as before.
	parentCanonHint := cert.ParentCanonical
	if parentCanonHint == ids.Empty {
		if _, _, _, pc, ok := dag.Parent(cert.Block); ok {
			parentCanonHint = pc
		}
	}
	for cur != led.tip {
		parent, curHeight, curCanonical, curParentCanon, ok := dag.Parent(cur)
		if !ok {
			// The exact outer envelope is untracked. Before the fail-closed behind-node
			// DEFER, try to stand a LOCAL wrapper of the SAME inner block (a
			// canonical-equivalent alias) in its place — collapsing sibling wrappers the
			// way convergedWinnerAtHeightLocked and finalizeLocalAliasFromVerifiedCert
			// already do everywhere else. A genuinely-unheld inner block (no wrapper at
			// all) still falls through to the DEFER, so safety is unchanged.
			if parentCanonHint != ids.Empty {
				if localID, found := dag.WrapperByCanonical(parentCanonHint, childHeight-1); found {
					// Rebase the child's parent link onto the LOCAL wrapper so BOTH the
					// accept path and the reject-subtree walk (losingSubtrees) key on the
					// wrapper the VM actually holds, never the unheld outer alias.
					steps[len(steps)-1].parentID = localID
					cur = localID
					// The rebase moves cur, so the loop condition must be re-tested
					// before the below-frontier check below. When the local wrapper IS
					// the finalized tip, the walk has arrived at the frontier and the
					// path is complete; falling through would reach `curHeight <=
					// led.height` with cur == led.tip and refuse a cert that extends
					// the tip exactly.
					if cur == led.tip {
						break
					}
					parent, curHeight, curCanonical, curParentCanon, ok = dag.Parent(cur)
				}
			}
			if !ok {
				return nil, &AncestorNotTracked{Missing: cur, Target: cert.Block}
			}
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
		parentCanonHint = curParentCanon
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
// This is the transitively-rejected reachable set.
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
