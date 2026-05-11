// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

// security_profile.go — the chain-wide knob that pins what a Z-Chain (and any
// other chain that consumes a ZProofEnvelope) is allowed to accept.
//
// A profile is the only safety surface the proof verifier consults. Backends
// CANNOT self-assert safety: every "is this proof OK" decision flows through
// a ChainSecurityProfile + a pinned VerifierManifest. This separation is what
// closes HIP-0078 §"Verifier API" and HIP-0077 red-review F1 (silent finality
// forks between hash-profile / proof-system islands).
//
// One locked profile per chain. The toolkit supports many modes; the profile
// picks exactly one tuple and pins it into the chain's genesis. The wire
// envelope of every cert references the profile by ProfileID + ProfileHash,
// so a verifier built against a different profile rejects the cert
// deterministically without re-deriving the producer's full configuration.
//
// Orthogonal axes (do not mix):
//
//	ProofPolicyID    — security class of the proof (what it proves about)
//	ProofBackendID   — implementation (who proved it)
//	ProofFormatID    — wire byte layout of the proof bytes themselves
//	VerifierID       — concrete verifier identity (pinned by VerifierManifest)
//	HashSuiteID      — hash family bound into transcripts
//	SigSchemeID      — threshold / identity signature schemes
//
// A profile constrains every axis. The verifier (protocol/zchain) checks
// fail-closed: anything not on the allow-list is refused. Validate() is the
// single audit gate; ComputeHash() is the genesis-pinning primitive.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"golang.org/x/crypto/sha3"
)

// ProfileID is the wire byte that identifies a chain-wide security profile.
// One profile = one row in the allow-list table; pinning the ID into a proof
// envelope makes a profile-flip into a transcript mismatch, not a silent
// classification change.
//
// Numbering:
//
//	0x00 — None / unspecified (rejected by every strict verifier)
//	0x01 — StrictPQ         — canonical strict-PQ profile: NIST-aligned
//	                          PQ across every axis. The single answer to
//	                          "what does PQ mean?" — no family branding,
//	                          no parallel byte for the same crypto.
//	0x02 — Permissive       — testnet / devnet, accepts dev backends and
//	                          permissive fallbacks alongside the strict-PQ
//	                          primitives.
//	0x03 — FIPS             — FIPS-204 / FIPS-205 only (P3Q backend
//	                          single-allowed); strict superset of StrictPQ
//	                          for FIPS-targeted deployments.
//	0x04 — ZooStrictPQ      — Zoo primary network; byte-identical to
//	                          StrictPQ except for ProfileID + ProfileName.
//	0x05 — HanzoStrictPQ    — Hanzo primary network; byte-identical to
//	                          StrictPQ except for ProfileID + ProfileName.
//	0x80..0xFF — reserved for downstream / white-label profiles (must register
//	             with consensus team before claiming a byte).
type ProfileID uint8

const (
	ProfileNone          ProfileID = 0x00
	ProfileStrictPQ      ProfileID = 0x01
	ProfilePermissive    ProfileID = 0x02
	ProfileFIPS          ProfileID = 0x03
	ProfileZooStrictPQ   ProfileID = 0x04
	ProfileHanzoStrictPQ ProfileID = 0x05
)

// Canonical profile-name strings.
const (
	ProfileNameStrictPQ      = "STRICT_PQ"
	ProfileNamePermissive    = "PERMISSIVE"
	ProfileNameFIPS          = "FIPS"
	ProfileNameZooStrictPQ   = "ZOO_STRICT_PQ"
	ProfileNameHanzoStrictPQ = "HANZO_STRICT_PQ"
)

// String returns the canonical lowercase profile name.
func (p ProfileID) String() string {
	switch p {
	case ProfileNone:
		return "none"
	case ProfileStrictPQ:
		return "strict-pq"
	case ProfilePermissive:
		return "permissive"
	case ProfileFIPS:
		return "fips"
	case ProfileZooStrictPQ:
		return "zoo-strict-pq"
	case ProfileHanzoStrictPQ:
		return "hanzo-strict-pq"
	default:
		return fmt.Sprintf("profile(0x%02x)", uint8(p))
	}
}

// ProofFormatID is the wire byte that identifies the **byte layout** of the
// proof bytes inside a ZProofEnvelope. Orthogonal to ProofBackendID: two
// backends can emit the same format (e.g. both SP1 and RISC0 can be
// serialised to a STARK_FRI_BINARY_V1 layout for cross-backend
// verifiability), and one backend can emit multiple formats across versions.
//
// The format byte is what tells the deserialiser how to parse `ProofBytes`
// before the backend verifier touches them.
//
// Numbering:
//
//	0x00       — None / opaque (verifier must dispatch by VerifierID only)
//	0x10..0x1F — STARK / FRI binary layouts (production)
//	               0x10 = STARK_FRI_BINARY_V1   canonical Lux STARK wire format
//	0x20..0x2F — Backend-native binary layouts (production)
//	               0x20 = SP1_BINARY_V1          SP1 succinct proof native layout
//	               0x21 = RISC0_BINARY_V1        RISC Zero receipt native layout
//	               0x22 = P3Q_BINARY_V1          Lux P3Q (Plonky3 fork) native
//	               0x23 = STONE_CAIRO_BINARY_V1  Stone / Cairo native
//	               0x24 = STWO_CIRCLE_BINARY_V1  Stwo Circle STARK native
//	0x70..0x7F — Dev-only formats (CI / fuzzing)
//	0x80..0xFF — Forbidden in strict-PQ mode (mirrors classical-wrapper IDs)
//	               0x80 = GROTH16_WRAPPED_BINARY classical wrapper format
//	               0x81 = KZG_WRAPPED_BINARY     classical wrapper format
type ProofFormatID uint8

const (
	ProofFormatNone               ProofFormatID = 0x00
	ProofFormatSTARKFRIBinaryV1   ProofFormatID = 0x10
	ProofFormatSP1BinaryV1        ProofFormatID = 0x20
	ProofFormatRISC0BinaryV1      ProofFormatID = 0x21
	ProofFormatP3QBinaryV1        ProofFormatID = 0x22
	ProofFormatStoneCairoBinaryV1 ProofFormatID = 0x23
	ProofFormatStwoCircleBinaryV1 ProofFormatID = 0x24

	ProofFormatGroth16WrappedForbid ProofFormatID = 0x80
	ProofFormatKZGWrappedForbid     ProofFormatID = 0x81
)

// String returns the canonical wire name.
func (f ProofFormatID) String() string {
	switch f {
	case ProofFormatNone:
		return "none"
	case ProofFormatSTARKFRIBinaryV1:
		return "stark-fri-binary-v1"
	case ProofFormatSP1BinaryV1:
		return "sp1-binary-v1"
	case ProofFormatRISC0BinaryV1:
		return "risc0-binary-v1"
	case ProofFormatP3QBinaryV1:
		return "p3q-binary-v1"
	case ProofFormatStoneCairoBinaryV1:
		return "stone-cairo-binary-v1"
	case ProofFormatStwoCircleBinaryV1:
		return "stwo-circle-binary-v1"
	case ProofFormatGroth16WrappedForbid:
		return "groth16-wrapped-classical-forbidden-in-pq"
	case ProofFormatKZGWrappedForbid:
		return "kzg-wrapped-classical-forbidden-in-pq"
	default:
		return fmt.Sprintf("proof-format(0x%02x)", uint8(f))
	}
}

// IsForbiddenInPQMode reports whether this format carries the explicit
// forbidden marker. Used by audit tooling to detect a misconfiguration
// where a classical-wrapped proof would otherwise sneak past a PQ-only
// deployment.
func (f ProofFormatID) IsForbiddenInPQMode() bool {
	return f == ProofFormatGroth16WrappedForbid ||
		f == ProofFormatKZGWrappedForbid
}

// VerifierID is the wire byte that identifies a **concrete pinned verifier**.
// Distinct from ProofBackendID: a single backend (e.g. SP1) can ship multiple
// pinned verifiers over time (different source commits, different verifier
// keys), and the VerifierID is what binds a proof envelope to ONE such
// pinning. Lookup goes through a VerifierManifestRegistry that holds the
// source-commit + verifier-key-hash + program-hash for each VerifierID.
//
// Numbering (open-ended). Initial block:
//
//	0x00       — None / unspecified (rejected)
//	0x10..0x1F — STARK / FRI canonical verifiers (production)
//	0x20..0x2F — Backend-native verifiers (production)
//	0x70..0x7F — Dev-only verifiers (CI, testnet fuzzing)
//	0x80..0xFF — Reserved for downstream / white-label deployments
//
// New entries claim the next free integer in their block; the entry MUST
// be paired with a VerifierManifest registered in the consensus boot path
// before any code references the VerifierID. Never reuse a retired ID.
type VerifierID uint16

const (
	VerifierNone VerifierID = 0x0000

	// Production canonical verifiers (matched 1:1 with ProofBackendID block).
	VerifierLuxSTARKFRISHA3PQ    VerifierID = 0x0010
	VerifierLuxSTARKFRIKeccakPQ  VerifierID = 0x0011
	VerifierSP1CompressedSTARKPQ VerifierID = 0x0020
	VerifierRISC0SuccinctSTARKPQ VerifierID = 0x0021
	VerifierP3QSTARKFRISHA3PQ    VerifierID = 0x0022
	VerifierStoneCairoSTARKPQ    VerifierID = 0x0023
	VerifierStwoCircleSTARKPQ    VerifierID = 0x0024

	// Dev-only verifiers.
	VerifierSP1CoreSTARKDev  VerifierID = 0x0070
	VerifierRISC0RawSTARKDev VerifierID = 0x0071
)

// String returns the canonical wire name.
func (v VerifierID) String() string {
	switch v {
	case VerifierNone:
		return "none"
	case VerifierLuxSTARKFRISHA3PQ:
		return "lux-stark-fri-sha3-pq"
	case VerifierLuxSTARKFRIKeccakPQ:
		return "lux-stark-fri-keccak-pq"
	case VerifierSP1CompressedSTARKPQ:
		return "sp1-compressed-stark-pq"
	case VerifierRISC0SuccinctSTARKPQ:
		return "risc0-succinct-stark-pq"
	case VerifierP3QSTARKFRISHA3PQ:
		return "p3q-stark-fri-sha3-pq"
	case VerifierStoneCairoSTARKPQ:
		return "stone-cairo-stark-pq"
	case VerifierStwoCircleSTARKPQ:
		return "stwo-circle-stark-pq"
	case VerifierSP1CoreSTARKDev:
		return "sp1-core-stark-dev"
	case VerifierRISC0RawSTARKDev:
		return "risc0-raw-stark-dev"
	default:
		return fmt.Sprintf("verifier(0x%04x)", uint16(v))
	}
}

// ChainSecurityProfile is the chain-wide allow-list. It is the only thing
// VerifyZProofUnderProfile consults to decide whether a proof envelope's
// declared axes are admissible on this chain. One profile per chain; the
// profile is locked at genesis and identified on the wire by ProfileID
// plus the 48-byte ProfileHash.
//
// Field invariants enforced by Validate():
//
//   - ProfileID > 0
//   - ProfileName non-empty
//   - HashSuiteID ∈ {SHA3_NIST, BLAKE3_LEGACY} (never None on a locked profile)
//   - IdentitySchemeID is raw FIPS 204 ML-DSA (never None)
//   - FinalitySchemeID is Pulsar-M (44/65/87); raw ML-DSA is identity, not finality
//   - HighValueSchemeID is Pulsar-M-65 or Pulsar-M-87 (not M-44; devnet only)
//   - ProofPolicyID.IsPostQuantum() AND not IsForbiddenInPQMode()
//   - AllowedProofBackends non-empty; every entry IsProductionPQ()
//   - AllowedProofFormats non-empty; every entry not None / not forbidden
//   - MinSoundnessBits ≥ 128
//   - MinHashOutputBits ≥ 256 (strict-PQ pins 384)
//   - At least one Forbid* flag set true (operators must enumerate refusals)
//
// Profiles produced by the constructor functions in profiles.go pass
// Validate by construction; Validate exists as a defensive gate against
// future drift, manifest deserialisation, and operator mistakes.
type ChainSecurityProfile struct {
	// ProfileID is the dense numeric identifier for this profile. uint32
	// wide so future profiles aren't squeezed; the well-known
	// ProfileStrictPQ / ProfilePermissive / ProfileFIPS bytes fit in the
	// low byte. Used alongside ProfileHash on the wire for fast lookup;
	// ProfileHash is authoritative.
	ProfileID uint32

	// ProfileName is the canonical human-readable name. Appears in logs,
	// audit reports, error messages. Non-empty on a locked profile.
	ProfileName string

	// ProfileHash is SHA3-384 over the canonical TupleHash256-style
	// encoding of every other field. Computed by ComputeHash; pinned in
	// genesis and bound into every cert envelope produced under this
	// profile. Any mutation to any other field changes the hash.
	ProfileHash [48]byte

	// HashSuiteID is the single mandatory hash family for transcripts /
	// Merkle commitments. Profiles pin exactly one; locked profiles never
	// carry HashSuiteNone.
	HashSuiteID HashSuiteID

	// IdentitySchemeID is the single mandatory identity-signature scheme
	// (validator registration / rotation / revocation). Locked profiles
	// pin raw FIPS 204 ML-DSA (the 0x40 block).
	IdentitySchemeID SigSchemeID

	// FinalitySchemeID is the single mandatory finality-signature scheme
	// (the Pulsar-M variant Q-Chain consumes). Locked profiles pin a
	// 0x50-block scheme.
	FinalitySchemeID SigSchemeID

	// HighValueSchemeID is the signature scheme governance checkpoints
	// and treasury actions sign under. Stronger than FinalitySchemeID so
	// rare high-stakes operations pay the per-signature cost. Locked
	// profiles pin Pulsar-M-65 or Pulsar-M-87 (never M-44 — devnet only).
	HighValueSchemeID SigSchemeID

	// ProofPolicyID is the single mandatory proof security class. Every
	// admitted proof MUST declare this policy on the wire; the profile
	// does not allow a list of policies.
	ProofPolicyID ProofPolicyID

	// AllowedProofBackends is the set of ProofBackendIDs admissible under
	// this profile. Strict-PQ profiles list only IsProductionPQ() backends.
	// Order is canonicalised by ComputeHash so manifest-ordering doesn't
	// change the digest.
	AllowedProofBackends []ProofBackendID

	// AllowedProofFormats is the set of ProofFormatIDs admissible under
	// this profile. Same dispatch shape as AllowedProofBackends.
	AllowedProofFormats []ProofFormatID

	// MinSoundnessBits is the floor on the proof's claimed soundness in
	// bits. Backends advertise their soundness in the envelope; the
	// verifier refuses any proof below this floor regardless of backend.
	// Strict-PQ profiles set ≥ 128.
	MinSoundnessBits uint16

	// MinHashOutputBits is the floor on the proof's claimed hash output
	// width in bits (e.g. STARK Merkle digest width). Strict-PQ pins ≥ 384.
	MinHashOutputBits uint16

	// RequireTransparent demands `TransparentSetup == true` in the
	// envelope. STARK / FRI proof systems are transparent (no trusted
	// setup); Groth16 / KZG are not. Strict-PQ profiles set this true.
	RequireTransparent bool

	// ForbidPairings demands `UsesPairings == false`. Closes the
	// "classical pairing primitive smuggled through" attack class.
	ForbidPairings bool

	// ForbidKZG demands `UsesKZG == false`. Closes the polynomial
	// commitment trapdoor attack.
	ForbidKZG bool

	// ForbidTrustedSetup demands `UsesTrustedSetup == false`. Subsumes
	// ForbidPairings / ForbidKZG for the classical SNARK families that
	// need a structured reference string.
	ForbidTrustedSetup bool

	// ForbidClassicalSNARKs demands `UsesClassicalSNARKWrapper == false`.
	// Defence against a backend that produces a PQ STARK then wraps it
	// in Groth16 for cheap EVM verification (the canonical anti-pattern
	// HIP-0078 names).
	ForbidClassicalSNARKs bool

	// ForbidDevProofs refuses any backend in the 0x70 dev block. Set on
	// production profiles; left false on testnet/devnet profiles.
	ForbidDevProofs bool

	// ForbidFallbacks refuses to fall back to a weaker primitive when
	// the preferred path is unavailable. Strict-PQ profiles set this so
	// a missing preferred-backend artifact fails the round rather than
	// silently downgrading.
	ForbidFallbacks bool

	// =========================================================================
	// End-to-end PQ axes (HIP-0078 §"E2E PQ surface").
	//
	// The fields above pin the proof-system / threshold-finality posture.
	// The fields below pin the wallet, transaction, contract-auth, KEM,
	// and recovery axes. Each axis carries its own forbidden-primitive
	// gate (Forbid* below) so a locked strict-PQ profile refuses
	// classical leakage at every layer where a Z-Chain envelope is
	// produced or consumed.
	// =========================================================================

	// WalletSchemeID pins the signature scheme accepted from wallets on
	// this chain. Strict-PQ pins ML-DSA-65; the chain refuses ECDSA
	// wallets via ForbidECDSAWallets.
	WalletSchemeID WalletSchemeID

	// TxSchemeID pins the signature scheme accepted on the outer
	// transaction authorisation envelope. May differ from
	// WalletSchemeID (a tx may be authorised by a session key rather
	// than the wallet's identity key).
	TxSchemeID TxSchemeID

	// ContractAuthID pins the default authorisation primitive smart
	// contracts use to gate calls on this chain. Contracts may opt into
	// stronger primitives (e.g. high-value treasury uses
	// ContractAuthMultisigMLDSA + ContractAuthMLDSA87) but anything
	// outside the strict-PQ acceptable set is refused at the chain
	// level via ForbidECDSAContractAuth / ForbidBLSContractAuth.
	ContractAuthID ContractAuthID

	// KeyExchangeID pins the default KEM used for session-key
	// establishment and encrypted-state-transition envelopes. Strict-PQ
	// pins ML-KEM-768; the chain refuses classical X25519 via
	// ForbidClassicalKEM.
	KeyExchangeID KeyExchangeID

	// HighValueKEM pins the stronger KEM the chain uses for high-value
	// operations (treasury withdrawals, governance roots, cross-chain
	// bridge state). Strict-PQ pins ML-KEM-1024. Distinct from
	// KeyExchangeID so the high-value cost is only paid for the
	// operations that need it.
	HighValueKEM KeyExchangeID

	// RecoverySchemeID pins the signature scheme the chain's account-
	// recovery / breakglass key signs under. Strict-PQ pins SLH-DSA-192
	// (stateless hash-based backstop). RecoverySchemeNone is permitted
	// only when HighValueSchemeID is Pulsar-M-87 / ML-DSA-87 — the
	// high-value path then carries the strong fallback recovery would
	// otherwise provide.
	RecoverySchemeID RecoverySchemeID

	// ForbidECDSAWallets refuses any wallet primitive in the 0x90
	// classical block at this chain. Strict-PQ profiles set true.
	ForbidECDSAWallets bool

	// ForbidECDSAContractAuth refuses any contract-authorisation
	// primitive that names classical ECDSA (ContractAuthECDSAUnsafe).
	// Strict-PQ profiles set true.
	ForbidECDSAContractAuth bool

	// ForbidBLSContractAuth refuses any contract-authorisation
	// primitive that names classical BLS aggregates
	// (ContractAuthBLSUnsafe). Strict-PQ profiles set true.
	ForbidBLSContractAuth bool

	// ForbidClassicalKEM refuses any KEM primitive in the 0x90
	// classical block (KeyExchangeX25519Unsafe). Strict-PQ profiles set
	// true.
	ForbidClassicalKEM bool

	// RequireTypedTxAuth demands that every transaction authorisation
	// envelope on this chain carry an explicit TxSchemeID byte (no
	// implicit fallback to "the wallet scheme inferred from the
	// public key"). Strict-PQ profiles set true so a receiver sees the
	// auth primitive named on the wire.
	RequireTypedTxAuth bool
}

// IsPQ reports whether this profile is a strict-PQ profile. PQ mode is
// binary: a chain is strict-PQ or it isn't — there is no relaxed-PQ.
// All canonical strict-PQ profiles (StrictPQ, FIPS, Zoo, Hanzo) return
// true; permissive and unknown profiles return false.
//
// This is the single canonical entry point for "is the chain on PQ?";
// callers MUST go through IsPQ rather than open-coding the ProfileID
// switch so a new strict-PQ profile added to the registry is picked up
// automatically.
//
// Equivalent to "the profile sets every Forbid* bit" (the Forbid bits
// are what make a profile strict-PQ), but checking ProfileID is cheaper
// and avoids the field-by-field walk on every call.
func (p *ChainSecurityProfile) IsPQ() bool {
	if p == nil {
		return false
	}
	switch ProfileID(p.ProfileID) {
	case ProfileStrictPQ,
		ProfileFIPS,
		ProfileZooStrictPQ,
		ProfileHanzoStrictPQ:
		return true
	}
	return false
}

// AllowsBackend reports whether b is in the profile's backend allowlist.
// Returns false for ProofBackendNone and any forbidden / non-production
// backend regardless of whether they appear in the allowlist
// (defence-in-depth — even if a profile somehow got constructed with a
// disallowed entry, AllowsBackend refuses it).
//
// Production-PQ-only is enforced when ForbidDevProofs is set on the
// profile; without that flag, dev backends are admissible if explicitly
// listed.
func (p *ChainSecurityProfile) AllowsBackend(b ProofBackendID) bool {
	if b == ProofBackendNone || b.IsForbiddenInPQMode() {
		return false
	}
	if p.ForbidDevProofs && !b.IsProductionPQ() {
		return false
	}
	for _, a := range p.AllowedProofBackends {
		if a == b {
			return true
		}
	}
	return false
}

// AllowsFormat reports whether f is in the profile's format allowlist.
// Mirrors AllowsBackend semantics.
func (p *ChainSecurityProfile) AllowsFormat(f ProofFormatID) bool {
	if f == ProofFormatNone || f.IsForbiddenInPQMode() {
		return false
	}
	for _, a := range p.AllowedProofFormats {
		if a == f {
			return true
		}
	}
	return false
}

// Typed validation errors. Validate() must catch every misconfiguration
// at boot, not at first-cert; each error names exactly which axis is
// inconsistent so a deploy that ships a malformed profile fails loud.
var (
	// ErrProfileNil — the profile pointer was nil.
	ErrProfileNil = errors.New("ChainSecurityProfile: nil profile")

	// ErrProfileFieldUnset — a required field was left at its zero value.
	ErrProfileFieldUnset = errors.New("ChainSecurityProfile: required field unset")

	// ErrProfileFieldInvalid — a field carries a value that violates the
	// profile's invariants (e.g. classical-only proof policy on a strict-PQ
	// profile, MinSoundnessBits below the floor).
	ErrProfileFieldInvalid = errors.New("ChainSecurityProfile: field has invalid value")

	// ErrProfileFieldUnknown — a field's value is not a known enum entry.
	// Indicates either a renumbering accident or an attacker-supplied byte
	// that does not appear in the local toolkit's enum table.
	ErrProfileFieldUnknown = errors.New("ChainSecurityProfile: field has unknown enum value")
)

// Validate returns nil iff p satisfies every invariant listed in the
// ChainSecurityProfile doc comment. Errors are explicit so operator
// tooling can name the misconfigured field precisely.
//
// Validate is the single audit gate. It runs:
//
//  1. On every profile load (from genesis, from manifest).
//  2. In ComputeHash before the digest is taken (so a malformed profile
//     cannot accidentally produce a stable hash).
//  3. From CI in the locked-profile test suite.
//
// All consumers of ChainSecurityProfile call Validate before trusting it.
func (p *ChainSecurityProfile) Validate() error {
	if err := p.validateStructural(); err != nil {
		return err
	}
	return p.validatePolicy()
}

// validateStructural runs only the structural-presence subset of
// Validate: every required field carries a non-zero value, every enum
// is a known entry, every allow-list is non-empty. Does NOT enforce
// policy-class rules (e.g. "strict-PQ profile must set every Forbid
// bit") — those live in validatePolicy.
//
// ComputeHash calls validateStructural so that a zero-init profile
// cannot accidentally produce a stable hash; the full Validate is the
// audit gate at genesis-load.
func (p *ChainSecurityProfile) validateStructural() error {
	if p == nil {
		return ErrProfileNil
	}
	if p.ProfileID == 0 {
		return fmt.Errorf("%w: ProfileID is zero", ErrProfileFieldUnset)
	}
	if p.ProfileName == "" {
		return fmt.Errorf("%w: ProfileName is empty", ErrProfileFieldUnset)
	}

	// HashSuite — None is a valid wire byte for BLS-only legacy certs but
	// is NEVER valid on a locked profile: a locked profile pins a PQ
	// posture and BLS-only carries no PQ posture.
	switch p.HashSuiteID {
	case HashSuiteSHA3NIST, HashSuiteBLAKE3Legacy:
		// allowed
	case HashSuiteNone:
		return fmt.Errorf("%w: HashSuiteID is None — locked profile must pin a hash family",
			ErrProfileFieldUnset)
	default:
		return fmt.Errorf("%w: HashSuiteID=0x%02x is unknown",
			ErrProfileFieldUnknown, uint8(p.HashSuiteID))
	}

	// Identity scheme — raw FIPS 204 ML-DSA (single-party).
	if p.IdentitySchemeID == SigSchemeNone {
		return fmt.Errorf("%w: IdentitySchemeID is None", ErrProfileFieldUnset)
	}
	if !p.IdentitySchemeID.IsRawMLDSA() {
		return fmt.Errorf("%w: IdentitySchemeID=%s is not raw FIPS 204 ML-DSA",
			ErrProfileFieldInvalid, p.IdentitySchemeID.String())
	}

	// Finality scheme — Pulsar-M threshold (0x50 block).
	if p.FinalitySchemeID == SigSchemeNone {
		return fmt.Errorf("%w: FinalitySchemeID is None", ErrProfileFieldUnset)
	}
	if !p.FinalitySchemeID.IsPulsarM() {
		return fmt.Errorf("%w: FinalitySchemeID=%s is not Pulsar-M threshold",
			ErrProfileFieldInvalid, p.FinalitySchemeID.String())
	}

	// High-value scheme — Pulsar-M-65 or Pulsar-M-87 only. M-44 is
	// devnet-only at NIST PQ Cat 2; high-value roots MUST sit at Cat 3+.
	if p.HighValueSchemeID != SigSchemePulsarM65 && p.HighValueSchemeID != SigSchemePulsarM87 {
		return fmt.Errorf("%w: HighValueSchemeID=%s — high-value roots require Pulsar-M-65 or Pulsar-M-87 (NIST PQ Cat 3+)",
			ErrProfileFieldInvalid, p.HighValueSchemeID.String())
	}

	// Proof policy — PQ-positive AND no forbidden marker. None invalid.
	if p.ProofPolicyID == ProofPolicyNone {
		return fmt.Errorf("%w: ProofPolicyID is None — locked profile must pin a policy",
			ErrProfileFieldUnset)
	}
	if p.ProofPolicyID.IsForbiddenInPQMode() {
		return fmt.Errorf("%w: ProofPolicyID=%s carries forbidden marker",
			ErrProfileFieldInvalid, p.ProofPolicyID.String())
	}
	if !p.ProofPolicyID.IsPostQuantum() {
		return fmt.Errorf("%w: ProofPolicyID=%s is not post-quantum",
			ErrProfileFieldInvalid, p.ProofPolicyID.String())
	}

	// Backend allowlist — non-empty; every entry IsProductionPQ.
	if len(p.AllowedProofBackends) == 0 {
		return fmt.Errorf("%w: AllowedProofBackends is empty", ErrProfileFieldUnset)
	}
	for _, b := range p.AllowedProofBackends {
		if b == ProofBackendNone {
			return fmt.Errorf("%w: AllowedProofBackends contains ProofBackendNone",
				ErrProfileFieldInvalid)
		}
		if b.IsForbiddenInPQMode() {
			return fmt.Errorf("%w: AllowedProofBackends contains forbidden %s",
				ErrProfileFieldInvalid, b.String())
		}
		if !b.IsProductionPQ() {
			return fmt.Errorf("%w: AllowedProofBackends contains non-production %s",
				ErrProfileFieldInvalid, b.String())
		}
	}

	// Format allowlist — non-empty; every entry not None / not forbidden.
	if len(p.AllowedProofFormats) == 0 {
		return fmt.Errorf("%w: AllowedProofFormats is empty", ErrProfileFieldUnset)
	}
	for _, f := range p.AllowedProofFormats {
		if f == ProofFormatNone {
			return fmt.Errorf("%w: AllowedProofFormats contains ProofFormatNone",
				ErrProfileFieldInvalid)
		}
		if f.IsForbiddenInPQMode() {
			return fmt.Errorf("%w: AllowedProofFormats contains forbidden %s",
				ErrProfileFieldInvalid, f.String())
		}
	}

	// Soundness and hash-output floors.
	if p.MinSoundnessBits < 128 {
		return fmt.Errorf("%w: MinSoundnessBits=%d < 128",
			ErrProfileFieldInvalid, p.MinSoundnessBits)
	}
	if p.MinHashOutputBits < 256 {
		return fmt.Errorf("%w: MinHashOutputBits=%d < 256",
			ErrProfileFieldInvalid, p.MinHashOutputBits)
	}

	// Operator-policy: at least one forbid-list bit must be set. Refusing
	// to set any forbid bits is itself the misconfiguration — a profile
	// with all-false forbid bits silently accepts every weak primitive.
	if !p.ForbidPairings &&
		!p.ForbidKZG &&
		!p.ForbidTrustedSetup &&
		!p.ForbidClassicalSNARKs &&
		!p.ForbidDevProofs &&
		!p.ForbidFallbacks {
		return fmt.Errorf("%w: no Forbid* flag set; locked profile must enumerate refusals explicitly",
			ErrProfileFieldInvalid)
	}

	// =========================================================================
	// E2E PQ axes — each enum must be a known non-invalid value.
	//
	// Strict-mode policy (profile-class-specific refusal of the 0x90
	// forbidden block) lives in validatePolicy below; here we only enforce
	// "must be a known enum entry, not zero/Invalid". A profile that pins
	// a 0x90 forbidden marker on these axes is still structurally well
	// formed; the policy gate is what refuses it under strict-PQ.
	// =========================================================================

	switch p.WalletSchemeID {
	case WalletSchemeMLDSA65, WalletSchemeMLDSA87, WalletSchemeECDSAUnsafe:
		// allowed at the structural layer; policy gates ECDSA under strict-PQ
	case WalletSchemeInvalid:
		return fmt.Errorf("%w: WalletSchemeID is Invalid", ErrProfileFieldUnset)
	default:
		return fmt.Errorf("%w: WalletSchemeID=0x%02x is unknown",
			ErrProfileFieldUnknown, uint8(p.WalletSchemeID))
	}

	switch p.TxSchemeID {
	case TxSchemeMLDSA65, TxSchemeMLDSA87, TxSchemeECDSAUnsafe:
	case TxSchemeInvalid:
		return fmt.Errorf("%w: TxSchemeID is Invalid", ErrProfileFieldUnset)
	default:
		return fmt.Errorf("%w: TxSchemeID=0x%02x is unknown",
			ErrProfileFieldUnknown, uint8(p.TxSchemeID))
	}

	switch p.ContractAuthID {
	case ContractAuthZChainProof,
		ContractAuthMultisigMLDSA,
		ContractAuthSessionPQ,
		ContractAuthMLDSA65,
		ContractAuthMLDSA87,
		ContractAuthECDSAUnsafe,
		ContractAuthBLSUnsafe:
	case ContractAuthInvalid:
		return fmt.Errorf("%w: ContractAuthID is Invalid", ErrProfileFieldUnset)
	default:
		return fmt.Errorf("%w: ContractAuthID=0x%02x is unknown",
			ErrProfileFieldUnknown, uint8(p.ContractAuthID))
	}

	switch p.KeyExchangeID {
	case KeyExchangeMLKEM768, KeyExchangeMLKEM1024, KeyExchangeX25519Unsafe:
	case KeyExchangeInvalid:
		return fmt.Errorf("%w: KeyExchangeID is Invalid", ErrProfileFieldUnset)
	default:
		return fmt.Errorf("%w: KeyExchangeID=0x%02x is unknown",
			ErrProfileFieldUnknown, uint8(p.KeyExchangeID))
	}

	switch p.HighValueKEM {
	case KeyExchangeMLKEM768, KeyExchangeMLKEM1024, KeyExchangeX25519Unsafe:
	case KeyExchangeInvalid:
		return fmt.Errorf("%w: HighValueKEM is Invalid", ErrProfileFieldUnset)
	default:
		return fmt.Errorf("%w: HighValueKEM=0x%02x is unknown",
			ErrProfileFieldUnknown, uint8(p.HighValueKEM))
	}

	switch p.RecoverySchemeID {
	case RecoverySchemeSLHDSA128,
		RecoverySchemeSLHDSA192,
		RecoverySchemeSLHDSA256,
		RecoverySchemeMLDSA87,
		RecoverySchemeNone:
	case RecoverySchemeInvalid:
		return fmt.Errorf("%w: RecoverySchemeID is Invalid", ErrProfileFieldUnset)
	default:
		return fmt.Errorf("%w: RecoverySchemeID=0x%02x is unknown",
			ErrProfileFieldUnknown, uint8(p.RecoverySchemeID))
	}

	// Cross-axis (structural): RecoverySchemeNone is only valid when the
	// high-value scheme is Pulsar-M-87 or ML-DSA-87 — the high-value path
	// then provides the strong fallback recovery would otherwise need.
	// Lives in validateStructural (not validatePolicy) because the rule
	// is a hard cross-axis presence check, not a profile-class policy.
	if p.RecoverySchemeID == RecoverySchemeNone &&
		p.HighValueSchemeID != SigSchemePulsarM87 &&
		p.HighValueSchemeID != SigSchemeMLDSA87 {
		return fmt.Errorf("%w: RecoverySchemeNone requires HighValueSchemeID ∈ {Pulsar-M-87, ML-DSA-87}; got %s",
			ErrProfileFieldInvalid, p.HighValueSchemeID.String())
	}

	return nil
}

// validatePolicy runs the policy-class specific subset of Validate:
// strict-PQ + FIPS profiles MUST forbid every classical primitive;
// hash suite must match the internal hash family of the signature
// schemes. NOT called by ComputeHash; called by the public Validate
// entry point only.
func (p *ChainSecurityProfile) validatePolicy() error {
	// Profile-class specific demands. Strict-PQ + FIPS profiles MUST
	// forbid every classical primitive; permissive may relax dev /
	// fallback only. Closes F55: a "strict-PQ" profile that left
	// ForbidPairings/KZG/TrustedSetup/ClassicalSNARKs false would
	// otherwise pass the "at-least-one" gate.
	//
	// Strict-PQ class membership:
	//   ProfileStrictPQ      (0x01) — canonical strict-PQ
	//   ProfileFIPS          (0x03) — FIPS is a stricter superset of strict-PQ
	//   ProfileZooStrictPQ   (0x04) — byte-identical to StrictPQ
	//   ProfileHanzoStrictPQ (0x05) — byte-identical to StrictPQ
	//
	// All ride the same gate so a sibling deployment cannot accidentally
	// relax a Forbid* bit relative to canonical strict-PQ.
	if p.ProfileID == uint32(ProfileStrictPQ) ||
		p.ProfileID == uint32(ProfileFIPS) ||
		p.ProfileID == uint32(ProfileZooStrictPQ) ||
		p.ProfileID == uint32(ProfileHanzoStrictPQ) {
		if !p.ForbidPairings {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidPairings=true", ErrProfileFieldInvalid)
		}
		if !p.ForbidKZG {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidKZG=true", ErrProfileFieldInvalid)
		}
		if !p.ForbidTrustedSetup {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidTrustedSetup=true", ErrProfileFieldInvalid)
		}
		if !p.ForbidClassicalSNARKs {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidClassicalSNARKs=true", ErrProfileFieldInvalid)
		}
		if !p.ForbidDevProofs {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidDevProofs=true", ErrProfileFieldInvalid)
		}
		if !p.ForbidFallbacks {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidFallbacks=true", ErrProfileFieldInvalid)
		}

		// E2E PQ axes — strict-PQ refuses every 0x90 classical primitive
		// on every axis. Each refusal names exactly which axis leaked,
		// so an operator who misconfigures one cannot guess which gate
		// caught it.
		if !p.ForbidECDSAWallets {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidECDSAWallets=true", ErrProfileFieldInvalid)
		}
		if !p.ForbidECDSAContractAuth {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidECDSAContractAuth=true", ErrProfileFieldInvalid)
		}
		if !p.ForbidBLSContractAuth {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidBLSContractAuth=true", ErrProfileFieldInvalid)
		}
		if !p.ForbidClassicalKEM {
			return fmt.Errorf("%w: strict-PQ profile must set ForbidClassicalKEM=true", ErrProfileFieldInvalid)
		}
		if !p.RequireTypedTxAuth {
			return fmt.Errorf("%w: strict-PQ profile must set RequireTypedTxAuth=true", ErrProfileFieldInvalid)
		}

		// Strict-PQ also refuses any axis that names a classical primitive
		// directly. The Forbid* bits above are the operator-visible
		// policy; the checks below are the defence-in-depth gate against
		// a profile whose Forbid bits are set true while one of the
		// scheme bytes still names the 0x90 forbidden marker.
		if p.WalletSchemeID.IsForbiddenInPQMode() {
			return fmt.Errorf("%w: strict-PQ profile names forbidden WalletSchemeID=%s",
				ErrProfileFieldInvalid, p.WalletSchemeID.String())
		}
		if p.TxSchemeID.IsForbiddenInPQMode() {
			return fmt.Errorf("%w: strict-PQ profile names forbidden TxSchemeID=%s",
				ErrProfileFieldInvalid, p.TxSchemeID.String())
		}
		if p.ContractAuthID.IsForbiddenInPQMode() {
			return fmt.Errorf("%w: strict-PQ profile names forbidden ContractAuthID=%s",
				ErrProfileFieldInvalid, p.ContractAuthID.String())
		}
		if p.KeyExchangeID.IsForbiddenInPQMode() {
			return fmt.Errorf("%w: strict-PQ profile names forbidden KeyExchangeID=%s",
				ErrProfileFieldInvalid, p.KeyExchangeID.String())
		}
		if p.HighValueKEM.IsForbiddenInPQMode() {
			return fmt.Errorf("%w: strict-PQ profile names forbidden HighValueKEM=%s",
				ErrProfileFieldInvalid, p.HighValueKEM.String())
		}
	}

	// Cross-axis: hash-suite must match the hash family used internally
	// by the finality signature kernel. Pulsar-M (FIPS 204 / SHAKE256)
	// is SHA-3 internal; binding BLAKE3 at the transcript layer over a
	// SHA-3 signature is a configuration inconsistency that audit
	// pipelines should catch at boot. Closes F54.
	if p.FinalitySchemeID.IsPulsarM() && p.HashSuiteID == HashSuiteBLAKE3Legacy {
		return fmt.Errorf("%w: HashSuiteID=BLAKE3_LEGACY paired with FinalitySchemeID=%s (SHA-3 internal); cross-axis mismatch",
			ErrProfileFieldInvalid, p.FinalitySchemeID.String())
	}
	if p.IdentitySchemeID.IsRawMLDSA() && p.HashSuiteID == HashSuiteBLAKE3Legacy {
		return fmt.Errorf("%w: HashSuiteID=BLAKE3_LEGACY paired with IdentitySchemeID=%s (SHAKE256 internal); cross-axis mismatch",
			ErrProfileFieldInvalid, p.IdentitySchemeID.String())
	}

	return nil
}

// profileHashCustomization is the cSHAKE256 customization tag for
// ChainSecurityProfile.ComputeHash. Pinned at "v1"; bumping the tag
// invalidates every prior profile hash (which is the correct behaviour
// for a hard-fork of the profile encoding).
const profileHashCustomization = "LUX-CHAIN-SECURITY-PROFILE-V1"

// ComputeHash returns a 48-byte (SHA3-384) commitment to this profile
// suitable for pinning in genesis. Two profiles MUST hash to the same
// bytes iff every field is byte-identical; AllowedProofBackends /
// AllowedProofFormats are sorted before hashing so listing order is not
// part of the identity.
//
// The hash is independent of HashSuiteID: it always uses cSHAKE256 /
// TupleHash256, so a profile that pins HashSuiteBLAKE3Legacy still
// hashes deterministically. Pinning the profile hash into genesis lets
// a verifier reject a cert whose profile silently drifted between
// genesis and runtime.
//
// ComputeHash runs Validate first; a malformed profile produces an error
// rather than a stable hash. This is the "no silent acceptance" property:
// an operator cannot deploy a partly initialised profile and learn its
// hash by trial-and-error.
func (p *ChainSecurityProfile) ComputeHash() ([48]byte, error) {
	var zero [48]byte
	// Only structural validation here — ComputeHash is the
	// genesis-pinning primitive and must be callable on policy-class
	// variants for testing. The full Validate() runs at genesis-load
	// where it has the authoritative role of refusing a misconfigured
	// profile.
	if err := p.validateStructural(); err != nil {
		return zero, fmt.Errorf("ChainSecurityProfile.ComputeHash: %w", err)
	}

	// Sort the allow-lists into canonical order (ascending byte value).
	backends := append([]ProofBackendID(nil), p.AllowedProofBackends...)
	sort.Slice(backends, func(i, j int) bool { return backends[i] < backends[j] })
	formats := append([]ProofFormatID(nil), p.AllowedProofFormats...)
	sort.Slice(formats, func(i, j int) bool { return formats[i] < formats[j] })

	parts := [][]byte{
		[]byte("Lux/ChainSecurityProfile/v1"),
		u32BEProfile(p.ProfileID),
		[]byte(p.ProfileName),
		{byte(p.HashSuiteID)},
		{byte(p.IdentitySchemeID)},
		{byte(p.FinalitySchemeID)},
		{byte(p.HighValueSchemeID)},
		{byte(p.ProofPolicyID)},
		backendsToBytes(backends),
		formatsToBytes(formats),
		u16BEProfile(p.MinSoundnessBits),
		u16BEProfile(p.MinHashOutputBits),
		{boolByteProfile(p.RequireTransparent)},
		{boolByteProfile(p.ForbidPairings)},
		{boolByteProfile(p.ForbidKZG)},
		{boolByteProfile(p.ForbidTrustedSetup)},
		{boolByteProfile(p.ForbidClassicalSNARKs)},
		{boolByteProfile(p.ForbidDevProofs)},
		{boolByteProfile(p.ForbidFallbacks)},
		// E2E PQ axes — appended in a fixed order so every field
		// mutation is bound into the transcript. Adding a new field
		// here changes every existing profile's hash; that is the
		// correct behaviour for a hash-encoding extension.
		{byte(p.WalletSchemeID)},
		{byte(p.TxSchemeID)},
		{byte(p.ContractAuthID)},
		{byte(p.KeyExchangeID)},
		{byte(p.HighValueKEM)},
		{byte(p.RecoverySchemeID)},
		{boolByteProfile(p.ForbidECDSAWallets)},
		{boolByteProfile(p.ForbidECDSAContractAuth)},
		{boolByteProfile(p.ForbidBLSContractAuth)},
		{boolByteProfile(p.ForbidClassicalKEM)},
		{boolByteProfile(p.RequireTypedTxAuth)},
	}

	// Inline SP 800-185 TupleHash256 → 48 bytes (384 bits). Mirrors the
	// helper in protocol/zchain/hash.go; vendored here so config has no
	// upward dependency on protocol.
	var x []byte
	for _, part := range parts {
		x = append(x, encodeStringSP800185Profile(part)...)
	}
	x = append(x, rightEncodeSP800185Profile(uint64(48)*8)...)

	h := sha3.NewCShake256([]byte("TupleHash"), []byte(profileHashCustomization))
	_, _ = h.Write(x)
	out := make([]byte, 48)
	_, _ = h.Read(out)

	var digest [48]byte
	copy(digest[:], out)
	return digest, nil
}

// MustComputeHash returns ComputeHash's value or panics. Used only at
// package init time inside profiles.go to compute the well-known
// ProfileHash of the canonical Lux strict-PQ profile — that hash must be
// computable without an error path, because a build that cannot compute
// the canonical profile hash cannot ship.
//
// MustComputeHash MUST NOT be called from request-path code. Use
// ComputeHash and handle the error.
func (p *ChainSecurityProfile) MustComputeHash() [48]byte {
	h, err := p.ComputeHash()
	if err != nil {
		panic(fmt.Sprintf("ChainSecurityProfile.MustComputeHash: %v", err))
	}
	return h
}

// backendsToBytes converts a sorted backend slice into its byte form for
// the canonical encoding. Standalone for testability and to keep the
// ComputeHash body terse.
func backendsToBytes(bs []ProofBackendID) []byte {
	out := make([]byte, len(bs))
	for i, b := range bs {
		out[i] = byte(b)
	}
	return out
}

// formatsToBytes mirrors backendsToBytes for ProofFormatID.
func formatsToBytes(fs []ProofFormatID) []byte {
	out := make([]byte, len(fs))
	for i, f := range fs {
		out[i] = byte(f)
	}
	return out
}

// u16BEProfile / u32BEProfile / boolByteProfile are small helpers used
// by ComputeHash. Suffix-Profile avoids collision with similar private
// helpers in this package's other files; this file owns its own copy so
// the encoding stays reviewable in one place.
func u16BEProfile(v uint16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return b[:]
}

func u32BEProfile(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}

func boolByteProfile(v bool) byte {
	if v {
		return 0x01
	}
	return 0x00
}

// encodeStringSP800185Profile is the SP 800-185 §2.3 left_encode prefix +
// raw bytes. Local to this file so config has no internal dependency
// surface; the equivalent helper in protocol/zchain/hash.go is
// byte-for-byte identical.
func encodeStringSP800185Profile(s []byte) []byte {
	out := leftEncodeSP800185Profile(uint64(len(s)) * 8)
	out = append(out, s...)
	return out
}

func leftEncodeSP800185Profile(x uint64) []byte {
	if x == 0 {
		return []byte{0x01, 0x00}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	i := 0
	for i < 7 && buf[i] == 0 {
		i++
	}
	out := make([]byte, 0, 9-i)
	out = append(out, byte(8-i))
	out = append(out, buf[i:]...)
	return out
}

func rightEncodeSP800185Profile(x uint64) []byte {
	if x == 0 {
		return []byte{0x00, 0x01}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	i := 0
	for i < 7 && buf[i] == 0 {
		i++
	}
	out := make([]byte, 0, 9-i)
	out = append(out, buf[i:]...)
	out = append(out, byte(8-i))
	return out
}

// ErrProfileUnknown is returned by ProfileByID when no canonical profile
// matches the supplied ProfileID byte.
var ErrProfileUnknown = errors.New("config: unknown ProfileID")

// ProfileByID returns a fresh pointer copy of the canonical profile for
// id. Single dispatch point for "look up a profile by its wire byte";
// every other call site goes through this function instead of switching
// on the constant locally. Refuses ProfileNone and any unknown ID.
//
// Closes F60: previously callers had to dispatch by switching on
// ProfileID, which scattered the canonical registry across every call
// site. With this function, the canonical mapping lives in one place.
func ProfileByID(id ProfileID) (*ChainSecurityProfile, error) {
	switch id {
	case ProfileStrictPQ:
		return StrictPQ(), nil
	case ProfilePermissive:
		return Permissive(), nil
	case ProfileFIPS:
		return FIPS(), nil
	case ProfileZooStrictPQ:
		return ZooStrictPQ(), nil
	case ProfileHanzoStrictPQ:
		return HanzoStrictPQ(), nil
	case ProfileNone:
		return nil, fmt.Errorf("%w: ProfileNone is not a valid profile", ErrProfileUnknown)
	default:
		return nil, fmt.Errorf("%w: 0x%02x", ErrProfileUnknown, uint8(id))
	}
}

// StrictPQ returns a fresh pointer copy of StrictPQProfile. The
// returned value is safe for the caller to retain and mutate without
// affecting other callers; the canonical immutable value lives in
// profiles.go.
//
// This is the profile the canonical Z-Chain runs in production.
func StrictPQ() *ChainSecurityProfile {
	p := StrictPQProfile
	return &p
}

// Permissive returns a fresh pointer copy of the Permissive
// profile constant defined in profiles.go. Testnet/devnet only.
func Permissive() *ChainSecurityProfile {
	p := PermissiveProfile
	return &p
}

// FIPS returns a fresh pointer copy of the FIPSProfile constant
// defined in profiles.go. Strictest FIPS-aligned profile.
func FIPS() *ChainSecurityProfile {
	p := FIPSProfile
	return &p
}

// ZooStrictPQ returns a fresh pointer copy of ZooStrictPQProfile. The
// returned value is byte-identical to StrictPQ except for ProfileID
// (0x04) and ProfileName ("ZOO_STRICT_PQ"). Used by Zoo primary network
// validators; LP-168..179, ZIP-0809..0820 cite this profile.
func ZooStrictPQ() *ChainSecurityProfile {
	p := ZooStrictPQProfile
	return &p
}

// HanzoStrictPQ returns a fresh pointer copy of HanzoStrictPQProfile.
// Byte-identical to StrictPQ except for ProfileID (0x05) and
// ProfileName ("HANZO_STRICT_PQ"). Used by Hanzo primary network
// validators; HIP-0085..0104 cite this profile.
func HanzoStrictPQ() *ChainSecurityProfile {
	p := HanzoStrictPQProfile
	return &p
}

