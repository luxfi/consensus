// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"crypto/sha256"
	"errors"
)

// =============================================================================
// ML-DSA CREDENTIAL TYPES FOR X-CHAIN
// =============================================================================
// ML-DSA (Module Lattice Digital Signature Algorithm) provides post-quantum
// secure signatures for X-Chain UTXOs. This ensures that spending credentials
// remain secure against quantum attacks.
//
// FIPS 204 (ML-DSA) security levels:
// - ML-DSA-44: Category 2 (128-bit security)
// - ML-DSA-65: Category 3 (192-bit security) [DEFAULT]
// - ML-DSA-87: Category 5 (256-bit security)
// =============================================================================

// Credential type tags for X-Chain
const (
	// CredentialTypeSecp256k1 is the legacy ECDSA credential (being phased out)
	CredentialTypeSecp256k1 byte = 0x00

	// CredentialTypeEd25519 is Edwards curve credential
	CredentialTypeEd25519 byte = 0x01

	// CredentialTypeBLS is BLS12-381 credential
	CredentialTypeBLS byte = 0x02

	// CredentialTypeMLDSA44 is ML-DSA Category 2 (128-bit post-quantum security)
	CredentialTypeMLDSA44 byte = 0x10

	// CredentialTypeMLDSA65 is ML-DSA Category 3 (192-bit post-quantum security) [RECOMMENDED]
	CredentialTypeMLDSA65 byte = 0x11

	// CredentialTypeMLDSA87 is ML-DSA Category 5 (256-bit post-quantum security)
	CredentialTypeMLDSA87 byte = 0x12
)

// ML-DSA signature sizes (FIPS 204)
const (
	// MLDSA44SignatureSize is the signature size for ML-DSA-44
	MLDSA44SignatureSize = 2420

	// MLDSA65SignatureSize is the signature size for ML-DSA-65
	MLDSA65SignatureSize = 3293

	// MLDSA87SignatureSize is the signature size for ML-DSA-87
	MLDSA87SignatureSize = 4595
)

// ML-DSA public key sizes (FIPS 204)
const (
	// MLDSA44PublicKeySize is the public key size for ML-DSA-44
	MLDSA44PublicKeySize = 1312

	// MLDSA65PublicKeySize is the public key size for ML-DSA-65
	MLDSA65PublicKeySize = 1952

	// MLDSA87PublicKeySize is the public key size for ML-DSA-87
	MLDSA87PublicKeySize = 2592
)

// Errors for credential validation
var (
	ErrInvalidCredentialType = errors.New("invalid credential type")
	ErrCredentialTooShort    = errors.New("credential data too short")
	ErrInvalidSignatureSize  = errors.New("invalid signature size for credential type")
	ErrInvalidPublicKeySize  = errors.New("invalid public key size for credential type")
)

// Credential represents a spending credential for X-Chain UTXOs
type Credential struct {
	// Type identifies the credential algorithm
	Type byte `json:"type"`

	// Signatures are the signatures needed to satisfy the output
	Signatures [][]byte `json:"signatures"`
}

// NewCredential creates a new credential of the specified type
func NewCredential(credType byte) *Credential {
	return &Credential{
		Type:       credType,
		Signatures: make([][]byte, 0),
	}
}

// AddSignature adds a signature to the credential
func (c *Credential) AddSignature(sig []byte) {
	c.Signatures = append(c.Signatures, sig)
}

// IsPostQuantum returns true if this is a post-quantum secure credential type
func (c *Credential) IsPostQuantum() bool {
	return c.Type == CredentialTypeMLDSA44 ||
		c.Type == CredentialTypeMLDSA65 ||
		c.Type == CredentialTypeMLDSA87
}

// ValidateSignatureSizes validates that all signatures match the expected size
func (c *Credential) ValidateSignatureSizes() error {
	expectedSize := 0
	switch c.Type {
	case CredentialTypeMLDSA44:
		expectedSize = MLDSA44SignatureSize
	case CredentialTypeMLDSA65:
		expectedSize = MLDSA65SignatureSize
	case CredentialTypeMLDSA87:
		expectedSize = MLDSA87SignatureSize
	default:
		// Non-ML-DSA types don't have strict size requirements here
		return nil
	}

	for i, sig := range c.Signatures {
		if len(sig) != expectedSize {
			return &CredentialValidationError{
				Index:    i,
				Expected: expectedSize,
				Got:      len(sig),
				Reason:   "signature size mismatch",
			}
		}
	}
	return nil
}

// CredentialValidationError provides details on validation failure
type CredentialValidationError struct {
	Index    int
	Expected int
	Got      int
	Reason   string
}

func (e *CredentialValidationError) Error() string {
	return e.Reason
}

// Serialize serializes the credential to bytes
// Format: [type][num_sigs][sig1_len][sig1]...[sigN_len][sigN]
func (c *Credential) Serialize() []byte {
	// Calculate total size
	size := 1 + 2 // type + num_sigs (2 bytes)
	for _, sig := range c.Signatures {
		size += 2 + len(sig) // sig_len (2 bytes) + sig
	}

	buf := make([]byte, 0, size)
	buf = append(buf, c.Type)
	buf = append(buf, byte(len(c.Signatures)>>8), byte(len(c.Signatures)))

	for _, sig := range c.Signatures {
		buf = append(buf, byte(len(sig)>>8), byte(len(sig)))
		buf = append(buf, sig...)
	}

	return buf
}

// DeserializeCredential deserializes a credential from bytes
func DeserializeCredential(data []byte) (*Credential, error) {
	if len(data) < 3 {
		return nil, ErrCredentialTooShort
	}

	c := &Credential{
		Type: data[0],
	}

	numSigs := int(data[1])<<8 | int(data[2])
	c.Signatures = make([][]byte, 0, numSigs)

	offset := 3
	for i := 0; i < numSigs; i++ {
		if offset+2 > len(data) {
			return nil, ErrCredentialTooShort
		}
		sigLen := int(data[offset])<<8 | int(data[offset+1])
		offset += 2

		if offset+sigLen > len(data) {
			return nil, ErrCredentialTooShort
		}
		sig := make([]byte, sigLen)
		copy(sig, data[offset:offset+sigLen])
		c.Signatures = append(c.Signatures, sig)
		offset += sigLen
	}

	return c, nil
}

// =============================================================================
// OUTPUT OWNERS: ML-DSA compatible output ownership
// =============================================================================

// OutputOwners defines who can spend an output
type OutputOwners struct {
	// Locktime is the Unix timestamp after which this output can be spent
	Locktime uint64 `json:"locktime"`

	// Threshold is the number of signatures required to spend
	Threshold uint32 `json:"threshold"`

	// AddressType indicates the address/key type for owners
	AddressType byte `json:"address_type"`

	// Addresses are the owners (public key hashes)
	// For ML-DSA, these are SHA-256 hashes of ML-DSA public keys
	Addresses [][]byte `json:"addresses"`
}

// NewMLDSAOutputOwners creates output owners for ML-DSA credentials
func NewMLDSAOutputOwners(threshold uint32, publicKeys [][]byte, mldType byte) *OutputOwners {
	addresses := make([][]byte, len(publicKeys))
	for i, pk := range publicKeys {
		hash := sha256.Sum256(pk)
		addresses[i] = hash[:]
	}

	return &OutputOwners{
		Threshold:   threshold,
		AddressType: mldType,
		Addresses:   addresses,
	}
}

// IsPostQuantum returns true if this output uses post-quantum address types
func (o *OutputOwners) IsPostQuantum() bool {
	return o.AddressType == CredentialTypeMLDSA44 ||
		o.AddressType == CredentialTypeMLDSA65 ||
		o.AddressType == CredentialTypeMLDSA87
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// CredentialTypeName returns the human-readable name for a credential type
func CredentialTypeName(credType byte) string {
	switch credType {
	case CredentialTypeSecp256k1:
		return "secp256k1"
	case CredentialTypeEd25519:
		return "ed25519"
	case CredentialTypeBLS:
		return "bls12-381"
	case CredentialTypeMLDSA44:
		return "ML-DSA-44"
	case CredentialTypeMLDSA65:
		return "ML-DSA-65"
	case CredentialTypeMLDSA87:
		return "ML-DSA-87"
	default:
		return "unknown"
	}
}

// RecommendedMLDSAType returns the recommended ML-DSA type for new outputs
// ML-DSA-65 provides Category 3 (192-bit) security as a good balance
func RecommendedMLDSAType() byte {
	return CredentialTypeMLDSA65
}

// SignatureSizeForType returns the expected signature size for an ML-DSA type
func SignatureSizeForType(mldType byte) int {
	switch mldType {
	case CredentialTypeMLDSA44:
		return MLDSA44SignatureSize
	case CredentialTypeMLDSA65:
		return MLDSA65SignatureSize
	case CredentialTypeMLDSA87:
		return MLDSA87SignatureSize
	default:
		return 0
	}
}

// PublicKeySizeForType returns the expected public key size for an ML-DSA type
func PublicKeySizeForType(mldType byte) int {
	switch mldType {
	case CredentialTypeMLDSA44:
		return MLDSA44PublicKeySize
	case CredentialTypeMLDSA65:
		return MLDSA65PublicKeySize
	case CredentialTypeMLDSA87:
		return MLDSA87PublicKeySize
	default:
		return 0
	}
}
