package consensustest

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

// Block status constants
const (
	Unknown  uint8 = 0
	Processing uint8 = 1
	Accepted uint8 = 2
	Rejected uint8 = 3
)

// Decidable is an embedded struct for test blocks
type Decidable struct {
	IDV     ids.ID
	StatusV choices.Status
}

// ID returns the ID of this decidable
func (d *Decidable) ID() ids.ID {
	return d.IDV
}

// Accept marks this as accepted
func (d *Decidable) Accept(context.Context) error {
	d.StatusV = choices.Accepted
	return nil
}

// Reject marks this as rejected
func (d *Decidable) Reject(context.Context) error {
	d.StatusV = choices.Rejected
	return nil
}

// Status returns the current status
func (d *Decidable) Status() choices.Status {
	return d.StatusV
}

// TestConfig provides test configuration
type TestConfig struct {
	NodeID    ids.NodeID
	NetworkID uint32
	ChainID   ids.ID
}

// NewTestConfig creates a test config
func NewTestConfig(t *testing.T) *TestConfig {
	return &TestConfig{
		NodeID:    ids.GenerateTestNodeID(),
		NetworkID: 1,
		ChainID:   ids.GenerateTestID(),
	}
}

// MockEngine is a mock consensus engine
type MockEngine struct {
	StartCalled  bool
	StopCalled   bool
	Bootstrapped bool
}

// Start starts the engine
func (m *MockEngine) Start(ctx context.Context, requestID uint32) error {
	m.StartCalled = true
	m.Bootstrapped = true
	return nil
}

// Stop stops the engine
func (m *MockEngine) Stop(ctx context.Context) error {
	m.StopCalled = true
	return nil
}

// IsBootstrapped returns bootstrap status
func (m *MockEngine) IsBootstrapped() bool {
	return m.Bootstrapped
}

// HealthCheck performs health check
func (m *MockEngine) HealthCheck(ctx context.Context) (interface{}, error) {
	return map[string]bool{"healthy": true}, nil
}
