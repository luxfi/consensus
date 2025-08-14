package interfaces

import "github.com/luxfi/ids"

// TestDecidable is a test implementation of Decidable
type TestDecidable struct {
	IDV       ids.ID
	StatusV   Status
	AcceptErr error
	RejectErr error
}

// ID returns the ID of this decidable
func (d *TestDecidable) ID() ids.ID {
	return d.IDV
}

// Status returns the current status
func (d *TestDecidable) Status() Status {
	return d.StatusV
}

// Accept accepts this decidable
func (d *TestDecidable) Accept() error {
	if d.AcceptErr != nil {
		return d.AcceptErr
	}
	d.StatusV = Accepted
	return nil
}

// Reject rejects this decidable
func (d *TestDecidable) Reject() error {
	if d.RejectErr != nil {
		return d.RejectErr
	}
	d.StatusV = Rejected
	return nil
}