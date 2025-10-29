// Package coretest provides test utilities for consensus engine core
package coretest

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// MockEngine provides a mock implementation for testing
type MockEngine struct {
	t              *testing.T
	startCalled    bool
	stopCalled     bool
	healthy        bool
	bootstrapped   bool
	lastAcceptedID ids.ID
}

// NewMockEngine creates a new mock engine for testing
func NewMockEngine(t *testing.T) *MockEngine {
	return &MockEngine{
		t:            t,
		healthy:      true,
		bootstrapped: false,
	}
}

// Start starts the mock engine
func (m *MockEngine) Start(ctx context.Context, startReqID uint32) error {
	m.startCalled = true
	return nil
}

// Stop stops the mock engine
func (m *MockEngine) Stop(ctx context.Context) error {
	m.stopCalled = true
	return nil
}

// IsBootstrapped returns whether the engine is bootstrapped
func (m *MockEngine) IsBootstrapped() bool {
	return m.bootstrapped
}

// SetBootstrapped sets the bootstrapped state
func (m *MockEngine) SetBootstrapped(bootstrapped bool) {
	m.bootstrapped = bootstrapped
}

// HealthCheck returns the health status
func (m *MockEngine) HealthCheck(ctx context.Context) (interface{}, error) {
	if !m.healthy {
		return nil, errUnhealthy
	}
	return map[string]string{"status": "healthy"}, nil
}

// SetHealthy sets the health status
func (m *MockEngine) SetHealthy(healthy bool) {
	m.healthy = healthy
}

// LastAccepted returns the last accepted ID
func (m *MockEngine) LastAccepted() ids.ID {
	return m.lastAcceptedID
}

// SetLastAccepted sets the last accepted ID
func (m *MockEngine) SetLastAccepted(id ids.ID) {
	m.lastAcceptedID = id
}

// AssertStartCalled asserts that Start was called
func (m *MockEngine) AssertStartCalled() {
	if !m.startCalled {
		m.t.Error("Start was not called")
	}
}

// AssertStopCalled asserts that Stop was called
func (m *MockEngine) AssertStopCalled() {
	if !m.stopCalled {
		m.t.Error("Stop was not called")
	}
}

var errUnhealthy = &healthError{msg: "engine is unhealthy"}

type healthError struct {
	msg string
}

func (e *healthError) Error() string {
	return e.msg
}
