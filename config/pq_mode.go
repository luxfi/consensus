// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package config — PQMode is the single switch by which an operator selects
// what cryptographic witnesses ride alongside BLS-12-381 in a Quasar finality
// certificate. Each mode is a fully orthogonal point in the design space and
// is named for the sub-protocol that defines its post-quantum surface.
//
// The truth, in one table:
//
//	mode      classical    threshold lattice          per-validator PQ        rollup             DKG required                          FIPS-approvable?
//	bls       BLS-12-381   —                          —                       —                  —                                     no  (BLS not in FIPS 186)
//	ringtail  BLS-12-381   Ringtail / BLAKE3          —                       —                  trusted dealer                        no  (BLAKE3 not in FIPS 202)
//	pulsar    BLS-12-381   Pulsar   / SHA-3 (SP-185)  —                       —                  Pedersen DKG over R_q + reshare       partial (BLS still classical)
//	quasar    BLS-12-381   Pulsar   / SHA-3 (SP-185)  ML-DSA-65 (rolled)      Groth16 / BN254-Z  Pedersen DKG over R_q + reshare       no  (Groth16/BN254 classical, not FIPS)
//	mldsa     BLS-12-381   —                          ML-DSA-65 raw (SHAKE)   —                  —                                     yes (drop BLS or use FIPS-approved aggregate)
//
// Two production paths:
//
//	BLS → Pulsar → Quasar    public chains (open validator set, epoch rotation)
//	BLS → Ringtail           federation / bridge MPC (fixed dealer, no rotation)
//
// Quasar is the Hanzo-mesh default. Ringtail is for fixed federations only —
// its trusted-dealer DKG is unsuitable for an open public chain. MLDSA is the
// audit-grade fallback **and** the FIPS-approvable path: ML-DSA-65 (FIPS 204)
// + SHAKE256 (FIPS 202) is the only mode where every PQ component sits inside
// the NIST-approved algorithm set. See HIP-0077 §"FIPS profile" for the full
// drop-list (no Pulsar/Ringtail/Groth16/secp256k1) and the implementation
// switch needed.
//
// Pulsar vs Quasar in one sentence: Pulsar = BLS + one R-LWE threshold layer;
// Quasar = Pulsar + a second M-LWE layer (ML-DSA-65) compressed via Groth16
// onto Z-Chain. Same threshold layer, plus a defense-in-depth lattice that
// keeps cert size constant.
package config

import (
	"fmt"
	"os"
	"strings"
)

// PQMode selects the post-quantum signature stack the consensus engine layers
// over the BLS-12-381 fast path. See the package doc-comment for the full
// option matrix; each constant below pins one row.
type PQMode uint8

const (
	// PQModeBLS — BLS aggregate only. 48-byte cert. Classical fast path,
	// no PQ surface. Benchmarks and legacy testnets only; rejected on any
	// production Hanzo mesh per HIP-0077.
	PQModeBLS PQMode = iota

	// PQModeRingtail — BLS + the academic Ringtail 2-round LWE threshold
	// signature. Hash profile is **BLAKE3** (`primitives/hash.go` uses
	// `github.com/zeebo/blake3` for every challenge / share / transcript
	// binding). Trusted-dealer DKG, fixed federation, no proactive
	// resharing. Suitable for bridge MPC and small fixed-membership
	// federations; **not** an open public chain stance because it has no
	// DKG that survives epoch rotation, and BLAKE3 is outside the NIST
	// FIPS 202 approved hash family so this mode is **not** FIPS-approvable.
	// Provided by `github.com/luxfi/ringtail` (academic port).
	PQModeRingtail

	// PQModePulsar — BLS + the Pulsar production fork of Ringtail. Same
	// 2-round threshold algorithm as Ringtail, but Pulsar's canonical
	// hash profile is **SHA-3 (cSHAKE256 / KMAC256 / TupleHash256,
	// FIPS 202 + NIST SP 800-185)** and Pulsar adds the production
	// lifecycle Ringtail lacks: Pedersen DKG over R_q with proper hiding,
	// and proactive secret resharing for epoch validator rotation.
	// Pulsar additionally ships a non-normative `Pulsar-BLAKE3` legacy
	// suite for byte-equality regressions only.
	// Suitable for open public chains and the Lux primary network.
	// Provided by `github.com/luxfi/pulsar`.
	PQModePulsar

	// PQModeQuasar — BLS + Pulsar + ML-DSA-65 rolled into a Groth16 proof
	// anchored on Z-Chain. Two distinct lattice constructions backing
	// each finality cert (Pulsar = R-LWE threshold; ML-DSA = MLWE per
	// validator), both still constant-size in the cert. Strongest
	// available; **Hanzo mesh default** per HIP-0077. Where the Z-Chain
	// witness path is not yet wired in a given deployment, Quasar
	// degrades to MLDSA semantics so the strong stance is preserved at
	// higher bandwidth cost.
	// Implemented by `lux/consensus/protocol/quasar`.
	PQModeQuasar

	// PQModeMLDSA — BLS + per-validator ML-DSA-65, no rollup, no
	// threshold. Cert size: 48 + N*3309 B (linear in validator count).
	// Audit-grade fallback when Quasar's Z-Chain Groth16 witness is
	// unavailable, or when an external auditor needs every validator's
	// raw signature in every cert. ML-DSA primitive comes from Pulsar's
	// FIPS-204 implementation.
	PQModeMLDSA
)

// String returns the canonical single-word name.
func (m PQMode) String() string {
	switch m {
	case PQModeBLS:
		return "bls"
	case PQModeRingtail:
		return "ringtail"
	case PQModePulsar:
		return "pulsar"
	case PQModeQuasar:
		return "quasar"
	case PQModeMLDSA:
		return "mldsa"
	default:
		return fmt.Sprintf("pq-mode(%d)", uint8(m))
	}
}

// PolicyID maps a PQMode to the canonical wire policy ID.
//
// Translation, kept here so we don't drag pkg/wire into config:
//
//	bls       -> 1 (PolicyQuorum)
//	ringtail  -> 5 (PolicyPQ, P+Q witness set, SHA-256 profile)
//	pulsar    -> 5 (PolicyPQ, P+Q witness set, SHA-3 profile)
//	quasar    -> 4 (PolicyQuantum, P+Q+Z witness set)
//	mldsa     -> 6 (PolicyPZ, P+Z witness set, per-validator ML-DSA)
//
// Ringtail and Pulsar share a wire PolicyID because the witness set on
// the wire is the same shape — the difference is the hash profile and
// DKG path, both negotiated out-of-band at network setup.
func (m PQMode) PolicyID() uint16 {
	switch m {
	case PQModeBLS:
		return 1 // PolicyQuorum
	case PQModeRingtail, PQModePulsar:
		return 5 // PolicyPQ
	case PQModeQuasar:
		return 4 // PolicyQuantum
	case PQModeMLDSA:
		return 6 // PolicyPZ
	default:
		return 1
	}
}

// HashSuiteID is the wire byte that identifies which hash family a Quasar
// finality certificate was produced under. It is a separate axis from
// PolicyID: Pulsar and Ringtail share PolicyID 5 (P+Q witness set) but use
// different hash families (SHA-3 vs BLAKE3), so the policy byte alone is
// not enough for a receiver to know which kernel to instantiate.
//
// HashSuiteID is bound into the cert transcript so that flipping the byte
// post-signing changes the digest the threshold signature covers — a
// cross-suite confusion attack therefore fails on signature verification,
// not just on a string-equality check. See HIP-0077 §"Lux consensus PQ
// modes" and proofs/pulsar/hash-suite-separation.tex for the formal
// argument.
//
// **Numbering policy (NIST-aligned).** SHA3_NIST is the normative entry
// (0x01) so reviewers reading a cert envelope see the FIPS-aligned suite
// first. BLAKE3 keeps its own ID (0x02) but is marked LEGACY because it
// is outside the FIPS 202 family; non-normative for any NIST-track
// submission. SHAKE256 does NOT get its own ID — SHAKE256 is FIPS 202
// and therefore part of SHA3_NIST. Modes whose only PQ hash is SHAKE256
// (e.g. raw ML-DSA-65) advertise SHA3_NIST.
//
// The byte is open-ended: future hash families MUST claim the next free
// ID and pin it in pq_mode_test.go before any code references it. Never
// reuse a retired ID. The wire reserves 0x00 for "no hash-family
// commitment" so legacy BLS-only certs round-trip cleanly.
type HashSuiteID uint8

const (
	// HashSuiteNone — no hash-family commitment. BLS-only modes whose
	// threshold layer is absent do not pin a hash suite at the consensus
	// layer; their kernels carry their own hash scoping internally.
	HashSuiteNone HashSuiteID = 0x00

	// HashSuiteSHA3NIST — the normative SP 800-185 hash family:
	// SHAKE256 + cSHAKE256 + KMAC256 + TupleHash256 (FIPS 202 +
	// NIST SP 800-185). The canonical Hanzo-mesh and Lux primary-network
	// choice. Every NIST-track Pulsar variant (Pulsar.R production
	// profile, Pulsar.M family) signs under this suite. ML-DSA-65 also
	// rides here because SHAKE256 is FIPS 202 and not a separate family.
	HashSuiteSHA3NIST HashSuiteID = 0x01

	// HashSuiteBLAKE3Legacy — Ringtail academic profile and Pulsar's
	// pre-pin legacy suite. BLAKE3 keyed XOF for every challenge / share
	// / transcript binding (see github.com/luxfi/ringtail
	// primitives/hash.go and github.com/luxfi/pulsar/hash/blake3.go).
	// BLAKE3 is outside FIPS 202 and therefore non-normative for any
	// NIST submission; the byte exists so legacy / academic / federation-
	// MPC deployments can still emit a verifiable cert.
	HashSuiteBLAKE3Legacy HashSuiteID = 0x02
)

// String returns the canonical lower-case name of the hash suite. Empty
// string for HashSuiteNone so cert printers can omit the line cleanly.
func (h HashSuiteID) String() string {
	switch h {
	case HashSuiteNone:
		return "none"
	case HashSuiteSHA3NIST:
		return "sha3-nist"
	case HashSuiteBLAKE3Legacy:
		return "blake3-legacy"
	default:
		return fmt.Sprintf("hash-suite(0x%02x)", uint8(h))
	}
}

// IsNormative reports whether this hash suite is on the NIST-approved
// algorithm path (FIPS 202 family). Production Hanzo meshes MUST emit
// only normative suites; legacy/academic suites are bridge-MPC + audit
// fixtures only.
func (h HashSuiteID) IsNormative() bool {
	return h == HashSuiteSHA3NIST
}

// HashSuiteID reports the wire HashSuiteID byte for this PQ mode. The
// byte is the receiver's only signal of which hash kernel to instantiate
// when verifying a cert built under this mode — Pulsar and Ringtail share
// PolicyID 5, so PolicyID alone cannot distinguish them.
//
// Closes HIP-0077 red-review F1 (silent finality forks between
// hash-profile islands sharing the same PolicyID).
//
// Mapping:
//
//	bls       -> HashSuiteNone          (0x00)  no hash-family commitment
//	ringtail  -> HashSuiteBLAKE3Legacy  (0x02)  academic profile, non-normative
//	pulsar    -> HashSuiteSHA3NIST      (0x01)  production profile (cSHAKE256 family)
//	quasar    -> HashSuiteSHA3NIST      (0x01)  same threshold-layer kernel as Pulsar
//	mldsa     -> HashSuiteSHA3NIST      (0x01)  ML-DSA-65 uses SHAKE256 (FIPS 202)
func (m PQMode) HashSuiteID() HashSuiteID {
	switch m {
	case PQModeBLS:
		return HashSuiteNone
	case PQModeRingtail:
		return HashSuiteBLAKE3Legacy
	case PQModePulsar, PQModeQuasar, PQModeMLDSA:
		return HashSuiteSHA3NIST
	default:
		return HashSuiteNone
	}
}

// HashProfile reports the canonical hash-family name for the mode. This
// is the human-readable string form of HashSuiteID and stays consistent
// with it for every mode: "none" / "sha3-nist" / "blake3-legacy".
// Receivers SHOULD compare HashSuiteID bytes (cheap, exact); HashProfile
// exists for logs, error messages, and cross-language wire-debug only.
func (m PQMode) HashProfile() string {
	return m.HashSuiteID().String()
}

// SigSchemeID is the wire byte that identifies which signature scheme
// produced the signature in this slot of a Quasar finality certificate.
// Orthogonal to PolicyID, HashSuiteID, and ProofSystemID.
//
// Bound into the cert transcript so a flipped byte breaks signature
// verification, not just type-equality.
//
// **Numbering blocks** (open-ended). The two ML-DSA-related blocks are
// kept distinct on purpose:
//
//   - 0x40 block names the **raw single-party** primitive (ML-DSA-44/65/87
//     per FIPS 204). Used when a per-validator signature appears in the
//     cert without threshold aggregation (mldsa audit-grade fallback,
//     identity attestations on Z-Chain, etc.).
//   - 0x50 block names the **Pulsar-M threshold variant** at the same
//     parameter sets. The verification *relation* is FIPS 204 ML-DSA.Verify
//     for both blocks, but the producing protocol differs (single-party
//     vs threshold) and the wire intentionally separates them so a
//     receiver knows which kernel emitted the bytes.
//
//	0x00       — None / unspecified (BLS-only certs do not commit to a sig scheme)
//	0x10..0x1F — Classical (BLS-12-381 aggregate at 0x10)
//	0x20..0x2F — Ringtail (academic R-LWE)
//	0x30..0x3F — Pulsar.R (production R-LWE family)
//	0x40..0x4F — Raw ML-DSA per FIPS 204 (single-party):
//	               0x41 = ML-DSA-44 (NIST PQ Cat 2)
//	               0x42 = ML-DSA-65 (NIST PQ Cat 3)
//	               0x43 = ML-DSA-87 (NIST PQ Cat 5)
//	0x50..0x5F — Pulsar-M threshold (M-LWE, FIPS 204-compatible):
//	               0x51 = Pulsar-M-44 (NIST PQ Cat 2, devnet only)
//	               0x52 = Pulsar-M-65 (NIST PQ Cat 3, **production default**)
//	               0x53 = Pulsar-M-87 (NIST PQ Cat 5, high-value roots)
//	0x60..      — reserved for future variants
//
// New entries claim the next free integer in their block and MUST land in
// pq_mode_test.go before any code references them. Never reuse a retired ID.
type SigSchemeID uint8

const (
	SigSchemeNone             SigSchemeID = 0x00
	SigSchemeBLS12381         SigSchemeID = 0x10
	SigSchemeRingtailAcademic SigSchemeID = 0x20
	SigSchemePulsarR          SigSchemeID = 0x30

	// Raw single-party ML-DSA (FIPS 204).
	SigSchemeMLDSA44 SigSchemeID = 0x41
	SigSchemeMLDSA65 SigSchemeID = 0x42
	SigSchemeMLDSA87 SigSchemeID = 0x43

	// Pulsar-M threshold (M-LWE, output-interchangeable with FIPS 204).
	SigSchemePulsarM44 SigSchemeID = 0x51
	SigSchemePulsarM65 SigSchemeID = 0x52 // production default
	SigSchemePulsarM87 SigSchemeID = 0x53
)

// String returns the canonical wire name of the signature scheme.
func (s SigSchemeID) String() string {
	switch s {
	case SigSchemeNone:
		return "none"
	case SigSchemeBLS12381:
		return "bls12-381"
	case SigSchemeRingtailAcademic:
		return "ringtail-academic"
	case SigSchemePulsarR:
		return "pulsar-r"
	case SigSchemeMLDSA44:
		return "ml-dsa-44"
	case SigSchemeMLDSA65:
		return "ml-dsa-65"
	case SigSchemeMLDSA87:
		return "ml-dsa-87"
	case SigSchemePulsarM44:
		return "pulsar-m-44"
	case SigSchemePulsarM65:
		return "pulsar-m-65"
	case SigSchemePulsarM87:
		return "pulsar-m-87"
	default:
		return fmt.Sprintf("sig-scheme(0x%02x)", uint8(s))
	}
}

// IsPulsarM reports whether this scheme is in the Pulsar-M threshold
// family (0x50 block). Pulsar-M outputs verify under unmodified FIPS 204
// ML-DSA.Verify; the threshold protocol that produced them is what this
// flag separates from raw ML-DSA.
func (s SigSchemeID) IsPulsarM() bool {
	return s == SigSchemePulsarM44 ||
		s == SigSchemePulsarM65 ||
		s == SigSchemePulsarM87
}

// IsRawMLDSA reports whether this scheme is in the raw single-party
// ML-DSA block (0x40). These signatures are produced by FIPS 204
// ML-DSA.Sign (one private key, no threshold protocol); the same
// verifier accepts them as IsPulsarM outputs.
func (s SigSchemeID) IsRawMLDSA() bool {
	return s == SigSchemeMLDSA44 ||
		s == SigSchemeMLDSA65 ||
		s == SigSchemeMLDSA87
}

// VerifiesUnderFIPS204 reports whether the unmodified FIPS 204 ML-DSA
// verifier will accept signatures emitted under this scheme. Both raw
// ML-DSA (0x40 block) and Pulsar-M (0x50 block) qualify; nothing else
// in the current enum does.
func (s SigSchemeID) VerifiesUnderFIPS204() bool {
	return s.IsRawMLDSA() || s.IsPulsarM()
}

// SigSchemeID reports the default threshold-signature scheme for this
// PQ mode. The default is the production-grade choice; operators MAY
// override (e.g. Pulsar-M-87 for governance checkpoints) by passing an
// explicit SigSchemeID through the cert assembler.
//
// Mapping (HIP-0077 production defaults):
//
//	bls       -> SigSchemeNone               BLS aggregate carried separately
//	ringtail  -> SigSchemeRingtailAcademic   federation MPC
//	pulsar    -> SigSchemePulsarR            R-LWE production
//	quasar    -> SigSchemePulsarR            threshold layer is Pulsar.R; Z-Chain witness separate
//	mldsa     -> SigSchemeMLDSA65            raw ML-DSA-65 (audit-grade); ops opt-in to Pulsar-M-65 explicitly
func (m PQMode) SigSchemeID() SigSchemeID {
	switch m {
	case PQModeBLS:
		return SigSchemeNone
	case PQModeRingtail:
		return SigSchemeRingtailAcademic
	case PQModePulsar, PQModeQuasar:
		return SigSchemePulsarR
	case PQModeMLDSA:
		return SigSchemeMLDSA65
	default:
		return SigSchemeNone
	}
}

// ProofPolicyID is the wire byte that identifies the **security class**
// (policy) the proof system implements. Orthogonal to ProofBackendID
// (which names the implementation). Strict-PQ deployments MUST accept
// only IDs whose `IsPostQuantum()` returns true; classical IDs are
// reserved as explicit forbidden markers so audit tooling can name a
// misconfiguration precisely.
//
// Numbering:
//
//	0x00       — None
//	0x10..0x1F — STARK / FRI policies (PQ-secure)
//	               0x10 = STARK_FRI_SHA3_PQ          canonical, NIST-aligned
//	               0x11 = STARK_FRI_KECCAK_PQ        Keccak Merkle (FIPS 202; non-NIST-recommended-default)
//	0x80..0xFF — Classical / forbidden in PQ mode
//	               0x80 = GROTH16_BN254_CLASSICAL
//	               0x81 = PLONK_KZG_CLASSICAL
//	               0x82 = HALO_EC_CLASSICAL
//	               0x83 = IPA_EC_CLASSICAL
//
// `ProofSystemID` is a deprecated alias for `ProofPolicyID` for one
// release; new code uses `ProofPolicyID`.
type ProofPolicyID uint8

// ProofSystemID is a deprecated type alias for ProofPolicyID. Existing
// callers continue to work; new code uses ProofPolicyID. Removed after
// the 2026-Aug-31 encoding-freeze gate.
type ProofSystemID = ProofPolicyID

const (
	ProofPolicyNone           ProofPolicyID = 0x00
	ProofPolicySTARKFRISHA3PQ ProofPolicyID = 0x10 // canonical Z-Chain policy
	ProofPolicySTARKFRIKeccak ProofPolicyID = 0x11 // FIPS 202 Keccak alternative

	// Forbidden in strict-PQ mode. The wire keeps an ID for each so
	// audit tooling can name a misconfiguration explicitly.
	ProofPolicyGroth16BN254Forbid ProofPolicyID = 0x80
	ProofPolicyPLONKKZGForbid     ProofPolicyID = 0x81
	ProofPolicyHaloECForbid       ProofPolicyID = 0x82
	ProofPolicyIPAECForbid        ProofPolicyID = 0x83

	// Legacy aliases — keep callers compiling through the rename.
	// New code uses the ProofPolicy* names.
	ProofSystemNone               = ProofPolicyNone
	ProofSystemSTARKFRISHA3PQ     = ProofPolicySTARKFRISHA3PQ
	ProofSystemSTARKFRIKeccakPQ   = ProofPolicySTARKFRIKeccak
	ProofSystemGroth16BN254Forbid = ProofPolicyGroth16BN254Forbid
	ProofSystemKZGForbid          = ProofPolicyPLONKKZGForbid
)

// String returns the canonical wire name.
func (p ProofPolicyID) String() string {
	switch p {
	case ProofPolicyNone:
		return "none"
	case ProofPolicySTARKFRISHA3PQ:
		return "stark-fri-sha3-pq"
	case ProofPolicySTARKFRIKeccak:
		return "stark-fri-keccak-pq"
	case ProofPolicyGroth16BN254Forbid:
		return "groth16-bn254-classical-forbidden-in-pq"
	case ProofPolicyPLONKKZGForbid:
		return "plonk-kzg-classical-forbidden-in-pq"
	case ProofPolicyHaloECForbid:
		return "halo-ec-classical-forbidden-in-pq"
	case ProofPolicyIPAECForbid:
		return "ipa-ec-classical-forbidden-in-pq"
	default:
		return fmt.Sprintf("proof-policy(0x%02x)", uint8(p))
	}
}

// IsPostQuantum reports whether this policy is acceptable in strict-PQ
// mode. Strict-PQ producers MUST emit only IsPostQuantum=true policies;
// verifiers in strict-PQ mode MUST refuse anything else.
func (p ProofPolicyID) IsPostQuantum() bool {
	switch p {
	case ProofPolicySTARKFRISHA3PQ,
		ProofPolicySTARKFRIKeccak:
		return true
	}
	return false
}

// IsForbiddenInPQMode reports whether this proof policy carries the
// explicit forbidden marker. Used by audit tooling to detect a
// misconfiguration where a classical proof would otherwise sneak past
// a PQ-only deployment.
func (p ProofPolicyID) IsForbiddenInPQMode() bool {
	return p == ProofPolicyGroth16BN254Forbid ||
		p == ProofPolicyPLONKKZGForbid ||
		p == ProofPolicyHaloECForbid ||
		p == ProofPolicyIPAECForbid
}

// ProofBackendID is the wire byte that identifies which **implementation**
// produced a Z-Chain proof. Orthogonal to ProofPolicyID (which names the
// security class). This separation lets Z-Chain benchmark SP1, RISC Zero,
// P3Q, Stone (Cairo), and Stwo (Circle STARK) without changing chain
// semantics — they all satisfy the same ProofPolicyID, just with
// different underlying implementations.
//
// Numbering:
//
//	0x00       — None (no backend; e.g. policy-only declarations)
//	0x10..0x1F — STARK / FRI implementations (production)
//	               0x20 = RISC0_SUCCINCT_STARK_PQ   RISC Zero succinct receipt path (no Groth16 wrapper)
//	               0x21 = SP1_COMPRESSED_STARK_PQ    SP1 compressed STARK (no Groth16 wrapper)
//	               0x22 = P3Q_STARK_FRI_SHA3_PQ     Lux P3Q (Plonky3 fork, cSHAKE256 Merkle)
//	               0x23 = STONE_CAIRO_STARK_PQ      Cairo / Stone backend
//	               0x24 = STWO_CIRCLE_STARK_PQ      Stwo Circle STARK
//	0x70..0x7F — Dev-only / non-production (CI, testnet, audit fuzzing)
//	               0x70 = RISC0_RAW_STARK_DEV
//	               0x71 = SP1_CORE_STARK_DEV
//	0x80..0xFF — Forbidden in strict-PQ mode (mirror of ProofPolicyID
//	             forbidden block; backends that wrap classical wrappers
//	             carry the same explicit refusal markers).
type ProofBackendID uint8

const (
	ProofBackendNone               ProofBackendID = 0x00
	ProofBackendRISC0SuccinctSTARK ProofBackendID = 0x20
	ProofBackendSP1CompressedSTARK ProofBackendID = 0x21
	ProofBackendP3QSTARKFRISHA3    ProofBackendID = 0x22
	ProofBackendStoneCairoSTARK    ProofBackendID = 0x23
	ProofBackendStwoCircleSTARK    ProofBackendID = 0x24

	// Dev-only — never produced in production strict-PQ deployments.
	ProofBackendRISC0RawSTARKDev ProofBackendID = 0x70
	ProofBackendSP1CoreSTARKDev  ProofBackendID = 0x71

	// Forbidden in strict-PQ mode. Mirrors ProofPolicyID forbidden block
	// at the backend layer (e.g. a backend that wraps STARK in Groth16
	// for cheaper EVM verification).
	ProofBackendGroth16WrapForbid ProofBackendID = 0x80
	ProofBackendKZGWrapForbid     ProofBackendID = 0x81
)

// String returns the canonical wire name.
func (b ProofBackendID) String() string {
	switch b {
	case ProofBackendNone:
		return "none"
	case ProofBackendRISC0SuccinctSTARK:
		return "risc0-succinct-stark-pq"
	case ProofBackendSP1CompressedSTARK:
		return "sp1-compressed-stark-pq"
	case ProofBackendP3QSTARKFRISHA3:
		return "p3q-stark-fri-sha3-pq"
	case ProofBackendStoneCairoSTARK:
		return "stone-cairo-stark-pq"
	case ProofBackendStwoCircleSTARK:
		return "stwo-circle-stark-pq"
	case ProofBackendRISC0RawSTARKDev:
		return "risc0-raw-stark-dev"
	case ProofBackendSP1CoreSTARKDev:
		return "sp1-core-stark-dev"
	case ProofBackendGroth16WrapForbid:
		return "groth16-wrap-classical-forbidden-in-pq"
	case ProofBackendKZGWrapForbid:
		return "kzg-wrap-classical-forbidden-in-pq"
	default:
		return fmt.Sprintf("proof-backend(0x%02x)", uint8(b))
	}
}

// IsProductionPQ reports whether this backend is acceptable in strict-PQ
// production mode. Dev backends (0x70 block) and forbidden backends
// (0x80 block) return false.
func (b ProofBackendID) IsProductionPQ() bool {
	switch b {
	case ProofBackendRISC0SuccinctSTARK,
		ProofBackendSP1CompressedSTARK,
		ProofBackendP3QSTARKFRISHA3,
		ProofBackendStoneCairoSTARK,
		ProofBackendStwoCircleSTARK:
		return true
	}
	return false
}

// IsForbiddenInPQMode mirrors ProofPolicyID.IsForbiddenInPQMode at the
// backend layer.
func (b ProofBackendID) IsForbiddenInPQMode() bool {
	return b == ProofBackendGroth16WrapForbid ||
		b == ProofBackendKZGWrapForbid
}

// IdentitySchemeID is the wire byte that identifies which signature scheme
// a validator uses for **identity** (registration, key rotation,
// revocation, attestation) — distinct from the threshold-finality
// SigSchemeID. Identity signatures are single-party, FIPS 204 ML-DSA.
//
// Numbering — block 0x40 mirrors raw ML-DSA in SigSchemeID so the byte
// pattern is consistent across the wire.
//
//	0x00 — None (no identity scheme committed; legacy / anonymous)
//	0x41 — ML_DSA_44 (NIST PQ Cat 2; devnet only)
//	0x42 — ML_DSA_65 (NIST PQ Cat 3; **production identity default**)
//	0x43 — ML_DSA_87 (NIST PQ Cat 5; high-value root identities)
type IdentitySchemeID uint8

const (
	IdentitySchemeNone    IdentitySchemeID = 0x00
	IdentitySchemeMLDSA44 IdentitySchemeID = 0x41
	IdentitySchemeMLDSA65 IdentitySchemeID = 0x42 // production default
	IdentitySchemeMLDSA87 IdentitySchemeID = 0x43
)

// String returns the canonical wire name.
func (i IdentitySchemeID) String() string {
	switch i {
	case IdentitySchemeNone:
		return "none"
	case IdentitySchemeMLDSA44:
		return "ml-dsa-44"
	case IdentitySchemeMLDSA65:
		return "ml-dsa-65"
	case IdentitySchemeMLDSA87:
		return "ml-dsa-87"
	default:
		return fmt.Sprintf("identity-scheme(0x%02x)", uint8(i))
	}
}

// IsFIPS204 reports whether this identity scheme uses unmodified
// FIPS 204 ML-DSA verification. All defined non-None values do; the
// helper exists so audit tooling can assert this property explicitly.
func (i IdentitySchemeID) IsFIPS204() bool {
	return i == IdentitySchemeMLDSA44 ||
		i == IdentitySchemeMLDSA65 ||
		i == IdentitySchemeMLDSA87
}

// Note: ProofFormatID and VerifierID are defined in security_profile.go
// because they are part of the locked-profile axis set, not the
// mode-selection enum group above. Look there for their numbering tables
// and behaviour predicates.

// DKGRequired reports whether an open public chain can run this mode.
// Modes whose threshold layer relies on a trusted dealer (Ringtail) cannot;
// modes with no threshold (BLS, MLDSA) are vacuously fine; modes with
// proper DKG (Pulsar, Quasar) are the production target.
func (m PQMode) DKGRequired() string {
	switch m {
	case PQModeBLS, PQModeMLDSA:
		return "none"
	case PQModeRingtail:
		return "trusted-dealer" // unsuitable for open public chains
	case PQModePulsar, PQModeQuasar:
		return "pedersen-dkg-over-rq"
	default:
		return "unknown"
	}
}

// ParsePQMode parses a canonical mode string (case-insensitive). Every
// accepted alias names an actual component in the cert; we deliberately
// don't accept counting words ("triple", "double") because they tell
// callers nothing about what's signed.
//
//	canonical                              aliases (component-named only)
//	"bls"            (BLS)                 "classical", "bls-only"
//	"ringtail"       (BLS + Ringtail)      "rt", "academic", "bls-rt", "bls-q", "sha256-rt"
//	"pulsar"         (BLS + Pulsar)        "sha3-rt", "production-rt", "bls-pulsar"
//	"quasar"         (BLS + Pulsar + Z)    "rollup", "groth16", "zk",
//	                                       "bls-z", "bls-zk", "bls-groth16",
//	                                       "z-chain", "pulsar-z"
//	"mldsa"          (BLS + ML-DSA raw)    "audit", "bls-mldsa", "bls+mldsa"
func ParsePQMode(s string) (PQMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "bls", "bls-only", "classical":
		return PQModeBLS, nil
	case "ringtail", "rt", "academic", "bls-rt", "bls-q", "sha256-rt":
		return PQModeRingtail, nil
	case "pulsar", "sha3-rt", "production-rt", "bls-pulsar":
		return PQModePulsar, nil
	case "quasar", "rollup", "groth16", "zk",
		"bls-z", "bls-zk", "bls-groth16", "z-chain", "pulsar-z":
		return PQModeQuasar, nil
	case "mldsa", "audit", "bls-mldsa", "bls+mldsa":
		return PQModeMLDSA, nil
	default:
		return PQModeBLS, fmt.Errorf("unknown PQMode %q", s)
	}
}

// PQModeFromEnv reads LUX_CONSENSUS_PQ_MODE and returns the resolved mode.
// Empty / unset returns the supplied default. Invalid values return def + error.
func PQModeFromEnv(def PQMode) (PQMode, error) {
	v := os.Getenv("LUX_CONSENSUS_PQ_MODE")
	if v == "" {
		return def, nil
	}
	mode, err := ParsePQMode(v)
	if err != nil {
		return def, err
	}
	return mode, nil
}

// PQModeFromBool collapses the enum onto a single boolean for operators
// who don't want to pick a mode explicitly:
//
//	true  -> PQModeQuasar   // BLS + Pulsar + Z-Chain Groth16(ML-DSA)
//	false -> PQModeBLS      // classical fast path
//
// Use the explicit constants for the middle modes (Ringtail, Pulsar, MLDSA).
func PQModeFromBool(postQuantum bool) PQMode {
	if postQuantum {
		return PQModeQuasar
	}
	return PQModeBLS
}

// IsPostQuantum reports whether the mode carries any PQ witness on top of
// BLS. False only for PQModeBLS.
func (m PQMode) IsPostQuantum() bool {
	return m != PQModeBLS
}

// SuitableForPublicChain reports whether the mode is appropriate for an
// open public chain (epoch rotation, no trusted dealer). False for
// PQModeRingtail (trusted-dealer DKG only) and PQModeBLS (no PQ at all);
// true for everything else.
func (m PQMode) SuitableForPublicChain() bool {
	switch m {
	case PQModePulsar, PQModeQuasar, PQModeMLDSA:
		return true
	default:
		return false
	}
}
