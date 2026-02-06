// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package appsender defines the interface for sending application messages
// between nodes in the consensus network.
package appsender

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// AppSender defines the interface for sending application-level messages
// in the consensus network.
type AppSender interface {
	// SendAppRequest sends an application request to the specified nodes.
	// The response will be received via the AppResponse callback.
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
