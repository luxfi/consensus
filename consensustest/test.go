package consensustest

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

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
