// Package consensus provides the Lux consensus implementation.
package consensus

import (
	"context"

	"github.com/luxfi/ids"
)

// Acceptor interface for consensus acceptors
type Acceptor interface {
	// Accept processes an accepted item
	Accept(ctx context.Context, containerID ids.ID, container []byte) error
}

// BasicAcceptor is a simple implementation of the Acceptor interface
type BasicAcceptor struct {
	accepted map[ids.ID][]byte
}

// NewBasicAcceptor creates a new basic acceptor
func NewBasicAcceptor() *BasicAcceptor {
	return &BasicAcceptor{
		accepted: make(map[ids.ID][]byte),
	}
}

// Accept marks an item as accepted
func (a *BasicAcceptor) Accept(ctx context.Context, containerID ids.ID, container []byte) error {
	a.accepted[containerID] = container
	return nil
}