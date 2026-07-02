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

import "github.com/luxfi/ids"

// rvKey identifies a per-(round, block) tally bucket.
type rvKey struct {
	round uint32
	block ids.ID
}

// rvAction is what the driver must do after a Step: at most one prevote, one
// precommit, and/or a finalize, plus whether the round advanced. Empty ids.ID means
// "no action of that kind this Step".
type rvAction struct {
	Prevote   ids.ID // broadcast a signed prevote for this block at CurRound
	Precommit ids.ID // broadcast a signed precommit (the α-of-K cert vote) for this block at CurRound
	Finalize  ids.ID // an α-of-K precommit quorum exists for this block at some round — commit it
	CurRound  uint32 // the round the prevote/precommit are cast at
	NewRound  bool   // the round advanced this Step
}

// roundView is one height's view-change state.
type roundView struct {
	height uint64
	alpha  int // POL / cert threshold (α-of-K)

	round      uint32
	elapsed    int64 // settle ticks accumulated in the current round (driver-supplied clock)
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

	finalized      bool
	finalizedBlock ids.ID
}

func newRoundView(height uint64, alpha int) *roundView {
	return &roundView{
		height:         height,
		alpha:          alpha,
		prevoted:       map[uint32]ids.ID{},
		precommitted:   map[uint32]ids.ID{},
		prevoteTally:   map[rvKey]map[ids.NodeID]struct{}{},
		precommitTally: map[rvKey]map[ids.NodeID]struct{}{},
		polRound:       map[ids.ID]uint32{},
		havePOL:        map[ids.ID]struct{}{},
	}
}

// observePrevote records a distinct signed prevote (own or peer, pre-verified by the
// caller) and updates the POL index. Returns true if this prevote COMPLETED a new POL.
func (v *roundView) observePrevote(nodeID ids.NodeID, round uint32, block ids.ID) bool {
	if block == ids.Empty {
		return false
	}
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
	}
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
	if winner == ids.Empty {
		return ids.Empty
	}
	if !v.haveLocked {
		return winner // free: prevote the proposal
	}
	if winner == v.lockBlock {
		return v.lockBlock
	}
	// Locked on a DIFFERENT value: prevote the winner ONLY with a POL for it strictly
	// above the lock round (the unlock justification); else stay on the lock.
	if v.polAbove(winner, v.lockRound) {
		return winner
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
		return act
	}

	// PRECOMMIT step first: if a POL exists at the CURRENT round, precommit that value
	// (unique per round) and LOCK, unless we already precommitted this round.
	if _, done := v.precommitted[v.round]; !done {
		if polBlk := v.polAtRound(v.round); polBlk != ids.Empty {
			act.Precommit = polBlk
		}
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
func (v *roundView) recordOwnPrecommit(block ids.ID) bool {
	if prev, done := v.precommitted[v.round]; done {
		return prev == block // idempotent for the same block; refuse a conflicting one
	}
	v.precommitted[v.round] = block
	v.haveLocked = true
	v.lockRound = v.round
	v.lockBlock = block
	return true
}
