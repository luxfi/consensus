// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// TransactionID represents a unique identifier for a transaction
type TransactionID [32]byte

// NewTransactionID creates a TransactionID from a byte slice
func NewTransactionID(b []byte) (TransactionID, error) {
	if len(b) != 32 {
		return TransactionID{}, fmt.Errorf("transaction ID must be exactly 32 bytes, got %d", len(b))
	}
	var id TransactionID
	copy(id[:], b)
	return id, nil
}

// String returns the hex string representation of the TransactionID
func (id TransactionID) String() string {
	return hex.EncodeToString(id[:])
}

// BlockID represents a unique identifier for a block
type BlockID [32]byte

// NewBlockID creates a BlockID from a byte slice
func NewBlockID(b []byte) (BlockID, error) {
	if len(b) != 32 {
		return BlockID{}, fmt.Errorf("block ID must be exactly 32 bytes, got %d", len(b))
	}
	var id BlockID
	copy(id[:], b)
	return id, nil
}

// String returns the hex string representation of the BlockID
func (id BlockID) String() string {
	return hex.EncodeToString(id[:])
}

// Compare returns -1, 0, or 1 if id is less than, equal to, or greater than other
func (id BlockID) Compare(other BlockID) int {
	return bytes.Compare(id[:], other[:])
}