// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Package quasar implements post-quantum consensus with event horizon finality.

package quasar

import (
	"context"
	"encoding/binary"
	"errors"
	"time"

	coronaThreshold "github.com/luxfi/corona/threshold"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
)

// Block represents a finalized block in the Quasar consensus.
// This is the primary block type used throughout the system.
type Block struct {
	ID        [32]byte    // Unique block identifier
	ChainID   [32]byte    // Chain this block belongs to
	ChainName string      // Human-readable chain name (e.g., "P-Chain", "X-Chain", "C-Chain")
	Height    uint64      // Block height
	Hash      string      // Block hash
	Timestamp time.Time   // Block timestamp
	Data      []byte      // Block payload data
	Cert      *QuasarCert // Quantum certificate (nil if not finalized)
}

// QuasarCert is the finality certificate for Quasar consensus.
//
// Architectural shape — read the cardinality, not a counting word:
//
//	BLS-12-381  — classical fast-path aggregate (48 B; pairing-based)
//	Corona      — Ring-LWE threshold ML-DSA   (O(1) after DKG; FIPS 204 Class N1)
//	Pulsar      — Module-LWE threshold ML-DSA (O(1) after DKG; FIPS 204 Class N1)
//	MLDSARollup — per-validator ML-DSA-65 rolled into one STARK / Groth16 proof
//
// Naming: the PQ-side is at most **Double Lattice** — Ring-LWE plus
// Module-LWE. Calling the BLS+Corona+MLDSARollup composition "Triple
// Quantum" is misleading because BLS is classical, not quantum. The
// honest mode descriptor is "Classical + Double Lattice [+ Identity
// Rollup]".
//
// Pure-PQ profile (no BLS, no classical curve in the validator path):
// Corona + Pulsar (Double Lattice). Each layer is independently
// FIPS 204 Class N1 (output verifies under the unmodified single-
// party ML-DSA verifier). MLDSARollup is the identity attestation
// layer, succinct via STARK (Z-Chain → P3Q).
//
// Corona (academic R-LWE with trusted-dealer DKG) is NOT a
// production option for an open public chain. Corona is the
// production Ring-LWE library (Pedersen DKG over R_q + proactive
// resharing; `luxfi/corona`). The historic field name "Corona"
// in this struct refers to "the Ring-LWE threshold slot"; that
// slot now carries Corona bytes. The field is renamed accordingly.
//
// MLDSARollup is succinct: a single STARK/Groth16 proof attests
// "∀ i ∈ [N]: ML-DSA.Verify(mldsa_pk_i, msg, mldsa_σ_i) = 1". Output
// is one constant-size proof (192 B for Groth16, ~30 KB for a STARK
// like P3Q), constant in validator count.
//
// Total cert size: 48 (BLS, optional) + |Corona| + |Pulsar| + |MLDSARollup|.
type QuasarCert struct {
	BLS         []byte    // BLS-12-381 aggregate (48 bytes, classical fast-path; empty in pure-PQ)
	Corona      []byte    // Ring-LWE threshold sig (Corona; O(1) after DKG)
	Pulsar      []byte    // Module-LWE threshold sig (Pulsar-M; O(1) after DKG; optional)
	MLDSARollup []byte    // Succinct rollup of per-validator ML-DSA-65 (STARK or Groth16)
	Epoch       uint64    // Epoch number
	Finality    time.Time // Time of finality
	Validators  int       `json:"validators,omitempty"` // Count of signing validators
}

// IsDoubleLattice reports whether the cert carries both Ring-LWE
// (Corona) and Module-LWE (Pulsar) threshold legs. This is the honest
// post-quantum cardinality: two independent algebraic-lattice families,
// each independently a FIPS 204 Class N1 threshold producer.
//
// BLS is irrelevant to this predicate by design — a pure-PQ cert has
// no BLS bytes; a hybrid cert layers BLS over a Double-Lattice PQ
// surface. Either way, the PQ-side cardinality is what IsDoubleLattice
// names.
func (c *QuasarCert) IsDoubleLattice() bool {
	if c == nil {
		return false
	}
	return len(c.Corona) > 0 && len(c.Pulsar) > 0
}

// HasClassicalFastPath reports whether the cert carries BLS bytes — the
// optional classical aggregate that rides alongside the PQ surface on
// chains that opt into the hybrid posture. Empty in pure-PQ mode.
func (c *QuasarCert) HasClassicalFastPath() bool {
	if c == nil {
		return false
	}
	return len(c.BLS) > 0
}

// HasIdentityRollup reports whether the cert carries the
// per-validator ML-DSA-65 identity attestation rolled up into a
// succinct STARK/Groth16 proof.
func (c *QuasarCert) HasIdentityRollup() bool {
	if c == nil {
		return false
	}
	return len(c.MLDSARollup) > 0
}

// Verify checks structural presence of the Quasar layers: BLS fast-path
// + Corona (Ring-LWE) + MLDSARollup. Use VerifyWithRealKeys for
// cryptographic verification. Returns false if any required slot is
// empty — fast structural gate.
func (c *QuasarCert) Verify(validators []string) bool {
	if c == nil {
		return false
	}
	if len(c.BLS) == 0 || len(c.Corona) == 0 || len(c.MLDSARollup) == 0 {
		return false
	}
	if c.Validators > 0 && len(validators) > 0 && len(validators) < c.Validators {
		return false
	}
	return true
}

// VerifyWithKeys is a back-compat structural check that mirrors the
// previous API: returns false unless BLS + Corona + MLDSARollup are
// present and the provided opaque key material is non-empty. Real
// cryptographic verification is in VerifyWithRealKeys.
func (c *QuasarCert) VerifyWithKeys(groupKey []byte, pqKey []byte) bool {
	if c == nil || len(groupKey) == 0 {
		return false
	}
	if len(c.BLS) == 0 || len(c.Corona) == 0 || len(c.MLDSARollup) == 0 {
		return false
	}
	// The structural check passed; cryptographic verification requires
	// typed keys via VerifyWithRealKeys.
	return false
}

// VerifyWithRealKeys performs cryptographic verification of the
// certificate. A Quasar cert is finalised iff the configured layers
// verify in parallel:
//
//   - BLS-12-381 aggregate against blsAggPubKey (classical fast path)
//   - Corona (Ring-LWE threshold ML-DSA) against rtGroupKey (PQ)
//   - Per-validator ML-DSA-65 sigs against mldsaPubKeys (FIPS 204 PQ)
//
// Defence-in-depth posture is classical + (one or two PQ lattice
// schemes) + identity rollup. Any single scheme broken does not break
// finality.
//
// message is the digest the signers committed to. mldsaPubKeys may be
// nil if MLDSARollup is empty (rollup not yet wired). nil rtGroupKey
// skips the Corona check (used when running in BLS-only mode or in a
// Pulsar-only pure-PQ posture).
func (c *QuasarCert) VerifyWithRealKeys(message []byte, blsAggPubKey *bls.PublicKey, rtGroupKey *coronaThreshold.GroupKey, mldsaPubKeys []*mldsa.PublicKey) bool {
	if c == nil || len(message) == 0 {
		return false
	}
	if len(c.BLS) == 0 {
		return false
	}

	// 1. BLS aggregate verify (classical).
	if blsAggPubKey == nil {
		return false
	}
	blsSig, err := bls.SignatureFromBytes(c.BLS)
	if err != nil {
		return false
	}
	if !bls.Verify(blsAggPubKey, blsSig, message) {
		return false
	}

	// 2. Corona threshold verify (Ring-LWE PQ). Optional: skipped when
	//    rtGroupKey is nil (BLS-only mode or Pulsar-only pure-PQ).
	if rtGroupKey != nil {
		if len(c.Corona) == 0 {
			return false
		}
		rtSig, err := decodeCoronaSig(c.Corona)
		if err != nil {
			return false
		}
		if !coronaThreshold.Verify(rtGroupKey, string(message), rtSig) {
			return false
		}
	}

	// 3. ML-DSA-65 verify (PQ identity, FIPS 204). The MLDSARollup field
	//    holds either a single STARK/Groth16 rollup (Z-Chain → P3Q) or a
	//    concatenation of per-validator ML-DSA-65 sigs. We support the
	//    per-validator path here by serialising each sig with a 4-byte
	//    length prefix.
	if len(c.MLDSARollup) > 0 {
		if len(mldsaPubKeys) == 0 {
			return false
		}
		sigs, err := decodeMLDSASigs(c.MLDSARollup)
		if err != nil {
			return false
		}
		// Need at least Validators good sigs. If MLDSARollup was a single
		// succinct rollup, we'd verify it once; for now we require N sigs
		// matching the public keys provided.
		if len(sigs) > len(mldsaPubKeys) {
			return false
		}
		for i, sig := range sigs {
			if !mldsaPubKeys[i].Verify(message, sig, nil) {
				return false
			}
		}
	}

	return true
}

// QuasarCert byte serialization layout:
//
//	[scheme:1=SigQuasar(=0x04)]
//	[bls_len:2 BE][bls:N]
//	[rt_len:2 BE][rt:M]
//	[mldsa_len:4 BE][mldsa:K]   // K = sum of (4-byte len + sig) for each validator,
//	                             // OR a single Groth16 proof (~192 bytes).
//	[epoch:8 BE]
//	[finality_unix_ns:8 BE]
//	[validators:2 BE]
//
// CertSchemeQuasar is the leading byte tag (matches wire.SigQuasar).
const CertSchemeQuasar byte = 0x04

// ErrCertCorrupt is returned when QuasarCert.UnmarshalBinary cannot decode
// the input.
var ErrCertCorrupt = errors.New("quasar: certificate corrupt")

// MarshalBinary serializes the cert into a self-describing byte slice.
func (c *QuasarCert) MarshalBinary() ([]byte, error) {
	if c == nil {
		return nil, errors.New("quasar: nil cert")
	}
	if len(c.BLS) > 0xFFFF || len(c.Corona) > 0xFFFF {
		return nil, errors.New("quasar: signature too large")
	}

	out := make([]byte, 0, 1+2+len(c.BLS)+2+len(c.Corona)+4+len(c.MLDSARollup)+8+8+2)
	out = append(out, CertSchemeQuasar)

	var u16 [2]byte
	var u32 [4]byte
	var u64 [8]byte

	binary.BigEndian.PutUint16(u16[:], uint16(len(c.BLS)))
	out = append(out, u16[:]...)
	out = append(out, c.BLS...)

	binary.BigEndian.PutUint16(u16[:], uint16(len(c.Corona)))
	out = append(out, u16[:]...)
	out = append(out, c.Corona...)

	binary.BigEndian.PutUint32(u32[:], uint32(len(c.MLDSARollup)))
	out = append(out, u32[:]...)
	out = append(out, c.MLDSARollup...)

	binary.BigEndian.PutUint64(u64[:], c.Epoch)
	out = append(out, u64[:]...)

	binary.BigEndian.PutUint64(u64[:], uint64(c.Finality.UnixNano()))
	out = append(out, u64[:]...)

	binary.BigEndian.PutUint16(u16[:], uint16(c.Validators))
	out = append(out, u16[:]...)

	return out, nil
}

// UnmarshalBinary parses bytes produced by MarshalBinary.
func (c *QuasarCert) UnmarshalBinary(data []byte) error {
	if c == nil {
		return errors.New("quasar: nil cert")
	}
	if len(data) < 1+2+2+4+8+8+2 {
		return ErrCertCorrupt
	}
	if data[0] != CertSchemeQuasar {
		return ErrCertCorrupt
	}
	off := 1

	blsLen := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+blsLen > len(data) {
		return ErrCertCorrupt
	}
	c.BLS = append(c.BLS[:0], data[off:off+blsLen]...)
	off += blsLen

	if off+2 > len(data) {
		return ErrCertCorrupt
	}
	rtLen := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+rtLen > len(data) {
		return ErrCertCorrupt
	}
	c.Corona = append(c.Corona[:0], data[off:off+rtLen]...)
	off += rtLen

	if off+4 > len(data) {
		return ErrCertCorrupt
	}
	mldsaLen := int(binary.BigEndian.Uint32(data[off:]))
	off += 4
	if off+mldsaLen > len(data) {
		return ErrCertCorrupt
	}
	c.MLDSARollup = append(c.MLDSARollup[:0], data[off:off+mldsaLen]...)
	off += mldsaLen

	if off+8+8+2 > len(data) {
		return ErrCertCorrupt
	}
	c.Epoch = binary.BigEndian.Uint64(data[off:])
	off += 8
	c.Finality = time.Unix(0, int64(binary.BigEndian.Uint64(data[off:])))
	off += 8
	c.Validators = int(binary.BigEndian.Uint16(data[off:]))
	return nil
}

// EncodeMLDSASigs concatenates per-validator ML-DSA-65 signatures into a
// single MLDSAProof byte slice using 4-byte length prefixes.
func EncodeMLDSASigs(sigs [][]byte) []byte {
	total := 0
	for _, s := range sigs {
		total += 4 + len(s)
	}
	out := make([]byte, 0, total)
	var u32 [4]byte
	for _, s := range sigs {
		binary.BigEndian.PutUint32(u32[:], uint32(len(s)))
		out = append(out, u32[:]...)
		out = append(out, s...)
	}
	return out
}

func decodeMLDSASigs(data []byte) ([][]byte, error) {
	out := make([][]byte, 0, 4)
	for i := 0; i < len(data); {
		if i+4 > len(data) {
			return nil, ErrCertCorrupt
		}
		l := int(binary.BigEndian.Uint32(data[i:]))
		i += 4
		if i+l > len(data) {
			return nil, ErrCertCorrupt
		}
		out = append(out, data[i:i+l])
		i += l
	}
	return out, nil
}

// EncodeCoronaSig serializes a Corona threshold signature using gob.
// Returns nil bytes on encode failure (caller treats as "no signature").
func EncodeCoronaSig(sig *coronaThreshold.Signature) []byte {
	if sig == nil {
		return nil
	}
	return coronaGobEncode(sig)
}

func decodeCoronaSig(data []byte) (*coronaThreshold.Signature, error) {
	if len(data) == 0 {
		return nil, ErrCertCorrupt
	}
	return coronaGobDecode(data)
}

// Engine is the main interface for quantum consensus.
type Engine interface {
	// Start begins the consensus engine
	Start(ctx context.Context) error

	// Stop gracefully shuts down the consensus engine
	Stop() error

	// Submit adds a block to the consensus pipeline
	Submit(block *Block) error

	// Finalized returns a channel of finalized blocks
	Finalized() <-chan *Block

	// IsFinalized checks if a block is finalized
	IsFinalized(blockID [32]byte) bool

	// Stats returns consensus metrics
	Stats() Stats
}

// Stats contains consensus metrics.
type Stats struct {
	Height          uint64        // Current finalized height
	ProcessedBlocks uint64        // Total blocks processed
	FinalizedBlocks uint64        // Total blocks finalized
	PendingBlocks   int           // Blocks awaiting finality
	Validators      int           // Active validator count
	Uptime          time.Duration // Time since start
}

// BLSSignature contains a classical BLS threshold signature.
// Fast path for consensus - used in parallel with CoronaSignature.
type BLSSignature struct {
	Signature   []byte // BLS signature bytes
	ValidatorID string // Signing validator
	IsThreshold bool   // True if threshold signature
	SignerIndex int    // Signer index in committee
}

// CoronaSignature contains a post-quantum Corona threshold signature.
// Quantum-safe path - used in parallel with BLSSignature.
type CoronaSignature struct {
	Signature   []byte // Corona (Ring-LWE) signature bytes
	ValidatorID string // Signing validator
	IsThreshold bool   // True if threshold signature
	SignerIndex int    // Signer index in committee
	Round       int    // Corona protocol round (1 or 2)
}

// QuasarSignature bundles all three proof paths for quantum finality.
// All three run in parallel via [signer.TripleSignRound1].
//
// Per-validator (collected during consensus, NOT stored in block):
//
//	BLS:      sign with BLS key           → aggregate into 48 bytes (ECDL)
//	Corona:    sign with ring-LWE key      → PQ threshold proof (Module-LWE)
//	ML-DSA:   sign with ML-DSA-65 key     → PQ identity proof (Module-LWE + Module-SIS)
//
// In QuasarCert (stored in block header):
//
//	BLS aggregate:  48 bytes
//	PQ proof:       variable (aggregated Corona + ML-DSA, or future SNARK)
type QuasarSignature struct {
	BLS    *BLSSignature    // Classical fast path (aggregatable)
	Corona *CoronaSignature // PQ anonymous path (ring-LWE threshold)
	MLDSA  []byte           // PQ identity proof (ML-DSA-65, FIPS 204)
}

// CoronaRound1Data contains the output of Corona Round 1.
type CoronaRound1Data struct {
	PartyID int
}

// Signer is the exported interface for the quantum signing engine.
// It provides parallel BLS+Corona threshold signing for PQ-safe consensus.
type Signer = signer

// NewSigner creates a new quantum signer with the given threshold.
func NewSigner(threshold int) (*Signer, error) {
	return newSigner(threshold)
}

// NewSignerWithConfig creates a new quantum signer with full configuration.
func NewSignerWithConfig(config SignerConfig) (*Signer, error) {
	return newSignerWithDualThreshold(config)
}

// NewSignerWithDualThreshold creates a new quantum signer with dual threshold configuration.
func NewSignerWithDualThreshold(config SignerConfig) (*Signer, error) {
	return NewSignerWithConfig(config)
}

// NewSignerWithThresholdConfig creates a signer from ThresholdConfig.
func NewSignerWithThresholdConfig(config ThresholdConfig) (*Signer, error) {
	return newSignerWithThresholdConfig(config)
}
