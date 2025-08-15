package appsender

import (
	"github.com/luxfi/ids"
	"time"
)

// AppSender sends application messages
type AppSender interface {
	SendAppRequest(nodeID ids.NodeID, requestID uint32, deadline time.Time, request []byte) error
	SendAppResponse(nodeID ids.NodeID, requestID uint32, response []byte) error
	SendAppGossip(nodeID ids.NodeID, msg []byte) error
}
