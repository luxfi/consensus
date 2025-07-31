// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package interfaces

import (
	"github.com/luxfi/ids"
)

// Status represents the current status of a decidable element
type Status int

const (
	Unknown Status = iota
	Accepted
	Rejected
)

func (s Status) String() string {
	switch s {
	case Accepted:
		return "Accepted"
	case Rejected:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// Decision represents a consensus decision
type Decision interface {
	// ID returns the unique identifier for this decision
	ID() ids.ID
	
	// Bytes returns the binary representation
	Bytes() []byte
	
	// Verify verifies the decision
	Verify() error
}