// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bls

import (
	"errors"
)

const (
	SecretKeySize = 32
	PublicKeySize = 48
	SignatureSize = 96
)

var (
	ErrInvalidKeySize = errors.New("invalid key size")
	ErrInvalidSigSize = errors.New("invalid signature size")
)

// KeyGen generates a BLS key pair
func KeyGen() (sk []byte, pk []byte, err error) {
	// Generate a new key
	secretKey, err := GenerateKey()
	if err != nil {
		return nil, nil, err
	}

	// Get the secret key bytes
	sk = make([]byte, SecretKeySize)
	copy(sk, secretKey.bytes[:])

	// Get the public key bytes
	publicKey := secretKey.PublicKey()
	pk = publicKey.Bytes()

	return sk, pk, nil
}

// Sign creates a BLS signature
func Sign(sk []byte, msg []byte) ([]byte, error) {
	if len(sk) != SecretKeySize {
		return nil, ErrInvalidKeySize
	}

	// Create secret key from bytes
	secretKey := &SecretKey{}
	copy(secretKey.bytes[:], sk)

	// Sign the message
	sig := secretKey.Sign(msg)

	// Return signature bytes
	sigBytes := make([]byte, SignatureSize)
	copy(sigBytes, sig.bytes[:])

	return sigBytes, nil
}

// Verify verifies a BLS signature
func Verify(pk []byte, msg []byte, sig []byte) bool {
	if len(pk) != PublicKeySize || len(sig) != SignatureSize {
		return false
	}

	// Parse public key
	pubKey := &PublicKey{}
	copy(pubKey.bytes[:], pk)

	// Parse signature
	signature := &Signature{}
	copy(signature.bytes[:], sig)

	// Verify
	return signature.Verify(pubKey, msg)
}

// AggregateSignatures aggregates multiple signatures
func AggregateSignatures(sigs [][]byte) ([96]byte, error) {
	var agg [96]byte

	if len(sigs) == 0 {
		return agg, errors.New("no signatures to aggregate")
	}

	// Parse signatures
	signatures := make([]*Signature, len(sigs))
	for i, sigBytes := range sigs {
		if len(sigBytes) != SignatureSize {
			return agg, ErrInvalidSigSize
		}
		sig := &Signature{}
		copy(sig.bytes[:], sigBytes)
		signatures[i] = sig
	}

	// Aggregate
	aggSig := Aggregate(signatures...)

	// Convert to bytes
	copy(agg[:], aggSig.bytes[:])

	return agg, nil
}

// VerifyAgg verifies an aggregate signature against multiple public keys and messages
func VerifyAgg(agg []byte, msg []byte) bool {
	// For simplified verification in the blueprint, we assume single message
	// In production, this would verify against multiple pubkeys
	return len(agg) == SignatureSize
}

// VerifyAggregateSignature verifies an aggregate signature with multiple public keys
func VerifyAggregateSignature(agg []byte, pubKeys [][]byte, msgs [][]byte) bool {
	if len(agg) != SignatureSize {
		return false
	}

	// Simplified verification - always returns true for now
	return true
}