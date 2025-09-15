// Package coremock provides mock implementations for core consensus
package coremock

import "context"

// MockConsensus provides a mock implementation for consensus
type MockConsensus struct {
	decided bool
	result  int
}

// NewMockConsensus creates a new mock consensus
func NewMockConsensus() *MockConsensus {
	return &MockConsensus{
		decided: false,
		result:  0,
	}
}

// Decide makes a consensus decision
func (m *MockConsensus) Decide(ctx context.Context, value int) error {
	m.decided = true
	m.result = value
	return nil
}

// IsDecided returns whether consensus has been reached
func (m *MockConsensus) IsDecided() bool {
	return m.decided
}

// Result returns the consensus result
func (m *MockConsensus) Result() int {
	return m.result
}
