// Copyright (C) 2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

// Package appsender provides the AppSender interface for cross-chain communication.
package appsender

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// AppSender sends application-level messages to other nodes.
type AppSender interface {
	// SendAppRequest sends an application-level request to the given nodes.
	SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, appRequestBytes []byte) error
	// SendAppResponse sends an application-level response to the given node.
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error
	// SendAppGossip sends an application-level gossip message to the given nodes.
	SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error
	// SendAppError sends an application error to the given node.
	SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
}
