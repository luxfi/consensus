// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"context"

	"github.com/luxfi/ids"
)

// Acceptor is implemented when a struct is monitoring if a message is accepted
type Acceptor interface {
	Accept(ctx context.Context, containerID ids.ID, container []byte) error
}

// AcceptorGroup is a group of acceptors for a specific chain
type AcceptorGroup interface {
	// RegisterAcceptor causes [acceptor] to be called when a container is
	// accepted on chain [chainID]. If [dieOnError], chain [chainID] will stop
	// if Accept returns a non-nil error.
	RegisterAcceptor(chainID ids.ID, acceptorName string, acceptor Acceptor, dieOnError bool) error

	// DeregisterAcceptor removes an acceptor that was previously registered
	DeregisterAcceptor(chainID ids.ID, acceptorName string) error
}
