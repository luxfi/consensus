// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// round_signer.go -- PulsarRoundSigner: the Quasar cert-profile adapter that
// implements luxfi/pulsar's RoundSigner by CONSUMING its primitives.
//
// The three round methods map onto the consensus hot path:
//
//	Round1   binds the canonical NonceCert (selected via
//	         pulsar.CanonicalNonceIndex over the session) to the consensus
//	         session id. It refuses a non-canonical nonce (anti-grind).
//	Round2   emits one signer's proof-carrying z Partial. The public bindings
//	         (party / session / nonce) are enforced by pulsar.VerifyZPartial.
//	Finalize selects the canonical signer subset (pulsar.CanonicalSignerSet),
//	         aggregates z (pulsar.MergeAggregates), assembles the ConsensusCert
//	         accountability artifact (signer bitmap + transcript root), and asks
//	         the registered BCC core to produce the final FIPS 204 signature.
//	         If no core is registered, the Signature is left empty and
//	         ErrProfileNotReady is returned for fallback to Corona.

package pulsar

import (
	pulsarlib "github.com/luxfi/pulsar/ref/go/pkg/pulsar"
)

// SignatureCore is the secret-bearing seam. Producing the final FIPS 204
// ML-DSA signature requires the module matrix A, t1, the challenge, and the
// per-coefficient public hint (FindHint over w' = A·z − c·t1·2^d). All of that
// is package-private to luxfi/pulsar, so a sound implementation MUST live
// inside that package and be injected here. Until luxfi/pulsar ships a
// reviewed Boundary-Cleared / Carry-Elimination signer, no core is registered
// and the profile fails closed (ErrProfileNotReady), falling back to Corona.
//
// The core receives only PUBLIC inputs: the aggregated z (Aggregate.ZSum), the
// nonce cert (which carries only w1 + a clearance QC, never w), and the
// session. It never sees secret residuals.
type SignatureCore interface {
	// AssembleSignature recovers the public hint and emits the FIPS 204
	// signature for the aggregated z under the joint key bound to the session.
	AssembleSignature(
		session PulsarSession,
		cert pulsarlib.NonceCert,
		agg pulsarlib.Aggregate,
	) (pulsarlib.Signature, error)
}

// PulsarRoundSigner is the consensus-side Pulsar cert profile. It is
// constructed by the Quasar orchestration layer (validator set, sampling, and
// QC aggregation live there); this type owns only the per-round crypto
// orchestration that the pulsar.RoundSigner contract names.
//
// It satisfies pulsarlib.RoundSigner.
type PulsarRoundSigner struct {
	// Session is the consensus binding for this round. SessionID() is the
	// 32-byte domain-separated session id every signer agrees on.
	Session PulsarSession

	// Pool is the background NonceCert pool. Round1 selects the canonical cert
	// from it; an empty pool yields ErrNonceCertPoolEmpty (fallback signal).
	Pool NonceCertPool

	// Threshold is T: the minimum number of valid partials Finalize aggregates.
	Threshold int

	// ValidatorSetSize bounds the signer bitmap (every set bit must index a
	// validator). Used by the ConsensusCert structural check.
	ValidatorSetSize int

	// L is the ML-DSA secret dimension (ℓ) for the active mode; it sizes the
	// z-vector aggregation. For ML-DSA-65 (ModeP65) this is 5.
	L int

	// Core produces the final FIPS 204 signature. nil means fail-closed: the
	// profile assembles the ConsensusCert but leaves the Signature empty and
	// returns ErrProfileNotReady.
	Core SignatureCore
}

// compile-time assertion: PulsarRoundSigner implements the lib contract.
var _ pulsarlib.RoundSigner = (*PulsarRoundSigner)(nil)

// Profile reports the Pulsar (Module-LWE threshold ML-DSA) certificate
// profile. This is the value the QuasarCert.Pulsar leg corresponds to.
func (s *PulsarRoundSigner) Profile() pulsarlib.CertProfile {
	return pulsarlib.ProfilePulsar
}

// Round1 binds the canonical NonceCert to the consensus session.
//
// The caller passes the session id (which MUST equal s.Session.SessionID();
// callers that already hold the session can pass it without recomputing) and a
// candidate (nonceID, cert). Round1 recomputes the canonical pool index from
// (sessionID, pool root, pool size) and refuses anything but the canonical
// cert -- this is the anti-grind rule: a coordinator cannot pick w1 (hence the
// challenge) by choosing among many boundary-clear certs after seeing the
// message.
func (s *PulsarRoundSigner) Round1(
	sessionID, nonceID [32]byte,
	cert pulsarlib.NonceCert,
) (pulsarlib.SignRound1, error) {
	if s.Pool == nil || s.Pool.Size() == 0 {
		return pulsarlib.SignRound1{}, ErrNonceCertPoolEmpty
	}

	// Canonical, non-grindable selection over the exact pool bound into the
	// session (s.Session.NoncePoolRoot must equal s.Pool.Root()).
	idx := pulsarlib.CanonicalNonceIndex(sessionID, s.Pool.Root(), s.Pool.Size())
	canonical, ok := s.Pool.At(idx)
	if !ok {
		return pulsarlib.SignRound1{}, ErrNonceCertPoolEmpty
	}

	// The supplied cert MUST be the canonical one for this session.
	if canonical.NonceID != nonceID || cert.NonceID != canonical.NonceID {
		return pulsarlib.SignRound1{}, ErrNonCanonicalNonce
	}

	return pulsarlib.SignRound1{
		SessionID: sessionID,
		NonceID:   canonical.NonceID,
		NonceCert: canonical,
	}, nil
}

// Round2 produces one signer's proof-carrying z Partial. The PartialInput
// carries the public bindings (party id, session, nonce, DKG/nonce
// commitments, the challenge, and the packed z-share); pulsar.VerifyZPartial
// enforces them and delegates the zero-knowledge correctness check to the
// registered (fail-closed) PartialZVerifier. The Partial itself carries no
// hint-secret field (no c*s2, c*t0, r0, LowBits).
func (s *PulsarRoundSigner) Round2(
	r1 pulsarlib.SignRound1,
	in pulsarlib.PartialInput,
) (pulsarlib.Partial, error) {
	if r1.NonceID == ([32]byte{}) {
		return pulsarlib.Partial{}, ErrNoNonceCert
	}
	// Bind the partial to this round's session + nonce, regardless of any
	// mismatched values the caller put in PartialInput.
	in.SessionID = r1.SessionID
	in.NonceID = r1.NonceID

	p := pulsarlib.Partial{
		PartyID:   in.PartyID,
		SessionID: r1.SessionID,
		NonceID:   r1.NonceID,
		ZShare:    in.ZShare,
	}
	// Enforce the public bindings (party/session/nonce/z) + the registered ZK
	// correctness check. Fails closed when no sound PartialZVerifier is set.
	if err := pulsarlib.VerifyZPartial(&p, in); err != nil {
		return pulsarlib.Partial{}, err
	}
	return p, nil
}

// Finalize selects the canonical signer subset, aggregates the z-shares,
// assembles the two-certificate accountability artifact, and asks the BCC core
// for the final FIPS 204 signature.
//
// Returns the Aggregate (z-sum + bitmap), the ConsensusCert (signer bitmap +
// transcript root + signature), and an error. When no signature core is
// registered the ConsensusCert is structurally complete but its Signature is
// empty and the error is ErrProfileNotReady (fallback to Corona). The Aggregate
// is returned in BOTH cases so the orchestrator can fan it into a higher tree
// node regardless.
func (s *PulsarRoundSigner) Finalize(
	r1 pulsarlib.SignRound1,
	partials []pulsarlib.Partial,
) (pulsarlib.Aggregate, pulsarlib.ConsensusCert, error) {
	// 1. Canonical, anti-grind signer subset: deterministic first-threshold by
	//    PartyID, so an aggregator cannot grind z / the hint / the signature
	//    bytes by choosing among valid signer sets.
	chosen, bitmap, err := pulsarlib.CanonicalSignerSet(partials, s.Threshold)
	if err != nil {
		return pulsarlib.Aggregate{}, pulsarlib.ConsensusCert{}, err
	}

	// 2. Aggregate z modulo q via the tree-merge path (associative; equals the
	//    flat sum). Each chosen partial becomes a singleton Aggregate so we can
	//    use the exported MergeAggregates (z-sums only; no hint material).
	children := make([]pulsarlib.Aggregate, len(chosen))
	for i, p := range chosen {
		children[i] = pulsarlib.Aggregate{
			SessionID:    r1.SessionID,
			NonceID:      r1.NonceID,
			SignerBitmap: singletonBitmap(p.PartyID),
			ZSum:         p.ZShare,
		}
	}
	agg, err := pulsarlib.MergeAggregates(children, s.L)
	if err != nil {
		return pulsarlib.Aggregate{}, pulsarlib.ConsensusCert{}, err
	}
	// Carry the canonical bitmap (MergeAggregates rebuilds the union, which
	// equals the canonical bitmap for a disjoint chosen set).
	agg.SignerBitmap = bitmap

	// 3. Structural accountability artifact. Transcript root binds the nonce
	//    transcript the signers cleared; the bitmap proves which validators
	//    participated (BLS-like accountability for a lattice scheme).
	cert := pulsarlib.ConsensusCert{
		Epoch:          s.Session.Epoch,
		Height:         s.Session.Height,
		Round:          s.Session.Round,
		BlockHash:      s.Session.BlockHash,
		JointPKID:      s.Session.JointPKID,
		SignerBitmap:   bitmap,
		TranscriptRoot: r1.NonceCert.NonceTranscriptRoot,
	}

	// 4. The secret-bearing FIPS 204 assembly. Fail closed if no core.
	if s.Core == nil {
		return agg, cert, ErrProfileNotReady
	}
	sig, err := s.Core.AssembleSignature(s.Session, r1.NonceCert, agg)
	if err != nil {
		return agg, cert, err
	}
	cert.Signature = sig
	return agg, cert, nil
}

// singletonBitmap returns a one-bit bitmap with only PartyID set, sized to hold
// that bit. MergeAggregates unions these into the full signer bitmap.
func singletonBitmap(partyID uint32) []byte {
	bm := make([]byte, partyID/8+1)
	bm[partyID/8] |= 1 << (partyID % 8)
	return bm
}
