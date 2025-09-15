package core

import (
	"context"

	consensus_core "github.com/luxfi/consensus/core"
	"github.com/luxfi/consensus/utils/set"
	"github.com/luxfi/ids"
)

// AppSender is a type alias for the core AppSender
type AppSender = consensus_core.AppSender

// AppSenderOriginal sends application-level messages
type AppSenderOriginal interface {
	// SendAppRequest sends an application-level request to the given nodes
	SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error

	// SendAppResponse sends an application-level response to the given node
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error

	// SendAppGossip sends an application-level gossip message
	SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], message []byte) error

	// SendCrossChainAppRequest sends a cross-chain app request
	SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, request []byte) error

	// SendCrossChainAppResponse sends a cross-chain app response
	SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, response []byte) error

	// SendCrossChainAppError sends a cross-chain app error
	SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error

	// SendAppError sends an application-level error response to the given node
	SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
}

// FakeSender is a type alias for the core FakeSender
type FakeSender = consensus_core.FakeSender

// SenderTest is a test implementation of AppSender
type SenderTest struct {
	FakeSender
}

// SendConfig is a type alias for the core SendConfig
type SendConfig = consensus_core.SendConfig
