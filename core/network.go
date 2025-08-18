package core

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// AppHandler handles application messages
type AppHandler interface {
	AppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, deadline time.Time, appRequestBytes []byte) error
	AppRequestFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32, appErr *AppError) error
	AppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error
	AppGossip(ctx context.Context, nodeID ids.NodeID, appGossipBytes []byte) error
}

// SendConfig provides configuration for sending messages
type SendConfig struct {
	SendType      int
	Validators    []ids.NodeID
	NodeIDs       interface{} // Can be []ids.NodeID or set.Set[ids.NodeID]
	NonValidators int
	Peers         int
}

// GetNodeIDsAsSlice returns the node IDs as a slice
func (c SendConfig) GetNodeIDsAsSlice() []ids.NodeID {
	switch v := c.NodeIDs.(type) {
	case []ids.NodeID:
		return v
	default:
		return c.Validators
	}
}
