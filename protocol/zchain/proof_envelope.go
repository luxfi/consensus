// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/luxfi/consensus/config"
)

// proof_envelope.go — the on-the-wire ZProofEnvelope plus the canonical
// public inputs ABI shared across every PQ proof backend.
//
// One envelope, every backend. SP1, RISC0, P3Q, Stone, and Stwo all map
// into this struct; the only thing that differs across backends is the
// VerifierID + the opaque ProofBytes layout (which the registered
// VerifierManifest knows how to dispatch into a BackendVerifier).
//
// Every byte that determines what the proof asserts is bound into
// TranscriptHash() via TupleHash256, so a post-sign mutation breaks
// signature verification — not just envelope equality.

// envelopeTranscriptCustomization is the SP 800-185 cSHAKE256
// customization tag for the envelope transcript. The tag is the schema
// identity; changing it produces a transcript stream that does not
// collide with any prior digest.
const envelopeTranscriptCustomization = "ZCHAIN-PROOF-ENVELOPE"

// envelopeProtocolTag is the in-band redundant protocol tag for the
// transcript. Defence-in-depth so a cross-customization-collision
// attacker also has to forge the leading TupleHash part.
const envelopeProtocolTag = "Z-Chain/ProofEnvelope"

// publicInputsCustomization is the SP 800-185 cSHAKE256 customization
// tag for HashZPublicInputs. Distinct from the envelope customization
// so the digest a backend computes over public inputs cannot collide
// with a digest over the envelope itself.
const publicInputsCustomization = "ZCHAIN-PUBLIC-INPUTS"

// publicInputsProtocolTag is the in-band redundant protocol tag for
// the public-inputs digest.
const publicInputsProtocolTag = "Z-Chain/PublicInputs"

// ZProofEnvelope is the canonical on-the-wire proof envelope that every
// PQ proof backend on Z-Chain produces. The struct is the union of every
// piece of policy / format / identity metadata the verifier needs BEFORE
// it touches ProofBytes — so cheap structural checks can refuse a proof
// without paying any backend cost.
//
// All fixed-width fields are 48 bytes wide: the HIP-0078 canonical hash
// width for Z-Chain transcripts and state roots (SHA3-384 / cSHAKE256
// squeezed to 384 bits). ProgramOrAirID is the only 16-byte field —
// it's a UUID-shaped opaque program / AIR identifier.
type ZProofEnvelope struct {
	Version   uint16
	ProfileID uint32 // wider than config.ProfileID so future profiles aren't squeezed
	ChainID   uint32
	NetworkID uint32
	Epoch     uint64

	ProofPolicyID    config.ProofPolicyID
	ProofBackendID   config.ProofBackendID
	ProofFormatID    config.ProofFormatID
	VerifierID       config.VerifierID
	HashSuiteID      config.HashSuiteID
	IdentitySchemeID config.SigSchemeID
	FinalitySchemeID config.SigSchemeID

	// PublicInputsHash is HashZPublicInputs(input) computed by the
	// producer over the same canonical inputs the backend proves about.
	// VerifyZProofUnderProfile re-hashes the verifier-side inputs and
	// refuses any mismatch — the binding closes the "wrong public
	// inputs, same proof bytes" attack.
	PublicInputsHash [48]byte

	// VerifierKeyHash is the manifest-pinned 48-byte hash of the
	// verifier key the backend was set up against. The registry holds
	// the same value; mismatch is a config-drift refusal.
	VerifierKeyHash [48]byte

	// ProgramOrAirID is a 16-byte opaque program / AIR identifier
	// (typically a UUID or the first 16 bytes of a content hash). One
	// program → one identifier across versions; bump only when the
	// program semantics change.
	ProgramOrAirID [16]byte

	// ProgramOrAirHash is the full 48-byte content hash of the program
	// / AIR bytes the backend proved against. The registry holds the
	// authoritative value; mismatch is a config-drift refusal.
	ProgramOrAirHash [48]byte

	SoundnessBitsClaimed uint16
	HashOutputBits       uint16

	// Backend-self-declared properties. These are NOT trusted — the
	// profile enforces the ban via Require / Forbid fields, and the
	// verifier refuses anything that contradicts the profile policy.
	// They appear in the envelope so audit tooling can name a
	// misconfiguration precisely without having to inspect the
	// backend implementation.
	TransparentSetup          bool
	UsesPairings              bool
	UsesKZG                   bool
	UsesTrustedSetup          bool
	UsesClassicalSNARKWrapper bool

	// ProofBytes is the backend-native proof payload. The
	// VerifierManifest registered under proof.VerifierID knows how to
	// parse this slice. The envelope does not look inside.
	ProofBytes []byte
}

// TranscriptHash returns the 48-byte digest the producer's identity /
// finality signature signs over for this envelope. Bound via
// TupleHash256 (customization "ZCHAIN-PROOF-ENVELOPE") so any byte
// flip breaks signature verification — not just envelope-equality.
//
// The hash family is fixed at cSHAKE256 / TupleHash256 here, independent
// of HashSuiteID. HashSuiteID is bound as DATA into the transcript, not
// as the hash family of the transcript itself. This avoids a circular
// dependency on suite selection and matches the Pulsar-SHA3 canonical
// profile.
func (e *ZProofEnvelope) TranscriptHash() [48]byte {
	parts := [][]byte{
		[]byte(envelopeProtocolTag),
		u16BE(e.Version),
		u32BE(e.ProfileID),
		u32BE(e.ChainID),
		u32BE(e.NetworkID),
		u64BE(e.Epoch),
		{byte(e.ProofPolicyID)},
		{byte(e.ProofBackendID)},
		{byte(e.ProofFormatID)},
		u16BE(uint16(e.VerifierID)),
		{byte(e.HashSuiteID)},
		{byte(e.IdentitySchemeID)},
		{byte(e.FinalitySchemeID)},
		e.PublicInputsHash[:],
		e.VerifierKeyHash[:],
		e.ProgramOrAirID[:],
		e.ProgramOrAirHash[:],
		u16BE(e.SoundnessBitsClaimed),
		u16BE(e.HashOutputBits),
		{boolByte(e.TransparentSetup)},
		{boolByte(e.UsesPairings)},
		{boolByte(e.UsesKZG)},
		{boolByte(e.UsesTrustedSetup)},
		{boolByte(e.UsesClassicalSNARKWrapper)},
		e.ProofBytes,
	}
	return tupleHash48(parts, envelopeTranscriptCustomization)
}

// =============================================================================
// Wire codec
// =============================================================================
//
// Layout (deterministic, big-endian):
//
//	version                      uint16 BE
//	profile_id                   uint32 BE
//	chain_id                     uint32 BE
//	network_id                   uint32 BE
//	epoch                        uint64 BE
//	proof_policy_id              uint8
//	proof_backend_id             uint8
//	proof_format_id              uint8
//	verifier_id                  uint16 BE
//	hash_suite_id                uint8
//	identity_scheme_id           uint8
//	finality_scheme_id           uint8
//	public_inputs_hash           [48]byte
//	verifier_key_hash            [48]byte
//	program_or_air_id            [16]byte
//	program_or_air_hash          [48]byte
//	soundness_bits_claimed       uint16 BE
//	hash_output_bits             uint16 BE
//	transparent_setup            uint8 (0/1)
//	uses_pairings                uint8 (0/1)
//	uses_kzg                     uint8 (0/1)
//	uses_trusted_setup           uint8 (0/1)
//	uses_classical_snark_wrapper uint8 (0/1)
//	proof_bytes_len              uint32 BE
//	proof_bytes                  []byte

// Typed codec errors. Each names exactly which invariant the decoder
// caught so the caller can route the failure without parsing a string.
var (
	ErrEnvelopeNil                = errors.New("zchain: nil envelope")
	ErrEnvelopeTruncated          = errors.New("zchain: envelope truncated")
	ErrEnvelopeProofBytesTooLong  = errors.New("zchain: declared proof_bytes_len exceeds input")
	ErrEnvelopeTrailingBytes      = errors.New("zchain: trailing bytes after envelope decode")
	ErrEnvelopeBoolByteOutOfRange = errors.New("zchain: bool byte must be 0 or 1")
	ErrEnvelopeZeroEnum           = errors.New("zchain: envelope has zero-value enum where a real value is required")
)

// Marshal returns the deterministic byte encoding of e. Panics on a
// nil receiver — Marshal is internal-caller-only and a nil pointer here
// is a programmer error that MUST surface at the call site, not three
// codec layers down.
func (e *ZProofEnvelope) Marshal() []byte {
	if e == nil {
		panic("zchain: Marshal called on nil *ZProofEnvelope")
	}
	// Fixed prefix size: 2 (ver) + 4 (profile) + 4 (chain) + 4 (network)
	// + 8 (epoch) + 8 (policy/backend/format/verifier(2)/suite/id/fin)
	// + 48 + 48 + 16 + 48 (hashes) + 2 + 2 (bits) + 5 (bool bytes) +
	// 4 (proof len) = 153 bytes + len(proof).
	size := 2 + 4 + 4 + 4 + 8 + 8 + 48 + 48 + 16 + 48 + 2 + 2 + 5 + 4 + len(e.ProofBytes)
	buf := make([]byte, 0, size)

	buf = appendU16(buf, e.Version)
	buf = appendU32(buf, e.ProfileID)
	buf = appendU32(buf, e.ChainID)
	buf = appendU32(buf, e.NetworkID)
	buf = appendU64(buf, e.Epoch)
	buf = append(buf, byte(e.ProofPolicyID))
	buf = append(buf, byte(e.ProofBackendID))
	buf = append(buf, byte(e.ProofFormatID))
	buf = appendU16(buf, uint16(e.VerifierID))
	buf = append(buf, byte(e.HashSuiteID))
	buf = append(buf, byte(e.IdentitySchemeID))
	buf = append(buf, byte(e.FinalitySchemeID))
	buf = append(buf, e.PublicInputsHash[:]...)
	buf = append(buf, e.VerifierKeyHash[:]...)
	buf = append(buf, e.ProgramOrAirID[:]...)
	buf = append(buf, e.ProgramOrAirHash[:]...)
	buf = appendU16(buf, e.SoundnessBitsClaimed)
	buf = appendU16(buf, e.HashOutputBits)
	buf = append(buf, boolByte(e.TransparentSetup))
	buf = append(buf, boolByte(e.UsesPairings))
	buf = append(buf, boolByte(e.UsesKZG))
	buf = append(buf, boolByte(e.UsesTrustedSetup))
	buf = append(buf, boolByte(e.UsesClassicalSNARKWrapper))
	buf = appendU32(buf, uint32(len(e.ProofBytes)))
	buf = append(buf, e.ProofBytes...)

	return buf
}

// UnmarshalZProofEnvelope is the round-trip inverse of Marshal. Returns
// a typed error from the ErrEnvelope* set on any framing failure so
// callers can distinguish truncation from boundary-violation without
// parsing strings.
func UnmarshalZProofEnvelope(data []byte) (*ZProofEnvelope, error) {
	r := &envelopeReader{buf: data}

	e := &ZProofEnvelope{}
	var err error
	if e.Version, err = r.u16(); err != nil {
		return nil, err
	}
	if e.ProfileID, err = r.u32(); err != nil {
		return nil, err
	}
	if e.ChainID, err = r.u32(); err != nil {
		return nil, err
	}
	if e.NetworkID, err = r.u32(); err != nil {
		return nil, err
	}
	if e.Epoch, err = r.u64(); err != nil {
		return nil, err
	}
	v, err := r.u8()
	if err != nil {
		return nil, err
	}
	e.ProofPolicyID = config.ProofPolicyID(v)
	if v, err = r.u8(); err != nil {
		return nil, err
	}
	e.ProofBackendID = config.ProofBackendID(v)
	if v, err = r.u8(); err != nil {
		return nil, err
	}
	e.ProofFormatID = config.ProofFormatID(v)
	vid, err := r.u16()
	if err != nil {
		return nil, err
	}
	e.VerifierID = config.VerifierID(vid)
	if v, err = r.u8(); err != nil {
		return nil, err
	}
	e.HashSuiteID = config.HashSuiteID(v)
	if v, err = r.u8(); err != nil {
		return nil, err
	}
	e.IdentitySchemeID = config.SigSchemeID(v)
	if v, err = r.u8(); err != nil {
		return nil, err
	}
	e.FinalitySchemeID = config.SigSchemeID(v)
	if err = r.read48(&e.PublicInputsHash); err != nil {
		return nil, err
	}
	if err = r.read48(&e.VerifierKeyHash); err != nil {
		return nil, err
	}
	if err = r.read16(&e.ProgramOrAirID); err != nil {
		return nil, err
	}
	if err = r.read48(&e.ProgramOrAirHash); err != nil {
		return nil, err
	}
	if e.SoundnessBitsClaimed, err = r.u16(); err != nil {
		return nil, err
	}
	if e.HashOutputBits, err = r.u16(); err != nil {
		return nil, err
	}
	if e.TransparentSetup, err = r.boolByte(); err != nil {
		return nil, err
	}
	if e.UsesPairings, err = r.boolByte(); err != nil {
		return nil, err
	}
	if e.UsesKZG, err = r.boolByte(); err != nil {
		return nil, err
	}
	if e.UsesTrustedSetup, err = r.boolByte(); err != nil {
		return nil, err
	}
	if e.UsesClassicalSNARKWrapper, err = r.boolByte(); err != nil {
		return nil, err
	}
	if e.ProofBytes, err = r.lenPrefixed(); err != nil {
		return nil, err
	}
	if len(r.buf) != 0 {
		return nil, fmt.Errorf("%w: %d bytes", ErrEnvelopeTrailingBytes, len(r.buf))
	}

	// Refuse zero values for the security-relevant enums. A zero-init
	// envelope is never a legitimate wire payload; making the codec
	// refuse it removes one degree of freedom an attacker has when
	// fuzzing a verifier whose downstream profile-check might miss a
	// path. See F90.
	switch {
	case e.HashSuiteID == config.HashSuiteNone:
		return nil, fmt.Errorf("%w: HashSuiteID", ErrEnvelopeZeroEnum)
	case e.ProofPolicyID == config.ProofPolicyNone:
		return nil, fmt.Errorf("%w: ProofPolicyID", ErrEnvelopeZeroEnum)
	case e.ProofBackendID == config.ProofBackendNone:
		return nil, fmt.Errorf("%w: ProofBackendID", ErrEnvelopeZeroEnum)
	case e.ProofFormatID == config.ProofFormatNone:
		return nil, fmt.Errorf("%w: ProofFormatID", ErrEnvelopeZeroEnum)
	case e.VerifierID == config.VerifierNone:
		return nil, fmt.Errorf("%w: VerifierID", ErrEnvelopeZeroEnum)
	case e.IdentitySchemeID == config.SigSchemeNone:
		return nil, fmt.Errorf("%w: IdentitySchemeID", ErrEnvelopeZeroEnum)
	case e.FinalitySchemeID == config.SigSchemeNone:
		return nil, fmt.Errorf("%w: FinalitySchemeID", ErrEnvelopeZeroEnum)
	}

	return e, nil
}

// =============================================================================
// ZPublicInputs — the stable ABI shared across backends.
// =============================================================================

// ZPublicInputs is the canonical public inputs every Z-Chain proof
// commits to. The wire layout is fixed: HashZPublicInputs(in) is the
// single 48-byte commitment the envelope binds, and every backend MUST
// derive its own public-inputs digest by the same canonical encoding —
// otherwise the binding check in VerifyZProofUnderProfile rejects.
//
// A change to this layout is a hard fork: the TupleHash injectivity
// means any change to the parts list produces a digest stream that
// does not collide with any prior digest, and downstream signature
// verification breaks. There is no compatibility window.
type ZPublicInputs struct {
	Version   uint16
	ProfileID uint32
	NetworkID uint32
	ChainID   uint32
	Epoch     uint64

	PreviousZStateRoot    [48]byte
	NewZStateRoot         [48]byte
	TxBatchHash           [48]byte
	IdentityRoot          [48]byte
	ValidatorRegistryRoot [48]byte
	RevocationRoot        [48]byte
	StakeWeightRoot       [48]byte
	CommitteeRoot         [48]byte
	DKGTranscriptRoot     [48]byte
	GroupPublicKeyHash    [48]byte
	QChainTipHash         [48]byte
	EpochCommitmentHash   [48]byte

	HashSuiteID      config.HashSuiteID
	IdentitySchemeID config.SigSchemeID
	FinalitySchemeID config.SigSchemeID
	ProofPolicyID    config.ProofPolicyID
}

// HashZPublicInputs returns the 48-byte canonical digest over in. Every
// field is bound exactly once; the customization tag pins the layout.
// Mutating any field MUST change the digest (see the
// TestHashZPublicInputs_FieldMutationChangesHash test).
//
// Panics on a nil input. A nil ZPublicInputs is a programmer error;
// there is no legitimate "prove over no public inputs" path, and
// returning a well-defined constant for nil opens a constant-collision
// attack class (see F84). Verifier callers MUST construct an explicit
// ZPublicInputs from chain state before invoking this function.
func HashZPublicInputs(in *ZPublicInputs) [48]byte {
	if in == nil {
		panic("zchain: HashZPublicInputs called with nil *ZPublicInputs")
	}
	parts := [][]byte{
		[]byte(publicInputsProtocolTag),
		u16BE(in.Version),
		u32BE(in.ProfileID),
		u32BE(in.NetworkID),
		u32BE(in.ChainID),
		u64BE(in.Epoch),
		in.PreviousZStateRoot[:],
		in.NewZStateRoot[:],
		in.TxBatchHash[:],
		in.IdentityRoot[:],
		in.ValidatorRegistryRoot[:],
		in.RevocationRoot[:],
		in.StakeWeightRoot[:],
		in.CommitteeRoot[:],
		in.DKGTranscriptRoot[:],
		in.GroupPublicKeyHash[:],
		in.QChainTipHash[:],
		in.EpochCommitmentHash[:],
		{byte(in.HashSuiteID)},
		{byte(in.IdentitySchemeID)},
		{byte(in.FinalitySchemeID)},
		{byte(in.ProofPolicyID)},
	}
	return tupleHash48(parts, publicInputsCustomization)
}

// =============================================================================
// Reader helpers (private to this file).
// =============================================================================

type envelopeReader struct {
	buf []byte
}

func (r *envelopeReader) need(n int) error {
	if len(r.buf) < n {
		return fmt.Errorf("%w: need %d bytes, have %d",
			io.ErrUnexpectedEOF, n, len(r.buf))
	}
	return nil
}

func (r *envelopeReader) u8() (uint8, error) {
	if err := r.need(1); err != nil {
		return 0, ErrEnvelopeTruncated
	}
	v := r.buf[0]
	r.buf = r.buf[1:]
	return v, nil
}

func (r *envelopeReader) u16() (uint16, error) {
	if err := r.need(2); err != nil {
		return 0, ErrEnvelopeTruncated
	}
	v := binary.BigEndian.Uint16(r.buf[:2])
	r.buf = r.buf[2:]
	return v, nil
}

func (r *envelopeReader) u32() (uint32, error) {
	if err := r.need(4); err != nil {
		return 0, ErrEnvelopeTruncated
	}
	v := binary.BigEndian.Uint32(r.buf[:4])
	r.buf = r.buf[4:]
	return v, nil
}

func (r *envelopeReader) u64() (uint64, error) {
	if err := r.need(8); err != nil {
		return 0, ErrEnvelopeTruncated
	}
	v := binary.BigEndian.Uint64(r.buf[:8])
	r.buf = r.buf[8:]
	return v, nil
}

func (r *envelopeReader) boolByte() (bool, error) {
	v, err := r.u8()
	if err != nil {
		return false, err
	}
	switch v {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("%w: got 0x%02x", ErrEnvelopeBoolByteOutOfRange, v)
	}
}

func (r *envelopeReader) read16(dst *[16]byte) error {
	if err := r.need(16); err != nil {
		return ErrEnvelopeTruncated
	}
	copy(dst[:], r.buf[:16])
	r.buf = r.buf[16:]
	return nil
}

func (r *envelopeReader) read48(dst *[48]byte) error {
	if err := r.need(48); err != nil {
		return ErrEnvelopeTruncated
	}
	copy(dst[:], r.buf[:48])
	r.buf = r.buf[48:]
	return nil
}

func (r *envelopeReader) lenPrefixed() ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if uint64(n) > uint64(len(r.buf)) {
		return nil, fmt.Errorf("%w: declared=%d remaining=%d",
			ErrEnvelopeProofBytesTooLong, n, len(r.buf))
	}
	out := make([]byte, n)
	copy(out, r.buf[:n])
	r.buf = r.buf[n:]
	return out, nil
}
