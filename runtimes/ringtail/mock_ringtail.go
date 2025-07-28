// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// +build !production

package ringtail

import (
	"crypto/rand"
	"errors"
	"fmt"
)

// MockScheme is a mock implementation of the Ringtail scheme for testing.
// In production, this would use the actual lattice-based cryptography.
type MockScheme struct{}

// NewScheme creates a new mock Ringtail scheme.
func NewScheme() *MockScheme {
	return &MockScheme{}
}

// KeyGen generates a mock key pair.
func (s *MockScheme) KeyGen() (*PrivateKey, *PublicKey, error) {
	sk := &PrivateKey{data: make([]byte, 256)}
	pk := &PublicKey{data: make([]byte, 1024)}
	
	// Generate random bytes for mock keys
	if _, err := rand.Read(sk.data); err != nil {
		return nil, nil, err
	}
	if _, err := rand.Read(pk.data); err != nil {
		return nil, nil, err
	}
	
	return sk, pk, nil
}

// Sign creates a mock signature.
func (s *MockScheme) Sign(sk *PrivateKey, message []byte) (*Signature, error) {
	sig := &Signature{data: make([]byte, 1800)}
	if _, err := rand.Read(sig.data); err != nil {
		return nil, err
	}
	return sig, nil
}

// Verify verifies a mock signature.
func (s *MockScheme) Verify(pk *PublicKey, message []byte, sig *Signature) error {
	if len(sig.data) < 100 {
		return errors.New("invalid signature")
	}
	return nil
}

// Precompute creates mock precomputed data.
func (s *MockScheme) Precompute(sk *PrivateKey) (*Precomputed, error) {
	pre := &Precomputed{data: make([]byte, 2048)}
	if _, err := rand.Read(pre.data); err != nil {
		return nil, err
	}
	return pre, nil
}

// BindPrecomputed binds precomputed data to a message.
func (s *MockScheme) BindPrecomputed(pre *Precomputed, message []byte) (*Signature, error) {
	sig := &Signature{data: make([]byte, 1800)}
	copy(sig.data, pre.data[:100])
	copy(sig.data[100:], message[:32])
	return sig, nil
}

// NewPrivateKey creates a new private key.
func (s *MockScheme) NewPrivateKey() *PrivateKey {
	return &PrivateKey{}
}

// NewPublicKey creates a new public key.
func (s *MockScheme) NewPublicKey() *PublicKey {
	return &PublicKey{}
}

// NewSignature creates a new signature.
func (s *MockScheme) NewSignature() *Signature {
	return &Signature{}
}

// NewPrecomputed creates new precomputed data.
func (s *MockScheme) NewPrecomputed() *Precomputed {
	return &Precomputed{}
}

// Key and signature types

type PrivateKey struct {
	data []byte
}

func (sk *PrivateKey) MarshalBinary() ([]byte, error) {
	return sk.data, nil
}

func (sk *PrivateKey) UnmarshalBinary(data []byte) error {
	sk.data = make([]byte, len(data))
	copy(sk.data, data)
	return nil
}

type PublicKey struct {
	data []byte
}

func (pk *PublicKey) MarshalBinary() ([]byte, error) {
	return pk.data, nil
}

func (pk *PublicKey) UnmarshalBinary(data []byte) error {
	pk.data = make([]byte, len(data))
	copy(pk.data, data)
	return nil
}

type Signature struct {
	data []byte
}

func (s *Signature) MarshalBinary() ([]byte, error) {
	return s.data, nil
}

func (s *Signature) UnmarshalBinary(data []byte) error {
	s.data = make([]byte, len(data))
	copy(s.data, data)
	return nil
}

type Precomputed struct {
	data []byte
}

func (p *Precomputed) MarshalBinary() ([]byte, error) {
	return p.data, nil
}

func (p *Precomputed) UnmarshalBinary(data []byte) error {
	p.data = make([]byte, len(data))
	copy(p.data, data)
	return nil
}

// Threshold scheme

type MockThresholdScheme struct {
	threshold int
	total     int
}

// NewThresholdScheme creates a new mock threshold scheme.
func NewThresholdScheme(threshold, total int) *MockThresholdScheme {
	return &MockThresholdScheme{
		threshold: threshold,
		total:     total,
	}
}

// NewSignatureShare creates a new signature share.
func (s *MockThresholdScheme) NewSignatureShare() *MockSignatureShare {
	return &MockSignatureShare{}
}

// NewAggregateSignature creates a new aggregate signature.
func (s *MockThresholdScheme) NewAggregateSignature() *AggregateSignature {
	return &AggregateSignature{}
}

// NewPublicKey creates a new public key.
func (s *MockThresholdScheme) NewPublicKey() *PublicKey {
	return &PublicKey{}
}

// AggregateShares aggregates signature shares.
func (s *MockThresholdScheme) AggregateShares(shares []*MockSignatureShare) (*AggregateSignature, error) {
	if len(shares) < s.threshold {
		return nil, fmt.Errorf("insufficient shares: %d < %d", len(shares), s.threshold)
	}
	
	agg := &AggregateSignature{data: make([]byte, 3072)}
	if _, err := rand.Read(agg.data); err != nil {
		return nil, err
	}
	return agg, nil
}

// VerifyAggregate verifies an aggregate signature.
func (s *MockThresholdScheme) VerifyAggregate(pks []*PublicKey, message []byte, agg *AggregateSignature) error {
	if len(pks) < s.threshold {
		return fmt.Errorf("insufficient public keys: %d < %d", len(pks), s.threshold)
	}
	return nil
}

// MockSignatureShare is a mock implementation for threshold signatures.
type MockSignatureShare struct {
	data []byte
}

func (s *MockSignatureShare) UnmarshalBinary(data []byte) error {
	s.data = make([]byte, len(data))
	copy(s.data, data)
	return nil
}

func (s *MockSignatureShare) MarshalBinary() ([]byte, error) {
	return s.data, nil
}

type AggregateSignature struct {
	data []byte
}

func (a *AggregateSignature) MarshalBinary() ([]byte, error) {
	return a.data, nil
}

func (a *AggregateSignature) UnmarshalBinary(data []byte) error {
	a.data = make([]byte, len(data))
	copy(a.data, data)
	return nil
}