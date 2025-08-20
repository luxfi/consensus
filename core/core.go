package core

import (
    "context"
    "time"
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/utils/set"
)

// AppSender interface for sending application messages
type AppSender interface {
    SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error
    SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error
    SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], message []byte) error
    SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, request []byte) error
    SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, response []byte) error
    SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error
}

// FakeSender is a fake implementation of AppSender for testing
type FakeSender struct{}

func (f *FakeSender) SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error {
    return nil
}

func (f *FakeSender) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
    return nil
}

func (f *FakeSender) SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], message []byte) error {
    return nil
}

func (f *FakeSender) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, request []byte) error {
    return nil
}

func (f *FakeSender) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, response []byte) error {
    return nil
}

func (f *FakeSender) SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
    return nil
}

// SenderTest is a test implementation of AppSender
type SenderTest struct {
    FakeSender
}

// Handler interface for handling messages
type Handler interface {
    AppRequest(ctx context.Context, nodeID ids.NodeID, deadline time.Time, request []byte) ([]byte, error)
    AppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error
    AppGossip(ctx context.Context, nodeID ids.NodeID, message []byte) error
}
