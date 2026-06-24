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
// A VerifiedQuorumCert is UNFORGEABLE outside this file: its only field (qc) is
// unexported, so no other package — and no other file in this package by
// accident — can construct a non-zero one with a struct literal. The sole
// producer is BuildVerifiedQuorumCert, which runs the full stake-weighted
// predicate (QuorumCert.VerifyWeighted, the strict >⅔-of-stake gate) before it
// will wrap a cert. A raw α-of-K COUNT ("enough voters responded",
// consensus.IsAccepted, "enough pending callbacks") is a LIVENESS signal only:
// it may trigger TryAccept, but it can never itself produce a VerifiedQuorumCert
// and therefore can never finalize. This is the structural form of HIGH-3: the
// count road is no longer an acceptance authority — it is a retry signal.
package chain

import "errors"

// ErrNoVerifiedQC is returned by TryAccept when no verified quorum certificate
// exists for the block yet. It is NOT an error condition to log loudly — it is
// the normal "not final yet, keep waiting / re-poll" answer on the liveness
// path. A trigger that gets it should retry on its next tick; it must NEVER
// finalize in response to it.
var ErrNoVerifiedQC = errors.New("chain: no verified quorum cert for block — not final (liveness retry, not an accept)")

// VerifiedQuorumCert is proof that a block met the finality predicate: α distinct
// validators signed ACCEPT over the exact position AND (on a stake-weighted
// chain) those voters hold a strict ⅔ supermajority of stake at the cert's
// epoch. Holding a non-zero VerifiedQuorumCert is the ONLY thing that authorizes
// finalization (AcceptWithCert takes one by value).
//
// The wrapped cert is unexported. There is deliberately NO exported field and NO
// exported raw constructor: a VerifiedQuorumCert can be produced ONLY by
// BuildVerifiedQuorumCert (which verifies) or BuildSingleValidatorCert (the
// 1-of-1 quorum for K==1). A zero VerifiedQuorumCert{} carries a nil cert and is
// rejected by AcceptWithCert — so even a zero literal cannot finalize anything.
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
// wrapping it. It is the sole multi-validator producer of the finality
// authority token.
//
//	verifier    — the chain's VoteVerifier (BLS / ML-DSA / secp256k1). nil ⇒ fail closed.
//	stake       — the chain's StakeSource. Non-nil ⇒ stake-weighted (VerifyWeighted,
//	              strict >⅔). nil ⇒ count-only Verify (equal-stake chains only; the
//	              chain MUST enforce the equal-stake admission invariant).
//	alpha       — the α-of-K count floor.
//	epochHeight — the P-chain epoch the per-voter pubkeys, set-root and stake tally
//	              are all read at (MEDIUM-1).
//	pos         — the consensus position the votes (and the cert) bind to.
//	votes       — the collected SIGNED accept records (caller has already filtered
//	              to those whose signature verified; Assemble+Verify re-check).
//
// Returns the zero VerifiedQuorumCert and ErrNoVerifiedQC (wrapping the precise
// predicate failure) if a verified ⅔-stake quorum is not yet present — this is
// the LIVENESS answer, never a force. NEVER weakens VerifyWeighted.
func BuildVerifiedQuorumCert(
	verifier VoteVerifier,
	stake StakeSource,
	alpha uint32,
	epochHeight uint64,
	pos VotePosition,
	votes []SignedVote,
) (VerifiedQuorumCert, error) {
	if verifier == nil {
		return VerifiedQuorumCert{}, ErrNoVerifiedQC
	}
	cert, err := AssembleQuorumCert(pos, alpha, votes)
	if err != nil {
		// Quorum not assembled yet (sub-threshold / not-yet-arrived). Liveness:
		// keep waiting. Wrap the precise cause for diagnosis, present
		// ErrNoVerifiedQC to the caller's control flow.
		return VerifiedQuorumCert{}, errors.Join(ErrNoVerifiedQC, err)
	}
	// THE finality predicate. Stake-weighted (strict >⅔) when a stake source is
	// wired; count-only Verify otherwise. Either way the cert must clear it
	// before it can wrap into the authority token.
	if stake != nil {
		if err := cert.VerifyWeighted(verifier, stake, epochHeight); err != nil {
			return VerifiedQuorumCert{}, errors.Join(ErrNoVerifiedQC, err)
		}
	} else if err := cert.Verify(verifier, epochHeight); err != nil {
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
