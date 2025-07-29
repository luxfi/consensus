// Copyright (C) 2019-2024, Lux Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"github.com/luxfi/ids"
)

// Decision represents a consensus decision
type Decision interface {
	// ID returns the unique identifier for this decision
	ID() ids.ID
	
	// Bytes returns the binary representation
	Bytes() []byte
	
	// Verify verifies the decision
	Verify() error
}