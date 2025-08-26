// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"time"

	"github.com/luxfi/consensus/core/appsender"
)

// Export AppSender type for convenience
type AppSender = appsender.AppSender

// AppHandler handles application messages
type AppHandler interface {
	AppRequest(ctx context.Context, nodeID interface{}, requestID uint32, deadline time.Time, msg []byte) error
	AppResponse(ctx context.Context, nodeID interface{}, requestID uint32, msg []byte) error
	AppGossip(ctx context.Context, nodeID interface{}, msg []byte) error
}

// SendConfig configures message sending
type SendConfig struct {
	NodeIDs       []interface{}
	Validators    int
	NonValidators int
	Peers         int
}
