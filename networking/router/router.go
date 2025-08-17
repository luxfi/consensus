package router

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// InboundMessage represents an inbound message
type InboundMessage interface {
	Op() interface{}
	OnFinishedHandling()
}

// Router routes messages between chains
type Router interface {
	ExternalHandler
	AddChain(chainID ids.ID, handler interface{})
	RemoveChain(chainID ids.ID)
	AppGossip(ctx context.Context, nodeID ids.NodeID, msg []byte) error
	Initialize(
		nodeID ids.NodeID,
		log interface{},
		timeoutManager interface{},
		closeTimeout time.Duration,
		criticalChains interface{},
		sybilProtectionEnabled bool,
		trackedSubnets interface{},
		onFatal func(int),
		healthConfig HealthConfig,
		metricsRegisterer interface{},
	) error
	HealthCheck(ctx context.Context) (interface{}, error)
}
