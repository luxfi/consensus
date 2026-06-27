// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quasar_finality.go — the ONE canonical Quasar finality message M.
//
// EVERY evidence lane in a Quasar cert (Beam/BLS, Pulsar/ML-DSA,
// Corona/Ringtail, P3Q rollup) signs the SAME M. M binds the full
// consensus position so a signature for one (chain, height, round, block,
// state, signer-set, key-era, policy) tuple can never be replayed for a
// different one (cross-domain non-transferability).
//
// Owner spec (the canonical tuple M MUST bind):
//
//	M = H( "QUASAR_FINALITY_V1"
//	       ‖ chainID ‖ height ‖ round ‖ blockID ‖ stateRoot
//	       ‖ signerSetID ‖ keyEraID ‖ evidencePolicyID )
//
// RECONCILIATION with the existing envelope domain message
// (consensus_cert.go). The envelope's consensusCertMessage already binds a
// SUPERSET of the spec tuple and is the message the four dispatched
// evidence verifiers check; it is wire-stable. Rather than fork a second
// finality message (which would violate "all lanes sign the SAME M"), this
// file owns the SINGLE message core, finalityMessage, and BOTH entry points
// delegate to it:
//
//   - consensusCertMessage(cert, requiredLegsRoot) — the envelope's
//     accessor, extracts the tuple from a *ConsensusCert (unchanged callers).
//   - QuasarFinalityMessage(params)                — the owner-named typed
//     constructor, builds the tuple from explicit finality values.
//
// The spec's field names map onto the envelope tuple as values, not places:
// signerSetID is the committed validator-set root; evidencePolicyID is the
// policy id; blockID is the block hash. The byte-level domain tag remains
// the wire-stable "Lux/ConsensusCert/v1" constant — "QUASAR_FINALITY_V1" is
// the canonical NAME of this message (the value), documented here.
//
// Decomplected: this file owns ONLY message construction. It decides nothing
// about which legs are required (policy), whether a leg verifies (the leg
// verifiers), or which era key to use (the KeyEra registry).
package quasar

import "encoding/binary"

// QuasarFinalityDomainName is the canonical name of the Quasar finality
// message per the owner spec. It documents the value; the wire-stable
// domain-separation bytes are consensusCertDomainTag /
// consensusCertMessageCustomization (consensus_cert.go) so the envelope's
// existing four leg verifiers continue to verify byte-for-byte.
const QuasarFinalityDomainName = "QUASAR_FINALITY_V1"

// finalityTuple is the full set of consensus-position values bound into the
// canonical finality message M. It is a pure value — the single argument to
// the one message builder.
//
// Field → owner-spec mapping:
//
//	ChainID          → chainID
//	Height           → height
//	Round            → round
//	BlockHash        → blockID
//	StateRoot        → stateRoot
//	ValidatorSetRoot → signerSetID   (the P-Chain-pinned committed signer set)
//	PolicyID         → evidencePolicyID
//	KeyEraID         → keyEraID       (the KeyEra this cert finalises under)
//
// Version, Profile, Epoch and RequiredLegsRoot are additional bindings the
// envelope already commits (defence in depth — more binding is strictly more
// secure); the spec tuple is a subset of what M binds.
type finalityTuple struct {
	Version          uint16
	Profile          uint8
	ChainID          uint32
	Epoch            uint64
	Height           uint64
	Round            uint32
	BlockHash        [32]byte
	StateRoot        [32]byte
	ValidatorSetRoot [48]byte
	PolicyID         uint32
	RequiredLegsRoot [32]byte
	SignerRoot       [32]byte
	KeyEraID         uint64
}

// finalityMessage is the ONE canonical finality-message builder. Both the
// envelope accessor (consensusCertMessage) and the typed constructor
// (QuasarFinalityMessage) delegate here, so there is exactly one M for a
// given tuple and all lanes provably sign the same bytes.
//
// Layout (TupleHash256, so flipping any single field's bytes changes the
// digest even if a neighbour could absorb them — see round_digest.go):
//
//	domain_tag ‖ version ‖ profile ‖ chain_id ‖ epoch ‖ height ‖ round ‖
//	block_hash ‖ validator_set_root ‖ policy_id ‖ required_legs_root ‖
//	signer_root ‖ state_root ‖ key_era_id
//
// The first twelve parts are byte-identical to the pre-existing envelope
// message; state_root and key_era_id are appended (the owner-spec extension).
func finalityMessage(t finalityTuple) []byte {
	var u16 [2]byte
	var u32 [4]byte
	var u64 [8]byte

	binary.BigEndian.PutUint16(u16[:], t.Version)
	verBytes := append([]byte(nil), u16[:]...)
	binary.BigEndian.PutUint32(u32[:], t.ChainID)
	chainBytes := append([]byte(nil), u32[:]...)
	binary.BigEndian.PutUint64(u64[:], t.Epoch)
	epochBytes := append([]byte(nil), u64[:]...)
	binary.BigEndian.PutUint64(u64[:], t.Height)
	heightBytes := append([]byte(nil), u64[:]...)
	binary.BigEndian.PutUint32(u32[:], t.Round)
	roundBytes := append([]byte(nil), u32[:]...)
	binary.BigEndian.PutUint32(u32[:], t.PolicyID)
	policyBytes := append([]byte(nil), u32[:]...)
	binary.BigEndian.PutUint64(u64[:], t.KeyEraID)
	keyEraBytes := append([]byte(nil), u64[:]...)

	parts := [][]byte{
		[]byte(consensusCertDomainTag),
		verBytes,
		{t.Profile},
		chainBytes,
		epochBytes,
		heightBytes,
		roundBytes,
		t.BlockHash[:],
		t.ValidatorSetRoot[:],
		policyBytes,
		t.RequiredLegsRoot[:],
		t.SignerRoot[:],
		t.StateRoot[:],
		keyEraBytes,
	}
	return tupleHash256RoundDigest(parts, 32, consensusCertMessageCustomization)
}

// QuasarFinalityParams carries the explicit finality values a caller signs
// over when producing a compact threshold cert outside the *ConsensusCert
// header path (e.g. a TALUS/Pulsar threshold ceremony, or a standalone
// finality attestation). It maps 1:1 to the owner-spec tuple; the additional
// envelope bindings (Version/Profile/Epoch/RequiredLegsRoot) default to the
// envelope's canonical values so QuasarFinalityMessage and the envelope's
// consensusCertMessage produce byte-identical M for the same cert.
type QuasarFinalityParams struct {
	ChainID          uint32
	Epoch            uint64
	Height           uint64
	Round            uint32
	BlockID          [32]byte // blockID
	StateRoot        [32]byte // stateRoot
	SignerSetID      [48]byte // signerSetID — committed validator-set root
	KeyEraID         uint64   // keyEraID
	EvidencePolicyID uint32   // evidencePolicyID
	RequiredLegsRoot [32]byte // policy-derived required-leg commitment
	SignerRoot       [32]byte // commitment to the actual signer set in evidence
	Profile          uint8    // chain security profile (defaults consistent with the cert)
}

// QuasarFinalityMessage builds the ONE canonical finality message M from
// explicit finality values. It is byte-identical to the envelope's
// consensusCertMessage for the same (chain, height, round, block, state,
// signer-set, key-era, policy) tuple — there is exactly one M.
func QuasarFinalityMessage(p QuasarFinalityParams) []byte {
	return finalityMessage(finalityTuple{
		Version:          consensusCertVersion,
		Profile:          p.Profile,
		ChainID:          p.ChainID,
		Epoch:            p.Epoch,
		Height:           p.Height,
		Round:            p.Round,
		BlockHash:        p.BlockID,
		StateRoot:        p.StateRoot,
		ValidatorSetRoot: p.SignerSetID,
		PolicyID:         p.EvidencePolicyID,
		RequiredLegsRoot: p.RequiredLegsRoot,
		SignerRoot:       p.SignerRoot,
		KeyEraID:         p.KeyEraID,
	})
}
