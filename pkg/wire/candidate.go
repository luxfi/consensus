// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"crypto/sha256"
	"encoding/binary"
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
	// 0x00 = none, 0x01 = Ed25519, 0x02 = BLS, 0x03 = Ringtail, 0x04 = hybrid
	Signature []byte `json:"signature,omitempty"`

	// TimestampMs for ordering
	TimestampMs int64 `json:"timestamp_ms"`
}

// Signature scheme tags
const (
	SigNone   byte = 0x00
	SigEd25519 byte = 0x01
	SigBLS    byte = 0x02
	SigRingtail byte = 0x03
	SigHybrid byte = 0x04 // BLS + Ringtail
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

	// PolicyQuorum - threshold signature (3/5, 2/3, etc.)
	PolicyQuorum PolicyID = 1

	// PolicySampleConvergence - metastable sampling (large N)
	PolicySampleConvergence PolicyID = 2

	// PolicyL1Inclusion - external chain inclusion (OP Stack)
	PolicyL1Inclusion PolicyID = 3

	// PolicyQuantum - BLS + Ringtail post-quantum
	PolicyQuantum PolicyID = 4
)

// Certificate is the proof of finalized agreement
type Certificate struct {
	// CandidateID is what was finalized
	CandidateID CandidateID `json:"candidate_id"`

	// Height at finalization
	Height uint64 `json:"height"`

	// PolicyID identifies how finality was achieved
	PolicyID PolicyID `json:"policy_id"`

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

// NewCertificate creates a certificate
func NewCertificate(candidateID CandidateID, height uint64, policy PolicyID, proof []byte) *Certificate {
	return &Certificate{
		CandidateID: candidateID,
		Height:      height,
		PolicyID:    policy,
		Proof:       proof,
		TimestampMs: time.Now().UnixMilli(),
	}
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
	SoftFinalized bool        `json:"soft_finalized"`
	SoftCert      *Certificate `json:"soft_cert,omitempty"`

	// Hard finality
	HardFinalized bool        `json:"hard_finalized"`
	HardCert      *Certificate `json:"hard_cert,omitempty"`
}

// =============================================================================
// VOTER IDENTITY
// =============================================================================

// VoterID is a 32-byte voter identifier
// For blockchain: NodeID (crypto-derived from public key)
// For AI agents: H(agent_string_id)
type VoterID [32]byte

// EmptyVoterID is the zero voter ID
var EmptyVoterID VoterID

// DeriveVoterID derives a VoterID from a string (e.g., agent name)
func DeriveVoterID(id string) VoterID {
	return sha256.Sum256([]byte(id))
}

// DeriveVoterIDFromBytes derives from raw bytes (e.g., public key)
func DeriveVoterIDFromBytes(data []byte) VoterID {
	return sha256.Sum256(data)
}

// =============================================================================
// HELPER: Content-addressed item ID (backwards compat with wire.go)
// =============================================================================

// ItemID is an alias for CandidateID for backwards compatibility
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
