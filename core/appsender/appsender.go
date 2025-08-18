package appsender

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/set"
)

// AppSender sends application messages
type AppSender interface {
	// Send an application-level request.
	SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, appRequestBytes []byte) error
	// Send an application-level response to a request.
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error
	// SendAppError sends an application-level error to an AppRequest
	SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
	// Gossip an application-level message.
	SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error
	// SendAppGossipSpecific sends a gossip message to a list of nodeIDs
	SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error
}
