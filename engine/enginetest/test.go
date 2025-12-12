// Package enginetest provides test utilities for consensus engines
package enginetest

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
	"github.com/luxfi/warp"
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

// Sender is a test implementation of warp.Sender for testing
type Sender struct {
	SendRequestF            func(context.Context, set.Set[ids.NodeID], uint32, []byte) error
	SendResponseF           func(context.Context, ids.NodeID, uint32, []byte) error
	SendErrorF              func(context.Context, ids.NodeID, uint32, int32, string) error
	SendGossipF             func(context.Context, warp.SendConfig, []byte) error
	SendCrossChainRequestF  func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainResponseF func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainErrorF    func(context.Context, ids.ID, uint32, int32, string) error
}

func (s *Sender) SendRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error {
	if s.SendRequestF != nil {
		return s.SendRequestF(ctx, nodeIDs, requestID, request)
	}
	return nil
}

func (s *Sender) SendResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
	if s.SendResponseF != nil {
		return s.SendResponseF(ctx, nodeID, requestID, response)
	}
	return nil
}

func (s *Sender) SendError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendErrorF != nil {
		return s.SendErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	return nil
}

func (s *Sender) SendGossip(ctx context.Context, config warp.SendConfig, gossip []byte) error {
	if s.SendGossipF != nil {
		return s.SendGossipF(ctx, config, gossip)
	}
	return nil
}

func (s *Sender) SendCrossChainRequest(ctx context.Context, chainID ids.ID, requestID uint32, request []byte) error {
	if s.SendCrossChainRequestF != nil {
		return s.SendCrossChainRequestF(ctx, chainID, requestID, request)
	}
	return nil
}

func (s *Sender) SendCrossChainResponse(ctx context.Context, chainID ids.ID, requestID uint32, response []byte) error {
	if s.SendCrossChainResponseF != nil {
		return s.SendCrossChainResponseF(ctx, chainID, requestID, response)
	}
	return nil
}

func (s *Sender) SendCrossChainError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendCrossChainErrorF != nil {
		return s.SendCrossChainErrorF(ctx, chainID, requestID, errorCode, errorMessage)
	}
	return nil
}
