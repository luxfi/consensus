// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"github.com/luxfi/ids"
)

// DecisionResult represents the outcome of a consensus decision
type DecisionResult int

const (
	// DecideUnknown indicates no decision yet
	DecideUnknown DecisionResult = iota
	// DecideAccept indicates the item was accepted
	DecideAccept
	// DecideReject indicates the item was rejected
	DecideReject
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
