// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// HIP-0079 Q-Chain finality block reference implementation.
//
// QBlock is the compact, O(1)-per-block finality unit Quasar emits per round.
// Each block carries only the roots that anchor the latest accepted Z-Chain
// EpochCommitment (HIP-0078) plus a single Pulsar-M threshold signature
// (HIP-0084). Bulky validator / identity / DKG state lives in Z-Chain; this
// finality lane is bounded in size regardless of validator-set N.
//
// The block binds every consensus envelope axis the threshold signature
// commits over: profile_id, hash_suite_id, identity_scheme_id,
// finality_scheme_id, proof_policy_id, proof_backend_id, proof_format_id,
// verifier_id, lux_state_root, zchain_state_root, validator_set_root,
// committee_root, dkg_transcript_root, group_public_key_hash,
// payload_root, da_root, and signer_bitmap_commitment. A flipped envelope
// byte therefore breaks signature verification at the threshold layer,
// not just at the receiver envelope-comparison layer.
//
// See LP-0079 (q-chain finality blocks) and LP-0077 red-review F34.

package quasar

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
	wirezap "github.com/luxfi/consensus/pkg/wire/zap"
)

// QBlock is a single Q-Chain finality block per HIP-0079 §"Q-Block structure".
// Wire layout is fixed: every byte that determines the block's meaning is
// bound into TranscriptHash() so a Pulsar-M signature is invalidated by any
// post-sign mutation. No field outside those listed in the spec is bound.
type QBlock struct {
	Version          uint16
	NetworkID        uint32
	ChainID          uint32
	Height           uint64
	RoundOrView      uint32
	ParentQBlockHash [32]byte

	// ProfileID names the ChainSecurityProfile envelope this block was
	// produced under. Binds the (HashSuiteID, FinalitySchemeID,
	// ProofPolicyID, ProofBackendID) tuple as a single identifier so the
	// transcript covers profile drift in one byte-length-stable field.
	// Zero is reserved for "no profile committed" (legacy / benchmarks).
	ProfileID uint32

	// State roots — each MUST equal the corresponding latest accepted
	// Z-Chain EpochCommitment field for the current epoch. Every state
	// root is 48 bytes (BLS12-381 G1 / KZG commitment width) so the same
	// field can carry either a hash digest or a commitment without an
	// envelope-format split.
	StateRoot          [48]byte
	ZChainStateRoot    [48]byte
	ValidatorSetRoot   [48]byte
	CommitteeRoot      [48]byte
	DKGTranscriptRoot  [48]byte
	GroupPublicKeyHash [48]byte

	// Payload anchors — block-specific data lives elsewhere.
	PayloadRoot [48]byte
	DARoot      [48]byte

	// Cert envelope — every byte here is bound into the transcript.
	ProofPolicyID  config.ProofPolicyID
	ProofBackendID config.ProofBackendID
	ProofFormatID  config.ProofFormatID
	VerifierID     config.VerifierID
	HashSuiteID    config.HashSuiteID

	// IdentitySchemeID is the validator identity-signature scheme (raw
	// FIPS 204 ML-DSA) pinned by the chain's security profile. Bound into
	// the transcript so cross-identity-scheme replay is closed.
	IdentitySchemeID config.IdentitySchemeID

	// FinalitySchemeID is the Pulsar-M variant (M-44 / M-65 / M-87) the
	// threshold signature was produced under.
	FinalitySchemeID config.SigSchemeID

	// SignerBitmapCommitment is the 48-byte commitment to the committee
	// signer set or weight Merkle root that attested to this block.
	// Length-stable: a missing / short commitment is unconditionally
	// below threshold.
	SignerBitmapCommitment [48]byte

	// Single Pulsar-M threshold signature over TranscriptHash().
	PulsarMThresholdSignature []byte
}

// qBlockTranscriptCustomization is the SP 800-185 cSHAKE256 customization
// tag for the canonical Pulsar-M signing transcript. The tag is the schema
// identity; changing it produces a transcript stream that does not collide
// with any prior signature.
//
// Tag is wire-format-stable; do NOT rename even after the family rebrand
// (Pulsar-M → Pulsar). The byte string `QUASAR-Q-BLOCK-V1` is part of
// every prior Q-Block signature's transcript — renaming it invalidates
// every historical cert. The string is a cryptographic constant, not
// user-facing prose; downstream rebrands MUST keep it byte-identical.
const qBlockTranscriptCustomization = "QUASAR-Q-BLOCK-V1"

// qBlockProtocolTag is the in-band redundant protocol tag for the
// transcript. Defence-in-depth so a cross-customization-collision attacker
// also has to forge the leading TupleHash part.
const qBlockProtocolTag = "Q-Chain"

// TranscriptHash returns the 32-byte digest the Pulsar-M threshold
// signature signs over for this Q-Block. Computed as
// TupleHash256(parts, customization="QUASAR-Q-BLOCK-V1") per HIP-0079
// §"Canonical transcript binding".
//
// Binds every field listed in the spec; binds NO field not listed. Length
// framing on every variable-length part is the explicit defence against
// boundary-shifting attacks (see TupleHash injectivity).
func (b *QBlock) TranscriptHash() [32]byte {
	var u16 [2]byte
	var u32 [4]byte
	var u64 [8]byte

	binary.BigEndian.PutUint16(u16[:], b.Version)
	versionBytes := append([]byte(nil), u16[:]...)

	binary.BigEndian.PutUint32(u32[:], b.ProfileID)
	profileBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint32(u32[:], b.NetworkID)
	netBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint32(u32[:], b.ChainID)
	chainBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint64(u64[:], b.Height)
	heightBytes := append([]byte(nil), u64[:]...)

	binary.BigEndian.PutUint32(u32[:], b.RoundOrView)
	roundBytes := append([]byte(nil), u32[:]...)

	var u16Verifier [2]byte
	binary.BigEndian.PutUint16(u16Verifier[:], uint16(b.VerifierID))
	verifierBytes := append([]byte(nil), u16Verifier[:]...)

	parts := [][]byte{
		[]byte(qBlockProtocolTag),
		versionBytes,
		profileBytes,
		netBytes,
		chainBytes,
		heightBytes,
		roundBytes,
		b.ParentQBlockHash[:],
		b.PayloadRoot[:],
		b.DARoot[:],
		b.StateRoot[:],
		b.ZChainStateRoot[:],
		b.ValidatorSetRoot[:],
		b.CommitteeRoot[:],
		b.DKGTranscriptRoot[:],
		b.GroupPublicKeyHash[:],
		{byte(b.HashSuiteID)},
		{byte(b.IdentitySchemeID)},
		{byte(b.FinalitySchemeID)},
		{byte(b.ProofPolicyID)},
		{byte(b.ProofBackendID)},
		{byte(b.ProofFormatID)},
		verifierBytes,
		b.SignerBitmapCommitment[:],
	}

	out := tupleHash256RoundDigest(parts, 32, qBlockTranscriptCustomization)
	var digest [32]byte
	copy(digest[:], out)
	return digest
}

// ============================================================================
// Wire codec — LP-182 schema 0x02 ZAP
// ============================================================================
//
// The wire layer (pkg/wire/zap.QBlock) is the LP-182 canonical wire form.
// This file delegates Marshal / UnmarshalQBlock to that package; the Go
// value-typed QBlock struct above remains the in-process representation
// for the AcceptQBlock rule logic and TranscriptHash construction.

// ErrQBlockTruncated is returned by UnmarshalQBlock when the input does
// not parse as LP-182 schema 0x02 ZAP bytes.
var ErrQBlockTruncated = errors.New("qblock: input truncated")

// ErrQBlockTooLong is kept as a typed sentinel for callers that route on
// errors.Is — the ZAP wire-layer rejects oversized length fields at
// pkg/wire/zap.WrapQBlock without producing a distinct error class, so
// every framing rejection surfaces as ErrQBlockTruncated.
var ErrQBlockTooLong = errors.New("qblock: declared length exceeds input")

// Marshal returns the LP-182 schema 0x02 ZAP wire-bytes for this block.
func (b *QBlock) Marshal() ([]byte, error) {
	if b == nil {
		return nil, errors.New("qblock: nil receiver")
	}
	return qBlockToZAPBytes(b), nil
}

// UnmarshalQBlock wraps LP-182 schema 0x02 ZAP wire-bytes and projects
// them back into a Go value-typed QBlock.
func UnmarshalQBlock(data []byte) (*QBlock, error) {
	wrap, err := qBlockFromZAP(data)
	if err != nil {
		return nil, ErrQBlockTruncated
	}
	return wrap, nil
}

// ============================================================================
// Acceptance rule (HIP-0079 §"Acceptance rule" with F34 closure)
// ============================================================================

// NetworkPolicy pins the per-network rules applied during AcceptQBlock.
// HIP-0079 distinguishes mainnet (strict-PQ) from testnet/devnet (permissive).
type NetworkPolicy uint8

const (
	// NetworkPolicyStrictPQ is the Lux primary-network policy: only
	// ProofPolicySTARKFRISHA3PQ (0x10) is accepted, HashSuiteSHA3NIST is
	// required, and devnet-only sig schemes (Pulsar-M-44) are refused.
	NetworkPolicyStrictPQ NetworkPolicy = 1

	// NetworkPolicyPermissive is the testnet/devnet policy: every
	// IsPostQuantum() proof system is accepted, BLAKE3-legacy hash suite
	// is accepted, and Pulsar-M-44 is accepted alongside M-65 and M-87.
	NetworkPolicyPermissive NetworkPolicy = 2
)

// SecurityProfile is the per-chain envelope the acceptance rule must
// enforce. Every block on a strict-PQ chain MUST commit to exactly this
// tuple; any drift in any byte is a typed rejection.
//
// ProfileID is the canonical name of this tuple. When ChainSecurityProfile
// is fully landed by its parallel agent, this struct collapses to a
// pointer to the canonical type; until then we carry the same fields here.
type SecurityProfile struct {
	ProfileID        uint32
	HashSuiteID      config.HashSuiteID
	FinalitySchemeID config.SigSchemeID
	ProofPolicyID    config.ProofPolicyID
	ProofBackendID   config.ProofBackendID
}

// EpochCommitment is the latest accepted Z-Chain epoch state. AcceptQBlock
// rejects any block whose corresponding fields drift from this commitment.
type EpochCommitment struct {
	ZChainStateRoot    [48]byte
	ValidatorSetRoot   [48]byte
	CommitteeRoot      [48]byte
	DKGTranscriptRoot  [48]byte
	GroupPublicKeyHash [48]byte
}

// AcceptanceContext is the latest accepted Z-Chain epoch state plus the
// chain tip the new block must extend, plus the security profile pinned
// by this chain's genesis.
type AcceptanceContext struct {
	// Profile is the ChainSecurityProfile this chain was configured under.
	// Every block MUST match every byte. ProfileID == 0 disables the
	// profile-match gate (legacy callers); production chains MUST set it.
	Profile SecurityProfile

	// EpochCommitment is the latest accepted Z-Chain epoch state.
	EpochCommitment EpochCommitment

	// ChainTipHash is the hash of the most recent accepted block.
	ChainTipHash [32]byte

	// NetworkPolicy is the per-network strictness policy. Required.
	NetworkPolicy NetworkPolicy

	// ThresholdVerifier is the committee threshold predicate. Required.
	// The reference implementation lives in luxfi/pulsar; this package
	// depends on the interface, not the implementation.
	ThresholdVerifier ThresholdVerifier

	// Verifier is the Pulsar-M signature verifier. Required.
	Verifier Verifier
}

// Verifier is the Pulsar-M threshold signature verifier interface used
// by AcceptQBlock. The reference implementation lives in luxfi/pulsar
// — this package stays below pulsar in the module graph.
type Verifier interface {
	// Verify returns true iff sig is a valid Pulsar-M threshold signature
	// over transcriptHash under the group public key identified by
	// groupPubKeyHash. Implementations MUST be deterministic and MUST
	// run unmodified FIPS 204 ML-DSA.Verify on the threshold output.
	Verify(transcriptHash [32]byte, sig []byte, groupPubKeyHash [32]byte) bool
}

// ThresholdVerifier is the committee-bitmap threshold predicate.
// BitmapMeetsThreshold returns true iff the bitmap commitment represents
// a signer set that satisfies the threshold defined by committeeRoot.
type ThresholdVerifier interface {
	BitmapMeetsThreshold(bitmapCommitment [48]byte, committeeRoot [48]byte) bool
}

// Typed acceptance errors. Each maps 1:1 to one of the acceptance rules
// so upstream callers and tests can name the exact failure mode.
var (
	ErrQBlockParentMismatch             = errors.New("qblock: parent_qblock_hash does not match chain tip")
	ErrQBlockProfileMismatch            = errors.New("qblock: profile_id does not match chain security profile")
	ErrQBlockHashSuiteMismatch          = errors.New("qblock: hash_suite_id does not match chain security profile")
	ErrQBlockFinalitySchemeMismatch     = errors.New("qblock: finality_scheme_id does not match chain security profile")
	ErrQBlockProofPolicyMismatch        = errors.New("qblock: proof_policy_id does not match chain security profile")
	ErrQBlockZChainStateRootMismatch    = errors.New("qblock: zchain_state_root does not match latest accepted Z-Chain root")
	ErrQBlockValidatorRootMismatch      = errors.New("qblock: validator_set_root does not match latest accepted Z-Chain validator_registry_root")
	ErrQBlockCommitteeRootMismatch      = errors.New("qblock: committee_root does not match latest accepted Z-Chain committee_root")
	ErrQBlockDKGTranscriptRootMismatch  = errors.New("qblock: dkg_transcript_root does not match latest accepted Z-Chain dkg_transcript_root")
	ErrQBlockGroupKeyMismatch           = errors.New("qblock: group_public_key_hash does not match latest accepted Z-Chain group_public_key_hash")
	ErrQBlockForbiddenProofSystem       = errors.New("qblock: proof_policy_id is forbidden in PQ mode")
	ErrQBlockProofSystemNotPQ           = errors.New("qblock: proof_policy_id is not post-quantum")
	ErrQBlockHashSuiteRefused           = errors.New("qblock: hash_suite_id refused by network policy")
	ErrQBlockSigSchemeRefused           = errors.New("qblock: finality_scheme_id refused by network policy")
	ErrQBlockSignerBitmapBelowThreshold = errors.New("qblock: signer bitmap commitment below required threshold")
	ErrQBlockPulsarMVerifyFail          = errors.New("qblock: pulsar_m_threshold_signature does not verify against transcript")
	ErrQBlockNilBlock                   = errors.New("qblock: nil block")
	ErrQBlockMissingVerifier            = errors.New("qblock: AcceptanceContext.Verifier required")
	ErrQBlockMissingThresholdVerifier   = errors.New("qblock: AcceptanceContext.ThresholdVerifier required")
	ErrQBlockMissingNetworkPolicy       = errors.New("qblock: AcceptanceContext.NetworkPolicy required")
)

// AcceptQBlock returns nil iff b satisfies every item in HIP-0079
// §"Acceptance rule". On failure returns a typed error from the set
// above so the caller can identify exactly which rule was violated.
//
// Rule order: cheap structural checks → profile match → epoch-commitment
// matches → policy enforcement → threshold predicate → signature verify.
// The expensive ML-DSA verify runs last so a malformed block is rejected
// without paying that cost.
func AcceptQBlock(b *QBlock, ctx AcceptanceContext) error {
	if b == nil {
		return ErrQBlockNilBlock
	}
	if ctx.Verifier == nil {
		return ErrQBlockMissingVerifier
	}
	if ctx.ThresholdVerifier == nil {
		return ErrQBlockMissingThresholdVerifier
	}
	if ctx.NetworkPolicy == 0 {
		return ErrQBlockMissingNetworkPolicy
	}

	// Rule 1: parent links the chain tip.
	if b.ParentQBlockHash != ctx.ChainTipHash {
		return ErrQBlockParentMismatch
	}

	// Rule 2: security profile match (only when Profile.ProfileID != 0;
	// zero disables the gate for legacy callers).
	if ctx.Profile.ProfileID != 0 {
		if b.ProfileID != ctx.Profile.ProfileID {
			return fmt.Errorf("%w: block=%d profile=%d",
				ErrQBlockProfileMismatch, b.ProfileID, ctx.Profile.ProfileID)
		}
		if b.HashSuiteID != ctx.Profile.HashSuiteID {
			return fmt.Errorf("%w: block=%s profile=%s",
				ErrQBlockHashSuiteMismatch, b.HashSuiteID, ctx.Profile.HashSuiteID)
		}
		if b.FinalitySchemeID != ctx.Profile.FinalitySchemeID {
			return fmt.Errorf("%w: block=%s profile=%s",
				ErrQBlockFinalitySchemeMismatch, b.FinalitySchemeID, ctx.Profile.FinalitySchemeID)
		}
		if b.ProofPolicyID != ctx.Profile.ProofPolicyID {
			return fmt.Errorf("%w: block=%s profile=%s",
				ErrQBlockProofPolicyMismatch, b.ProofPolicyID, ctx.Profile.ProofPolicyID)
		}
	}

	// Rule 3..7: epoch-commitment match.
	if b.ZChainStateRoot != ctx.EpochCommitment.ZChainStateRoot {
		return ErrQBlockZChainStateRootMismatch
	}
	if b.ValidatorSetRoot != ctx.EpochCommitment.ValidatorSetRoot {
		return ErrQBlockValidatorRootMismatch
	}
	if b.CommitteeRoot != ctx.EpochCommitment.CommitteeRoot {
		return ErrQBlockCommitteeRootMismatch
	}
	if b.DKGTranscriptRoot != ctx.EpochCommitment.DKGTranscriptRoot {
		return ErrQBlockDKGTranscriptRootMismatch
	}
	if b.GroupPublicKeyHash != ctx.EpochCommitment.GroupPublicKeyHash {
		return ErrQBlockGroupKeyMismatch
	}

	// Rule 8: proof_policy_id.
	if b.ProofPolicyID.IsForbiddenInPQMode() {
		return fmt.Errorf("%w: %s", ErrQBlockForbiddenProofSystem, b.ProofPolicyID.String())
	}
	switch ctx.NetworkPolicy {
	case NetworkPolicyStrictPQ:
		if b.ProofPolicyID != config.ProofPolicySTARKFRISHA3PQ {
			return fmt.Errorf("%w: strict-PQ requires STARK_FRI_SHA3_PQ (0x10), got %s",
				ErrQBlockProofSystemNotPQ, b.ProofPolicyID.String())
		}
	case NetworkPolicyPermissive:
		if !b.ProofPolicyID.IsPostQuantum() {
			return fmt.Errorf("%w: %s", ErrQBlockProofSystemNotPQ, b.ProofPolicyID.String())
		}
	}

	// Rule 9: hash_suite_id.
	switch ctx.NetworkPolicy {
	case NetworkPolicyStrictPQ:
		if b.HashSuiteID != config.HashSuiteSHA3NIST {
			return fmt.Errorf("%w: strict-PQ requires SHA3_NIST (0x01), got %s",
				ErrQBlockHashSuiteRefused, b.HashSuiteID.String())
		}
	case NetworkPolicyPermissive:
		// Permissive: SHA3_NIST or BLAKE3_LEGACY. HashSuiteNone refused.
		if b.HashSuiteID != config.HashSuiteSHA3NIST && b.HashSuiteID != config.HashSuiteBLAKE3Legacy {
			return fmt.Errorf("%w: %s", ErrQBlockHashSuiteRefused, b.HashSuiteID.String())
		}
	}

	// Rule 10: finality_scheme_id.
	switch ctx.NetworkPolicy {
	case NetworkPolicyStrictPQ:
		// Mainnet: Pulsar-M-65 or Pulsar-M-87 only. M-44 is devnet-only.
		if b.FinalitySchemeID != config.SigSchemePulsar65 && b.FinalitySchemeID != config.SigSchemePulsar87 {
			return fmt.Errorf("%w: strict-PQ accepts Pulsar-M-65/M-87, got %s",
				ErrQBlockSigSchemeRefused, b.FinalitySchemeID.String())
		}
	case NetworkPolicyPermissive:
		if !b.FinalitySchemeID.IsPulsarM() {
			return fmt.Errorf("%w: permissive policy accepts Pulsar-M family, got %s",
				ErrQBlockSigSchemeRefused, b.FinalitySchemeID.String())
		}
	}

	// Rule 11: signer bitmap commitment threshold predicate.
	//
	// SignerBitmapCommitment is the canonical 48-byte field. A zero
	// commitment is unconditionally below threshold (length-stable refusal).
	var zeroCommitment [48]byte
	if b.SignerBitmapCommitment == zeroCommitment {
		return ErrQBlockSignerBitmapBelowThreshold
	}
	if !ctx.ThresholdVerifier.BitmapMeetsThreshold(b.SignerBitmapCommitment, b.CommitteeRoot) {
		return ErrQBlockSignerBitmapBelowThreshold
	}

	// Rule 12: Pulsar-M threshold signature verifies against transcript.
	if len(b.PulsarMThresholdSignature) == 0 {
		return ErrQBlockPulsarMVerifyFail
	}
	transcript := b.TranscriptHash()
	// Verifier interface takes a 32-byte groupPubKeyHash; the 48-byte
	// field is folded to 32 by taking its first 32 bytes (the canonical
	// group-key SHA3-256 hash always lives in the leading 32 bytes; the
	// trailing 16 bytes are zero for hash commitments and carry the KZG
	// curve-point suffix for commitment commitments).
	var groupKey32 [32]byte
	copy(groupKey32[:], b.GroupPublicKeyHash[:32])
	if !ctx.Verifier.Verify(transcript, b.PulsarMThresholdSignature, groupKey32) {
		return ErrQBlockPulsarMVerifyFail
	}

	return nil
}

// ============================================================================
// ZAP wire bridge — LP-182 schema 0x02
// ============================================================================
//
// qBlockToZAPBytes / qBlockFromZAP are thin adapters between the Go
// value-typed QBlock struct (which the AcceptQBlock rule logic and
// TranscriptHash() consume directly) and pkg/wire/zap.QBlock (which is
// the LP-182 canonical wire form).

func qBlockToZAPBytes(b *QBlock) []byte {
	return wirezap.NewQBlock(wirezap.QBlockFields{
		Version:                b.Version,
		NetworkID:              b.NetworkID,
		ChainID:                b.ChainID,
		Height:                 b.Height,
		RoundOrView:            b.RoundOrView,
		ProfileID:              b.ProfileID,
		ParentQBlockHash:       b.ParentQBlockHash,
		StateRoot:              b.StateRoot,
		ZChainStateRoot:        b.ZChainStateRoot,
		ValidatorSetRoot:       b.ValidatorSetRoot,
		CommitteeRoot:          b.CommitteeRoot,
		DKGTranscriptRoot:      b.DKGTranscriptRoot,
		GroupPublicKeyHash:     b.GroupPublicKeyHash,
		PayloadRoot:            b.PayloadRoot,
		DARoot:                 b.DARoot,
		SignerBitmapCommitment: b.SignerBitmapCommitment,
		HashSuiteID:            uint8(b.HashSuiteID),
		IdentitySchemeID:       uint8(b.IdentitySchemeID),
		FinalitySchemeID:       uint8(b.FinalitySchemeID),
		ProofPolicyID:          uint8(b.ProofPolicyID),
		ProofBackendID:         uint8(b.ProofBackendID),
		ProofFormatID:          uint8(b.ProofFormatID),
		VerifierID:             uint16(b.VerifierID),
		Signature:              b.PulsarMThresholdSignature,
	}).Bytes()
}

func qBlockFromZAP(data []byte) (*QBlock, error) {
	wrap, err := wirezap.WrapQBlock(data)
	if err != nil {
		return nil, err
	}
	b := &QBlock{
		Version:                wrap.Version(),
		NetworkID:              wrap.NetworkID(),
		ChainID:                wrap.ChainID(),
		Height:                 wrap.Height(),
		RoundOrView:            wrap.RoundOrView(),
		ProfileID:              wrap.ProfileID(),
		ParentQBlockHash:       wrap.ParentQBlockHash(),
		StateRoot:              wrap.StateRoot(),
		ZChainStateRoot:        wrap.ZChainStateRoot(),
		ValidatorSetRoot:       wrap.ValidatorSetRoot(),
		CommitteeRoot:          wrap.CommitteeRoot(),
		DKGTranscriptRoot:     wrap.DKGTranscriptRoot(),
		GroupPublicKeyHash:    wrap.GroupPublicKeyHash(),
		PayloadRoot:           wrap.PayloadRoot(),
		DARoot:                wrap.DARoot(),
		SignerBitmapCommitment: wrap.SignerBitmapCommitment(),
		HashSuiteID:           config.HashSuiteID(wrap.HashSuiteID()),
		IdentitySchemeID:      config.IdentitySchemeID(wrap.IdentitySchemeID()),
		FinalitySchemeID:      config.SigSchemeID(wrap.FinalitySchemeID()),
		ProofPolicyID:         config.ProofPolicyID(wrap.ProofPolicyID()),
		ProofBackendID:        config.ProofBackendID(wrap.ProofBackendID()),
		ProofFormatID:         config.ProofFormatID(wrap.ProofFormatID()),
		VerifierID:            config.VerifierID(wrap.VerifierID()),
	}
	if sig := wrap.Signature(); len(sig) > 0 {
		b.PulsarMThresholdSignature = append([]byte(nil), sig...)
	}
	return b, nil
}
