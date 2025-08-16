// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowtest

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

// MockBlock is a mock implementation for testing
type MockBlock struct {
	ID_     ids.ID
	Parent_ ids.ID
	Height_ uint64
	Status_ choices.Status
	VerifyF func() error
	AcceptF func() error
	RejectF func() error
}

func (b *MockBlock) ID() ids.ID             { return b.ID_ }
func (b *MockBlock) Parent() ids.ID         { return b.Parent_ }
func (b *MockBlock) Height() uint64         { return b.Height_ }
func (b *MockBlock) Status() choices.Status { return b.Status_ }
func (b *MockBlock) Verify() error {
	if b.VerifyF != nil {
		return b.VerifyF()
	}
	return nil
}

func (b *MockBlock) Accept() error {
	if b.AcceptF != nil {
		return b.AcceptF()
	}
	b.Status_ = choices.Accepted
	return nil
}

func (b *MockBlock) Reject() error {
	if b.RejectF != nil {
		return b.RejectF()
	}
	b.Status_ = choices.Rejected
	return nil
}

// MockConsensus is a mock consensus engine
type MockConsensus struct {
	T *testing.T
}

func (c *MockConsensus) Initialize(ctx context.Context) error {
	return nil
}

func (c *MockConsensus) NumProcessing() int {
	return 0
}

// Engine returns a mock engine
func Engine(t *testing.T) *MockConsensus {
	return &MockConsensus{T: t}
}

// ValidatorSet for testing
type ValidatorSet struct {
	validators map[ids.NodeID]uint64
}

func NewValidatorSet(validators map[ids.NodeID]uint64) *ValidatorSet {
	return &ValidatorSet{validators: validators}
}

func (v *ValidatorSet) Weight(id ids.NodeID) uint64 {
	return v.validators[id]
}

func (v *ValidatorSet) TotalWeight() uint64 {
	var total uint64
	for _, w := range v.validators {
		total += w
	}
	return total
}

// ErrTest is a test error
var ErrTest = errors.New("test error")
