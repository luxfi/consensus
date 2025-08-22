package core

import (
	"context"
	"github.com/luxfi/ids"
)

// TestDecidable is a test implementation of Decidable
type TestDecidable struct {
	TestID     ids.ID
	TestStatus Status
	AcceptFunc func(context.Context) error
	RejectFunc func(context.Context) error
}

// NewTestDecidable creates a new test decidable
func NewTestDecidable(id ids.ID) *TestDecidable {
	return &TestDecidable{
		TestID:     id,
		TestStatus: StatusPending,
		AcceptFunc: func(context.Context) error { return nil },
		RejectFunc: func(context.Context) error { return nil },
	}
}

// ID returns the ID
func (t *TestDecidable) ID() ids.ID {
	return t.TestID
}

// Accept marks as accepted
func (t *TestDecidable) Accept(ctx context.Context) error {
	if t.AcceptFunc != nil {
		if err := t.AcceptFunc(ctx); err != nil {
			return err
		}
	}
	t.TestStatus = StatusAccepted
	return nil
}

// Reject marks as rejected
func (t *TestDecidable) Reject(ctx context.Context) error {
	if t.RejectFunc != nil {
		if err := t.RejectFunc(ctx); err != nil {
			return err
		}
	}
	t.TestStatus = StatusRejected
	return nil
}

// Status returns current status
func (t *TestDecidable) Status() Status {
	return t.TestStatus
}