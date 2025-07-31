// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package interfaces

import (
	"context"

	"github.com/luxfi/ids"
)

// Decidable represents element that can be decided.
//
// Decidable objects are typically thought of as either transactions, blocks, or
// vertices.
type Decidable interface {
	// ID returns a unique ID for this element.
	//
	// Typically, this is implemented by using a cryptographic hash of a
	// binary representation of this element. An element should return the same
	// IDs upon repeated calls.
	ID() ids.ID

	// Accept this element.
	//
	// This element will be accepted by every correct node in the network.
	Accept(context.Context) error

	// Reject this element.
	//
	// This element will not be accepted by any correct node in the network.
	Reject(context.Context) error
}

// TestDecidable is a decidable that can be used for testing
type TestDecidable struct {
	IDV         ids.ID
	AcceptV     error
	RejectV     error
	StatusV     Status
	StatusErrV  error
}

func (d *TestDecidable) ID() ids.ID {
	return d.IDV
}

func (d *TestDecidable) Accept(context.Context) error {
	return d.AcceptV
}

func (d *TestDecidable) Reject(context.Context) error {
	return d.RejectV
}

func (d *TestDecidable) Status() (Status, error) {
	return d.StatusV, d.StatusErrV
}
