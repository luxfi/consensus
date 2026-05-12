// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"
)

// =============================================================================
// CORE INVARIANT: Everything is a Candidate
// =============================================================================

// CandidateID is a 32-byte content-addressed identifier.
// candidate_id = H(domain || payload_bytes)
// This ensures "same decision" is objectively the same ID regardless of proposer.
type CandidateID [32]byte

// EmptyCandidateID is the zero candidate ID
var EmptyCandidateID CandidateID

// Candidate represents anything being sequenced: blocks, transactions,
// AI decisions, state transitions, etc.
//
// Core invariant: ID = H(Domain || Payload)
type Candidate struct {
	// ID is the content-addressed identifier (computed from Domain + Payload)
	ID CandidateID `json:"id"`

	// ParentID links to the previous candidate (optional for DAG/genesis)
	ParentID CandidateID `json:"parent_id,omitempty"`

	// Height is the sequence number / slot / round
	Height uint64 `json:"height"`

	// Domain identifies the context (chain_id, network_id, "ai-mesh", etc.)
	Domain []byte `json:"domain"`

	// Payload is the actual content being ordered
	// For blocks: serialized transactions
	// For AI: decision bundle / synthesis text
	Payload []byte `json:"payload"`

	// DARef is where the full payload bytes live (for DA layer)
	// Can be: local path, IPFS CID, blob hash, Lux warp ref, etc.
	DARef string `json:"da_ref,omitempty"`

	// Meta contains additional metadata
	Meta CandidateMeta `json:"meta"`
}

// CandidateMeta holds optional metadata
type CandidateMeta struct {
	// ProposerID is who proposed this candidate
	ProposerID VoterID `json:"proposer_id,omitempty"`

	// TimestampMs is when the candidate was created
	TimestampMs int64 `json:"timestamp_ms"`

	// ChainID for multi-chain contexts
	ChainID []byte `json:"chain_id,omitempty"`

	// Extra is for domain-specific extensions
	Extra []byte `json:"extra,omitempty"`
}

// NewCandidate creates a candidate with computed ID
func NewCandidate(domain, payload []byte, parent CandidateID, height uint64) *Candidate {
	c := &Candidate{
		ParentID: parent,
		Height:   height,
		Domain:   domain,
		Payload:  payload,
		Meta: CandidateMeta{
			TimestampMs: time.Now().UnixMilli(),
		},
	}
	c.ID = c.ComputeID()
	return c
}

// ComputeID calculates the content-addressed ID: H(domain || payload)
func (c *Candidate) ComputeID() CandidateID {
	h := sha256.New()
	h.Write(c.Domain)
	h.Write(c.Payload)
	var id CandidateID
	copy(id[:], h.Sum(nil))
	return id
}

// Verify checks that the ID matches the content
func (c *Candidate) Verify() bool {
	return c.ID == c.ComputeID()
}

// =============================================================================
// ATTESTATIONS: Who agrees
// =============================================================================

// Vote represents an attestation on a candidate
type Vote struct {
	// CandidateID is what's being voted on
	CandidateID CandidateID `json:"candidate_id"`

	// VoterID is who is voting
	VoterID VoterID `json:"voter_id"`

	// Round is the voting round (for multi-round protocols)
	Round uint64 `json:"round"`

	// Preference is the vote direction (true=accept, false=reject)
	Preference bool `json:"preference"`

	// Signature is scheme-tagged: first byte indicates scheme
	// 0x00 = none, 0x01 = Ed25519, 0x02 = BLS, 0x03 = Ringtail, 0x04 = Quasar
	Signature []byte `json:"signature,omitempty"`

	// TimestampMs for ordering
	TimestampMs int64 `json:"timestamp_ms"`
}

// Signature scheme tags
const (
	SigNone     byte = 0x00
	SigEd25519  byte = 0x01
	SigBLS      byte = 0x02
	SigRingtail byte = 0x03
	SigQuasar   byte = 0x04 // BLS + Ringtail (Quasar protocol)
)

// NewVote creates a vote with current timestamp
func NewVote(candidateID CandidateID, voterID VoterID, round uint64, preference bool) *Vote {
	return &Vote{
		CandidateID: candidateID,
		VoterID:     voterID,
		Round:       round,
		Preference:  preference,
		TimestampMs: time.Now().UnixMilli(),
	}
}

// SignatureScheme returns the signature scheme tag
func (v *Vote) SignatureScheme() byte {
	if len(v.Signature) == 0 {
		return SigNone
	}
	return v.Signature[0]
}

// =============================================================================
// CERTIFICATES: Proof of agreement
// =============================================================================

// PolicyID identifies the finality policy used
type PolicyID uint16

const (
	// PolicyNone - no finality required (K=1 self-sequencing)
	PolicyNone PolicyID = 0

	// PolicyQuorum - threshold signature (3/5, 2/3, etc.).
	// Witness set: {P}. P-Chain BLS only, the default.
	PolicyQuorum PolicyID = 1

	// PolicySampleConvergence - metastable sampling (large N)
	PolicySampleConvergence PolicyID = 2

	// PolicyL1Inclusion - external chain inclusion (OP Stack)
	PolicyL1Inclusion PolicyID = 3

	// PolicyQuantum - parallel-witness PQ finality, witness set {P, Q, Z}.
	// Maximum security level: P-Chain BLS + Q-Chain Ringtail + Z-Chain MLDSAGroth16.
	// All three witnesses run in parallel.
	PolicyQuantum PolicyID = 4

	// PolicyPQ - parallel-witness finality, witness set {P, Q}.
	// P-Chain BLS + Q-Chain Ringtail threshold. No Z-Chain rollup.
	PolicyPQ PolicyID = 5

	// PolicyPZ - parallel-witness finality, witness set {P, Z}.
	// P-Chain BLS + Z-Chain MLDSAGroth16 rollup. No Q-Chain ceremony.
	PolicyPZ PolicyID = 6
)

// =============================================================================
// PARALLEL-WITNESS FINALITY MODEL (LP-020 Quasar)
// =============================================================================
//
// Lux finality is layered, parallel witnesses. P-Chain BLS is always required.
// Q-Chain (Ringtail threshold) and Z-Chain (MLDSAGroth16 rollup) are
// independently toggleable parallel witnesses producing additional finality
// artifacts at the same round-rate as P. Adding witnesses does not increase
// per-block latency, only parallel verification cost.
// =============================================================================

// FinalityWitnesses is a bitset selecting which parallel finality witnesses
// must sign each round. WitnessP is always required.
type FinalityWitnesses uint8

const (
	// WitnessP - P-Chain BLS aggregate. Always required.
	WitnessP FinalityWitnesses = 1 << 0
	// WitnessQ - Q-Chain Ringtail threshold (Module-LWE, eprint 2024/1113).
	WitnessQ FinalityWitnesses = 1 << 1
	// WitnessZ - Z-Chain MLDSAGroth16 rollup (per-validator ML-DSA-65
	// aggregated via Groth16 SNARK).
	WitnessZ FinalityWitnesses = 1 << 2
)

// Witnesses returns the FinalityWitnesses bitset for a given PolicyID.
// Returns 0 for non-witnessed policies (None, Sample, L1).
func (p PolicyID) Witnesses() FinalityWitnesses {
	switch p {
	case PolicyQuorum:
		return WitnessP
	case PolicyPQ:
		return WitnessP | WitnessQ
	case PolicyPZ:
		return WitnessP | WitnessZ
	case PolicyQuantum:
		return WitnessP | WitnessQ | WitnessZ
	default:
		return 0
	}
}

// PolicyForWitnesses returns the canonical PolicyID for a witness set.
// Valid sets: {P}, {P,Q}, {P,Z}, {P,Q,Z}. Returns PolicyNone if invalid
// (WitnessP missing, or unknown bits set).
func PolicyForWitnesses(w FinalityWitnesses) PolicyID {
	if w&WitnessP == 0 {
		return PolicyNone
	}
	switch w {
	case WitnessP:
		return PolicyQuorum
	case WitnessP | WitnessQ:
		return PolicyPQ
	case WitnessP | WitnessZ:
		return PolicyPZ
	case WitnessP | WitnessQ | WitnessZ:
		return PolicyQuantum
	default:
		return PolicyNone
	}
}

// Has reports whether w includes every witness in required.
func (w FinalityWitnesses) Has(required FinalityWitnesses) bool {
	return w&required == required
}

// Validate returns nil iff WitnessP is set and no unknown bits are present.
func (w FinalityWitnesses) Validate() error {
	if w&WitnessP == 0 {
		return ErrWitnessPRequired
	}
	if w & ^(WitnessP|WitnessQ|WitnessZ) != 0 {
		return ErrWitnessUnknown
	}
	return nil
}

// ErrWitnessPRequired is returned when a witness set lacks the mandatory
// P-Chain BLS witness.
var ErrWitnessPRequired = errors.New("WitnessP required: P-Chain BLS is the always-on finality witness")

// ErrWitnessUnknown is returned when a witness set contains undefined bits.
var ErrWitnessUnknown = errors.New("witness set contains unknown bits")

// HashSuiteID identifies which hash family a Quasar finality certificate was
// produced under. It is a separate axis from PolicyID: Pulsar (SHA3_NIST)
// and Ringtail (BLAKE3-legacy) share PolicyPQ (5) but use different hash
// kernels. PolicyID alone is therefore insufficient to know which verifier to
// instantiate — the receiver must consult HashSuiteID.
//
// HashSuiteID is bound into the cert transcript (see TranscriptHash) so any
// post-signing mutation of the byte changes the digest the threshold
// signature covers; cross-suite confusion attacks fail on signature verify,
// not just on a string/byte-equality check.
//
// Closes HIP-0077 red-review F1: silent finality forks between hash-profile
// islands sharing the same PolicyID. Mirrors the Warp 2.0 envelope pattern
// (lux/warp/envelope.go HashSuiteID) at the consensus layer so the two
// envelopes don't drift.
//
// **Numbering is NIST-aligned.** SHA3_NIST is the normative entry (0x01),
// BLAKE3 is non-normative legacy (0x02), and SHAKE256 does not get its own
// ID because SHAKE256 is FIPS 202 and therefore part of SHA3_NIST.
// Numbering is open-ended; future hash families claim the next free ID
// and pin it in a config table-driven test. Never reuse a retired ID.
//
// Wire-side values must match config.HashSuiteID exactly (the producer
// surfaces config.HashSuiteID through RoundWitnesses; the cert assembler
// stamps the envelope with `wire.HashSuiteID(rw.HashSuiteID)`).
type HashSuiteID uint8

const (
	// HashSuiteNone — no hash-family commitment. Used by BLS-only certs and
	// by intermediate states that haven't been signed under any PQ kernel.
	HashSuiteNone HashSuiteID = 0x00

	// HashSuiteSHA3NIST — the normative SP 800-185 hash family: SHAKE256 +
	// cSHAKE256 + KMAC256 + TupleHash256 (FIPS 202 + NIST SP 800-185).
	// Pulsar / Quasar / raw ML-DSA-65 all sign under this suite.
	HashSuiteSHA3NIST HashSuiteID = 0x01

	// HashSuiteBLAKE3Legacy — Ringtail academic profile and Pulsar's pre-pin
	// legacy suite. BLAKE3 is outside FIPS 202 and non-normative for any
	// NIST submission; the byte exists so legacy / academic / federation-MPC
	// deployments can still emit a verifiable cert.
	HashSuiteBLAKE3Legacy HashSuiteID = 0x02
)

// String returns the canonical lower-case name of the hash suite.
func (h HashSuiteID) String() string {
	switch h {
	case HashSuiteNone:
		return "none"
	case HashSuiteSHA3NIST:
		return "sha3-nist"
	case HashSuiteBLAKE3Legacy:
		return "blake3-legacy"
	default:
		return "hash-suite(unknown)"
	}
}

// SigSchemeID identifies which threshold signature scheme produced the
// threshold-layer signature in this certificate. Orthogonal to PolicyID
// and HashSuiteID; bound into the transcript so a flipped byte breaks
// signature verification.
//
// Numbering blocks: 0x00 None / 0x10..0x1F classical (BLS) / 0x20..0x2F
// Ringtail / 0x30..0x3F Pulsar.R / 0x40..0x4F Pulsar.M (low nibble pins
// the parameter set).
//
// Wire-side values must match config.SigSchemeID exactly.
type SigSchemeID uint8

const (
	SigSchemeNone             SigSchemeID = 0x00
	SigSchemeBLS12381         SigSchemeID = 0x10
	SigSchemeRingtailAcademic SigSchemeID = 0x20
	SigSchemePulsarR          SigSchemeID = 0x30
	SigSchemePulsarM44        SigSchemeID = 0x41
	SigSchemePulsarM65        SigSchemeID = 0x42 // production default
	SigSchemePulsarM87        SigSchemeID = 0x43
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
	case SigSchemePulsarM44:
		return "pulsar-m-44"
	case SigSchemePulsarM65:
		return "pulsar-m-65"
	case SigSchemePulsarM87:
		return "pulsar-m-87"
	default:
		return "sig-scheme(unknown)"
	}
}

// Certificate is the proof of finalized agreement
type Certificate struct {
	// CandidateID is what was finalized
	CandidateID CandidateID `json:"candidate_id"`

	// Height at finalization
	Height uint64 `json:"height"`

	// PolicyID identifies how finality was achieved
	PolicyID PolicyID `json:"policy_id"`

	// HashSuiteID identifies which hash family this cert was produced under.
	// Bound into TranscriptHash so post-sign mutation breaks signature
	// verification — see HIP-0077 F1 fix and TranscriptHash below. Zero is
	// the explicit "no hash-family commitment" value (PolicyQuorum / BLS-only
	// certs); non-zero modes MUST set this to match the producing PQMode.
	HashSuiteID HashSuiteID `json:"hash_suite_id"`

	// Proof is policy-specific (scheme-tagged bytes)
	// For Quorum: aggregated signature + signer bitmap
	// For L1Inclusion: merkle proof + block hash
	// For SampleConvergence: confidence score + sample proofs
	Proof []byte `json:"proof"`

	// Signers is a bitmap or list of who attested
	Signers []byte `json:"signers,omitempty"`

	// TimestampMs when certificate was created
	TimestampMs int64 `json:"timestamp_ms"`
}

// NewCertificate creates a certificate. HashSuiteID defaults to HashSuiteNone;
// callers using PolicyPQ / PolicyPZ / PolicyQuantum MUST set it explicitly to
// match the producing PQMode (config.PQMode.HashSuiteID()).
func NewCertificate(candidateID CandidateID, height uint64, policy PolicyID, proof []byte) *Certificate {
	return &Certificate{
		CandidateID: candidateID,
		Height:      height,
		PolicyID:    policy,
		Proof:       proof,
		TimestampMs: time.Now().UnixMilli(),
	}
}

// NewCertificateWithSuite creates a certificate with an explicit HashSuiteID.
// Use this from PQ producers to bind the suite into the cert at construction
// time; callers using BLS-only PolicyQuorum can pass HashSuiteNone or use the
// HashSuiteID-free NewCertificate.
func NewCertificateWithSuite(candidateID CandidateID, height uint64, policy PolicyID, suite HashSuiteID, proof []byte) *Certificate {
	return &Certificate{
		CandidateID: candidateID,
		Height:      height,
		PolicyID:    policy,
		HashSuiteID: suite,
		Proof:       proof,
		TimestampMs: time.Now().UnixMilli(),
	}
}

// TranscriptHash returns the 32-byte digest that bound this certificate at
// signing time. The hash binds every envelope field that fixes the cert's
// meaning — including HashSuiteID — so that flipping the suite byte after
// signing changes the digest the threshold signature covers and the
// signature fails to verify.
//
// Layout (big-endian, length-prefixed where variable):
//
//	"CertTranscript/v1" || domain-sep
//	candidate_id (32 B)
//	height       (uint64 BE, 8 B)
//	policy_id    (uint16 BE, 2 B)
//	hash_suite   (uint8,     1 B)
//	proof_len    (uint32 BE, 4 B) || proof
//	signers_len  (uint32 BE, 4 B) || signers
//
// TimestampMs is deliberately excluded: it is informational metadata, not
// part of the agreement that the signature covers.
func (c *Certificate) TranscriptHash() [32]byte {
	h := sha256.New()
	h.Write([]byte("CertTranscript/v1"))
	h.Write(c.CandidateID[:])

	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], c.Height)
	h.Write(u64[:])

	var u16 [2]byte
	binary.BigEndian.PutUint16(u16[:], uint16(c.PolicyID))
	h.Write(u16[:])

	h.Write([]byte{byte(c.HashSuiteID)})

	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(len(c.Proof)))
	h.Write(u32[:])
	h.Write(c.Proof)

	binary.BigEndian.PutUint32(u32[:], uint32(len(c.Signers)))
	h.Write(u32[:])
	h.Write(c.Signers)

	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// =============================================================================
// TWO-PHASE AGREEMENT: Soft → Hard
// =============================================================================

// Phase represents the agreement phase
type Phase uint8

const (
	// PhaseSoft is fast, optimistic finality
	// Can be: metastable sampling, leader quorum, sequencer head
	PhaseSoft Phase = 1

	// PhaseHard is slow, strong finality
	// Can be: threshold certificate, L1 inclusion, PQ threshold
	PhaseHard Phase = 2
)

// AgreementState tracks two-phase agreement
type AgreementState struct {
	CandidateID CandidateID `json:"candidate_id"`

	// Soft finality
	SoftFinalized bool         `json:"soft_finalized"`
	SoftCert      *Certificate `json:"soft_cert,omitempty"`

	// Hard finality
	HardFinalized bool         `json:"hard_finalized"`
	HardCert      *Certificate `json:"hard_cert,omitempty"`
}

// =============================================================================
// VOTER IDENTITY
// =============================================================================
// Single canonical derivation: VoterID = H(domain || data)
// Domain separator ensures VoterID == NodeID when using same domain.
// =============================================================================

// NodeIDDomain is the canonical domain separator (matches node repo)
const NodeIDDomain = "SignerNodeID/v1"

// VoterID is a 32-byte voter identifier
type VoterID [32]byte

// EmptyVoterID is the zero voter ID
var EmptyVoterID VoterID

// DeriveVoterID is the single canonical derivation function.
// VoterID = H(domain || data)
//
// For validators: DeriveVoterID(NodeIDDomain, mldsaPublicKey)
// For AI agents:  DeriveVoterID("agent", []byte(agentName))
func DeriveVoterID(domain string, data []byte) VoterID {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write(data)
	var v VoterID
	copy(v[:], h.Sum(nil))
	return v
}

// VoterIDFromPublicKey derives VoterID from any public key using NodeIDDomain.
// This ensures VoterID == NodeID for the same public key.
func VoterIDFromPublicKey(publicKey []byte) VoterID {
	return DeriveVoterID(NodeIDDomain, publicKey)
}

// ItemID is a content-addressed item identifier.
type ItemID = CandidateID

// DeriveItemID derives an ItemID from arbitrary data
func DeriveItemID(data []byte) ItemID {
	return sha256.Sum256(data)
}

// =============================================================================
// SERIALIZATION HELPERS
// =============================================================================

// Bytes serializes a CandidateID to bytes
func (id CandidateID) Bytes() []byte {
	return id[:]
}

// Bytes serializes a VoterID to bytes
func (id VoterID) Bytes() []byte {
	return id[:]
}

// Uint64ToBytes converts uint64 to big-endian bytes
func Uint64ToBytes(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

// BytesToUint64 converts big-endian bytes to uint64
func BytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
