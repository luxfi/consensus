// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"github.com/luxfi/ids"
)

// Tx represents a transaction in the consensus system
type Tx interface {
	// ID returns the unique identifier for this transaction
	ID() ids.ID

	// Bytes returns the binary representation
	Bytes() []byte

	// Verify verifies the transaction
	Verify() error
}
