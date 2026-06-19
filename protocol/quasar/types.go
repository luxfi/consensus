// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Package quasar implements post-quantum consensus with event horizon finality.

package quasar

import (
	"context"
	"encoding/binary"
	"errors"
	"time"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
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
//	Pulsar      — Module-LWE threshold ML-DSA  (O(1) after DKG; FIPS 204 Class N1)
//	Corona      — Ring-LWE  threshold ML-DSA   (O(1) after DKG; FIPS 204 Class N1)
//	Magnetar    — SLH-DSA per-validator        (FIPS 205; hash-based; cross-family backstop)
//	MLDSARollup — per-validator ML-DSA-65 rolled into one strict-PQ STARK / FRI proof (P3Q)
//
// Three profile selectors map to which legs are populated, per
// papers/lux-quasar-composition/sections/04-profiles.tex:
//
//	Pulsar profile  (minimum PQ posture):
//	   BLS ‖ Puls ‖ ZK  — one classical, one Module-LWE lattice, rollup
//
//	Aurora profile  (intra-lattice diversity):
//	   BLS ‖ Puls ‖ Cor ‖ ZK  — two independent lattice families
//
//	Polaris profile (cross-family maximum assurance):
//	   BLS ‖ Puls ‖ Cor ‖ Mag ‖ ZK  — two lattices + hash-based backstop
//
// The Polaris profile is the headline "double lattice + hash-based"
// PQ defense-in-depth — Module-LWE ∧ Ring-LWE ∧ SHA3-OW must all be
// broken for an adversary to forge a cert. See polaris.go for the
// composition function and SPEC.md §QuasarCert for the wire format.
//
// Total cert size:
//
//	Pulsar profile:  48 (BLS) + |Pulsar|         + |MLDSARollup|  ≈ 3.5 KB
//	Aurora profile:  48 (BLS) + |Pulsar|+|Corona|+ |MLDSARollup|  ≈ 37 KB
//	Polaris profile: 48 (BLS) + |Pulsar|+|Corona|+|Magnetar|+|MLDSARollup| ≈ 53 KB
//
// Magnetar carries a magnetar.ValidatorAggregateCert wire envelope
// (N per-validator standalone SLH-DSA sigs over the round digest)
// produced by magnetar.BuildAggregateCert and verified by
// magnetar.VerifyAggregateCert.
type QuasarCert struct {
	BLS         []byte    // BLS-12-381 aggregate (48 bytes, classical fast-path; empty in pure-PQ)
	Corona      []byte    // Ring-LWE threshold sig (Corona; O(1) after DKG)
	Pulsar      []byte    // Module-LWE threshold sig (Pulsar-M; O(1) after DKG)
	Magnetar    []byte    // SLH-DSA hash-based defense-in-depth (Polaris profile)
	MLDSARollup []byte    // Succinct rollup of per-validator ML-DSA-65 (strict-PQ STARK/FRI via P3Q)
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
// succinct strict-PQ STARK/FRI proof (P3Q).
func (c *QuasarCert) HasIdentityRollup() bool {
	if c == nil {
		return false
	}
	return len(c.MLDSARollup) > 0
}

// HasHashBased reports whether the cert carries the Magnetar
// (SLH-DSA / FIPS 205) hash-based defense-in-depth leg. This is the
// Polaris profile's load-bearing cross-family guarantee: if every
// lattice family (Module-LWE, Ring-LWE) breaks, hash-based one-wayness
// still secures finality.
func (c *QuasarCert) HasHashBased() bool {
	if c == nil {
		return false
	}
	return len(c.Magnetar) > 0
}

// IsPolaris reports whether this cert composes all three production
// PQ schemes: Pulsar (Module-LWE threshold), Corona (Ring-LWE
// threshold), and Magnetar (SLH-DSA hash-based). This is the maximum-
// assurance profile per papers/lux-quasar-composition/sections/04.
func (c *QuasarCert) IsPolaris() bool {
	if c == nil {
		return false
	}
	return len(c.Pulsar) > 0 && len(c.Corona) > 0 && len(c.Magnetar) > 0
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
// message is the digest the signers committed to.
//
// POLICY NOTE (Red H3). This is the implied-policy entry point:
// production code that knows the chain's config.CertPolicy MUST prefer
// VerifyUnderPolicy, which decides the mandatory leg set from the policy.
// Here the *supplied key set* is the policy declaration — a trusted,
// caller-supplied input (NOT attacker-controlled cert bytes):
//
//	BLS     — required iff blsAggPubKey != nil. nil ⇒ pure-PQ posture,
//	          and a no-BLS cert is accepted (the old hard-mandatory BLS
//	          check that wrongly rejected pure-PQ certs is removed).
//	Corona  — required iff rtGroupKey != nil. A supplied key means the
//	          leg MUST be present and verify (an attacker cannot strip it).
//	Rollup  — present-implies-verify (never silently skipped).
//
// When blsAggPubKey == nil, at least one PQ leg (Corona or rollup) MUST
// be present and verify — an all-empty cert never finalises.
//
// To verify the Polaris-profile Pulsar + Magnetar legs as well, call
// VerifyWithRealKeysPolaris.
func (c *QuasarCert) VerifyWithRealKeys(message []byte, blsAggPubKey *bls.PublicKey, rtGroupKey *coronaThreshold.GroupKey, mldsaPubKeys []*mldsa.PublicKey) bool {
	if c == nil || len(message) == 0 {
		return false
	}

	verifiedAnyPQ := false

	// BLS aggregate (classical). Required iff a key is supplied.
	if blsAggPubKey != nil {
		if !verifyBLSLeg(message, blsAggPubKey, c.BLS) {
			return false
		}
	}

	// Corona threshold (Ring-LWE PQ). Required iff a key is supplied;
	// a supplied key forbids stripping the leg.
	if rtGroupKey != nil {
		if !verifyCoronaLeg(message, rtGroupKey, c.Corona) {
			return false
		}
		verifiedAnyPQ = true
	}

	// ML-DSA-65 identity rollup (FIPS 204 PQ). Required iff ML-DSA keys
	// are supplied — symmetric with the Corona/Pulsar/Magnetar legs: a
	// supplied key means the verifier was configured to check this leg, so
	// an absent rollup is a downgrade and is rejected. Without keys, a
	// present rollup cannot be verified and is likewise rejected (never
	// silently skipped).
	if len(mldsaPubKeys) > 0 {
		if !verifyMLDSARollupLeg(message, mldsaPubKeys, c.MLDSARollup) {
			return false
		}
		verifiedAnyPQ = true
	} else if len(c.MLDSARollup) > 0 {
		// Rollup bytes present but no key to verify them: fail closed.
		return false
	}

	// An all-classical cert with no BLS key, or an empty cert, never
	// finalises: a pure-PQ posture (blsAggPubKey == nil) demands a PQ
	// leg actually verified.
	if blsAggPubKey == nil && !verifiedAnyPQ {
		return false
	}

	return true
}

// VerifyWithRealKeysPolaris performs cryptographic verification of
// every leg present in a Polaris-profile cert. In addition to the
// classical BLS + Corona + ML-DSA legs verified by VerifyWithRealKeys,
// this method also verifies:
//
//   - Pulsar (Module-LWE threshold ML-DSA), if c.Pulsar is non-empty
//     and pulsarGroupKey is non-nil. The Pulsar leg carries a single
//     FIPS 204 ML-DSA signature byte-equal to what unmodified
//     ML-DSA.Verify accepts, so the routine routes through the
//     pulsar package's stateless VerifyBytes.
//
//   - Magnetar (SLH-DSA / FIPS 205 hash-based), if c.Magnetar is
//     non-empty. The Magnetar leg carries a magnetar.ValidatorAggregateCert
//     wire blob (per-validator standalone SLH-DSA signatures) and
//     verifies through magnetar.VerifyAggregateCert against
//     knownMagnetarValidators. Verification accepts the cert iff
//     every claimed signer's signature verifies AND the count meets
//     the configured quorum (here, every signer must verify).
//
// A nil entry skips that leg's verification — exactly the semantics
// of VerifyWithRealKeys for the legs it covers.
func (c *QuasarCert) VerifyWithRealKeysPolaris(
	message []byte,
	blsAggPubKey *bls.PublicKey,
	coronaGroupKey *coronaThreshold.GroupKey,
	pulsarGroupKey []byte,
	mldsaPubKeys []*mldsa.PublicKey,
	knownMagnetarValidators map[magnetar.NodeID][]byte,
) bool {
	if !c.VerifyWithRealKeys(message, blsAggPubKey, coronaGroupKey, mldsaPubKeys) {
		return false
	}

	// Pulsar leg — Module-LWE threshold ML-DSA. Required iff a group key
	// is supplied; a supplied key forbids stripping the leg.
	if len(pulsarGroupKey) > 0 {
		if !verifyPulsarLeg(message, pulsarGroupKey, c.Pulsar) {
			return false
		}
	}

	// Magnetar leg — SLH-DSA / FIPS 205 hash-based. Required iff the
	// known-validator set is supplied; a supplied set forbids stripping
	// the leg. Polaris quorum policy: every claimed signer must verify
	// (enforced inside verifyMagnetarLeg).
	if len(knownMagnetarValidators) > 0 {
		if !verifyMagnetarLeg(message, knownMagnetarValidators, c.Magnetar) {
			return false
		}
	}

	return true
}

// QuasarCert byte serialization layout (Polaris-ready, five legs):
//
//	[scheme:1=SigQuasarPolaris(=0x05)]
//	[bls_len:2 BE][bls:N]
//	[corona_len:4 BE][corona:M]      // CORS-framed corona/threshold.Signature
//	[pulsar_len:4 BE][pulsar:P]      // PULS-framed pulsar.Signature
//	[magnetar_len:4 BE][magnetar:Q]  // MAGS-framed magnetar.Signature
//	                                  // OR magnetar.ValidatorAggregateCert wire bytes
//	[mldsa_len:4 BE][mldsa:K]        // EncodeMLDSASigs or single strict-PQ STARK/FRI (P3Q)
//	[epoch:8 BE]
//	[finality_unix_ns:8 BE]
//	[validators:2 BE]
//
// CertSchemeQuasar is the leading byte tag. 0x05 carries the
// five-leg layout (Polaris-ready); the legacy 0x04 three-leg layout
// is dropped per "no backwards compatibility only forwards
// perfection" — pre-Polaris chains MUST re-cert at the rotation
// window.
const CertSchemeQuasar byte = 0x05

// ErrCertCorrupt is returned when QuasarCert.UnmarshalBinary cannot decode
// the input.
var ErrCertCorrupt = errors.New("quasar: certificate corrupt")

// minCertSize is the smallest possible serialized cert:
// scheme(1) + bls_len(2) + corona_len(4) + pulsar_len(4) +
// magnetar_len(4) + mldsa_len(4) + epoch(8) + finality(8) +
// validators(2) = 37 bytes with every leg empty.
const minCertSize = 1 + 2 + 4 + 4 + 4 + 4 + 8 + 8 + 2

// MarshalBinary serializes the cert into a self-describing byte slice.
func (c *QuasarCert) MarshalBinary() ([]byte, error) {
	if c == nil {
		return nil, errors.New("quasar: nil cert")
	}
	if len(c.BLS) > 0xFFFF {
		return nil, errors.New("quasar: BLS aggregate too large")
	}

	total := minCertSize + len(c.BLS) + len(c.Corona) + len(c.Pulsar) + len(c.Magnetar) + len(c.MLDSARollup)
	out := make([]byte, 0, total)
	out = append(out, CertSchemeQuasar)

	var u16 [2]byte
	var u32 [4]byte
	var u64 [8]byte

	binary.BigEndian.PutUint16(u16[:], uint16(len(c.BLS)))
	out = append(out, u16[:]...)
	out = append(out, c.BLS...)

	binary.BigEndian.PutUint32(u32[:], uint32(len(c.Corona)))
	out = append(out, u32[:]...)
	out = append(out, c.Corona...)

	binary.BigEndian.PutUint32(u32[:], uint32(len(c.Pulsar)))
	out = append(out, u32[:]...)
	out = append(out, c.Pulsar...)

	binary.BigEndian.PutUint32(u32[:], uint32(len(c.Magnetar)))
	out = append(out, u32[:]...)
	out = append(out, c.Magnetar...)

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
	if len(data) < minCertSize {
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

	if off+4 > len(data) {
		return ErrCertCorrupt
	}
	coronaLen := int(binary.BigEndian.Uint32(data[off:]))
	off += 4
	if off+coronaLen > len(data) {
		return ErrCertCorrupt
	}
	c.Corona = append(c.Corona[:0], data[off:off+coronaLen]...)
	off += coronaLen

	if off+4 > len(data) {
		return ErrCertCorrupt
	}
	pulsarLen := int(binary.BigEndian.Uint32(data[off:]))
	off += 4
	if off+pulsarLen > len(data) {
		return ErrCertCorrupt
	}
	c.Pulsar = append(c.Pulsar[:0], data[off:off+pulsarLen]...)
	off += pulsarLen

	if off+4 > len(data) {
		return ErrCertCorrupt
	}
	magnetarLen := int(binary.BigEndian.Uint32(data[off:]))
	off += 4
	if off+magnetarLen > len(data) {
		return ErrCertCorrupt
	}
	c.Magnetar = append(c.Magnetar[:0], data[off:off+magnetarLen]...)
	off += magnetarLen

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
