package core

import (
	"context"
	"github.com/luxfi/ids"
)

// Acceptor interface for consensus acceptors
type Acceptor interface {
	// Accept processes an accepted item
	Accept(ctx context.Context, containerID ids.ID, container []byte) error
}