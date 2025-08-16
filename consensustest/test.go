// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

// Decidable is a test decidable
type Decidable struct {
	IDV       ids.ID
	StatusV   choices.Status
	ParentV   ids.ID
	HeightV   uint64
	VerifyV   error
	AcceptV   error
	RejectV   error
}

// ID returns the decidable's ID
func (d *Decidable) ID() ids.ID {
	return d.IDV
}

// Status returns the decidable's status
func (d *Decidable) Status() choices.Status {
	return d.StatusV
}

// Parent returns the decidable's parent ID
func (d *Decidable) Parent() ids.ID {
	return d.ParentV
}

// Height returns the decidable's height
func (d *Decidable) Height() uint64 {
	return d.HeightV
}

// Verify verifies the decidable
func (d *Decidable) Verify(context.Context) error {
	return d.VerifyV
}

// Accept accepts the decidable
func (d *Decidable) Accept(context.Context) error {
	if d.AcceptV != nil {
		return d.AcceptV
	}
	d.StatusV = choices.Accepted
	return nil
}

// Reject rejects the decidable
func (d *Decidable) Reject(context.Context) error {
	if d.RejectV != nil {
		return d.RejectV
	}
	d.StatusV = choices.Rejected
	return nil
}

// Bytes returns the decidable's bytes
func (d *Decidable) Bytes() []byte {
	return d.IDV[:]
}
