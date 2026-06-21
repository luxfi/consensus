// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_message.go — the domain-separated consensus message a weighted
// quorum certificate's signers sign over.
//
// One message function. It binds EVERY consensus envelope axis so a single
// signature is non-transferable across domains: a signature produced for
// (chain A, epoch e, height h, round r, value v, qc_type prepare) MUST NOT
// verify for any other (chain, epoch, height, round, value, qc_type) tuple,
// nor across the proof-backend / verifier / profile axes.
//
// The message is built ON TOP of ComputeRoundDigest (round_digest.go) — the
// canonical round-digest machinery that already binds the proof backend,
// profile, hash suite, identity/finality schemes, network/chain ids, epoch,
// height, round, and every state/validator/committee root via TupleHash256
// under "QUASAR-ROUND-DIGEST". This wrapper adds the two axes a quorum cert
// needs that the round digest expresses positionally — qc_type and the
// value/block hash — and re-frames the result under a quorum-specific
// customization tag so a round-digest byte string can never be replayed as
// a quorum message and vice versa.
package quasar

import (
	"encoding/binary"

	"github.com/luxfi/consensus/config"
)

// quorumMessageCustomization is the cSHAKE256 customization tag for the
// quorum-certificate signing message. Distinct from the round-digest tag so
// the two message spaces are domain-separated. Wire-stable cryptographic
// constant.
const quorumMessageCustomization = "QUASAR-WQC-MESSAGE-V1"

// quorumMessageProtocolTag is the in-band redundant protocol tag for the
// quorum message.
const quorumMessageProtocolTag = "Quasar/WQC/Message"

// QuorumMessageEnvelope carries every axis the quorum message binds. The
// consensus-position fields (chain_id, epoch, height, round, value_hash,
// qc_type, validator_set_root) MUST equal the matching fields of the cert
// that will carry the resulting signatures — the cert verifier rebuilds the
// message from its own fields, so any mismatch makes every signature fail.
//
// The envelope-axis fields (profile, hash suite, schemes, proof backend,
// format, verifier, network) are the round-digest envelope; they pin the
// security posture into the signed transcript so a cert produced under one
// posture cannot be re-presented under another.
type QuorumMessageEnvelope struct {
	// Round-digest envelope axes (see ComputeRoundDigest).
	ProfileID       uint32
	HashSuite       config.HashSuiteID
	IdentityScheme  config.IdentitySchemeID
	FinalityScheme  config.SigSchemeID
	ProofPolicy     config.ProofPolicyID
	ProofBackend    config.ProofBackendID
	ProofFormat     config.ProofFormatID
	VerifierID      config.VerifierID
	EffectivePolicy uint8
	NetworkID       uint32

	// Consensus position — the quorum-cert binding axes.
	ChainID          uint32
	Epoch            uint64
	Height           uint64
	Round            uint32
	ValueHash        [32]byte // block_id / value being finalised
	QCType           uint8    // certificate role (prepare/commit/finality/…)
	ValidatorSetRoot [48]byte // weighted-validator-set commitment

	// QuorumThreshold is the minimum total voting weight the cert asserts as
	// its security parameter. Bound into the SIGNED message so a signature is
	// only valid for the threshold it was produced under — a cert may not
	// re-present signatures under a lowered threshold (sub-quorum finality
	// forgery). This is a quorum-cert-specific axis (like qc_type and the
	// value hash) layered here, NOT in the shared round digest.
	QuorumThreshold uint64

	// Additional round-digest roots. These default to the value hash / set
	// root when a caller has nothing more specific, but are exposed so a
	// deployment that runs the full Q-Block envelope can bind its real
	// committee / DKG / group-key / DA / state roots too.
	ParentQBlockHash   [32]byte
	DARoot             [48]byte
	SourceStateRoot    [48]byte
	ZChainStateRoot    [48]byte
	CommitteeRoot      [48]byte
	DKGTranscriptRoot  [48]byte
	GroupPublicKeyHash [48]byte
	SignerSetCommit    [48]byte
}

// QuorumConsensusMessage builds the domain-separated message the signers
// sign over. It is the SINGLE message function for weighted quorum certs;
// the prover hands it to each signer, and the verifier rebuilds it from the
// cert's fields. Identical inputs yield identical bytes.
//
// Construction:
//  1. ComputeRoundDigest binds the full envelope (proof backend, profile,
//     schemes, network/chain, epoch, height, round, and the roots). The
//     value hash is bound as the round digest's payload_root so a different
//     block under the same position yields a different digest.
//  2. The 32-byte round digest is re-framed under
//     "QUASAR-WQC-MESSAGE-V1" together with qc_type and the value hash
//     (bound a second time, explicitly) to produce the final message. The
//     re-framing guarantees a round-digest byte string is never itself a
//     valid quorum message.
//
// Returns the message bytes and any error from ComputeRoundDigest (which
// refuses zero-value security-relevant inputs — so a caller cannot build a
// message under an unset profile / backend / chain).
func QuorumConsensusMessage(env QuorumMessageEnvelope) ([]byte, error) {
	// Bind the value hash into the round digest's payload_root slot. The
	// payload_root is [48]byte; left-pad the 32-byte value hash into it
	// deterministically (high 16 bytes zero) so the binding is canonical.
	var payloadRoot [48]byte
	copy(payloadRoot[16:], env.ValueHash[:])

	digest, err := ComputeRoundDigest(
		env.ProfileID,
		env.HashSuite,
		env.IdentityScheme,
		env.FinalityScheme,
		env.ProofPolicy,
		env.ProofBackend,
		env.ProofFormat,
		env.VerifierID,
		env.EffectivePolicy,
		env.NetworkID,
		env.ChainID,
		env.Epoch,
		env.Height,
		env.Round,
		env.ParentQBlockHash,
		payloadRoot,
		env.DARoot,
		env.SourceStateRoot,
		env.ZChainStateRoot,
		env.ValidatorSetRoot,
		env.CommitteeRoot,
		env.DKGTranscriptRoot,
		env.GroupPublicKeyHash,
		env.SignerSetCommit,
	)
	if err != nil {
		return nil, err
	}

	// Re-frame the round digest under the quorum message tag together with
	// qc_type, the value hash, and the quorum threshold. This is the seam
	// that makes a quorum message non-interchangeable with a bare round
	// digest AND binds the quorum security parameter into the signature: a
	// signature produced under threshold T verifies ONLY for threshold T, so
	// honest signatures over the chain's real BFT quorum cannot be re-framed
	// under a lowered threshold (sub-quorum finality forgery).
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], env.ChainID)
	chainBytes := append([]byte(nil), u32[:]...)

	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], env.QuorumThreshold)
	threshBytes := append([]byte(nil), u64[:]...)

	parts := [][]byte{
		[]byte(quorumMessageProtocolTag),
		digest[:],
		{env.QCType},
		env.ValueHash[:],
		chainBytes,
		env.ValidatorSetRoot[:],
		threshBytes,
	}
	out := tupleHash256RoundDigest(parts, 32, quorumMessageCustomization)
	return out, nil
}

// QuorumMessageForCert is the verifier-side convenience that rebuilds the
// quorum message from a certificate's consensus-position fields plus the
// envelope axes the chain pins. The caller supplies the envelope axes (from
// the chain security profile) and the cert; this fills the position fields
// from the cert so the two can never disagree.
//
// This is the function a verifier SHOULD use: it guarantees the message is
// derived from the SAME (chain_id, epoch, height, round, value_hash,
// qc_type, validator_set_root, quorum_threshold) the cert claims, so a cert
// whose signers signed a different message fails verification, and a cert
// that lies about its position or lowers its threshold fails because the
// rebuilt message no longer matches the signatures.
func QuorumMessageForCert(envelope QuorumMessageEnvelope, cert *WeightedQuorumCert) ([]byte, error) {
	if cert == nil {
		return nil, ErrQCNil
	}
	env := envelope
	env.ChainID = cert.ChainID
	env.Epoch = cert.Epoch
	env.Height = cert.Height
	env.Round = cert.Round
	env.ValueHash = cert.ValueHash
	env.QCType = cert.QCType
	env.ValidatorSetRoot = cert.ValidatorSetRoot
	env.QuorumThreshold = cert.QuorumThreshold
	return QuorumConsensusMessage(env)
}
