// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warptest

import (
	"github.com/luxfi/consensus/vms/platformvm/warp"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// Signer represents a BLS signer interface
type Signer interface {
	PublicKey() *bls.PublicKey
	Sign(msg []byte) (*bls.Signature, error)
}

// NewSigner creates a test warp signer
func NewSigner(secretKey Signer, chainID ids.ID) warp.Signer {
	return &testSigner{
		secretKey: secretKey,
		chainID:   chainID,
	}
}

type testSigner struct {
	secretKey Signer
	chainID   ids.ID
}

func (s *testSigner) Sign(msg *warp.Message) (*bls.Signature, error) {
	// Convert message to bytes for signing
	unsignedMsg := &warp.UnsignedMessage{
		SourceChainID:      msg.SourceChainID,
		DestinationChainID: msg.DestinationChainID,
		Payload:            msg.Payload,
	}
	msgBytes := unsignedMsg.Bytes()
	
	// Sign with the secret key
	return s.secretKey.Sign(msgBytes)
}