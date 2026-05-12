// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Parallel finality witness producers for Quasar (LP-020).
//
// Lux Quasar finality is layered, parallel witnesses. P-Chain BLS is the
// always-on finality witness. Q-Chain (Ringtail threshold) and Z-Chain
// (MLDSAGroth16 rollup) are independently toggleable parallel witnesses
// that produce additional finality artifacts at the same round-rate as P.
//
// Each round, the consensus driver computes a 32-byte round digest binding
// chain id, epoch, round, mode, and parent state. It then asks each enabled
// witness producer for that round's witness in parallel. Witnesses do not
// pipeline -- adding Q and/or Z does not change finality latency, only
// parallel verification cost.

package quasar

import (
	"context"
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
)

// RoundDigest is the 32-byte certificate subject for a consensus round.
// See LP-020 §2.3 (Definition: Certificate Subject).
//
// The canonical constructor is ComputeRoundDigest (round_digest.go),
// which binds HashSuiteID + IdentitySchemeID + SigSchemeID + ProofPolicyID
// + ProofBackendID + ProofFormatID + VerifierID + chainID + networkID +
// epoch + height + parent_state + payload/DA/state/validator roots into
// the digest via TupleHash256. WitnessSet.Run refuses the zero digest at
// runtime, so callers cannot bypass the canonical constructor (HIP-0077
// F34 closure).
type RoundDigest [32]byte

// IsZero reports whether the digest is all-zero, i.e. uninitialised. The
// zero digest is never a valid certificate subject -- ComputeRoundDigest
// itself refuses zero-value inputs and TupleHash256 never produces an
// all-zero output for non-zero inputs.
func (d RoundDigest) IsZero() bool {
	var z RoundDigest
	return d == z
}

// ErrWitnessUnavailable is returned by a witness producer when the underlying
// chain cannot produce a witness for the given round (e.g. Q-Chain DKG not
// complete, Z-Chain prover missed deadline). The consensus driver downgrades
// to the next-lower witness set per operator policy when this occurs.
var ErrWitnessUnavailable = errors.New("parallel witness unavailable for round")

// PWitnessProducer produces P-Chain BLS aggregate witnesses. Always required.
//
// Implementations live in P-Chain's consensus path; this interface exists for
// symmetry with Q and Z and lets test harnesses substitute fakes.
type PWitnessProducer interface {
	// Witness returns a BLS12-381 aggregate signature plus a signer bitmap
	// over the round digest. Returns ErrWitnessUnavailable on quorum failure.
	Witness(ctx context.Context, digest RoundDigest) (sig []byte, signers []byte, err error)
}

// QWitnessProducer produces Q-Chain Ringtail threshold-signature witnesses.
//
// The Q-Chain VM (chains/quantumvm) implements this by driving a 2-round
// Ringtail threshold ceremony per consensus round once a t-of-n DKG has
// produced the combined public key recorded in qchain_ceremony_root.
type QWitnessProducer interface {
	// Witness returns a Ringtail threshold signature over the round digest,
	// or ErrWitnessUnavailable if the ceremony fails or quorum is missed.
	Witness(ctx context.Context, digest RoundDigest) ([]byte, error)
}

// ZWitnessProducer produces Z-Chain MLDSAGroth16 rollup witnesses.
//
// The Z-Chain VM (chains/zkvm) implements this by collecting per-validator
// ML-DSA-65 signatures over the round digest and producing a single Groth16
// proof attesting "for every i in [N], MLDSA.Verify(pk_i, digest, sig_i) = 1".
//
// The validator ML-DSA public-key list is bound to pchain_validator_root for
// the round; the prover takes those keys as a public input. The proof is
// verified by the Groth16 (bn254) precompile on Z-Chain.
type ZWitnessProducer interface {
	// Witness returns a Groth16 proof aggregating per-validator ML-DSA-65
	// signatures over the round digest. validatorMLDSAPubs is the canonical
	// public-key list rooted in pchain_validator_root for the round.
	Witness(ctx context.Context, digest RoundDigest, validatorMLDSAPubs [][]byte) ([]byte, error)
}

// WitnessSet bundles the witness producers configured for a network. Nil Q
// and/or Z producers are valid; the driver simply skips them and produces
// the corresponding lower-level certificate (PolicyQuorum, PolicyPQ, or
// PolicyPZ instead of PolicyQuantum).
//
// MinPolicy pins the lowest finality policy this set is willing to emit.
// Run() refuses to return a witness bundle below this floor — even if all
// optional producers fail. This closes the silent-downgrade attack
// (HIP-0077 red-review F2): a malicious operator advertising "quasar" in
// genesis but installing a broken Z producer would otherwise emit
// PolicyPQ certs the network thinks are PolicyQuantum.
//
// MinPolicy = 0 (PolicyUnspecified) preserves the legacy "best-effort
// downgrade" behaviour for callers that haven't migrated yet. Production
// chains MUST set MinPolicy explicitly — go through NewWitnessSet to get
// it derived from the chain's ChainSecurityProfile in one place.
type WitnessSet struct {
	P PWitnessProducer
	Q QWitnessProducer // optional
	Z ZWitnessProducer // optional

	// MinPolicy is the lowest acceptable finality policy ID. Zero = unset.
	// Mapping (mirrors config.PQMode.PolicyID()):
	//   1 = PolicyQuorum   (BLS only — bls mode)
	//   5 = PolicyPQ       (BLS + Q witness — pulsar / ringtail mode)
	//   6 = PolicyPZ       (BLS + Z witness — mldsa fallback path)
	//   4 = PolicyQuantum  (BLS + Q + Z — quasar mode)
	MinPolicy uint16

	// Mode is the configured PQMode the cert envelope produced by Run() will
	// carry. Run() copies config.PQMode.HashSuiteID() into RoundWitnesses so
	// the downstream cert assembler can stamp the envelope's HashSuiteID
	// byte and bind it into the transcript signed by the threshold layer.
	// Zero (PQModeBLS) is the default and means HashSuiteNone on the wire,
	// matching the legacy BLS-only cert shape.
	//
	// HIP-0077 §"Lux consensus PQ modes" red-review F1: Pulsar and Ringtail
	// share PolicyID 5 but use different hash kernels (SHA-3 vs BLAKE3); a
	// cert that does not carry HashSuiteID is undecodable by a verifier
	// built against the other kernel.
	Mode config.PQMode
}

// ErrWitnessFloorBreached is returned by Run when the produced witness
// bundle would not satisfy WitnessSet.MinPolicy. Surfaces what's missing
// so the operator can see WHY the round was refused (Q failed? Z failed?
// both?) rather than chasing a downgraded cert through observability.
var ErrWitnessFloorBreached = errors.New("witness producer floor breached: refusing to emit downgraded cert")

// RoundWitnesses is the result of running a WitnessSet for one round.
// Q and/or Z may be nil if their producer was disabled, returned
// ErrWitnessUnavailable, or missed the round deadline.
type RoundWitnesses struct {
	PSig     []byte
	PSigners []byte
	Q        []byte // nil if no Q producer or unavailable
	Z        []byte // nil if no Z producer or unavailable

	// HashSuiteID is the hash-family byte the cert envelope produced from
	// this bundle MUST carry. Derived from WitnessSet.Mode at Run() time;
	// receivers consult this to know which hash kernel to instantiate when
	// verifying the cert. Bound into Certificate.TranscriptHash() so a
	// mutation post-signing breaks signature verification.
	HashSuiteID config.HashSuiteID
}

// Run executes the configured witness producers in parallel against digest.
// P is mandatory: a P failure aborts the round. Q and Z run concurrently.
//
// If WitnessSet.MinPolicy is set, Run REFUSES to emit a bundle whose actual
// witness set falls below that floor — even if it could produce a valid
// downgraded cert. This is the explicit anti-silent-downgrade defence
// (HIP-0077 §"PQ defaults": Quasar MUST NOT auto-degrade to MLDSA when an
// operator declared Quasar). When MinPolicy = 0, legacy best-effort
// behaviour applies: missing optional witnesses just become nil slots.
//
// The caller is responsible for bounding ctx with the round window.
func (ws WitnessSet) Run(ctx context.Context, digest RoundDigest, validatorMLDSAPubs [][]byte) (*RoundWitnesses, error) {
	if ws.P == nil {
		return nil, errors.New("WitnessSet: P producer required")
	}
	// Reject the all-zero digest: a valid RoundDigest is the
	// TupleHash256 output of ComputeRoundDigest over non-zero
	// security-relevant inputs. An all-zero digest is either an
	// uninitialised buffer or a caller bypassing the canonical
	// constructor -- both are protocol bugs.
	if digest.IsZero() {
		return nil, errors.New("WitnessSet: RoundDigest is zero; use ComputeRoundDigest to build the canonical subject")
	}

	type qz struct {
		sig []byte
		err error
	}
	qch := make(chan qz, 1)
	zch := make(chan qz, 1)

	go func() {
		if ws.Q == nil {
			qch <- qz{}
			return
		}
		sig, err := ws.Q.Witness(ctx, digest)
		qch <- qz{sig: sig, err: err}
	}()

	go func() {
		if ws.Z == nil {
			zch <- qz{}
			return
		}
		sig, err := ws.Z.Witness(ctx, digest, validatorMLDSAPubs)
		zch <- qz{sig: sig, err: err}
	}()

	pSig, pSigners, err := ws.P.Witness(ctx, digest)
	if err != nil {
		return nil, err
	}

	q := <-qch
	z := <-zch

	out := &RoundWitnesses{
		PSig:        pSig,
		PSigners:    pSigners,
		HashSuiteID: ws.Mode.HashSuiteID(),
	}
	if q.err == nil {
		out.Q = q.sig
	}
	if z.err == nil {
		out.Z = z.sig
	}

	// Enforce the minimum-policy floor BEFORE returning. Effective policy is
	// derived from which witnesses we actually have:
	//   only P                     -> PolicyQuorum   (1)
	//   P + Q                      -> PolicyPQ       (5)
	//   P + Z                      -> PolicyPZ       (6)
	//   P + Q + Z                  -> PolicyQuantum  (4)
	// (Numerically lower IDs are not strictly weaker — PolicyQuantum=4 is
	// the strongest; ordering for "floor" semantics is below in policyAtLeast.)
	effective := effectivePolicyID(out)
	if ws.MinPolicy != 0 && !policyAtLeast(effective, ws.MinPolicy) {
		return nil, fmt.Errorf("%w: declared floor=%d, produced=%d (Q=%v Z=%v); Q.err=%v Z.err=%v",
			ErrWitnessFloorBreached, ws.MinPolicy, effective,
			out.Q != nil, out.Z != nil, q.err, z.err)
	}
	return out, nil
}

// effectivePolicyID returns the wire policy ID implied by which witnesses
// the bundle actually carries. Mirrors config.PQMode.PolicyID() inverse.
func effectivePolicyID(rw *RoundWitnesses) uint16 {
	hasQ := rw.Q != nil
	hasZ := rw.Z != nil
	switch {
	case hasQ && hasZ:
		return 4 // PolicyQuantum
	case hasQ:
		return 5 // PolicyPQ
	case hasZ:
		return 6 // PolicyPZ
	default:
		return 1 // PolicyQuorum
	}
}

// policyAtLeast returns true iff effective ≥ floor in security strength.
// Strength ordering (strongest → weakest):
//
//	PolicyQuantum (4) > PolicyPZ (6) ≈ PolicyPQ (5) > PolicyQuorum (1)
//
// PolicyPZ and PolicyPQ are incomparable in absolute strength (different
// witness families), so a floor of one does not satisfy the other. The
// only floor a Quasar-mode chain can declare is PolicyQuantum; a Pulsar-
// mode chain declares PolicyPQ; an MLDSA-mode chain declares PolicyPZ.
func policyAtLeast(effective, floor uint16) bool {
	if effective == floor {
		return true
	}
	// Quantum dominates everything below it in this lattice.
	if effective == 4 {
		return true
	}
	return false
}

// DisabledZWitnessProducer is the explicit "no Z lane" sentinel producer.
// Networks that do not run a Z-Chain Groth16 prover (smaller deployments,
// classical-only configurations, or chains during their pre-Z bootstrap
// window) install this producer; the driver treats every call as
// ErrWitnessUnavailable and finalises at the next-lower witness set.
//
// The chains/zkvm package supplies the active-Z producer that proves
// "for each i, MLDSA.Verify(pk_i, digest, sig_i) = 1" via Groth16/bn254
// with the validator ML-DSA pubkey list as a public input. See LP-020 §6
// and proofs/quasar-cert-soundness.tex App. B for the R1CS constraint
// count and prover-cost analysis.
type DisabledZWitnessProducer struct{}

// Witness always returns ErrWitnessUnavailable so the driver downgrades
// uniformly with the rest of the optional-lane fallback path.
func (DisabledZWitnessProducer) Witness(ctx context.Context, digest RoundDigest, validatorMLDSAPubs [][]byte) ([]byte, error) {
	return nil, ErrWitnessUnavailable
}

// ErrNilProfile is returned by NewWitnessSet when the caller supplies a nil
// profile. Closes CR-4: the zero-value WitnessSet (MinPolicy=0) was the
// silent-downgrade attack surface. Routing every production caller through
// NewWitnessSet ensures MinPolicy is derived from a vetted profile.
var ErrNilProfile = errors.New("WitnessSet: nil ChainSecurityProfile")

// NewWitnessSet constructs a WitnessSet whose MinPolicy floor and Mode are
// derived from the chain's ChainSecurityProfile. This is the single
// canonical entry point for production callers; the zero-value literal
// WitnessSet{} (MinPolicy=0) is reserved for legacy / test paths that
// have not migrated yet and is the source of the silent-downgrade attack
// CR-4 closes.
//
// MinPolicy is pinned from the profile:
//
//	StrictPQ / FIPS   -> PolicyQuantum (4)  — refuses any cert below P+Q+Z
//	Permissive        -> PolicyQuorum  (1)  — testnet/devnet, BLS-only OK
//	(other / unknown) -> error
//
// Mode follows the profile's finality scheme: strict-PQ / FIPS chains
// declare PQModeQuasar (P+Q+Z, SHA-3 hash suite); permissive declares
// PQModeBLS. Operators that need a different finality posture (e.g.
// Pulsar-only on a strict chain) construct the WitnessSet directly and
// own the audit consequence.
//
// The constructor copies the supplied P/Q/Z producers in; nil Q or Z is
// admissible at a permissive floor but a strict-PQ profile WITH a nil
// Q or Z producer at construction will still produce a WitnessSet whose
// Run() refuses every round under floor — this is intentional. The right
// way to deploy a strict-PQ chain without one of the optional witnesses
// is to lower the profile, not to install a nil producer.
func NewWitnessSet(profile *config.ChainSecurityProfile, p PWitnessProducer, q QWitnessProducer, z ZWitnessProducer) (WitnessSet, error) {
	if profile == nil {
		return WitnessSet{}, ErrNilProfile
	}
	if p == nil {
		return WitnessSet{}, errors.New("WitnessSet: P witness producer is required")
	}

	floor, mode, err := minPolicyForProfile(profile)
	if err != nil {
		return WitnessSet{}, err
	}

	return WitnessSet{
		P:         p,
		Q:         q,
		Z:         z,
		MinPolicy: floor,
		Mode:      mode,
	}, nil
}

// minPolicyForProfile maps a chain-wide security profile to the WitnessSet
// floor and Mode the producer layer must enforce. One mapping, one place:
// every call site that asks "what witness floor does this profile imply?"
// goes through here so a profile-class renumber lands in exactly one diff.
func minPolicyForProfile(profile *config.ChainSecurityProfile) (uint16, config.PQMode, error) {
	switch config.ProfileID(profile.ProfileID) {
	case config.ProfileStrictPQ, config.ProfileFIPS:
		// Strict-PQ + FIPS chains pin Quasar: all three witnesses
		// required. Cert envelope advertises SHA-3 NIST hash suite.
		return 4, config.PQModeQuasar, nil
	case config.ProfilePermissive:
		// Permissive testnet/devnet: P alone suffices. BLS-only cert.
		return 1, config.PQModeBLS, nil
	case config.ProfileNone:
		return 0, config.PQModeBLS, fmt.Errorf("%w: ProfileNone has no witness floor", config.ErrProfileFieldUnset)
	default:
		return 0, config.PQModeBLS, fmt.Errorf("WitnessSet: unknown ProfileID 0x%02x", uint8(profile.ProfileID))
	}
}
