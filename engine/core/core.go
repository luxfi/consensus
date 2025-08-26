package core

import (
	"context"
	"time"

	"github.com/luxfi/consensus/engine/core/common"
	"github.com/luxfi/ids"
)

// Fx represents a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}

// AppError represents an application error
type AppError = common.AppError

// AppHandler handles application messages
type AppHandler interface {
	AppGossip(context.Context, ids.NodeID, []byte)
	AppRequest(context.Context, ids.NodeID, time.Time, []byte) ([]byte, *AppError)
	CrossChainAppRequest(context.Context, ids.ID, time.Time, []byte) ([]byte, error)
}
