// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package ringtail provides post-quantum cryptographic primitives for the Lux consensus.
// This is a stub implementation that forwards to the actual implementation in runtimes/ringtail.
package ringtail

import (
	rt "github.com/luxfi/crypto/ringtail"
)

// Type aliases for cleaner API
type (
	Precomp   = rt.Precomp
	Share     = rt.Share
	Cert      = rt.Cert
	SecretKey = []byte
	PublicKey = []byte
	Signature = []byte // Alias for Share or Cert depending on context
)

// SecurityLevel represents the post-quantum security level
type SecurityLevel int

const (
	SecurityLow    SecurityLevel = 0
	SecurityMedium SecurityLevel = 1 
	SecurityHigh   SecurityLevel = 2
)

// Engine represents the post-quantum consensus engine
type Engine interface {
	// Initialize the engine with security parameters
	Initialize(level SecurityLevel) error
	
	// Sign a message with the secret key
	Sign(msg []byte, sk SecretKey) (Signature, error)
	
	// Verify a signature
	Verify(msg []byte, sig Signature, pk PublicKey) bool
	
	// Generate a new key pair
	GenerateKeyPair() (SecretKey, PublicKey, error)
}

// Re-export error variables
// TODO: Uncomment when exported from crypto/ringtail
// var (
// 	ErrInvalidCertificate  = rt.ErrInvalidCertificate
// 	ErrMissingCertificate  = rt.ErrMissingCertificate
// 	ErrCertificateMismatch = rt.ErrCertificateMismatch
// )

// KeyGen generates a key pair from seed
func KeyGen(seed []byte) ([]byte, []byte, error) {
	return rt.KeyGen(seed)
}

// Precompute generates a precomputed share
func Precompute(sk []byte) (Precomp, error) {
	return rt.Precompute(sk)
}

// QuickSign signs using a precomputed share
func QuickSign(precomp Precomp, msg []byte) (Share, error) {
	return rt.QuickSign(precomp, msg)
}

// VerifyShare verifies a share signature
func VerifyShare(pk []byte, msg []byte, share []byte) bool {
	return rt.VerifyShare(pk, msg, share)
}

// Aggregate aggregates shares into a certificate
func Aggregate(shares []Share) (Cert, error) {
	return rt.Aggregate(shares)
}

// Verify verifies a certificate
func Verify(pk []byte, msg []byte, cert []byte) bool {
	return rt.Verify(pk, msg, cert)
}

// NewCertificate creates a new empty certificate
// func NewCertificate(round, height uint64, blockHash [32]byte) *Certificate {
// 	return rt.NewCertificate(round, height, blockHash)
// }

// NewCertificateManager creates a new certificate manager
// func NewCertificateManager(nodeID ids.NodeID, blsKey, rtKey interface{}, validators ValidatorSet) *CertificateManager {
// 	return rt.NewCertificateManager(nodeID, blsKey, rtKey, validators)
// }

// VerifyAggregate verifies an aggregate signature
// func VerifyAggregate(pubKeys [][]byte, msg []byte, aggregateData []byte, threshold int) error {
// 	return rt.VerifyAggregate(pubKeys, msg, aggregateData, threshold)
// }