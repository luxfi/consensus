// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/core/interfaces"
)

// Polyadic represents a polyadic consensus object
type Polyadic interface {
	interfaces.Decidable
	
	// ID returns the unique identifier
	ID() ids.ID
	
	// Conflicts returns the IDs of conflicting polyadics
	Conflicts() ([]ids.ID, error)
	
	// Dependencies returns the IDs of dependent polyadics
	Dependencies() ([]ids.ID, error)
	
	// Bytes returns the byte representation
	Bytes() []byte
}