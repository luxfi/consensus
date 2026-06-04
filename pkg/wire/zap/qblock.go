// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// QBlock is the LP-182 schema 0x02 wire view of a Q-Chain finality
// block (HIP-0079). Distinct from the protocol/quasar/qblock.go
// TranscriptHash domain — that's the cryptographic constant computed over
// these bytes via TupleHash256. This wire schema produces the bytes the
// transcript hashes.
//
// Fixed-section layout: every field listed in HIP-0079 §"Q-Block
// structure". Variable-length tail: the Pulsar-M threshold signature.
type QBlock struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetQBlock_Version                uint16Off = 0
	OffsetQBlock_NetworkID              uint32Off = 2
	OffsetQBlock_ChainID                uint32Off = 6
	OffsetQBlock_Height                 uint64Off = 10
	OffsetQBlock_RoundOrView            uint32Off = 18
	OffsetQBlock_ProfileID              uint32Off = 22
	OffsetQBlock_ParentQBlockHash       bytesOff  = 26
	OffsetQBlock_StateRoot              bytesOff  = 34
	OffsetQBlock_ZChainStateRoot        bytesOff  = 42
	OffsetQBlock_ValidatorSetRoot       bytesOff  = 50
	OffsetQBlock_CommitteeRoot          bytesOff  = 58
	OffsetQBlock_DKGTranscriptRoot      bytesOff  = 66
	OffsetQBlock_GroupPublicKeyHash     bytesOff  = 74
	OffsetQBlock_PayloadRoot            bytesOff  = 82
	OffsetQBlock_DARoot                 bytesOff  = 90
	OffsetQBlock_SignerBitmapCommitment bytesOff  = 98
	OffsetQBlock_HashSuiteID            uint8Off  = 106
	OffsetQBlock_IdentitySchemeID       uint8Off  = 107
	OffsetQBlock_FinalitySchemeID       uint8Off  = 108
	OffsetQBlock_ProofPolicyID          uint8Off  = 109
	OffsetQBlock_ProofBackendID         uint8Off  = 110
	OffsetQBlock_ProofFormatID          uint8Off  = 111
	OffsetQBlock_VerifierID             uint16Off = 112
	OffsetQBlock_Signature              bytesOff  = 114
	SizeQBlock                                    = 122
)

// Type aliases improve readability above; field-offset semantics are
// just int. Each typed offset is the byte position of the field's slot
// within the fixed object section.
type (
	uint8Off  = int
	uint16Off = int
	uint32Off = int
	uint64Off = int
	bytesOff  = int
)

// WrapQBlock parses b as a ZAP message and returns a zero-copy QBlock
// window.
func WrapQBlock(b []byte) (QBlock, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return QBlock{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return QBlock{}, ErrInvalidQuasarCert
	}
	return QBlock{msg: msg, obj: obj}, nil
}

// QBlockFields carries the per-field input set for NewQBlock. Mirrors
// the canonical QBlock struct field-for-field; the wire layer takes a
// value-typed input rather than a pointer so callers cannot accidentally
// mutate the source after the build.
type QBlockFields struct {
	Version                uint16
	NetworkID              uint32
	ChainID                uint32
	Height                 uint64
	RoundOrView            uint32
	ProfileID              uint32
	ParentQBlockHash       [32]byte
	StateRoot              [48]byte
	ZChainStateRoot        [48]byte
	ValidatorSetRoot       [48]byte
	CommitteeRoot          [48]byte
	DKGTranscriptRoot      [48]byte
	GroupPublicKeyHash     [48]byte
	PayloadRoot            [48]byte
	DARoot                 [48]byte
	SignerBitmapCommitment [48]byte
	HashSuiteID            uint8
	IdentitySchemeID       uint8
	FinalitySchemeID       uint8
	ProofPolicyID          uint8
	ProofBackendID         uint8
	ProofFormatID          uint8
	VerifierID             uint16
	Signature              []byte
}

// NewQBlock builds a QBlock wire buffer from fields.
func NewQBlock(f QBlockFields) QBlock {
	b := zap.NewBuilder(512)
	ob := b.StartObject(SizeQBlock)
	ob.SetUint16(OffsetQBlock_Version, f.Version)
	ob.SetUint32(OffsetQBlock_NetworkID, f.NetworkID)
	ob.SetUint32(OffsetQBlock_ChainID, f.ChainID)
	ob.SetUint64(OffsetQBlock_Height, f.Height)
	ob.SetUint32(OffsetQBlock_RoundOrView, f.RoundOrView)
	ob.SetUint32(OffsetQBlock_ProfileID, f.ProfileID)
	ob.SetBytes(OffsetQBlock_ParentQBlockHash, f.ParentQBlockHash[:])
	ob.SetBytes(OffsetQBlock_StateRoot, f.StateRoot[:])
	ob.SetBytes(OffsetQBlock_ZChainStateRoot, f.ZChainStateRoot[:])
	ob.SetBytes(OffsetQBlock_ValidatorSetRoot, f.ValidatorSetRoot[:])
	ob.SetBytes(OffsetQBlock_CommitteeRoot, f.CommitteeRoot[:])
	ob.SetBytes(OffsetQBlock_DKGTranscriptRoot, f.DKGTranscriptRoot[:])
	ob.SetBytes(OffsetQBlock_GroupPublicKeyHash, f.GroupPublicKeyHash[:])
	ob.SetBytes(OffsetQBlock_PayloadRoot, f.PayloadRoot[:])
	ob.SetBytes(OffsetQBlock_DARoot, f.DARoot[:])
	ob.SetBytes(OffsetQBlock_SignerBitmapCommitment, f.SignerBitmapCommitment[:])
	ob.SetUint8(OffsetQBlock_HashSuiteID, f.HashSuiteID)
	ob.SetUint8(OffsetQBlock_IdentitySchemeID, f.IdentitySchemeID)
	ob.SetUint8(OffsetQBlock_FinalitySchemeID, f.FinalitySchemeID)
	ob.SetUint8(OffsetQBlock_ProofPolicyID, f.ProofPolicyID)
	ob.SetUint8(OffsetQBlock_ProofBackendID, f.ProofBackendID)
	ob.SetUint8(OffsetQBlock_ProofFormatID, f.ProofFormatID)
	ob.SetUint16(OffsetQBlock_VerifierID, f.VerifierID)
	ob.SetBytes(OffsetQBlock_Signature, f.Signature)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewQBlock produced unparseable bytes: " + err.Error())
	}
	return QBlock{msg: msg, obj: msg.Root()}
}

// Accessors. Variable-length byte fields return the underlying slice
// (zero-copy); fixed-width hash fields are returned as a fresh array
// copy because Go arrays are value-typed.

func (q QBlock) Version() uint16     { return q.obj.Uint16(OffsetQBlock_Version) }
func (q QBlock) NetworkID() uint32   { return q.obj.Uint32(OffsetQBlock_NetworkID) }
func (q QBlock) ChainID() uint32     { return q.obj.Uint32(OffsetQBlock_ChainID) }
func (q QBlock) Height() uint64      { return q.obj.Uint64(OffsetQBlock_Height) }
func (q QBlock) RoundOrView() uint32 { return q.obj.Uint32(OffsetQBlock_RoundOrView) }
func (q QBlock) ProfileID() uint32   { return q.obj.Uint32(OffsetQBlock_ProfileID) }
func (q QBlock) HashSuiteID() uint8  { return q.obj.Uint8(OffsetQBlock_HashSuiteID) }
func (q QBlock) IdentitySchemeID() uint8 {
	return q.obj.Uint8(OffsetQBlock_IdentitySchemeID)
}
func (q QBlock) FinalitySchemeID() uint8 { return q.obj.Uint8(OffsetQBlock_FinalitySchemeID) }
func (q QBlock) ProofPolicyID() uint8    { return q.obj.Uint8(OffsetQBlock_ProofPolicyID) }
func (q QBlock) ProofBackendID() uint8   { return q.obj.Uint8(OffsetQBlock_ProofBackendID) }
func (q QBlock) ProofFormatID() uint8    { return q.obj.Uint8(OffsetQBlock_ProofFormatID) }
func (q QBlock) VerifierID() uint16      { return q.obj.Uint16(OffsetQBlock_VerifierID) }

func (q QBlock) ParentQBlockHash() [32]byte {
	var out [32]byte
	copy(out[:], q.obj.Bytes(OffsetQBlock_ParentQBlockHash))
	return out
}

func (q QBlock) hash48(off int) [48]byte {
	var out [48]byte
	copy(out[:], q.obj.Bytes(off))
	return out
}

func (q QBlock) StateRoot() [48]byte          { return q.hash48(OffsetQBlock_StateRoot) }
func (q QBlock) ZChainStateRoot() [48]byte    { return q.hash48(OffsetQBlock_ZChainStateRoot) }
func (q QBlock) ValidatorSetRoot() [48]byte   { return q.hash48(OffsetQBlock_ValidatorSetRoot) }
func (q QBlock) CommitteeRoot() [48]byte      { return q.hash48(OffsetQBlock_CommitteeRoot) }
func (q QBlock) DKGTranscriptRoot() [48]byte  { return q.hash48(OffsetQBlock_DKGTranscriptRoot) }
func (q QBlock) GroupPublicKeyHash() [48]byte { return q.hash48(OffsetQBlock_GroupPublicKeyHash) }
func (q QBlock) PayloadRoot() [48]byte        { return q.hash48(OffsetQBlock_PayloadRoot) }
func (q QBlock) DARoot() [48]byte             { return q.hash48(OffsetQBlock_DARoot) }
func (q QBlock) SignerBitmapCommitment() [48]byte {
	return q.hash48(OffsetQBlock_SignerBitmapCommitment)
}

// Signature returns the Pulsar-M threshold signature bytes.
func (q QBlock) Signature() []byte { return q.obj.Bytes(OffsetQBlock_Signature) }

// Bytes returns the underlying ZAP buffer.
func (q QBlock) Bytes() []byte {
	if q.msg == nil {
		return nil
	}
	return q.msg.Bytes()
}

// IsZero reports whether the QBlock is the zero value.
func (q QBlock) IsZero() bool { return q.msg == nil }
