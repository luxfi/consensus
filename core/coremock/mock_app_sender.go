// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package coremock provides mock implementations for testing
package coremock

import (
	"context"
	"sync"

	"github.com/luxfi/consensus/utils/set"
	"github.com/luxfi/ids"
)

// MockAppSender is a mock implementation of AppSender
type MockAppSender struct {
	mu sync.RWMutex
	
	SendAppRequestF        func(context.Context, set.Set[ids.NodeID], uint32, []byte) error
	SendAppResponseF       func(context.Context, ids.NodeID, uint32, []byte) error
	SendAppErrorF          func(context.Context, ids.NodeID, uint32, int32, string) error
	SendAppGossipF         func(context.Context, set.Set[ids.NodeID], []byte) error
	SendAppGossipSpecificF func(context.Context, set.Set[ids.NodeID], []byte) error
}

// NewMockAppSender creates a new mock AppSender
func NewMockAppSender(ctrl interface{}) *MockAppSender {
	return &MockAppSender{}
}

// SendAppRequest implements AppSender
func (m *MockAppSender) SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, appRequestBytes []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendAppRequestF != nil {
		return m.SendAppRequestF(ctx, nodeIDs, requestID, appRequestBytes)
	}
	return nil
}

// SendAppResponse implements AppSender
func (m *MockAppSender) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendAppResponseF != nil {
		return m.SendAppResponseF(ctx, nodeID, requestID, appResponseBytes)
	}
	return nil
}

// SendAppError implements AppSender
func (m *MockAppSender) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendAppErrorF != nil {
		return m.SendAppErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	return nil
}

// SendAppGossip implements AppSender
func (m *MockAppSender) SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendAppGossipF != nil {
		return m.SendAppGossipF(ctx, nodeIDs, appGossipBytes)
	}
	return nil
}

// SendAppGossipSpecific implements AppSender
func (m *MockAppSender) SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], appGossipBytes []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.SendAppGossipSpecificF != nil {
		return m.SendAppGossipSpecificF(ctx, nodeIDs, appGossipBytes)
	}
	return nil
}

// EXPECT returns mock expectation handler
func (m *MockAppSender) EXPECT() *MockAppSenderExpects {
	return &MockAppSenderExpects{mock: m}
}

// MockAppSenderExpects handles expectations
type MockAppSenderExpects struct {
	mock *MockAppSender
}

// SendAppRequest sets expectation for SendAppRequest
func (e *MockAppSenderExpects) SendAppRequest() *MockAppSenderCall {
	return &MockAppSenderCall{mock: e.mock}
}

// MockAppSenderCall represents a mock call
type MockAppSenderCall struct {
	mock *MockAppSender
}

// Times sets the number of times the call is expected
func (c *MockAppSenderCall) Times(n int) *MockAppSenderCall {
	return c
}

// Return sets the return value
func (c *MockAppSenderCall) Return(err error) *MockAppSenderCall {
	return c
}