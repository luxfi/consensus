// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// SendConfig configures message sending.
type SendConfig struct {
	NodeIDs       []interface{}
	Validators    int
	NonValidators int
	Peers         int
}

// AppError represents an application-level error that can be sent to peers.
type AppError struct {
	Code    int32
	Message string
}

// Error implements error interface.
func (e *AppError) Error() string {
	return fmt.Sprintf("app error (code=%d): %s", e.Code, e.Message)
}

// Is implements errors.Is by comparing error codes.
func (e *AppError) Is(target error) bool {
	if t, ok := target.(*AppError); ok {
		return e.Code == t.Code
	}
	return false
}

// AppSender sends application-level messages to other nodes.
type AppSender interface {
	// SendAppRequest sends an application-level request to the given nodes.
	SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, appRequestBytes []byte) error
	// SendAppResponse sends an application-level response to the given node.
	SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, appResponseBytes []byte) error
	// SendAppGossip sends an application-level gossip message.
	SendAppGossip(ctx context.Context, config SendConfig, appGossipBytes []byte) error
	// SendAppError sends an application error to the given node.
	SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
}
