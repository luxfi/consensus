// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"errors"

	"github.com/luxfi/crypto/bls"
)

var (
	ErrNotImplemented = errors.New("warp signing not implemented")
)

// Message represents a warp message
type Message struct {
	SourceChainID      string
	DestinationChainID string
	Payload            []byte
}

// Signature represents a BLS signature on a warp message
type Signature struct {
	Signature []byte
	Signer    *bls.PublicKey
}

// Signer provides warp message signing capabilities
type Signer interface {
	// Sign signs a warp message
	Sign(message *Message) (*Signature, error)

	// Verify verifies a signature on a message
	Verify(message *Message, signature *Signature) error
}

// NoopSigner is a no-op implementation of Signer
type NoopSigner struct{}

// NewNoopSigner returns a new no-op signer
func NewNoopSigner() Signer {
	return &NoopSigner{}
}

// Sign returns an error as it's not implemented
func (n *NoopSigner) Sign(message *Message) (*Signature, error) {
	return nil, ErrNotImplemented
}

// Verify returns an error as it's not implemented
func (n *NoopSigner) Verify(message *Message, signature *Signature) error {
	return ErrNotImplemented
}
