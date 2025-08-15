package enginetest

import (
	"context"
	
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/engine/core"
	"github.com/luxfi/node/utils/set"
)

// Sender is a test sender implementation that implements core.AppSender
type Sender struct {
	SendAppRequestF  func(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error
	SendAppResponseF func(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error
	SendAppGossipF   func(ctx context.Context, sendConfig core.SendConfig, gossipBytes []byte) error
	SendAppErrorF    func(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
}

// NewSender creates a new test sender
func NewSender() *Sender {
	return &Sender{}
}

// SendAppRequest sends an app request
func (s *Sender) SendAppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, request []byte) error {
	if s.SendAppRequestF != nil {
		nodeIDs := set.Set[ids.NodeID]{}
		nodeIDs.Add(nodeID)
		return s.SendAppRequestF(ctx, nodeIDs, requestID, request)
	}
	return nil
}

// SendAppResponse sends an app response
func (s *Sender) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
	if s.SendAppResponseF != nil {
		return s.SendAppResponseF(ctx, nodeID, requestID, response)
	}
	return nil
}

// SendAppGossip sends app gossip
func (s *Sender) SendAppGossip(ctx context.Context, appGossipBytes []byte) error {
	if s.SendAppGossipF != nil {
		return s.SendAppGossipF(ctx, core.SendConfig{}, appGossipBytes)
	}
	return nil
}

// SendAppError sends an app error
func (s *Sender) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendAppErrorF != nil {
		return s.SendAppErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	return nil
}

// SendCrossChainAppRequest sends a cross-chain app request
func (s *Sender) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, appRequestBytes []byte) error {
	// Not implemented for testing
	return nil
}

// SendCrossChainAppResponse sends a cross-chain app response
func (s *Sender) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, appResponseBytes []byte) error {
	// Not implemented for testing
	return nil
}

// SendCrossChainAppError sends a cross-chain app error
func (s *Sender) SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	// Not implemented for testing
	return nil
}