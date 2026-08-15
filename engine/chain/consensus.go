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
	// import must not silently regress local finalized height (which would let a
	// shorter imported chain un-finalize blocks the node already finalized).
	ErrSyncStateRegression = errors.New("chain: SyncState refused — import height is below the already-finalized height (would regress finalized history)")

	// ErrSyncStateEmptyWithHeight is returned by SyncState when an empty import head
	// (lastAcceptedID == ids.Empty) is paired with a positive height. An empty head is
	// the genesis/teardown reset, meaningful only at height 0; above it the pair is
	// contradictory. Allowing it would set finalizedTip=Empty while leaving
	// finalizedHeight/finalizedByHeight stale (the non-empty seed branch is skipped)
	// and prune blocks below the positive height, so the tip and the height would
	// describe different chains. SyncState refuses it fail-closed before mutating any
	// state.
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

	// recall reads back a block this process never saw but the VM has accepted, so
	// the finality walk can cross the seam a restart leaves between the durable
	// finalized tip and the memory-only tree above it. nil confines the walk to
	// what this process has seen. Set once at construction; see blocksAncestry.at.
	recall func(ids.ID) (*Block, bool)

	// Consensus tracking
	bootstrapped bool

	// ledger is the committed NOVA finality VALUE — the append-only prefix of accepted
	// history (ledger.go), the one source of truth for which block is LOCALLY ACCEPTED at
	// each height. It is only ever REPLACED WHOLE, never poked field-by-field, so there is
	// no markFinalized to call. Exactly two paths replace it: the cert fold
	// (applyCertLocked, the production path — now driven by the NOVA majority cert) and
	// SyncState (the out-of-band import reconcile). A single non-branching accepted chain is
	// the OUTPUT of the cert's reorg — pruning the losing sibling subtree — never an
	// admission refusal. This is the ACCEPT (local-execution) frontier; export finality is
	// the separate, trailing quasar frontier below.
	ledger FinalityLedger

	// quasar is the EXPORT (Quasar) frontier — the highest block that has reached a
	// ⅔-by-stake certificate. It TRAILS the Nova ledger: a ⅔-stake accept quorum is a
	// superset of a bare majority, so every Quasar block is already a Nova block, and this
	// frontier is always a prefix of the certified Nova chain. It advances ONLY via
	// PromoteQuasar, which requires the promoted block to MATCH the Nova ledger's accepted
	// canonical at that height — so the "no two conflicting Quasar certs" invariant is
	// inherited from the Nova ledger's single-canonical-per-height gate. GetQuasarTip reads
	// it; it is the ONLY tip bridges / DEX settlement / cross-chain messages may consume.
	quasar quasarFrontier

	// preference is the preliminary BUILD tip used BEFORE the first finalize — the
	// DECOMPLECTED preference concern (preference is kept separate from the
	// committed `lastAcceptedID`). Once the ledger is set the finalized tip wins and
	// this is unused; ForcePreference seeds it, Preference() reads it only pre-finalize.
	preference ids.ID
}

// quasarFrontier is the EXPORT (Quasar) frontier value: the highest block that has
// reached a ⅔-by-stake certificate. It is a pure {height, canonical, envelope} tip, NOT a
// full ledger — it needs no ancestry fold of its own because every Quasar block is a Nova
// block and the Nova ledger (which PromoteQuasar validates against) already proved the
// single non-branching accepted chain. The "no two conflicting Quasar certs" invariant is
// therefore inherited: PromoteQuasar only ever moves this frontier to a canonical the Nova
// ledger already accepted at that height.
type quasarFrontier struct {
	height    uint64 // highest export-final height
	canonical ids.ID // inner execution commitment of the Quasar tip (ids.Empty until the first cert)
	envelope  ids.ID // outer transport id of the Quasar tip (serving / diagnostics)
	set       bool   // false until the first ⅔-stake cert promotes
}

// SetRecall gives the finality walk a durable reader for blocks this process never
// saw. The engine wires it to the VM, whose accepted chain outlives any one process.
// Without it the walk sees only what this process has gossiped, which is empty above
// the finalized tip on every restart.
func (c *ChainConsensus) SetRecall(f func(ids.ID) (*Block, bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recall = f
}

// NewChainConsensus creates a chain consensus engine.
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

// FinalizeBranch finalizes a block whose CANONICAL commitment equals its OUTER id —
// the non-proposervm-wrapped convenience (canonical == outer). It is the call shape
// for the safety tests and any in-process chain where there is no inner/outer split.
// A block that DOES carry a distinct inner commitment must use ApplyCert with an
// explicit Cert.Canonical (the production cert path and bootstrap do). parentID may
// be ids.Empty only for the genesis / first finalize.
func (c *ChainConsensus) FinalizeBranch(target ids.ID, height uint64, parentID ids.ID) (Plan, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Canonical left ids.Empty ⇒ the fold treats canonical == target (outer == inner).
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
	c.ledger = led    // the only way finality advances — one value assignment after a pure fold
	c.applyPlan(plan) // DAG-side effects only (accepted/rejected/tips); never finality
	return plan, nil
}

// applyPlan applies the fold's plan to the live DAG: mark the Accept path accepted and
// drop it from the build tips; mark the Reject (losing-sibling) subtrees rejected and
// remove them from the live DAG/tips — accept-preferred-child plus transitive reject
// on the DAG side. It never touches c.ledger; finality already advanced in the fold.
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
		"finalized_tip": c.ledger.Tip().String(),
	}
}

// SyncState synchronizes consensus state with external state (e.g., after RLP import).
// This updates the finalizedTip and marks the consensus as bootstrapped so that
// new blocks can be built on top of the imported chain.
//
// This is called by the syncer after admin_importChain to reconcile consensus
// state with the EVM state database.
//
// Monotonicity is re-asserted here, defence in depth. SyncState bypasses
// FinalizeBranch by design — an import is an out-of-band reconcile with the VM's
// last-accepted head, not an α-of-K finalize, so it legitimately seeds
// finalizedTip/Height directly. Bypassing the finalize path does not mean bypassing
// the monotonic invariant: a backward import (height below the already-finalized
// height) is refused with ErrSyncStateRegression and leaves all finalized state
// untouched. Otherwise a shorter/older imported chain could silently regress
// finalizedHeight and un-finalize blocks the node already finalized, re-opening the
// fork window the per-height guard closes. A re-import at the same height with the
// same block is idempotent; a forward import advances. The only backward move
// allowed is the genesis/empty reset (lastAcceptedID==Empty), valid only at height 0
// — an empty head above it is contradictory and refused with
// ErrSyncStateEmptyWithHeight, since it would leave finalizedTip and
// finalizedHeight describing different chains and prune live blocks.
func (c *ChainConsensus) SyncState(lastAcceptedID ids.ID, height uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Refuse an empty import head paired with a positive height. An empty head is the
	// genesis/teardown reset — valid only at height 0. Above it, it would assign
	// finalizedTip=Empty while skipping the seed branch (so
	// finalizedHeight/finalizedByHeight stay stale) and prune blocks below the height,
	// leaving the tip and the height describing different chains. Fail closed before
	// any mutation. The sole caller, SyncStateFromVM, pairs Empty with height 0, so
	// this is unreachable today; it is guarded so a future caller cannot open the gap.
	if lastAcceptedID == ids.Empty && height > 0 {
		return fmt.Errorf("%w: refused empty head at height %d", ErrSyncStateEmptyWithHeight, height)
	}

	// Refuse a backward regression of CERTIFIED finalized history. Only enforced once
	// a cert has finalized and for a concrete (non-empty) import head; an empty reset
	// is a deliberate teardown, not a regression. A hint can never un-finalize a
	// QC-backed block: an import below the certified height is refused outright.
	if c.ledger.set && lastAcceptedID != ids.Empty && height < c.ledger.height {
		return fmt.Errorf("%w: import height %d < finalized height %d (block %s)",
			ErrSyncStateRegression, height, c.ledger.height, lastAcceptedID)
	}
	// There is deliberately no equal-height "different block ⇒ equivocation" check
	// here. An import is a non-authoritative hint, not a finality seed, so there is
	// nothing for it to equivocate against: equivocation is decided exclusively by the
	// cert fold over canonical commitments (Finalize). Were a local seed to enter
	// finality, a locally-chosen sibling could manufacture a finalized entry that a
	// peer's legitimate cert then reads as a conflict — refusing the honest chain
	// forever, since the local entry never moves.

	// Apply the import as a recovery hint — a build anchor only, never finality.
	// vm.LastAccepted is the local node's boot/import snapshot; after a crash it can
	// name an uncertified local block whose inner block the network actually finalized
	// under a different envelope. As a hint, Height() stays (0,false) until a real
	// cert, byHeight stays empty, and no cert can equivocate against it.
	if lastAcceptedID != ids.Empty {
		// Preserve any CERTIFIED history (whole-value copy) and update only the hint
		// fields — a hint must never wipe a QC-backed frontier. When no certified
		// history exists, this is just the hint anchor.
		c.ledger = c.ledger.withHint(lastAcceptedID, height)
	} else {
		// genesis/teardown reset (empty head at height 0): reset finality to a CLEAN
		// genesis VALUE — no certified state, no hint, no per-height record. A whole-
		// value reset cannot desync: the next cert simply seeds finality afresh.
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

// GetFinalizedTip returns the BUILD ANCHOR — the certified finalized tip (outer
// id) once a cert has finalized, else the recovery hint (vm.LastAccepted). This is
// the "what is the VM building on" view used by health checks and preference; it is
// NOT the finality-authority height (that is GetFinalizedHeight, certified-only).
func (c *ChainConsensus) GetFinalizedTip() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if anchor, ok := c.ledger.BuildAnchor(); ok {
		return anchor
	}
	return ids.Empty
}

// GetQuasarTip returns the canonical execution commitment of the highest EXPORT-FINAL
// (Quasar, ⅔-by-stake) block — the ONLY tip a bridge / DEX settlement / cross-chain
// message / validator-set transition may treat as deterministically final. It is
// ids.Empty until the first ⅔-stake certificate forms, and it never returns the Nova
// (accept) tip: a Nova block is locally executed but a lone equivocator can fork a bare
// majority, so it is reorgable and non-exportable until it reaches Quasar. Gating export
// on this frontier rather than on the accept frontier is the two-tier ladder's whole point.
func (c *ChainConsensus) GetQuasarTip() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quasar.canonical
}

// GetNovaTip returns the canonical execution commitment of the highest LOCALLY ACCEPTED
// (Nova) block — the accepted/execution head that drove VM.Accept. It is the correct tip
// for local build/preference and for equivocation evidence, but it authorizes local
// execution only: it is reorgable in theory until it reaches Quasar and must not be
// exported (use GetQuasarTip for any bridge / settlement / cross-chain decision). The
// tier is in the name so no caller can mistake accept for export.
func (c *ChainConsensus) GetNovaTip() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ledger.CanonicalTip()
}

// QuasarHeight returns the highest EXPORT-FINAL (Quasar) height and whether any ⅔-stake
// cert has formed. GetFinalizedHeight() (the Nova/accept height) is always ≥ this; a
// positive gap means the chain is PRODUCING Nova but not yet CERTIFYING Quasar — the
// degraded mode (production continues, certification pauses) the two-tier split enables.
func (c *ChainConsensus) QuasarHeight() (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quasar.height, c.quasar.set
}

// PromoteQuasar advances the EXPORT (Quasar) frontier to a block that has reached a
// ⅔-by-stake certificate (the trailing attestation sidecar assembled it, strictly AFTER
// the Nova accept). `cert` names the export cert's subject (height + canonical + outer
// envelope). It advances ONLY forward and ONLY to a block the Nova ledger ALREADY accepted
// at that height with the SAME canonical, so a Quasar cert can never certify a block Nova
// did not accept:
//
//   - advanced=true  → the frontier moved to a new (strictly higher) height.
//   - err != nil (ErrHeightAlreadyFinalized) → the export cert's canonical conflicts with the
//     canonical Nova already accepted at that height. That is a provable ⅔-stake
//     equivocation — it needs >⅓ stake to double-sign, beyond the f<⅓ assumption — so the
//     caller halts fail-closed and surfaces it as slashable evidence. The frontier does not
//     advance on error.
//   - advanced=false, err=nil → idempotent/no-op: the height is at/below the current Quasar
//     frontier (a duplicate/late cert), or the Nova ledger has not yet (or no longer, past the
//     equivocation window) recorded this height — in which case it is an ancestor of the Nova
//     tip and already implicitly final, or the promotion will retry once Nova catches up.
//
// This is the SOLE writer of the Quasar frontier; the "no two conflicting Quasar certs"
// invariant reduces to the Nova ledger's single-canonical-per-height gate that this checks
// against (c.ledger.At), which itself fail-closes on a conflicting Nova accept.
// SyncQuasarFrontier conservatively (re)seeds the EXPORT (Quasar) frontier from a DURABLE source
// — the VM's persisted export-final block on boot (symmetric with how SyncState seeds the Nova
// ledger's build anchor from vm.LastAccepted). The in-memory quasarFrontier is otherwise reset on
// restart, so GetQuasarTip/QuasarHeight would regress to (Empty, 0) until a fresh cert re-forms;
// this holds the frontier at the last durably-exported height so a consumer never observes a
// backward jump. It ONLY ever moves the frontier FORWARD (advance-only, never regress): a seed at
// or below the current height is ignored. The durable source (the VM's persisted Quasar height)
// only ever advances, so this can never over-claim. canonical may be ids.Empty (height-only seed);
// PromoteQuasar refines it with the real canonical as certs re-form at runtime.
func (c *ChainConsensus) SyncQuasarFrontier(canonical ids.ID, height uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.quasar.set && height <= c.quasar.height {
		return // advance-only: never regress the export frontier
	}
	c.quasar = quasarFrontier{height: height, canonical: canonical, envelope: canonical, set: true}
}

func (c *ChainConsensus) PromoteQuasar(cert Cert) (advanced bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	canonical := cert.Canonical
	if canonical == ids.Empty {
		canonical = cert.Block
	}
	novaCanonical, have := c.ledger.At(cert.Height)
	if !have {
		// Nova has not (yet / no longer within the equivocation window) recorded this height.
		// Do not advance: either Nova will catch up and the promotion retries, or the height
		// aged out and is an ancestor of the Nova tip (already implicitly final). Not an error.
		return false, nil
	}
	if novaCanonical != canonical {
		// A ⅔-stake export cert for a canonical the Nova ledger did not accept at this height
		// breaks the two-tier invariant. Fail closed; the caller halts and slashes.
		return false, fmt.Errorf("%w: quasar export cert canonical %s conflicts with nova-accepted canonical %s at height %d",
			ErrHeightAlreadyFinalized, canonical, novaCanonical, cert.Height)
	}
	if c.quasar.set && cert.Height <= c.quasar.height {
		return false, nil // idempotent / monotonic: only ever advance forward
	}
	c.quasar = quasarFrontier{height: cert.Height, canonical: canonical, envelope: cert.Block, set: true}
	return true, nil
}

// GetFinalizedHeight returns the current finalized height and whether any block
// has been finalized yet. The engine uses this to gate an incoming cert on height
// — a cert at or below the last-finalized height is rejected — before running the
// cert through the per-height guard.
func (c *ChainConsensus) GetFinalizedHeight() (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ledger.Height()
}

// GetNovaAcceptedFloor returns a lower bound on the height this node has LOCALLY ACCEPTED
// (Nova) through: max(Nova ledger height, recovery-hint height). Nova acceptance is
// explicitly REORGABLE (see Finality.AuthorizesLocalExecution — "crash-safe but reorgable,
// so a caller acting here must be prepared to reorg"), so this is a BUILD / CATCH-UP /
// REPORTING bound only.
//
// Not a signing floor. Welding a validator's permanent sign-refusal floor to a reorgable
// height strands that validator: heights accepted on a bare Nova majority that was never
// ⅔-certified could never be signed at again, including the rebuild every honest peer
// agreed on. "Refusing more is always fail-safe" is false on the signing path — over-refusal
// there is not caution but a liveness fault that no restart, resync or peer can clear. Use
// GetQuasarSigningFloor for anything that permanently closes a height.
//
// Read-only: its value never enters byHeight, the equivocation index, or the authoritative
// ledger.Height(). It reads the hint's height only, never the hint's (untrustworthy) identity.
func (c *ChainConsensus) GetNovaAcceptedFloor() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	floor := uint64(0)
	if h, ok := c.ledger.Height(); ok {
		floor = h
	}
	if c.ledger.hasHint && c.ledger.hintHeight > floor {
		floor = c.ledger.hintHeight
	}
	return floor
}

// GetQuasarSigningFloor returns the ONLY height a validator may permanently refuse to sign at
// or below: the EXPORT (Quasar) frontier — a height carrying a ⅔-by-stake certificate.
//
// This is the one durable safety floor because Quasar is the only tier whose quorum
// intersection exceeds f. For n=5, f=1:
//
//	Nova   ⌊n/2⌋+1 = 3  → 2α−n = 1  NOT > f  ⇒ two conflicting Nova histories can exist
//	Quasar strict ⅔ = 4  → 2α−n = 3      > f  ⇒ intersection guaranteed, irreversible
//
// So a Quasar height is agreed by every honest peer: closing it forever cannot strand this
// node relative to the fleet. A Nova height is not, and closing it can — permanently.
//
// 0 until the first ⅔-stake cert forms. A zero floor is SAFE, not permissive: anti-equivocation
// is carried by the per-height vote BINDINGS (committedSlot / the vote-guard file), which are
// retained for every height above this floor precisely so the window between the Quasar frontier
// and the Nova head stays individually guarded rather than bluntly closed.
func (c *ChainConsensus) GetQuasarSigningFloor() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.quasar.set {
		return 0
	}
	return c.quasar.height
}

// FinalizedBlockAtHeight returns the CANONICAL execution commitment finalized at
// the given height, if any. Used to produce equivocation evidence and to recognise
// duplicates: a second cert at an already-finalized height is equivocation ONLY if
// its canonical id differs from this one (a different envelope wrapping the same
// canonical id is a harmless alias).
func (c *ChainConsensus) FinalizedBlockAtHeight(height uint64) (ids.ID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ledger.At(height)
}

// IsFinalizedAt reports whether `id` is the block this ledger has already finalized at
// `height` — matching either the canonical commitment or the outer transport envelope,
// since a differing envelope wrapping the same canonical id is an alias, not a rival.
//
// This is the whole authority behind replaying a finalized block onto a VM that has not
// applied it. The ledger, not the caller and not the sender, decides which block belongs
// at a height; a block that does not match is refused. A height the ledger does not know
// answers false, so an unknown height can never be replayed.
func (c *ChainConsensus) IsFinalizedAt(height uint64, id ids.ID) bool {
	if id == ids.Empty {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if canonical, ok := c.ledger.At(height); ok && canonical == id {
		return true
	}
	envelope, ok := c.ledger.EnvelopeAt(height)
	return ok && envelope == id
}

// K returns the consensus sample size (the K of α-of-K). Used by the engine to
// select the single-validator (K==1) force path vs. the multi-validator
// cert-witnessed path, and by the fail-closed DEX guard.
func (c *ChainConsensus) K() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.k
}

// Reclamp updates the committee sample size k and quorum alpha to track a changed live
// validator set. The committee is otherwise fixed at NewChainConsensus (the construction-time
// bftCommittee clamp); without a re-clamp a chain that launched single-validator would keep k
// stuck at 1, and after it added validators via its PoS staking contract the lone validator
// would still take the K==1 path (synthesize a 1-of-1 cert / accept a single-signer stake cert
// at alpha=1) and finalize unilaterally — a cross-node fork against validators 2..N who require
// a real k=N quorum. That is the failure mode on the path from one validator to many. beta (the
// confidence threshold) is committee-size-independent and is left unchanged. A degenerate update
// (k<1, alpha<1, or alpha>k) is ignored so a mis-derived call can never open a sub-quorum. The
// engine only ever calls this to move the committee up toward the live count
// (reclampCommitteeLocked, up-only), so a transient low read can never shrink the quorum here.
func (c *ChainConsensus) Reclamp(k, alpha int) {
	if k < 1 || alpha < 1 || alpha > k {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.k = k
	c.alpha = alpha
}

// Alpha returns the acceptance quorum threshold (α of α-of-K). A block is
// accepted once acceptVotes >= Alpha; a QuorumCert proves exactly this many
// distinct signed accepts.
func (c *ChainConsensus) Alpha() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.alpha
}
