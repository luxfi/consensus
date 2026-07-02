// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// round_view.go — the round-scoped view-change core that makes α-of-K finality LIVE
// under competing siblings WITHOUT reintroducing double-finalization.
//
// THE PROBLEM IT FIXES (see competing_fork_deadlock_test.go): the pre-existing
// convergence bound this node's ONE irrevocable per-height cert signature on a
// settle timer + lowest-canonical guess. When gossip is asymmetric (a validator
// freshly dead → α with zero margin; a partition that outlasts the settle window),
// honest nodes bind that one signature to DIFFERENT siblings and the durable
// one-signature-per-height lock makes the split PERMANENT — no node may switch to
// converge. That is the 415→416 testnet freeze.
//
// THE FIX (Tendermint/HotStuff view-change, two-phase):
//   - PREVOTE (fluid, non-binding): each round a node prevotes the deterministic
//     winner (or its lock, per the rule below). Prevotes are FREE to change across
//     rounds, so a split re-converges — this is the liveness the irrevocable signature
//     destroyed.
//   - POL (proof-of-lock): α distinct prevotes for one value v at round r. Because a
//     node casts at most ONE prevote per (height,round), at most one value reaches α
//     per round (quorum intersection) → a POL is UNIQUE per round.
//   - PRECOMMIT (binding, the α-of-K cert signature): cast ONLY on a POL at the
//     current round, and the node LOCKS on that value+round. This is the irrevocable
//     signature — one per (height,round).
//   - LOCK / UNLOCK: a locked node prevotes its locked value, and may prevote a
//     conflicting value only if it has seen a POL for that value at a round STRICTLY
//     ABOVE its lock round (a fresher quorum justification). This is what preserves
//     safety across rounds.
//   - FINALIZE: α precommits for v at the SAME round r (the cert binds the round).
//
// SAFETY (no two committing certs at one height): a value v committed at round r means
// α nodes precommitted v@r, hence α nodes locked (r,v). Any conflicting v'≠v needs a
// POL(v',r'>r) to be precommitted, which needs α prevotes for v' at r'; but ≥2α−n of
// the locked-on-v nodes are in every α-set and will NOT prevote v' without a POL(v')
// at a round >r — none exists by induction. Holds when 2α−n > f (n=5,α=4,f=1: 3>1).
// This is a REFINEMENT of the v1.35.5 guard: one-signature-per-height becomes
// one-precommit-per-(height,round) + the lock rule — a DIFFERENT (but sound) safety
// construction, flagged for owner sign-off.
//
// This type is PURE: no engine, network, or clock dependency. The driver feeds it
// observed prevotes/precommits + the deterministic winner + a settle tick, and it
// returns the ACTIONS to take. All view-change safety lives here so it is exhaustively
// unit-testable in isolation (round_view_test.go) — the surface Red attacks.

package chain

import (
	"encoding/binary"
	"fmt"

	"github.com/luxfi/ids"
)

// CanonicalPrevoteMessage is the domain-separated signed message for a PREVOTE at a
// (height, round) on a canonical block. A prevote is a NON-BINDING preference signal
// (it drives the POL, not the cert), so it carries its OWN domain tag distinct from the
// precommit/accept message (canonicalVoteMessageFor, tag "LUX/chain/vote/v2") — a prevote
// signature can NEVER be presented as a precommit and vice-versa. It binds the minimal
// POL identity: (chainID, height, round, canonical). That is sufficient for the lock/
// unlock safety (a POL is "which block at this height/round"); the full execution
// identity + set-root binding lives in the precommit cert (canonicalVoteMessageFor).
func CanonicalPrevoteMessage(chainID ids.ID, height uint64, round uint32, canonical ids.ID) []byte {
	const tag = "LUX/chain/prevote/v1\x00"
	buf := make([]byte, 0, len(tag)+32+8+4+32)
	buf = append(buf, tag...)
	buf = append(buf, chainID[:]...)
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], height)
	buf = append(buf, u64[:]...)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], round)
	buf = append(buf, u32[:]...)
	buf = append(buf, canonical[:]...)
	return buf
}

// encodeSignedPrevote encodes a broadcastable prevote: (nodeID, height, round, canonical, sig).
// The (height, round, canonical) travel in the wire so a receiver can rebuild the exact
// signed message and verify — a prevote can never be replayed at a different position.
func encodeSignedPrevote(nodeID ids.NodeID, height uint64, round uint32, canonical ids.ID, sig []byte) []byte {
	buf := make([]byte, 0, ids.NodeIDLen+8+4+32+4+len(sig))
	buf = append(buf, nodeID[:]...)
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], height)
	buf = append(buf, u64[:]...)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], round)
	buf = append(buf, u32[:]...)
	buf = append(buf, canonical[:]...)
	binary.BigEndian.PutUint32(u32[:], uint32(len(sig)))
	buf = append(buf, u32[:]...)
	buf = append(buf, sig...)
	return buf
}

// decodeSignedPrevote is the inverse of encodeSignedPrevote (strict; fail-closed).
func decodeSignedPrevote(data []byte) (nodeID ids.NodeID, height uint64, round uint32, canonical ids.ID, sig []byte, err error) {
	const fixed = ids.NodeIDLen + 8 + 4 + 32 + 4
	if len(data) < fixed {
		return nodeID, 0, 0, canonical, nil, fmt.Errorf("%w: prevote too short", ErrVoteWireCorrupt)
	}
	off := 0
	copy(nodeID[:], data[off:off+ids.NodeIDLen])
	off += ids.NodeIDLen
	height = binary.BigEndian.Uint64(data[off : off+8])
	off += 8
	round = binary.BigEndian.Uint32(data[off : off+4])
	off += 4
	copy(canonical[:], data[off:off+32])
	off += 32
	sigLen := binary.BigEndian.Uint32(data[off : off+4])
	off += 4
	if uint64(sigLen) != uint64(len(data)-off) {
		return nodeID, 0, 0, canonical, nil, fmt.Errorf("%w: prevote sig_len %d != remaining %d", ErrVoteWireCorrupt, sigLen, len(data)-off)
	}
	sig = make([]byte, sigLen)
	copy(sig, data[off:])
	return nodeID, height, round, canonical, sig, nil
}

// pvSigKey keys the stored signed prevotes for POL relay.
type pvSigKey struct {
	height uint64
	round  uint32
	canon  ids.ID
}

// storePrevoteSigLocked records a verified signed prevote for later POL relay. Caller holds slotMu.
func (t *Transitive) storePrevoteSigLocked(nodeID ids.NodeID, height uint64, round uint32, canon ids.ID, sig []byte) {
	k := pvSigKey{height: height, round: round, canon: canon}
	m := t.prevoteSigs[k]
	if m == nil {
		m = map[ids.NodeID][]byte{}
		t.prevoteSigs[k] = m
	}
	if _, ok := m[nodeID]; !ok {
		m[nodeID] = append([]byte(nil), sig...)
	}
}

// viewForLocked returns (creating if needed) the round-scoped view machine for a height.
// Caller holds slotMu. alpha is the current α-of-K threshold (POL + cert). A view whose
// height is at/below the decided floor is never created (it is decided).
func (t *Transitive) viewForLocked(height uint64, alpha, n int) *roundView {
	if v, ok := t.views[height]; ok {
		return v
	}
	v := newRoundView(height, alpha, n)
	// RESTART SEED: if a v3-recovered lock round exists for this height, seed the machine's
	// lock (round + value) so a restarted node re-converges under the unlock rule instead of
	// starting unlocked (which would let it precommit a conflicting value — the double-precommit
	// the durable lock prevents). Only fires for a RECOVERED height: a fresh in-session height
	// has no lockRounds entry at view-creation time (it is set on precommit, after this).
	if round, ok := t.lockRounds[height]; ok {
		if bound, ok2 := t.committedSlot[SlotKey{Height: height}]; ok2 {
			v.haveLocked = true
			v.lockRound = round
			v.lockBlock = bound
		}
	}
	t.views[height] = v
	return v
}

// pruneViewsBelow drops view machines strictly below a finalized height (bounded memory);
// the finalized height itself is decided and its view is dropped too. Caller need NOT hold
// slotMu (self-locks). Called from the finalizer alongside pruneCommittedSlotsBelow.
func (t *Transitive) pruneViewsBelow(height uint64) {
	t.slotMu.Lock()
	for h := range t.views {
		if h <= height {
			delete(t.views, h)
		}
	}
	for k := range t.prevoteSigs {
		if k.height <= height {
			delete(t.prevoteSigs, k)
		}
	}
	t.slotMu.Unlock()
}

// viewSettleTicks is how many convergence ticks a view-change round lasts before it
// advances (the convergence loop ticks at ~settleWindow/3, so a round ≈ the settle
// window — long enough for prevotes to gossip in, short enough to re-converge quickly).
const viewSettleTicks = int64(3)

// rvKey identifies a per-(round, block) tally bucket.
type rvKey struct {
	round uint32
	block ids.ID
}

// rvAction is what the driver must do after a Step: at most one prevote, one
// precommit, and/or a finalize, plus whether the round advanced. Empty ids.ID means
// "no action of that kind this Step".
type rvAction struct {
	Prevote        ids.ID // broadcast a signed prevote for this block at CurRound
	Precommit      ids.ID // broadcast a signed precommit (the α-of-K cert vote) for this block at PrecommitRound
	PrecommitRound uint32 // the round the precommit is cast at (the POL round — NOT necessarily CurRound)
	Finalize       ids.ID // an α-of-K precommit quorum exists for this block at some round — commit it
	CurRound       uint32 // the round the prevote is cast at
	NewRound       bool   // the round advanced this Step
}

// roundView is one height's view-change state.
type roundView struct {
	height uint64
	alpha  int // POL / cert threshold (α-of-K)
	n      int // committee size (for the safety bound 2α−n>f and the f+1 round-skip threshold)
	f      int // Byzantine budget ⌊(n-1)/3⌋

	round      uint32
	elapsed    int64 // settle ticks accumulated in the current round (driver-supplied clock)
	// roundSenders[r] = distinct validators observed to have sent ANY message (prevote or
	// precommit) at round r. Drives ROUND-SKIP: on f+1 senders at a round above ours, at
	// least one is honest and genuinely ahead, so we JUMP to that round (Tendermint's
	// round-skip) — the rule that re-aligns a node left phase-offset by asymmetric gossip
	// (the residual ~3% freeze RED found: a drifted node's 3<α prevotes never form a POL).
	roundSenders map[uint32]map[ids.NodeID]struct{}
	haveLocked bool
	lockRound  uint32
	lockBlock  ids.ID

	// prevoted/precommitted record THIS node's one cast per round (idempotence + the
	// one-vote-per-(height,round) discipline that makes a POL unique per round).
	prevoted     map[uint32]ids.ID
	precommitted map[uint32]ids.ID

	// tallies of DISTINCT signed votes observed (own included), de-duped by NodeID.
	prevoteTally   map[rvKey]map[ids.NodeID]struct{}
	precommitTally map[rvKey]map[ids.NodeID]struct{}

	// polRound[block] = the highest round at which `block` reached α prevotes (a POL).
	// Drives the unlock rule (prevote a conflicting value only with a POL above the lock).
	polRound map[ids.ID]uint32
	havePOL  map[ids.ID]struct{}

	// validBlock / validRound track the value carrying the HIGHEST-round POL this node has
	// observed (Tendermint's validValue/validRound). It is the "proposal" the prevote rule
	// converges on: because every node computes it from the SAME gossiped prevote tallies,
	// all honest nodes independently pick the SAME target, which is what defeats an
	// equivocator that manufactures competing POLs to split-lock the honest set (RED's
	// liveness vector). validRound = -1 until the first POL.
	validBlock ids.ID
	validRound int64

	finalized      bool
	finalizedBlock ids.ID
	finalizedRound uint32
}

func newRoundView(height uint64, alpha, n int) *roundView {
	return &roundView{
		height:         height,
		alpha:          alpha,
		n:              n,
		f:              (n - 1) / 3,
		validRound:     -1,
		prevoted:       map[uint32]ids.ID{},
		precommitted:   map[uint32]ids.ID{},
		prevoteTally:   map[rvKey]map[ids.NodeID]struct{}{},
		precommitTally: map[rvKey]map[ids.NodeID]struct{}{},
		polRound:       map[ids.ID]uint32{},
		havePOL:        map[ids.ID]struct{}{},
		roundSenders:   map[uint32]map[ids.NodeID]struct{}{},
	}
}

// safeConfig reports whether this committee meets the fork-safety bound 2α−n > f. When
// false, ONE equivocator can sit in two α-quorums and double-finalize (RED proved this at
// α=3, n=5). The engine MUST refuse to run view-change on a config that fails this.
func (v *roundView) safeConfig() bool { return 2*v.alpha-v.n > v.f }

// noteSender records that `nodeID` sent a message at `round` and applies the ROUND-SKIP
// rule: if some round r strictly above the current round has ≥ f+1 distinct senders, at
// least one honest node is genuinely at r, so JUMP there (reset the settle clock). Only
// ever advances the round; never regresses it, so it cannot undo the lock/round monotonicity.
func (v *roundView) noteSender(round uint32, nodeID ids.NodeID) {
	if round <= v.round {
		return
	}
	set := v.roundSenders[round]
	if set == nil {
		set = map[ids.NodeID]struct{}{}
		v.roundSenders[round] = set
	}
	set[nodeID] = struct{}{}
	if len(set) >= v.f+1 && round > v.round {
		v.round = round
		v.elapsed = 0
	}
}

// observePrevote records a distinct signed prevote (own or peer, pre-verified by the
// caller) and updates the POL index. Returns true if this prevote COMPLETED a new POL.
func (v *roundView) observePrevote(nodeID ids.NodeID, round uint32, block ids.ID) bool {
	if block == ids.Empty {
		return false
	}
	v.noteSender(round, nodeID) // ROUND-SKIP tracking
	k := rvKey{round: round, block: block}
	set := v.prevoteTally[k]
	if set == nil {
		set = map[ids.NodeID]struct{}{}
		v.prevoteTally[k] = set
	}
	if _, dup := set[nodeID]; dup {
		return false
	}
	set[nodeID] = struct{}{}
	if len(set) < v.alpha {
		return false
	}
	// POL reached (or re-confirmed at a possibly-higher round).
	if prev, ok := v.polRound[block]; !ok || round > prev {
		v.polRound[block] = round
	}
	// Track the highest-round POL as validValue/validRound — the value every honest node
	// converges its prevote on (all compute it from the same gossiped tallies).
	if int64(round) > v.validRound {
		v.validRound = int64(round)
		v.validBlock = block
	}
	if _, ok := v.havePOL[block]; ok {
		return false // already had a POL for this block (this call only raised the round)
	}
	v.havePOL[block] = struct{}{}
	return true
}

// observePrecommit records a distinct signed precommit and reports whether an α-of-K
// precommit quorum now exists for some (round, block) — the finality condition (the
// cert binds the round, so the quorum must be at ONE round).
func (v *roundView) observePrecommit(nodeID ids.NodeID, round uint32, block ids.ID) {
	if block == ids.Empty || v.finalized {
		return
	}
	v.noteSender(round, nodeID) // ROUND-SKIP tracking
	k := rvKey{round: round, block: block}
	set := v.precommitTally[k]
	if set == nil {
		set = map[ids.NodeID]struct{}{}
		v.precommitTally[k] = set
	}
	set[nodeID] = struct{}{}
	if len(set) >= v.alpha {
		v.finalized = true
		v.finalizedBlock = block
		v.finalizedRound = round
	}
}

// polCert returns the prevotes constituting this node's validValue POL (its highest-round
// POL) so the driver can GOSSIP them: a peer that missed those prevotes (it was partitioned
// when they were cast, and the casters have since moved on) can ingest them, form the same
// POL, and adopt the same validValue — which is what lets a 2|2 split where one side holds a
// POL the other never saw still converge. This is the pure-core analogue of Tendermint's
// proposal carrying (validValue, validRound) + its POL, or anti-entropy prevote gossip.
func (v *roundView) polCert() (block ids.ID, round uint32, voters []ids.NodeID, ok bool) {
	if v.validRound < 0 {
		return ids.Empty, 0, nil, false
	}
	round = uint32(v.validRound)
	block = v.validBlock
	for id := range v.prevoteTally[rvKey{round: round, block: block}] {
		voters = append(voters, id)
	}
	return block, round, voters, len(voters) > 0
}

// polAbove reports whether `block` has a POL at a round strictly greater than `round`.
func (v *roundView) polAbove(block ids.ID, round uint32) bool {
	r, ok := v.polRound[block]
	return ok && r > round
}

// prevoteTarget applies the Tendermint prevote rule for the current round, given the
// deterministic winner W (lowest-canonical live sibling). Returns Empty if the node
// has no valid target (no live winner).
func (v *roundView) prevoteTarget(winner ids.ID) ids.ID {
	if winner == ids.Empty && v.validRound < 0 {
		return ids.Empty
	}
	// The PROPOSAL each honest node converges on is validValue (the highest-round POL) when
	// one exists — computed from the same gossiped prevote tallies, so all nodes pick the
	// SAME target and an equivocator cannot split-lock them — else the deterministic winner
	// (lowest-canonical live sibling) as the fresh round-0 proposal.
	proposal := winner
	if v.validRound >= 0 {
		proposal = v.validBlock
	}
	if proposal == ids.Empty {
		return ids.Empty
	}
	if !v.haveLocked {
		return proposal // free: prevote the proposal
	}
	if proposal == v.lockBlock {
		return proposal
	}
	// Locked on a DIFFERENT value: prevote the proposal ONLY with a POL for it strictly
	// above the lock round (the unlock justification); else stay on the lock.
	if v.polAbove(proposal, v.lockRound) {
		return proposal
	}
	return v.lockBlock
}

// step advances the machine one driver tick. `winner` is the deterministic
// lowest-canonical live sibling this round; `tick` is added to the settle clock;
// `settle` is the per-round settle budget (advance the round once exceeded without a
// cert). It returns the actions the driver must broadcast/apply. The driver is
// responsible for calling observePrevote/observePrecommit for the OWN votes it casts
// (so the tally includes them) — step does NOT mutate tallies, keeping observe the
// single tally authority.
func (v *roundView) step(winner ids.ID, tick, settle int64) rvAction {
	act := rvAction{CurRound: v.round}
	if v.finalized {
		act.Finalize = v.finalizedBlock
		// Keep RE-BROADCASTING our precommit for the finalized block+round so a laggard that
		// missed the quorum (it was partitioned when the others committed, and they then went
		// quiet) can still assemble α precommits and finalize. This is the pure-core analogue
		// of the engine's cert gossip (GossipCert/HandleIncomingCert). Idempotent (dedup by
		// NodeID at the receiver).
		act.Precommit = v.finalizedBlock
		act.PrecommitRound = v.finalizedRound
		return act
	}

	// PRECOMMIT step: precommit the value carrying the highest-round POL (validValue) at
	// that POL round, unless we already precommitted at that round. Precommitting at the POL
	// ROUND (not the — possibly already-advanced — current round) is what stops a node from
	// racing past its own POL when the settle timer advances the round in the same tick the
	// prevote was cast (the miss RED's equivocator-liveness vector exposed). All honest nodes
	// share validRound (same gossiped tallies), so they precommit the SAME value at the SAME
	// round → one α-of-K cert. The lock rule (below, in prevoteTarget) keeps it safe.
	if v.validRound >= 0 {
		pr := uint32(v.validRound)
		if _, done := v.precommitted[pr]; !done {
			act.Precommit = v.validBlock
			act.PrecommitRound = pr
		}
	}
	// Re-broadcast our current LOCK every step (when no fresher POL is being precommitted) so
	// a laggard that missed our precommit during a partition can still assemble α precommits
	// at the lock round — the precommit analogue of block/POL gossip. Idempotent at the
	// receiver (dedup by NodeID); without it, a precommit lost during the split is never
	// re-sent and the laggard stalls forever despite everyone being locked on the same value.
	if act.Precommit == ids.Empty && v.haveLocked {
		act.Precommit = v.lockBlock
		act.PrecommitRound = v.lockRound
	}

	// PREVOTE step: cast our one prevote for this round if we have not yet.
	if _, done := v.prevoted[v.round]; !done {
		if target := v.prevoteTarget(winner); target != ids.Empty {
			act.Prevote = target
		}
	}

	// ROUND ADVANCE: accumulate the settle clock; once the budget is exceeded without a
	// cert, move to the next round (carry the lock). Only advance once we have at least
	// prevoted this round (so a round is never skipped without participating).
	v.elapsed += tick
	if v.elapsed >= settle {
		if _, prevotedThisRound := v.prevoted[v.round]; prevotedThisRound || act.Prevote != ids.Empty {
			v.round++
			v.elapsed = 0
			act.NewRound = true
		}
	}
	return act
}

// polAtRound returns the value that has a POL (α prevotes) AT round r, or Empty. Unique
// per round by the one-prevote-per-round discipline.
func (v *roundView) polAtRound(r uint32) ids.ID {
	for k, set := range v.prevoteTally {
		if k.round == r && len(set) >= v.alpha {
			return k.block
		}
	}
	return ids.Empty
}

// recordOwnPrevote marks that this node prevoted `block` at the current round (called
// by the driver after step returns a Prevote action, before broadcasting). Enforces
// one prevote per round.
func (v *roundView) recordOwnPrevote(block ids.ID) {
	if _, done := v.prevoted[v.round]; done {
		return
	}
	v.prevoted[v.round] = block
}

// recordOwnPrecommit marks that this node precommitted `block` at the current round and
// LOCKS on it. Called by the driver after step returns a Precommit action. Enforces one
// precommit per round + the lock update. Returns false if this would be a SECOND,
// conflicting precommit at the same round (a safety violation the driver must never cause).
func (v *roundView) recordOwnPrecommit(block ids.ID, round uint32) bool {
	if prev, done := v.precommitted[round]; done {
		return prev == block // idempotent for the same block; refuse a conflicting one
	}
	v.precommitted[round] = block
	// LOCK at the POL round. Monotone: a lock only advances to a higher round (a fresher POL);
	// a precommit at a round below the current lock never regresses it (defence in depth —
	// step only offers the highest-round POL, so this is belt-and-braces).
	if !v.haveLocked || round >= v.lockRound {
		v.haveLocked = true
		v.lockRound = round
		v.lockBlock = block
	}
	return true
}
