// Package syncer provides state synchronization for blockchain engines
package syncer

import (
	"context"

	"github.com/luxfi/consensus/core/types"
)

// Config holds syncer configuration
type Config struct {
	GetHandler     interface{}
	Context        interface{}
	StartupTracker interface{}
	Sender         interface{}
	Beacons        []types.NodeID
	VM             interface{}
}

// Syncer provides state synchronization
type Syncer struct {
	config         *Config
	onDoneCallback func(ctx context.Context, lastReqID uint32) error
}

// NewConfig creates a new syncer configuration
func NewConfig(
	getHandler interface{},
	ctx interface{},
	startupTracker interface{},
	sender interface{},
	beacons []types.NodeID,
	vm interface{},
) (*Config, error) {
	return &Config{
		GetHandler:     getHandler,
		Context:        ctx,
		StartupTracker: startupTracker,
		Sender:         sender,
		Beacons:        beacons,
		VM:             vm,
	}, nil
}

// New creates a new state syncer
func New(config *Config, onDone func(ctx context.Context, lastReqID uint32) error) *Syncer {
	return &Syncer{
		config:         config,
		onDoneCallback: onDone,
	}
}

// Start starts the syncer
func (s *Syncer) Start(ctx context.Context, startReqID uint32) error {
	// State sync is optional - call the done callback immediately
	if s.onDoneCallback != nil {
		return s.onDoneCallback(ctx, startReqID)
	}
	return nil
}

// HealthCheck returns syncer health
func (s *Syncer) HealthCheck(ctx context.Context) (interface{}, error) {
	return map[string]string{"status": "healthy"}, nil
}
