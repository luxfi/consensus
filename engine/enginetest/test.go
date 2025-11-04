// Package enginetest provides test utilities for consensus engines
package enginetest

import (
	"context"

	"github.com/luxfi/consensus/core"
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// TestEngine provides a test implementation for consensus engines
type TestEngine struct {
	started bool
	height  uint64
}

// NewTestEngine creates a new test engine
func NewTestEngine() *TestEngine {
	return &TestEngine{
		started: false,
		height:  0,
	}
}

// Start starts the test engine
func (t *TestEngine) Start(ctx context.Context) error {
	t.started = true
	return nil
}

// Stop stops the test engine
func (t *TestEngine) Stop(ctx context.Context) error {
	t.started = false
	return nil
}

// IsStarted returns whether the engine is started
func (t *TestEngine) IsStarted() bool {
	return t.started
}

// Height returns the current height
func (t *TestEngine) Height() uint64 {
	return t.height
}

// SetHeight sets the engine height
func (t *TestEngine) SetHeight(height uint64) {
	t.height = height
}

// Sender is a test implementation of core.Sender for testing
type Sender struct {
	SendAppRequestF func(context.Context, set.Set[ids.NodeID], uint32, []byte) error
	SendAppResponseF func(context.Context, ids.NodeID, uint32, []byte) error
	SendAppErrorF func(context.Context, ids.NodeID, uint32, int32, string) error
	SendAppGossipF func(context.Context, core.SendConfig, []byte) error
	SendAppGossipSpecificF func(context.Context, core.SendConfig, []byte) error
	SendCrossChainAppRequestF func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainAppResponseF func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainAppErrorF func(context.Context, ids.ID, uint32, int32, string) error
}

func (s *Sender) SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error {
	if s.SendAppRequestF != nil {
		return s.SendAppRequestF(ctx, nodeIDs, requestID, request)
	}
	return nil
}

func (s *Sender) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
	if s.SendAppResponseF != nil {
		return s.SendAppResponseF(ctx, nodeID, requestID, response)
	}
	return nil
}

func (s *Sender) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendAppErrorF != nil {
		return s.SendAppErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	return nil
}

func (s *Sender) SendAppGossip(ctx context.Context, config core.SendConfig, gossip []byte) error {
	if s.SendAppGossipF != nil {
		return s.SendAppGossipF(ctx, config, gossip)
	}
	return nil
}

func (s *Sender) SendAppGossipSpecific(ctx context.Context, config core.SendConfig, gossip []byte) error {
	if s.SendAppGossipSpecificF != nil {
		return s.SendAppGossipSpecificF(ctx, config, gossip)
	}
	return nil
}

func (s *Sender) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, request []byte) error {
	if s.SendCrossChainAppRequestF != nil {
		return s.SendCrossChainAppRequestF(ctx, chainID, requestID, request)
	}
	return nil
}

func (s *Sender) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, response []byte) error {
	if s.SendCrossChainAppResponseF != nil {
		return s.SendCrossChainAppResponseF(ctx, chainID, requestID, response)
	}
	return nil
}

func (s *Sender) SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendCrossChainAppErrorF != nil {
		return s.SendCrossChainAppErrorF(ctx, chainID, requestID, errorCode, errorMessage)
	}
	return nil
}
