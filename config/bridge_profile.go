// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

// bridge_profile.go — the chain-wide knob that pins what a bridge / teleport
// entry point is allowed to accept on each side of the wire.
//
// A bridge is a four-corners object:
//
//	source-chain finality ──► relay/MPC ──► dest-chain proof verifier
//	     │                       │                    │
//	     │                       │                    └─ ProofPolicyID
//	     │                       └─ BridgeAdminScheme + BridgePauseScheme
//	     └─ SourceFinalityScheme           DestFinalityScheme
//
// End-to-end PQ on a bridge means: both finality schemes are PQ, the proof
// system terminating on the destination chain is PQ, and the admin /
// pause primitives that can intervene are PQ. Any classical primitive on
// any one of these surfaces breaks the chain.
//
// The wire-level handshake between source and destination is OUT OF SCOPE
// for this file — only the *labelling* and *refusal* policy is here.
// HIP-0078 closes the actual transit protocol; this file is the audit
// gate.
//
// The bridge profile is OUT OF BAND from ChainSecurityProfile. A chain
// has one ChainSecurityProfile pinned in genesis; a bridge is a separate
// object connecting two chains, and may run at a posture distinct from
// either side. A strict-PQ Lux chain may operate a BRIDGE_CLASSICAL_COMPAT
// bridge to an Ethereum L1 — the chain is still strict-PQ; the bridge is
// labelled non-E2E-PQ so an auditor can never mistake a deposit funnelled
// through it for a strict-PQ-vouched deposit.

import (
	"errors"
	"fmt"
)

// BridgeProfileID is the wire byte that identifies a bridge profile.
//
// Numbering:
//
//	0x00       — None / unspecified (rejected by every strict bridge)
//	0x01       — LUX_STRICT_PQ_BRIDGE   — production E2E-PQ
//	0x02..0x7F — reserved for future strict bridge postures
//	0x80..0xFF — non-strict / compat profiles; 0x90 = BRIDGE_CLASSICAL_COMPAT_UNSAFE
type BridgeProfileID uint32

const (
	BridgeProfileIDNone            BridgeProfileID = 0x00
	BridgeProfileIDLuxStrictPQ     BridgeProfileID = 0x01
	BridgeProfileIDClassicalCompat BridgeProfileID = 0x90
)

// String returns the canonical wire name of the bridge-profile ID.
func (p BridgeProfileID) String() string {
	switch p {
	case BridgeProfileIDNone:
		return "none"
	case BridgeProfileIDLuxStrictPQ:
		return "lux-strict-pq-bridge"
	case BridgeProfileIDClassicalCompat:
		return "bridge-classical-compat-unsafe"
	default:
		return fmt.Sprintf("bridge-profile(0x%08x)", uint32(p))
	}
}

// BridgeProfile is the chain-wide allow-list for a single bridge or
// teleport entry point. Every bridge package consults a *BridgeProfile
// before signing an outbound action or verifying an inbound proof.
//
// Field invariants enforced by Validate():
//
//   - ProfileID > 0
//   - Name non-empty
//   - SourceFinalityScheme + DestFinalityScheme name a known SigSchemeID
//   - ProofPolicyID is a known policy
//   - BridgeAdminScheme + BridgePauseScheme name a known ContractAuthID
//   - HashSuiteID ∈ {SHA3_NIST, BLAKE3_LEGACY} (never None on a real profile)
//   - PostQuantumEndToEnd is consistent with the gates and finality schemes
//     (Validate refuses a profile that lies about its posture)
//
// PostQuantumEndToEnd is COMPUTED, not declared: the operator MAY set the
// field to assert an intent, and Validate refuses the profile if the
// stated value disagrees with the implied value. This closes the
// "operator labels a classical bridge as PQ" misconfiguration class.
type BridgeProfile struct {
	// ProfileID is the dense numeric identifier for this bridge profile.
	// Used alongside Name in RPC + block-explorer metadata; auditors filter
	// on it to separate strict-PQ deposits from classical-compat ones.
	ProfileID uint32

	// Name is the canonical human-readable name. Appears in logs, RPC,
	// explorer metadata, and bridge_classical_compat_total metric labels.
	Name string

	// SourceFinalityScheme is the SigSchemeID consensus uses on the source
	// chain to declare a block final. The relay refuses any inbound
	// finality witness that names a different scheme.
	SourceFinalityScheme SigSchemeID

	// DestFinalityScheme is the SigSchemeID consensus uses on the
	// destination chain to declare a block final.
	DestFinalityScheme SigSchemeID

	// ProofPolicyID is the proof-policy class the destination-side
	// verifier requires for cross-chain proofs. Strict-PQ bridges pin
	// STARK_FRI_SHA3_PQ; classical-compat bridges may carry a Groth16 /
	// KZG marker (and PostQuantumEndToEnd is then false).
	ProofPolicyID ProofPolicyID

	// BridgeAdminScheme is the ContractAuthID required to authorise an
	// admin action (relay set, validator rotation, fee parameter change).
	// Strict-PQ pins ML-DSA-87 (Cat 5).
	BridgeAdminScheme ContractAuthID

	// BridgePauseScheme is the ContractAuthID required to authorise the
	// pause / break-glass action. Strict-PQ pins ML-DSA-65 multisig
	// (ContractAuthMultisigMLDSA) so pause requires a committee, not a
	// single key.
	BridgePauseScheme ContractAuthID

	// HashSuiteID is the hash family bound into every bridge transcript
	// (deposit hashes, withdrawal commitments, proof Merkle roots).
	HashSuiteID HashSuiteID

	// AllowsClassicalAdmin permits ecrecover / ECDSA / secp256k1 / EIP-191
	// authorisation in the bridge admin path. false on strict-PQ.
	AllowsClassicalAdmin bool

	// AllowsBLSAggregate permits BLS-12-381 aggregate signatures in the
	// bridge finality-verification path (Avalanche Warp / Teleporter
	// style). false on strict-PQ.
	AllowsBLSAggregate bool

	// AllowsKZGCommitment permits KZG polynomial commitments in any
	// bridge proof artifact (data-availability commitments, batch
	// commitments). false on strict-PQ.
	AllowsKZGCommitment bool

	// AllowsGroth16Wrap permits Groth16-on-BN254 wrapping of an inner
	// proof for cheap EVM verification. false on strict-PQ (closes
	// HIP-0078's "PQ STARK wrapped in classical SNARK" anti-pattern).
	AllowsGroth16Wrap bool

	// AllowsPairingPrecompile permits use of the BN254 pairing precompile
	// (EVM 0x08) in any bridge verification path. false on strict-PQ.
	AllowsPairingPrecompile bool

	// PostQuantumEndToEnd is the audit-visible label. true ONLY when:
	//
	//   - SourceFinalityScheme is PQ-positive (raw ML-DSA or Pulsar-M)
	//   - DestFinalityScheme   is PQ-positive
	//   - ProofPolicyID.IsPostQuantum() and not IsForbiddenInPQMode()
	//   - BridgeAdminScheme.IsPostQuantum() and not IsForbiddenInPQMode()
	//   - BridgePauseScheme.IsPostQuantum() and not IsForbiddenInPQMode()
	//   - every AllowsClassical* flag is false
	//
	// Validate refuses the profile if PostQuantumEndToEnd is declared
	// true but any of the above is violated, OR if PostQuantumEndToEnd
	// is declared false but every input is strict-PQ (operators should
	// not mislabel a PQ bridge as classical).
	PostQuantumEndToEnd bool
}

// Typed validation errors. Mirrors ChainSecurityProfile's error surface so
// downstream auditing tooling has one error vocabulary.
var (
	// ErrBridgeProfileNil — the profile pointer was nil.
	ErrBridgeProfileNil = errors.New("BridgeProfile: nil profile")

	// ErrBridgeProfileFieldUnset — a required field was left zero.
	ErrBridgeProfileFieldUnset = errors.New("BridgeProfile: required field unset")

	// ErrBridgeProfileFieldInvalid — a field violates an invariant.
	ErrBridgeProfileFieldInvalid = errors.New("BridgeProfile: field has invalid value")

	// ErrBridgeProfileFieldUnknown — a field is not a known enum entry.
	ErrBridgeProfileFieldUnknown = errors.New("BridgeProfile: field has unknown enum value")

	// ErrBridgeProfileClassicalUnderStrict — a strict-PQ profile cannot
	// allow a classical primitive. Returned by Validate when
	// PostQuantumEndToEnd=true but a forbidden flag is true.
	ErrBridgeProfileClassicalUnderStrict = errors.New("BridgeProfile: strict-PQ profile cannot allow classical primitive")

	// ErrBridgeProfileForbidden — a runtime check refused an operation
	// because the profile forbids the primitive that was invoked.
	ErrBridgeProfileForbidden = errors.New("BridgeProfile: primitive forbidden by profile")
)

// sigSchemeIsPostQuantum reports whether s is a strict-PQ acceptable
// signature scheme. Local helper rather than a method on SigSchemeID so
// the policy stays in one file — the consensus signature toolkit does
// not declare a single "PQ-positive" predicate elsewhere because it
// distinguishes Pulsar-M (threshold) from raw ML-DSA (single-party) in
// other dispatch paths.
func sigSchemeIsPostQuantum(s SigSchemeID) bool {
	// Both raw FIPS 204 ML-DSA and Pulsar-M threshold verify under
	// unmodified ML-DSA.Verify. Either is acceptable on either side of
	// a strict-PQ bridge.
	return s.VerifiesUnderFIPS204()
}

// sigSchemeIsClassical reports whether s is one of the explicit classical
// markers. Used by Validate so a profile that declares
// PostQuantumEndToEnd=true cannot pin BLS-12-381 as finality.
func sigSchemeIsClassical(s SigSchemeID) bool {
	return s == SigSchemeBLS12381
}

// Validate returns nil iff p satisfies every invariant listed in the
// BridgeProfile doc comment. The PostQuantumEndToEnd value is checked
// against the implied value: mismatched profiles are rejected.
func (p *BridgeProfile) Validate() error {
	if p == nil {
		return ErrBridgeProfileNil
	}
	if p.ProfileID == 0 {
		return fmt.Errorf("%w: ProfileID is zero", ErrBridgeProfileFieldUnset)
	}
	if p.Name == "" {
		return fmt.Errorf("%w: Name is empty", ErrBridgeProfileFieldUnset)
	}

	// Finality schemes must be known enum entries. SigSchemeNone is
	// not acceptable on a real bridge profile.
	if p.SourceFinalityScheme == SigSchemeNone {
		return fmt.Errorf("%w: SourceFinalityScheme is None", ErrBridgeProfileFieldUnset)
	}
	if p.DestFinalityScheme == SigSchemeNone {
		return fmt.Errorf("%w: DestFinalityScheme is None", ErrBridgeProfileFieldUnset)
	}
	// Hash suite is mandatory; bridges that pin no hash suite cannot
	// commit to a transcript and therefore cannot be audited.
	switch p.HashSuiteID {
	case HashSuiteSHA3NIST, HashSuiteBLAKE3Legacy:
		// allowed
	case HashSuiteNone:
		return fmt.Errorf("%w: HashSuiteID is None — bridge profile must pin a hash family",
			ErrBridgeProfileFieldUnset)
	default:
		return fmt.Errorf("%w: HashSuiteID=0x%02x is unknown",
			ErrBridgeProfileFieldUnknown, uint8(p.HashSuiteID))
	}
	if p.ProofPolicyID == ProofPolicyNone {
		return fmt.Errorf("%w: ProofPolicyID is None", ErrBridgeProfileFieldUnset)
	}
	if p.BridgeAdminScheme == ContractAuthInvalid {
		return fmt.Errorf("%w: BridgeAdminScheme is Invalid", ErrBridgeProfileFieldUnset)
	}
	if p.BridgePauseScheme == ContractAuthInvalid {
		return fmt.Errorf("%w: BridgePauseScheme is Invalid", ErrBridgeProfileFieldUnset)
	}

	// Compute the implied PQ posture from the gates + finality schemes.
	impliedPQ := p.computeImpliedPQ()

	// Refuse a profile that lies about its posture in either direction.
	if p.PostQuantumEndToEnd && !impliedPQ {
		return fmt.Errorf("%w: PostQuantumEndToEnd=true but profile carries a classical primitive or non-PQ finality scheme",
			ErrBridgeProfileClassicalUnderStrict)
	}
	if !p.PostQuantumEndToEnd && impliedPQ {
		return fmt.Errorf("%w: PostQuantumEndToEnd=false but every input is strict-PQ — do not mislabel a strict bridge as classical",
			ErrBridgeProfileFieldInvalid)
	}

	return nil
}

// computeImpliedPQ derives the bridge's actual PQ-E2E posture from its
// fields. The output is what PostQuantumEndToEnd should be set to;
// Validate refuses a profile whose declared value disagrees with this.
func (p *BridgeProfile) computeImpliedPQ() bool {
	if !sigSchemeIsPostQuantum(p.SourceFinalityScheme) || sigSchemeIsClassical(p.SourceFinalityScheme) {
		return false
	}
	if !sigSchemeIsPostQuantum(p.DestFinalityScheme) || sigSchemeIsClassical(p.DestFinalityScheme) {
		return false
	}
	if !p.ProofPolicyID.IsPostQuantum() || p.ProofPolicyID.IsForbiddenInPQMode() {
		return false
	}
	if !p.BridgeAdminScheme.IsPostQuantum() || p.BridgeAdminScheme.IsForbiddenInPQMode() {
		return false
	}
	if !p.BridgePauseScheme.IsPostQuantum() || p.BridgePauseScheme.IsForbiddenInPQMode() {
		return false
	}
	if p.AllowsClassicalAdmin {
		return false
	}
	if p.AllowsBLSAggregate {
		return false
	}
	if p.AllowsKZGCommitment {
		return false
	}
	if p.AllowsGroth16Wrap {
		return false
	}
	if p.AllowsPairingPrecompile {
		return false
	}
	return true
}

// IsPostQuantumEndToEnd is the public accessor for the audit label.
// Callers MUST consult this rather than re-deriving the predicate; the
// label is what RPC + block-explorer metadata expose.
func (p *BridgeProfile) IsPostQuantumEndToEnd() bool {
	return p.PostQuantumEndToEnd
}

// RequireAdminPQ refuses an admin authorisation that is not strict-PQ
// under this profile. Bridge code paths that gate an admin action call
// this before invoking ecrecover / BLS verify.
//
// Returns nil iff the profile allows the operation. Strict-PQ profiles
// refuse any classical authorisation regardless of whether the call
// site would otherwise accept it; classical-compat profiles return nil
// (the call site MUST then log the bridge_classical_compat_total metric;
// see the bridge package helper).
func (p *BridgeProfile) RequireAdminPQ() error {
	if p == nil {
		return ErrBridgeProfileNil
	}
	if p.AllowsClassicalAdmin {
		return nil
	}
	// Strict-PQ path: refuse anything that is not a PQ contract-auth.
	if !p.BridgeAdminScheme.IsPostQuantum() {
		return fmt.Errorf("%w: BridgeAdminScheme=%s is not post-quantum",
			ErrBridgeProfileForbidden, p.BridgeAdminScheme.String())
	}
	return nil
}

// RefuseClassicalAdmin is the gate every ecrecover / ECDSA-verify call
// site in bridge code MUST consult. Returns a non-nil error under any
// strict-PQ profile; returns nil under a classical-compat profile. The
// caller is responsible for the bridge_classical_compat_total metric.
func (p *BridgeProfile) RefuseClassicalAdmin() error {
	if p == nil {
		return ErrBridgeProfileNil
	}
	if p.AllowsClassicalAdmin {
		return nil
	}
	return fmt.Errorf("%w: profile %s forbids classical admin (ecrecover/ECDSA/secp256k1)",
		ErrBridgeProfileForbidden, p.Name)
}

// RefuseBLSAggregate is the gate every bls.Verify call site in bridge
// code MUST consult.
func (p *BridgeProfile) RefuseBLSAggregate() error {
	if p == nil {
		return ErrBridgeProfileNil
	}
	if p.AllowsBLSAggregate {
		return nil
	}
	return fmt.Errorf("%w: profile %s forbids BLS aggregate verification",
		ErrBridgeProfileForbidden, p.Name)
}

// RefuseKZGCommitment is the gate every KZG verification call site MUST
// consult.
func (p *BridgeProfile) RefuseKZGCommitment() error {
	if p == nil {
		return ErrBridgeProfileNil
	}
	if p.AllowsKZGCommitment {
		return nil
	}
	return fmt.Errorf("%w: profile %s forbids KZG commitments",
		ErrBridgeProfileForbidden, p.Name)
}

// RefuseGroth16Wrap is the gate every Groth16 verification call site
// MUST consult.
func (p *BridgeProfile) RefuseGroth16Wrap() error {
	if p == nil {
		return ErrBridgeProfileNil
	}
	if p.AllowsGroth16Wrap {
		return nil
	}
	return fmt.Errorf("%w: profile %s forbids Groth16 wrappers",
		ErrBridgeProfileForbidden, p.Name)
}

// RefusePairingPrecompile is the gate every BN254-pairing-precompile
// call site MUST consult.
func (p *BridgeProfile) RefusePairingPrecompile() error {
	if p == nil {
		return ErrBridgeProfileNil
	}
	if p.AllowsPairingPrecompile {
		return nil
	}
	return fmt.Errorf("%w: profile %s forbids pairing-precompile use",
		ErrBridgeProfileForbidden, p.Name)
}

// StrictPQBridgeProfile is the canonical Lux strict-PQ bridge
// posture. Both source and destination finalise under Pulsar-M-65
// (NIST PQ Cat 3, FIPS 204-compatible threshold); the destination-side
// verifier admits STARK/FRI/SHA-3-NIST proofs only; admin is ML-DSA-87
// single-key; pause is a ML-DSA multisig committee; every classical
// gate is closed.
//
// A bridge connecting two Lux primary-network chains (X→C, C→P,
// teleport-to-Z) pins this profile.
//
// Operators CANNOT override; the profile is a const var and the gate
// methods consult it directly. Cross-chain code that wants to use a
// classical primitive must pin BridgeClassicalCompat instead and
// accept the non-E2E-PQ label.
var StrictPQBridgeProfile = BridgeProfile{
	ProfileID:               uint32(BridgeProfileIDLuxStrictPQ),
	Name:                    "LUX_STRICT_PQ_BRIDGE",
	SourceFinalityScheme:    SigSchemePulsarM65,
	DestFinalityScheme:      SigSchemePulsarM65,
	ProofPolicyID:           ProofPolicySTARKFRISHA3PQ,
	BridgeAdminScheme:       ContractAuthMLDSA87,
	BridgePauseScheme:       ContractAuthMultisigMLDSA,
	HashSuiteID:             HashSuiteSHA3NIST,
	AllowsClassicalAdmin:    false,
	AllowsBLSAggregate:      false,
	AllowsKZGCommitment:     false,
	AllowsGroth16Wrap:       false,
	AllowsPairingPrecompile: false,
	PostQuantumEndToEnd:     true,
}

// BridgeClassicalCompat is the bridge profile every cross-chain
// connector to a chain with classical finality (Ethereum L1, Bitcoin,
// any EVM L2 still on ecrecover) MUST pin. It is explicitly labelled
// non-E2E-PQ: PostQuantumEndToEnd=false and the gates allow every
// classical primitive.
//
// A chain may pin StrictPQ as its ChainSecurityProfile *and* operate
// a BridgeClassicalCompat bridge to an external L1: the chain is still
// strict-PQ, but the bridge into / out of it is labelled as the
// classical-compat surface it actually is. Block-explorer metadata
// carries the BridgeProfileID on every deposit / withdrawal so an
// auditor can never mistake a classical-compat deposit for a strict-PQ
// one.
//
// CRITICAL: This profile MUST NOT be marketed as "Lux strict-PQ
// bridge." The Name explicitly says "BRIDGE_CLASSICAL_COMPAT_UNSAFE"
// so an operator who pins it cannot accidentally claim the strict-PQ
// posture; the bridge_classical_compat_total metric fires on every
// classical-gated call.
var BridgeClassicalCompat = BridgeProfile{
	ProfileID:               uint32(BridgeProfileIDClassicalCompat),
	Name:                    "BRIDGE_CLASSICAL_COMPAT_UNSAFE",
	SourceFinalityScheme:    SigSchemeBLS12381, // classical aggregate (Ethereum / Avalanche)
	DestFinalityScheme:      SigSchemePulsarM65,
	ProofPolicyID:           ProofPolicySTARKFRISHA3PQ, // destination-side verifier stays PQ
	BridgeAdminScheme:       ContractAuthECDSAUnsafe,
	BridgePauseScheme:       ContractAuthECDSAUnsafe,
	HashSuiteID:             HashSuiteSHA3NIST,
	AllowsClassicalAdmin:    true,
	AllowsBLSAggregate:      true,
	AllowsKZGCommitment:     true,
	AllowsGroth16Wrap:       true,
	AllowsPairingPrecompile: true,
	PostQuantumEndToEnd:     false,
}

// MustValidate panics on validation failure. Used by init() so a build
// whose canonical bridge profiles fail Validate cannot initialise.
func (p *BridgeProfile) MustValidate() {
	if err := p.Validate(); err != nil {
		panic(fmt.Sprintf("BridgeProfile %q failed Validate: %v", p.Name, err))
	}
}

// init validates the canonical bridge profiles at package load. A
// build whose canonical bridge profiles fail Validate cannot start; the
// panic message names the failing profile so a misconfiguration is
// immediate and visible in the boot log.
func init() {
	StrictPQBridgeProfile.MustValidate()
	BridgeClassicalCompat.MustValidate()
}
