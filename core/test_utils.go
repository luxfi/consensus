// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/set"
)

// FakeSender is a test utility for mocking network sends
type FakeSender struct {
	SentAppGossip            chan []byte
	SentAppRequest           chan []byte
	SentCrossChainAppRequest chan []byte
	SentAppResponse          chan []byte
	SentAppError             chan *AppError
}

func (f *FakeSender) SendAppGossip(_ context.Context, appGossipBytes []byte) error {
	if f.SentAppGossip != nil {
		f.SentAppGossip <- appGossipBytes
	}
	return nil
}

func (f *FakeSender) SendAppGossipSpecific(_ context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error {
	if f.SentAppGossip != nil {
		f.SentAppGossip <- appGossipBytes
	}
	return nil
}

func (f *FakeSender) SendAppRequest(_ context.Context, nodeID ids.NodeID, requestID uint32, appRequestBytes []byte) error {
	if f.SentAppRequest != nil {
		f.SentAppRequest <- appRequestBytes
	}
	return nil
}

func (f *FakeSender) SendAppResponse(_ context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error {
	if f.SentAppResponse != nil {
		f.SentAppResponse <- appResponseBytes
	}
	return nil
}

func (f *FakeSender) SendAppError(_ context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if f.SentAppError != nil {
		f.SentAppError <- &AppError{
			Code:    errorCode,
			Message: errorMessage,
		}
	}
	return nil
}

func (f *FakeSender) SendCrossChainAppRequest(_ context.Context, chainID ids.ID, requestID uint32, appRequestBytes []byte) error {
	if f.SentCrossChainAppRequest != nil {
		f.SentCrossChainAppRequest <- appRequestBytes
	}
	return nil
}

func (f *FakeSender) SendCrossChainAppResponse(_ context.Context, chainID ids.ID, requestID uint32, appResponseBytes []byte) error {
	return nil
}

func (f *FakeSender) SendCrossChainAppError(_ context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	return nil
}

// SenderTest is a test interface for testing network sends
type SenderTest struct {
	T *testing.T

	CantSendAppGossip, CantSendAppGossipSpecific,
	CantSendAppRequest, CantSendAppResponse, CantSendAppError,
	CantSendCrossChainAppRequest, CantSendCrossChainAppResponse, CantSendCrossChainAppError bool

	SendAppGossipF             func(context.Context, []byte) error
	SendAppGossipSpecificF     func(context.Context, set.Set[ids.NodeID], []byte) error
	SendAppRequestF            func(context.Context, ids.NodeID, uint32, []byte) error
	SendAppResponseF           func(context.Context, ids.NodeID, uint32, []byte) error
	SendAppErrorF              func(context.Context, ids.NodeID, uint32, int32, string) error
	SendCrossChainAppRequestF  func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainAppResponseF func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainAppErrorF    func(context.Context, ids.ID, uint32, int32, string) error
}

func (s *SenderTest) Default(cant bool) {
	s.CantSendAppGossip = cant
	s.CantSendAppGossipSpecific = cant
	s.CantSendAppRequest = cant
	s.CantSendAppResponse = cant
	s.CantSendAppError = cant
	s.CantSendCrossChainAppRequest = cant
	s.CantSendCrossChainAppResponse = cant
	s.CantSendCrossChainAppError = cant
}

func (s *SenderTest) SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error {
	if s.SendAppGossipF != nil {
		// For backwards compatibility, call the old function signature
		return s.SendAppGossipF(ctx, appGossipBytes)
	}
	if s.CantSendAppGossip && s.T != nil {
		s.T.Fatal("unexpected SendAppGossip")
	}
	return nil
}

func (s *SenderTest) SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error {
	if s.SendAppGossipSpecificF != nil {
		return s.SendAppGossipSpecificF(ctx, nodeIDs, appGossipBytes)
	}
	if s.CantSendAppGossipSpecific && s.T != nil {
		s.T.Fatal("unexpected SendAppGossipSpecific")
	}
	return nil
}

func (s *SenderTest) SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, appRequestBytes []byte) error {
	if s.SendAppRequestF != nil {
		// For backwards compatibility, if there's exactly one nodeID, use it
		for nodeID := range nodeIDs {
			return s.SendAppRequestF(ctx, nodeID, requestID, appRequestBytes)
		}
		return nil
	}
	if s.CantSendAppRequest && s.T != nil {
		s.T.Fatal("unexpected SendAppRequest")
	}
	return nil
}

func (s *SenderTest) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error {
	if s.SendAppResponseF != nil {
		return s.SendAppResponseF(ctx, nodeID, requestID, appResponseBytes)
	}
	if s.CantSendAppResponse && s.T != nil {
		s.T.Fatal("unexpected SendAppResponse")
	}
	return nil
}

func (s *SenderTest) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendAppErrorF != nil {
		return s.SendAppErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	if s.CantSendAppError && s.T != nil {
		s.T.Fatal("unexpected SendAppError")
	}
	return nil
}

func (s *SenderTest) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, appRequestBytes []byte) error {
	if s.SendCrossChainAppRequestF != nil {
		return s.SendCrossChainAppRequestF(ctx, chainID, requestID, appRequestBytes)
	}
	if s.CantSendCrossChainAppRequest && s.T != nil {
		s.T.Fatal("unexpected SendCrossChainAppRequest")
	}
	return nil
}

func (s *SenderTest) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, appResponseBytes []byte) error {
	if s.SendCrossChainAppResponseF != nil {
		return s.SendCrossChainAppResponseF(ctx, chainID, requestID, appResponseBytes)
	}
	if s.CantSendCrossChainAppResponse && s.T != nil {
		s.T.Fatal("unexpected SendCrossChainAppResponse")
	}
	return nil
}

func (s *SenderTest) SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendCrossChainAppErrorF != nil {
		return s.SendCrossChainAppErrorF(ctx, chainID, requestID, errorCode, errorMessage)
	}
	if s.CantSendCrossChainAppError && s.T != nil {
		s.T.Fatal("unexpected SendCrossChainAppError")
	}
	return nil
}
