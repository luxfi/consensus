// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
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

// Simple decisions for tests and internal use
type simpleDecision byte

func (s simpleDecision) ID() ids.ID    { return ids.Empty }
func (s simpleDecision) Bytes() []byte { return []byte{byte(s)} }
func (s simpleDecision) Verify() error { return nil }

var (
	DecideReject Decision = simpleDecision(0)
	DecideAccept Decision = simpleDecision(1)
)
