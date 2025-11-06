// Package uptimemock provides mock implementations for uptime tracking
package uptimemock

import (
	"time"

	"github.com/luxfi/consensus/core/types"
)

// MockUptimeTracker provides a mock implementation for uptime tracking
type MockUptimeTracker struct {
	uptimes map[types.NodeID]time.Duration
	starts  map[types.NodeID]time.Time
}

// NewMockUptimeTracker creates a new mock uptime tracker
func NewMockUptimeTracker() *MockUptimeTracker {
	return &MockUptimeTracker{
		uptimes: make(map[types.NodeID]time.Duration),
		starts:  make(map[types.NodeID]time.Time),
	}
}

// StartTracking starts tracking uptime for a node
func (m *MockUptimeTracker) StartTracking(nodeID types.NodeID) {
	m.starts[nodeID] = time.Now()
}

// StopTracking stops tracking uptime for a node
func (m *MockUptimeTracker) StopTracking(nodeID types.NodeID) {
	if start, exists := m.starts[nodeID]; exists {
		duration := time.Since(start)
		m.uptimes[nodeID] += duration
		delete(m.starts, nodeID)
	}
}

// GetUptime returns the total uptime for a node
func (m *MockUptimeTracker) GetUptime(nodeID types.NodeID) time.Duration {
	uptime := m.uptimes[nodeID]
	if start, exists := m.starts[nodeID]; exists {
		uptime += time.Since(start)
	}
	return uptime
}

// IsTracking returns whether a node is currently being tracked
func (m *MockUptimeTracker) IsTracking(nodeID types.NodeID) bool {
	_, exists := m.starts[nodeID]
	return exists
}
