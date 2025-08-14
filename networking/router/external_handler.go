// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/message"
	"github.com/luxfi/node/version"
)

// ExternalHandler handles messages from the network
// It's a message router that receives consensus messages
type ExternalHandler interface {
	InboundHandler
	RegisterRequest(
		ctx context.Context,
		nodeID ids.NodeID,
		chainID ids.ID,
		requestID uint32,
		op message.Op,
		failedMsg message.InboundMessage,
		engineType int,
	)
	Connected(ids.NodeID, *version.Application) error
	Disconnected(ids.NodeID) error
}