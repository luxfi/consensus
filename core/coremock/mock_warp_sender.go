// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package coremock provides mock implementations for testing
package coremock

import (
	"context"
	"sync"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
	"github.com/luxfi/p2p"
)

// MockWarpSender is a mock implementation of p2p.Sender (warp.Sender)
type MockWarpSender struct {
	mu sync.RWMutex

	SendRequestF  func(context.Context, set.Set[ids.NodeID], uint32, []byte) error
	SendResponseF func(context.Context, ids.NodeID, uint32, []byte) error
	SendErrorF    func(context.Context, ids.NodeID, uint32, int32, string) error
	SendGossipF   func(context.Context, p2p.SendConfig, []byte) error
}

// NewMockWarpSender creates a new mock WarpSender
func NewMockWarpSender(ctrl interface{}) *MockWarpSender {
	return &MockWarpSender{}
}

// SendRequest implements p2p.Sender
func (m *MockWarpSender) SendRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, requestBytes []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendRequestF != nil {
		return m.SendRequestF(ctx, nodeIDs, requestID, requestBytes)
	}
	return nil
}

// SendResponse implements p2p.Sender
func (m *MockWarpSender) SendResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, responseBytes []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendResponseF != nil {
		return m.SendResponseF(ctx, nodeID, requestID, responseBytes)
	}
	return nil
}

// SendError implements p2p.Sender
func (m *MockWarpSender) SendError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendErrorF != nil {
		return m.SendErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	return nil
}

// SendGossip implements p2p.Sender
func (m *MockWarpSender) SendGossip(ctx context.Context, config p2p.SendConfig, gossipBytes []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendGossipF != nil {
		return m.SendGossipF(ctx, config, gossipBytes)
	}
	return nil
}

// EXPECT returns mock expectation handler
func (m *MockWarpSender) EXPECT() *MockWarpSenderExpects {
	return &MockWarpSenderExpects{mock: m}
}

// MockWarpSenderExpects handles expectations
type MockWarpSenderExpects struct {
	mock *MockWarpSender
}

// SendRequest sets expectation for SendRequest
func (e *MockWarpSenderExpects) SendRequest() *MockWarpSenderCall {
	return &MockWarpSenderCall{mock: e.mock}
}

// MockWarpSenderCall represents a mock call
type MockWarpSenderCall struct {
	mock *MockWarpSender
}

// Times sets the number of times the call is expected
func (c *MockWarpSenderCall) Times(n int) *MockWarpSenderCall {
	return c
}

// Return sets the return value
func (c *MockWarpSenderCall) Return(err error) *MockWarpSenderCall {
	return c
}

// Deprecated: Use MockWarpSender instead
type MockAppSender = MockWarpSender

// Deprecated: Use NewMockWarpSender instead
var NewMockAppSender = NewMockWarpSender

// MockSender is an alias for MockWarpSender for backward compatibility
type MockSender = MockWarpSender

// NewMockSender is an alias for NewMockWarpSender for backward compatibility
var NewMockSender = NewMockWarpSender
