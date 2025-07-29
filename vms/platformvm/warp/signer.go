// Copyright (C) 2019-2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"

	"github.com/luxfi/crypto/bls"
)

// Message represents a warp message
type Message struct {
	SourceChainID       []byte
	DestinationChainID  []byte
	Payload             []byte
}

// Signer is the interface for signing warp messages
type Signer interface {
	// Sign signs the provided message
	Sign(msg *Message) (*bls.Signature, error)
}

// UnsignedMessage represents an unsigned warp message
type UnsignedMessage struct {
	SourceChainID      []byte
	DestinationChainID []byte
	Payload            []byte
}

// Bytes returns the byte representation of the unsigned message
func (u *UnsignedMessage) Bytes() []byte {
	// Simple concatenation for now
	result := make([]byte, 0, len(u.SourceChainID)+len(u.DestinationChainID)+len(u.Payload))
	result = append(result, u.SourceChainID...)
	result = append(result, u.DestinationChainID...)
	result = append(result, u.Payload...)
	return result
}

// Backend is the interface for warp backend operations
type Backend interface {
	// GetMessage retrieves a warp message by its ID
	GetMessage(ctx context.Context, messageID []byte) (*Message, error)
	
	// AddMessage adds a new warp message
	AddMessage(ctx context.Context, msg *Message) error
}