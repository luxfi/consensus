// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"time"

	"github.com/luxfi/consensus/core/appsender"
	"github.com/luxfi/ids"
)

// Export AppSender type for convenience
type AppSender = appsender.AppSender

// AppHandler handles application messages
type AppHandler interface {
	AppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, deadline time.Time, msg []byte) error
	AppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, msg []byte) error
	AppGossip(ctx context.Context, nodeID ids.NodeID, msg []byte) error
}

// SendConfig configures message sending
type SendConfig struct {
	NodeIDs       []interface{}
	Validators    int
	NonValidators int
	Peers         int
}
