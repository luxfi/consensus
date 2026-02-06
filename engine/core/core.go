// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package core provides common engine types and interfaces used across
// the consensus system.
package core

import (
	"context"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// AppError represents an application-level error that can be returned
// when handling requests.
type AppError struct {
	Code    int32
	Message string
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Is implements errors.Is for AppError comparison.
func (e *AppError) Is(target error) bool {
	if target == nil || e == nil {
		return e == target
	}
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// AppSender is the interface for sending application messages.
type AppSender interface {
	// SendAppRequest sends an application request to the specified nodes.
	SendAppRequest(
		ctx context.Context,
		nodeIDs set.Set[ids.NodeID],
		requestID uint32,
		appRequestBytes []byte,
	) error

	// SendAppResponse sends a response to a previously received request.
	SendAppResponse(
		ctx context.Context,
		nodeID ids.NodeID,
		requestID uint32,
		appResponseBytes []byte,
	) error

	// SendAppError sends an error response to a previously received request.
	SendAppError(
		ctx context.Context,
		nodeID ids.NodeID,
		requestID uint32,
		errorCode int32,
		errorMessage string,
	) error

	// SendAppGossip sends a gossip message to nodes.
	SendAppGossip(
		ctx context.Context,
		nodeIDs set.Set[ids.NodeID],
		appGossipBytes []byte,
	) error

	// SendAppGossipSpecific sends a gossip message to specific nodes.
	SendAppGossipSpecific(
		ctx context.Context,
		nodeIDs set.Set[ids.NodeID],
		appGossipBytes []byte,
	) error

	// SendRequest sends a request to a specific node (p2p.Sender compatible).
	SendRequest(
		ctx context.Context,
		nodeID ids.NodeID,
		handlerID uint64,
		requestBytes []byte,
	) error

	// SendResponse sends a response (p2p.Sender compatible).
	SendResponse(
		ctx context.Context,
		nodeID ids.NodeID,
		handlerID uint64,
		requestID uint32,
		responseBytes []byte,
	) error

	// SendError sends an error (p2p.Sender compatible).
	SendError(
		ctx context.Context,
		nodeID ids.NodeID,
		handlerID uint64,
		requestID uint32,
		errorCode int32,
		errorMessage string,
	) error
}

// AppHandler is the interface for handling application messages.
type AppHandler interface {
	// AppRequest handles an incoming request.
	AppRequest(
		ctx context.Context,
		nodeID ids.NodeID,
		deadline time.Time,
		requestBytes []byte,
	) ([]byte, *AppError)

	// AppGossip handles an incoming gossip message.
	AppGossip(
		ctx context.Context,
		nodeID ids.NodeID,
		gossipBytes []byte,
	) error

	// AppRequestFailed handles a request failure notification.
	AppRequestFailed(
		ctx context.Context,
		nodeID ids.NodeID,
		requestID uint32,
		appErr *AppError,
	) error

	// CrossChainAppRequest handles a cross-chain request.
	CrossChainAppRequest(
		ctx context.Context,
		chainID ids.ID,
		deadline time.Time,
		requestBytes []byte,
	) ([]byte, error)

	// CrossChainAppRequestFailed handles a cross-chain request failure.
	CrossChainAppRequestFailed(
		ctx context.Context,
		chainID ids.ID,
		requestID uint32,
		appErr *AppError,
	) error
}

// Fx represents a feature extension that can be registered with a VM.
type Fx struct {
	ID   ids.ID
	Name string
}

// NoOpAppSender is an AppSender that does nothing.
type NoOpAppSender struct{}

func (NoOpAppSender) SendAppRequest(context.Context, set.Set[ids.NodeID], uint32, []byte) error {
	return nil
}

func (NoOpAppSender) SendAppResponse(context.Context, ids.NodeID, uint32, []byte) error {
	return nil
}

func (NoOpAppSender) SendAppError(context.Context, ids.NodeID, uint32, int32, string) error {
	return nil
}

func (NoOpAppSender) SendAppGossip(context.Context, set.Set[ids.NodeID], []byte) error {
	return nil
}

func (NoOpAppSender) SendAppGossipSpecific(context.Context, set.Set[ids.NodeID], []byte) error {
	return nil
}

func (NoOpAppSender) SendRequest(context.Context, ids.NodeID, uint64, []byte) error {
	return nil
}

func (NoOpAppSender) SendResponse(context.Context, ids.NodeID, uint64, uint32, []byte) error {
	return nil
}

func (NoOpAppSender) SendError(context.Context, ids.NodeID, uint64, uint32, int32, string) error {
	return nil
}

// NoOpAppHandler is an AppHandler that does nothing.
type NoOpAppHandler struct{}

func (NoOpAppHandler) AppRequest(context.Context, ids.NodeID, time.Time, []byte) ([]byte, *AppError) {
	return nil, nil
}

func (NoOpAppHandler) AppGossip(context.Context, ids.NodeID, []byte) error {
	return nil
}

func (NoOpAppHandler) AppRequestFailed(context.Context, ids.NodeID, uint32, *AppError) error {
	return nil
}

func (NoOpAppHandler) CrossChainAppRequest(context.Context, ids.ID, time.Time, []byte) ([]byte, error) {
	return nil, nil
}

func (NoOpAppHandler) CrossChainAppRequestFailed(context.Context, ids.ID, uint32, *AppError) error {
	return nil
}
