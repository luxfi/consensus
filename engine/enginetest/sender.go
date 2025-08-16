// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package enginetest

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/engine/core"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/set"
)

// Sender is a test utility for mocking network sends
type Sender struct {
	T *testing.T

	CantSendAppGossip, CantSendAppGossipSpecific,
	CantSendAppRequest, CantSendAppResponse, CantSendAppError,
	CantSendCrossChainAppRequest, CantSendCrossChainAppResponse, CantSendCrossChainAppError bool

	SendAppGossipF             func(context.Context, core.SendConfig, []byte) error
	SendAppGossipSpecificF     func(context.Context, set.Set[ids.NodeID], []byte) error
	SendAppRequestF            func(context.Context, ids.NodeID, uint32, []byte) error
	SendAppResponseF           func(context.Context, ids.NodeID, uint32, []byte) error
	SendAppErrorF              func(context.Context, ids.NodeID, uint32, int32, string) error
	SendCrossChainAppRequestF  func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainAppResponseF func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainAppErrorF    func(context.Context, ids.ID, uint32, int32, string) error
}

func (s *Sender) Default(cant bool) {
	s.CantSendAppGossip = cant
	s.CantSendAppGossipSpecific = cant
	s.CantSendAppRequest = cant
	s.CantSendAppResponse = cant
	s.CantSendAppError = cant
	s.CantSendCrossChainAppRequest = cant
	s.CantSendCrossChainAppResponse = cant
	s.CantSendCrossChainAppError = cant
}

// SendAppGossip implements core.AppSender
func (s *Sender) SendAppGossip(ctx context.Context, appGossipBytes []byte) error {
	if s.SendAppGossipF != nil {
		// Convert to SendConfig for the test function
		return s.SendAppGossipF(ctx, core.SendConfig{}, appGossipBytes)
	}
	if s.CantSendAppGossip && s.T != nil {
		s.T.Fatal("unexpected SendAppGossip")
	}
	return nil
}

// SendAppGossipWithConfig is a test helper that accepts SendConfig
func (s *Sender) SendAppGossipWithConfig(ctx context.Context, sendConfig core.SendConfig, appGossipBytes []byte) error {
	if s.SendAppGossipF != nil {
		return s.SendAppGossipF(ctx, sendConfig, appGossipBytes)
	}
	if s.CantSendAppGossip && s.T != nil {
		s.T.Fatal("unexpected SendAppGossip")
	}
	return nil
}

func (s *Sender) SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error {
	if s.SendAppGossipSpecificF != nil {
		return s.SendAppGossipSpecificF(ctx, nodeIDs, appGossipBytes)
	}
	if s.CantSendAppGossipSpecific && s.T != nil {
		s.T.Fatal("unexpected SendAppGossipSpecific")
	}
	return nil
}

func (s *Sender) SendAppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, appRequestBytes []byte) error {
	if s.SendAppRequestF != nil {
		return s.SendAppRequestF(ctx, nodeID, requestID, appRequestBytes)
	}
	if s.CantSendAppRequest && s.T != nil {
		s.T.Fatal("unexpected SendAppRequest")
	}
	return nil
}

func (s *Sender) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error {
	if s.SendAppResponseF != nil {
		return s.SendAppResponseF(ctx, nodeID, requestID, appResponseBytes)
	}
	if s.CantSendAppResponse && s.T != nil {
		s.T.Fatal("unexpected SendAppResponse")
	}
	return nil
}

func (s *Sender) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendAppErrorF != nil {
		return s.SendAppErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	if s.CantSendAppError && s.T != nil {
		s.T.Fatal("unexpected SendAppError")
	}
	return nil
}

func (s *Sender) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, appRequestBytes []byte) error {
	if s.SendCrossChainAppRequestF != nil {
		return s.SendCrossChainAppRequestF(ctx, chainID, requestID, appRequestBytes)
	}
	if s.CantSendCrossChainAppRequest && s.T != nil {
		s.T.Fatal("unexpected SendCrossChainAppRequest")
	}
	return nil
}

func (s *Sender) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, appResponseBytes []byte) error {
	if s.SendCrossChainAppResponseF != nil {
		return s.SendCrossChainAppResponseF(ctx, chainID, requestID, appResponseBytes)
	}
	if s.CantSendCrossChainAppResponse && s.T != nil {
		s.T.Fatal("unexpected SendCrossChainAppResponse")
	}
	return nil
}

func (s *Sender) SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendCrossChainAppErrorF != nil {
		return s.SendCrossChainAppErrorF(ctx, chainID, requestID, errorCode, errorMessage)
	}
	if s.CantSendCrossChainAppError && s.T != nil {
		s.T.Fatal("unexpected SendCrossChainAppError")
	}
	return nil
}
