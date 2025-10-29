// Package trackermock provides mock implementations for connection tracking
package trackermock

import (
	"time"

	"github.com/luxfi/ids"
)

// MockTracker provides a mock implementation for connection tracking
type MockTracker struct {
	connections map[ids.NodeID]*ConnectionInfo
}

// ConnectionInfo holds connection information
type ConnectionInfo struct {
	NodeID        ids.NodeID
	Connected     bool
	ConnectedAt   time.Time
	DisconnectedAt time.Time
	BytesSent     uint64
	BytesReceived uint64
}

// NewMockTracker creates a new mock tracker
func NewMockTracker() *MockTracker {
	return &MockTracker{
		connections: make(map[ids.NodeID]*ConnectionInfo),
	}
}

// Connected marks a node as connected
func (m *MockTracker) Connected(nodeID ids.NodeID) {
	m.connections[nodeID] = &ConnectionInfo{
		NodeID:      nodeID,
		Connected:   true,
		ConnectedAt: time.Now(),
	}
}

// Disconnected marks a node as disconnected
func (m *MockTracker) Disconnected(nodeID ids.NodeID) {
	if info, exists := m.connections[nodeID]; exists {
		info.Connected = false
		info.DisconnectedAt = time.Now()
	}
}

// IsConnected checks if a node is connected
func (m *MockTracker) IsConnected(nodeID ids.NodeID) bool {
	if info, exists := m.connections[nodeID]; exists {
		return info.Connected
	}
	return false
}

// GetConnectionInfo returns connection info for a node
func (m *MockTracker) GetConnectionInfo(nodeID ids.NodeID) *ConnectionInfo {
	return m.connections[nodeID]
}

// Reset clears all tracking data
func (m *MockTracker) Reset() {
	m.connections = make(map[ids.NodeID]*ConnectionInfo)
}
