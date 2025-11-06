// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package appsender

import (
	"context"

	"github.com/luxfi/consensus/utils/set"
	"github.com/luxfi/ids"
)

// FakeSender is a fake implementation of AppSender for testing
type FakeSender struct {
	SentAppRequest           []byte
	SentAppGossip            []byte
	SentCrossChainAppRequest []byte
}

func (f *FakeSender) SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, appRequestBytes []byte) error {
	f.SentAppRequest = appRequestBytes
	return nil
}

func (f *FakeSender) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error {
	return nil
}

func (f *FakeSender) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	return nil
}

func (f *FakeSender) SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error {
	f.SentAppGossip = appGossipBytes
	return nil
}

func (f *FakeSender) SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error {
	f.SentAppGossip = appGossipBytes
	return nil
}

func (f *FakeSender) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, appRequestBytes []byte) error {
	f.SentCrossChainAppRequest = appRequestBytes
	return nil
}

func (f *FakeSender) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, appResponseBytes []byte) error {
	return nil
}

func (f *FakeSender) SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	return nil
}
