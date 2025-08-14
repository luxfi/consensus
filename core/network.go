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

// AppSender sends application messages
type AppSender interface {
    SendAppRequest(ctx context.Context, nodeIDs ids.NodeID, requestID uint32, appRequestBytes []byte) error
    SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error
    SendAppGossip(ctx context.Context, appGossipBytes []byte) error
    SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
    SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, appRequestBytes []byte) error
    SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, appResponseBytes []byte) error
}

// SendConfig provides configuration for sending messages
type SendConfig struct {
    SendType    int
    Validators  []ids.NodeID
}