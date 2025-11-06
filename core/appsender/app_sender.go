// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package appsender defines the AppSender interface for application-level messaging
package appsender

import (
	"context"

	"github.com/luxfi/consensus/utils/set"
	"github.com/luxfi/ids"
)

// AppSender sends application-level messages
type AppSender interface {
	// SendAppRequest sends an application-level request to the given nodes.
	// The meaning of request, and what should be sent in response is application-defined.
	SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, appRequestBytes []byte) error

	// SendAppResponse sends an application-level response to a request.
	// This response must be in response to an AppRequest that was previously
	// received by this node.
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error

	// SendAppError sends an application-level error to an AppRequest
	SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error

	// SendAppGossip sends an application-level gossip message.
	SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error

	// SendAppGossipSpecific sends an application-level gossip message to a specific set of nodes
	SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error
}
