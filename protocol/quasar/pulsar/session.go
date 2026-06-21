// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// session.go -- the consensus-side session binding for a Pulsar round.
//
// PulsarSession captures every field that pins one threshold-sign ceremony to
// exactly one consensus round, so a signature produced for (epoch, height,
// round, block) can never be replayed into a different round, committee, or
// joint key. The 32-byte session_id is the domain-separated hash of all
// fields; every signer recomputes it and refuses to sign a mismatched session.

package pulsar

import (
	"encoding/binary"

	"golang.org/x/crypto/sha3"
)

// sessionDomain domain-separates the Pulsar session-id hash from every other
// hash in the system (FIPS 204 mu, the nonce-cert payload root, the round
// digest, etc.). cSHAKE customization plus an in-band protocol tag give two
// independent separation layers.
const (
	sessionCustomization = "QUASAR-PULSAR-SESSION-v1"
	sessionProtocolTag   = "Quasar/Pulsar/Session/v1"
)

// PulsarSession is the consensus binding for one Pulsar threshold-sign round.
// Every field is consensus-owned; none is secret. The zero value is invalid
// (Validate rejects all-zero security-relevant roots) so a caller cannot
// silently sign an unbound session.
type PulsarSession struct {
	ChainID           [32]byte // chain this round finalizes on
	Epoch             uint64   // DKG / committee epoch
	Height            uint64   // block height
	Round             uint64   // intra-height round / view
	BlockHash         [32]byte // the block (item) being certified
	ValidatorSetRoot  [32]byte // commitment to the active validator set
	JointPKID         [32]byte // identifier of the committee joint ML-DSA key
	DKGTranscriptRoot [32]byte // commitment to the DKG transcript that made the key
	CommitteeID       [32]byte // identifier of the sampled committee
	SignerSetRoot     [32]byte // commitment to the eligible signer set
	NoncePoolRoot     [32]byte // Merkle root of the background NonceCert pool
	ProtocolVersion   uint32   // wire/protocol version
}

// SessionID derives the 32-byte session identifier by hashing every binding
// field under a domain-separated cSHAKE256. Each variable-position field is
// fixed-width (arrays) or length-free fixed integers, so the concatenation is
// unambiguous (canonical encoding -- no field-boundary grinding).
//
// Field order is fixed and MUST match the task's session_id definition:
//
//	H(chain_id ‖ epoch ‖ height ‖ round ‖ block_hash ‖ validator_set_root ‖
//	  joint_pk_id ‖ dkg_transcript_root ‖ committee_id ‖ signer_set_root ‖
//	  nonce_pool_root ‖ protocol_version)
func (s *PulsarSession) SessionID() [32]byte {
	h := sha3.NewCShake256(nil, []byte(sessionCustomization))
	_, _ = h.Write([]byte(sessionProtocolTag))

	var u8 [8]byte
	var u4 [4]byte

	_, _ = h.Write(s.ChainID[:])
	binary.BigEndian.PutUint64(u8[:], s.Epoch)
	_, _ = h.Write(u8[:])
	binary.BigEndian.PutUint64(u8[:], s.Height)
	_, _ = h.Write(u8[:])
	binary.BigEndian.PutUint64(u8[:], s.Round)
	_, _ = h.Write(u8[:])
	_, _ = h.Write(s.BlockHash[:])
	_, _ = h.Write(s.ValidatorSetRoot[:])
	_, _ = h.Write(s.JointPKID[:])
	_, _ = h.Write(s.DKGTranscriptRoot[:])
	_, _ = h.Write(s.CommitteeID[:])
	_, _ = h.Write(s.SignerSetRoot[:])
	_, _ = h.Write(s.NoncePoolRoot[:])
	binary.BigEndian.PutUint32(u4[:], s.ProtocolVersion)
	_, _ = h.Write(u4[:])

	var out [32]byte
	_, _ = h.Read(out[:])
	return out
}

// Validate refuses a session whose security-relevant roots are all zero. A
// zero root means an unbound field (e.g. a missing DKG transcript or
// validator-set commitment); signing under it would let an adversary reuse the
// signature across rounds. ProtocolVersion and the numeric height/round/epoch
// are allowed to be zero (genesis, view 0).
func (s *PulsarSession) Validate() error {
	zero := [32]byte{}
	switch {
	case s.ChainID == zero:
		return ErrSessionFieldUnset
	case s.BlockHash == zero:
		return ErrSessionFieldUnset
	case s.ValidatorSetRoot == zero:
		return ErrSessionFieldUnset
	case s.JointPKID == zero:
		return ErrSessionFieldUnset
	case s.DKGTranscriptRoot == zero:
		return ErrSessionFieldUnset
	case s.CommitteeID == zero:
		return ErrSessionFieldUnset
	case s.SignerSetRoot == zero:
		return ErrSessionFieldUnset
	case s.NoncePoolRoot == zero:
		return ErrSessionFieldUnset
	}
	return nil
}
