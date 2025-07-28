// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"bytes"
	"errors"
	"time"
	
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/ringtail"
)

var (
	ErrBLS      = errors.New("BLS signature verification failed")
	ErrRingtail = errors.New("Ringtail certificate verification failed")
)

// CertBundle contains both BLS and Ringtail certificates
type CertBundle struct {
	BLSAgg [96]byte      // BLS aggregate signature
	RTCert []byte        // Ringtail certificate (nil until aggregated)
}

// Header represents a block header
type Header struct {
	ChainID      ids.ID
	Height       uint64
	ParentID     ids.ID
	Timestamp    time.Time
	ProposerID   ids.NodeID
	StateRoot    ids.ID
	TransRoot    ids.ID
	ReceiptsRoot ids.ID
}

// Block represents a complete block
type Block struct {
	Header
	Certs        CertBundle
	Transactions [][]byte
	blockID      ids.ID // cached
}

// ID returns the block ID
func (b *Block) ID() ids.ID {
	if b.blockID == ids.Empty {
		// Compute block ID from header
		b.blockID = ids.ID(b.Header.Hash())
	}
	return b.blockID
}

// Hash returns the block hash for signing
func (h *Header) Hash() [32]byte {
	buf := bytes.Buffer{}
	buf.Write(h.ChainID[:])
	buf.Write(h.ParentID[:])
	// Add other header fields...
	return ids.Checksum256(buf.Bytes())
}

// VerifyBlock verifies both BLS and Ringtail certificates
func VerifyBlock(b *Block, q *quasar) error {
	msg := b.Header.Hash()
	
	// TODO: Implement proper BLS verification once validator keys are available
	// For now, check that BLS signature is present
	if b.Certs.BLSAgg == [96]byte{} {
		return ErrBLS
	}
	
	// Verify Ringtail certificate
	if !ringtail.VerifyCert(q.pkGroup, msg[:], b.Certs.RTCert) {
		return ErrRingtail
	}
	
	return nil
}