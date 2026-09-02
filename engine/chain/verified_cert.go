// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// verified_cert.go — the SINGLE finality authority token.
//
// THE RULE (one rule, one place): No VerifiedQuorumCert, no finality.
//
// Acceptance is collapsed onto ONE structurally-enforced path. Every finality
// trigger — a vote arrived, a re-poll fired, the pending queue changed, a block
// was built/verified, a poll timeout ticked, a cert was gossiped in — routes to
// Transitive.TryAccept, which either obtains a VerifiedQuorumCert or refuses
// (ErrNoVerifiedQC) and lets the trigger retry later. The ONLY function that can
// finalize a block is Transitive.AcceptWithCert, and it CANNOT be called without
// a VerifiedQuorumCert value.
//
// A VerifiedQuorumCert is UNFORGEABLE outside this package: its only field (qc) is
// unexported, so no other package can construct a non-zero one with a struct
// literal. FIVE things produce it, and every one either runs the full predicate
// first or has no votes to run it on:
//
//  1. BuildVerifiedQuorumCert — the exported minting door, from raw votes over a set.
//  2. HandleIncomingCert — a gossiped certificate, through verifyCert.
//  3. VerifyCatchupCertificate/AcceptCatchupBlock — the same, on the catch-up road.
//  4. assembleVerifiedCertLocked → assembleCertLocked — the engine's own assembly,
//     which verifies before it caches.
//  5. buildSingleValidatorCertLocked — the K==1 road, and the one that is not a
//     predicate: on a genuinely single-validator chain it synthesizes a zero-vote
//     Nova token, because the sole validator's own accept IS the quorum and there
//     is no other party to protect against. Fenced three ways (both callers gate on
//     K()==1, a real signed 1-of-1 is preferred whenever one assembles, and the
//     zero token is returned once the live count passes one on a chain that has
//     finalized anything), and Nova only, so it is never an export door.
//
// 2 through 4 promote through wrapVerifiedCert, which is unexported and refuses
// nil — there is no exported escape hatch that skips verification. A raw α-of-K
// COUNT ("enough voters responded",
// consensus.IsAccepted, "enough pending callbacks") is a LIVENESS signal only:
// it may trigger TryAccept, but it can never itself produce a VerifiedQuorumCert
// and therefore can never finalize. This is the structural form of HIGH-3: the
// count road is no longer an acceptance authority — it is a retry signal.
package chain

import (
	"errors"
	"math"
)

// ErrNoVerifiedQC is returned by TryAccept when no verified quorum certificate
// exists for the block yet. It is NOT an error condition to log loudly — it is
// the normal "not final yet, keep waiting / re-poll" answer on the liveness
// path. A trigger that gets it should retry on its next tick; it must NEVER
// finalize in response to it.
var ErrNoVerifiedQC = errors.New("chain: no verified quorum cert for block — not final (liveness retry, not an accept)")

// ErrCertNeedsStake is the derived-authority refusal at the minting door: a
// certificate was asked for with no stake source to derive its floor from. Every
// floor a certificate declares and is held to is a function of the validator set
// and the rung; with no set there is no floor, and a caller naming one in its
// place is the certificate choosing its own quorum with an extra step.
var ErrCertNeedsStake = errors.New("chain: a cert cannot be minted without a stake source — the floor it declares is derived from the validator set, and there is no set here")

// ErrCertFloorUnstatable is the refusal for a floor no certificate can carry: a
// tier with no floor at all (an unknown rung), a set that derives none (n<1), or —
// at about six billion signers — a floor past what the certificate's uint32
// threshold can hold. Narrowing it would wrap to a number a lone signer meets.
var ErrCertFloorUnstatable = errors.New("chain: the floor this set derives for this tier is not a quorum a cert can state")

// VerifiedQuorumCert is proof that a block met the finality predicate: α distinct
// validators signed ACCEPT over the exact position AND (on a stake-weighted
// chain) those voters hold a strict ⅔ supermajority of stake at the cert's
// epoch. Holding a non-zero VerifiedQuorumCert is the ONLY thing that authorizes
// finalization (AcceptWithCert takes one by value).
//
// The wrapped cert is unexported. There is deliberately NO exported field and NO
// exported raw constructor: a VerifiedQuorumCert can be produced ONLY by
// BuildVerifiedQuorumCert, which verifies. A zero VerifiedQuorumCert{} carries a
// nil cert and is rejected by AcceptWithCert — so even a zero literal cannot
// finalize anything.
type VerifiedQuorumCert struct {
	// qc is the verified witness. Unexported: unforgeable outside this file.
	// nil ⇒ the zero value ⇒ NOT a finality authority (AcceptWithCert refuses it).
	qc *QuorumCert
}

// IsZero reports whether this is the zero VerifiedQuorumCert (no verified witness
// inside). AcceptWithCert refuses a zero cert; a TryAccept that cannot build a
// real cert returns the zero value alongside ErrNoVerifiedQC.
func (v VerifiedQuorumCert) IsZero() bool { return v.qc == nil }

// Cert returns the underlying verified QuorumCert (for gossip / logging /
// per-height-guard finalization). It is the cert that already passed
// VerifyWeighted; callers must not mutate it. nil for the zero value.
func (v VerifiedQuorumCert) Cert() *QuorumCert { return v.qc }

// BuildVerifiedQuorumCert assembles a quorum certificate from the collected
// SIGNED accept votes and verifies it under the FULL finality predicate before
// wrapping it. It is the exported producer of the finality authority token.
//
//	verifier    — the chain's VoteVerifier (BLS / ML-DSA / secp256k1). nil ⇒ fail closed.
//	stake       — the chain's StakeSource: the set this certificate's floor is read
//	              off. nil ⇒ ErrCertNeedsStake, at BOTH rungs. See below.
//	tier        — Nova (local-execution majority) or Quasar (export ⅔-by-stake); selects
//	              which floor is derived and which stake clause VerifyWeighted enforces.
//	epochHeight — the P-chain epoch the per-voter pubkeys, set-root and stake tally
//	              are all read at (MEDIUM-1).
//	pos         — the consensus position the votes (and the cert) bind to.
//	votes       — the collected SIGNED accept records (caller has already filtered
//	              to those whose signature verified; Assemble+Verify re-check).
//
// NO CALLER STATES A QUORUM. The floor is SignerFloor(tier, n) over the set as it
// stands at this epoch, read here and now, and it is what the certificate carries.
// It was an argument once, and the argument was the hole: on the count-only road
// nothing re-derived it, so one vote and an alpha of one minted an authority token
// on a chain of five while the same certificate arriving over the wire was refused.
// A door that can be opened with a number the caller chose is not a door. Now there
// is nothing to state, so there is nothing to disagree with — the shape Rust's
// Tally::cert and the C++ engine already have.
//
// NO SET, NO CERTIFICATE, at both rungs. A floor is a property of the set; with no
// stake source there is no set, no floor and nothing to check a would-be quorum
// against. This is where the arrival road and the minting road part, and the
// difference is what the roads are: Transitive.verifyCert admits a certificate that
// ALREADY carries checked signatures from distinct in-set validators, so with no
// stake source it can still derive a floor from this node's own committee and hold
// the declaration to it. Here there are no arrived signatures — the caller hands
// over raw votes — so a configured committee would be checking the caller against
// the caller. An equal-stake chain mints through the engine, whose committee is its
// own, or supplies a uniform StakeSource; both are a set.
//
// Returns the zero VerifiedQuorumCert and ErrNoVerifiedQC (wrapping the precise
// predicate failure) if a verified quorum is not yet present — this is the
// LIVENESS answer, never a force. NEVER weakens VerifyWeighted.
func BuildVerifiedQuorumCert(
	verifier VoteVerifier,
	stake StakeSource,
	tier Finality,
	epochHeight uint64,
	pos VotePosition,
	votes []SignedVote,
) (VerifiedQuorumCert, error) {
	if verifier == nil {
		return VerifiedQuorumCert{}, ErrNoVerifiedQC
	}
	if stake == nil {
		return VerifiedQuorumCert{}, errors.Join(ErrNoVerifiedQC, ErrCertNeedsStake)
	}
	// The floor, derived. An unknown tier derives 0 and an unresolved set derives a
	// number no honest assembler could have meant, so both land here rather than in a
	// certificate. The upper bound is the width a certificate states its quorum in:
	// SignerFloor counts at full width precisely so this narrowing is a decision and
	// not an accident.
	alpha := SignerFloor(tier, stake.SignerCount(epochHeight))
	if alpha < 1 || alpha > math.MaxUint32 {
		return VerifiedQuorumCert{}, errors.Join(ErrNoVerifiedQC, ErrCertFloorUnstatable)
	}
	cert, err := AssembleQuorumCert(pos, tier, uint32(alpha), votes)
	if err != nil {
		// Quorum not assembled yet (sub-threshold / not-yet-arrived). Liveness:
		// keep waiting. Wrap the precise cause for diagnosis, present
		// ErrNoVerifiedQC to the caller's control flow.
		return VerifiedQuorumCert{}, errors.Join(ErrNoVerifiedQC, err)
	}
	// THE finality predicate — the strict stake floors, the rung's signer floor, and
	// the derived-threshold clause, which re-reads the set and so re-checks the very
	// number this function just stamped. Nothing wraps without clearing it.
	if err := cert.VerifyWeighted(verifier, stake, epochHeight); err != nil {
		return VerifiedQuorumCert{}, errors.Join(ErrNoVerifiedQC, err)
	}
	return VerifiedQuorumCert{qc: cert}, nil
}

// wrapVerifiedCert promotes an ALREADY-verified *QuorumCert into the authority
// token. It is intentionally unexported and used ONLY on paths that have just
// verified the cert through the same predicate (the incoming-cert path, which
// runs VerifyWeighted/Verify in HandleIncomingCert, and the engine's
// assembleCertLocked, which verifies before caching). It refuses nil so the zero
// value can never be promoted. Within-package only — there is no exported escape
// hatch that skips verification.
func wrapVerifiedCert(cert *QuorumCert) (VerifiedQuorumCert, bool) {
	if cert == nil {
		return VerifiedQuorumCert{}, false
	}
	return VerifiedQuorumCert{qc: cert}, true
}
