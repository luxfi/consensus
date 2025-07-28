// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ringtail

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/luxfi/ids"
)

const (
	// RTKeySize is the size of a Ringtail private key in bytes.
	RTKeySize = 256 // Placeholder - actual size depends on lattice params

	// RTPubKeySize is the size of a Ringtail public key in bytes.
	RTPubKeySize = 1024 // Placeholder - actual size depends on lattice params

	// RTKeyFilename is the default filename for Ringtail keys.
	RTKeyFilename = "rt.key"
)

var (
	// ErrInvalidKeySize is returned when a key has invalid size.
	ErrInvalidKeySize = errors.New("invalid key size")

	// ErrKeyNotFound is returned when a key file is not found.
	ErrKeyNotFound = errors.New("key not found")
)

// KeyPair represents a Ringtail key pair.
type KeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
	NodeID     ids.NodeID
}

// GenerateKeyPair generates a new Ringtail key pair.
func GenerateKeyPair() (*KeyPair, error) {
	// Use actual lattice-based key generation
	scheme := NewScheme()
	
	// Generate lattice key pair
	privKey, pubKey, err := scheme.KeyGen()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ringtail keys: %w", err)
	}

	// Serialize keys
	privKeyBytes, err := privKey.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	pubKeyBytes, err := pubKey.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Derive node ID from public key
	// Use SHA256 to generate a deterministic node ID from the public key
	hash := sha256.Sum256(pubKeyBytes)
	var nodeID ids.NodeID
	copy(nodeID[:], hash[:20]) // NodeID is 20 bytes

	return &KeyPair{
		PrivateKey: privKeyBytes,
		PublicKey:  pubKeyBytes,
		NodeID:     nodeID,
	}, nil
}

// SaveKeyPair saves a key pair to disk.
func SaveKeyPair(kp *KeyPair, dir string) error {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Save private key
	privPath := filepath.Join(dir, RTKeyFilename)
	if err := os.WriteFile(privPath, kp.PrivateKey, 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	// Save public key
	pubPath := filepath.Join(dir, RTKeyFilename+".pub")
	if err := os.WriteFile(pubPath, kp.PublicKey, 0644); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}

	return nil
}

// LoadKeyPair loads a key pair from disk.
func LoadKeyPair(dir string) (*KeyPair, error) {
	// Load private key
	privPath := filepath.Join(dir, RTKeyFilename)
	privKey, err := os.ReadFile(privPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	if len(privKey) != RTKeySize {
		return nil, ErrInvalidKeySize
	}

	// Load public key
	pubPath := filepath.Join(dir, RTKeyFilename+".pub")
	pubKey, err := os.ReadFile(pubPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	if len(pubKey) != RTPubKeySize {
		return nil, ErrInvalidKeySize
	}

	// Derive node ID
	hash := sha256.Sum256(pubKey)
	var nodeID ids.NodeID
	copy(nodeID[:], hash[:20]) // NodeID is 20 bytes

	return &KeyPair{
		PrivateKey: privKey,
		PublicKey:  pubKey,
		NodeID:     nodeID,
	}, nil
}

// GetOrCreateKeyPair loads existing keys or generates new ones.
func GetOrCreateKeyPair(dir string) (*KeyPair, error) {
	// Try to load existing
	kp, err := LoadKeyPair(dir)
	if err == nil {
		return kp, nil
	}

	if err != ErrKeyNotFound {
		return nil, err
	}

	// Generate new
	kp, err = GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	// Save
	if err := SaveKeyPair(kp, dir); err != nil {
		return nil, err
	}

	return kp, nil
}

// KeyManager manages multiple Ringtail keys.
type KeyManager struct {
	keys map[ids.NodeID]*KeyPair
}

// NewKeyManager creates a new key manager.
func NewKeyManager() *KeyManager {
	return &KeyManager{
		keys: make(map[ids.NodeID]*KeyPair),
	}
}

// AddKey adds a key pair to the manager.
func (km *KeyManager) AddKey(kp *KeyPair) {
	km.keys[kp.NodeID] = kp
}

// GetKey retrieves a key pair by node ID.
func (km *KeyManager) GetKey(nodeID ids.NodeID) (*KeyPair, bool) {
	kp, ok := km.keys[nodeID]
	return kp, ok
}

// GetPublicKey retrieves just the public key.
func (km *KeyManager) GetPublicKey(nodeID ids.NodeID) ([]byte, bool) {
	kp, ok := km.keys[nodeID]
	if !ok {
		return nil, false
	}
	return kp.PublicKey, true
}

// ListNodeIDs returns all node IDs with keys.
func (km *KeyManager) ListNodeIDs() []ids.NodeID {
	nodeIDs := make([]ids.NodeID, 0, len(km.keys))
	for nodeID := range km.keys {
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs
}

// ExportPublicKey exports a public key in hex format.
func ExportPublicKey(pubKey []byte) string {
	return hex.EncodeToString(pubKey)
}

// ImportPublicKey imports a public key from hex format.
func ImportPublicKey(hexKey string) ([]byte, error) {
	pubKey, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}

	if len(pubKey) != RTPubKeySize {
		return nil, ErrInvalidKeySize
	}

	return pubKey, nil
}

// SignatureShare represents a Ringtail signature share.
type SignatureShare struct {
	ValidatorID ids.NodeID
	ShareIndex  uint32
	ShareData   []byte
}

// Sign creates a signature share using a private key.
func Sign(privKey []byte, message []byte) (*SignatureShare, error) {
	// Implement actual lattice-based signing
	scheme := NewScheme()
	
	// Unmarshal private key
	sk := scheme.NewPrivateKey()
	if err := sk.UnmarshalBinary(privKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal private key: %w", err)
	}

	// Create signature
	sig, err := scheme.Sign(sk, message)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Serialize signature
	sigBytes, err := sig.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signature: %w", err)
	}

	return &SignatureShare{
		ShareIndex: 0, // For threshold, this would be the share index
		ShareData:  sigBytes,
	}, nil
}

// Verify verifies a signature share using a public key.
func Verify(pubKey []byte, message []byte, share *SignatureShare) error {
	// Implement actual lattice-based verification
	scheme := NewScheme()
	
	// Unmarshal public key
	pk := scheme.NewPublicKey()
	if err := pk.UnmarshalBinary(pubKey); err != nil {
		return fmt.Errorf("failed to unmarshal public key: %w", err)
	}

	// Unmarshal signature
	sig := scheme.NewSignature()
	if err := sig.UnmarshalBinary(share.ShareData); err != nil {
		return fmt.Errorf("failed to unmarshal signature: %w", err)
	}

	// Verify signature
	if err := scheme.Verify(pk, message, sig); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

// AggregateShares combines shares into a threshold signature.
func AggregateShares(shares []*SignatureShare, threshold int) ([]byte, error) {
	if len(shares) < threshold {
		return nil, fmt.Errorf("insufficient shares: have %d, need %d", len(shares), threshold)
	}

	// Use threshold signature aggregation
	scheme := NewThresholdScheme(threshold, len(shares))
	
	// Convert shares to mock format
	rtShares := make([]*MockSignatureShare, len(shares))
	for i, share := range shares {
		rtShare := &MockSignatureShare{}
		if err := rtShare.UnmarshalBinary(share.ShareData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal share %d: %w", i, err)
		}
		rtShares[i] = rtShare
	}

	// Aggregate shares
	aggSig, err := scheme.AggregateShares(rtShares[:threshold])
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate shares: %w", err)
	}

	// Serialize aggregate signature
	aggBytes, err := aggSig.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal aggregate: %w", err)
	}

	return aggBytes, nil
}

// VerifyAggregate verifies an aggregated threshold signature.
func VerifyAggregate(pubKeys [][]byte, message []byte, aggregate []byte, threshold int) error {
	if len(pubKeys) < threshold {
		return fmt.Errorf("insufficient public keys: have %d, need %d", len(pubKeys), threshold)
	}

	// Use threshold verification
	scheme := NewThresholdScheme(threshold, len(pubKeys))
	
	// Unmarshal aggregate signature
	aggSig := scheme.NewAggregateSignature()
	if err := aggSig.UnmarshalBinary(aggregate); err != nil {
		return fmt.Errorf("failed to unmarshal aggregate: %w", err)
	}

	// Convert public keys to ringtail format
	rtPubKeys := make([]*PublicKey, len(pubKeys))
	for i, pkBytes := range pubKeys {
		pk := scheme.NewPublicKey()
		if err := pk.UnmarshalBinary(pkBytes); err != nil {
			return fmt.Errorf("failed to unmarshal public key %d: %w", i, err)
		}
		rtPubKeys[i] = pk
	}

	// Verify aggregate signature
	if err := scheme.VerifyAggregate(rtPubKeys[:threshold], message, aggSig); err != nil {
		return fmt.Errorf("aggregate verification failed: %w", err)
	}

	return nil
}