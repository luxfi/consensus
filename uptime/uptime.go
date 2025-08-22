package uptime

import (
    "time"
    "github.com/luxfi/ids"
)

// Manager manages uptime tracking
type Manager interface {
    // IsConnected checks if a node is connected
    IsConnected(nodeID ids.NodeID) bool
    
    // Connected marks a node as connected
    Connected(nodeID ids.NodeID)
    
    // Disconnected marks a node as disconnected
    Disconnected(nodeID ids.NodeID)
}

// TestManager is a test implementation
type TestManager struct {
    connected map[ids.NodeID]bool
}

// NewTestManager creates a new test manager
func NewTestManager() *TestManager {
    return &TestManager{
        connected: make(map[ids.NodeID]bool),
    }
}

// IsConnected checks if a node is connected
func (m *TestManager) IsConnected(nodeID ids.NodeID) bool {
    return m.connected[nodeID]
}

// Connected marks a node as connected
func (m *TestManager) Connected(nodeID ids.NodeID) {
    m.connected[nodeID] = true
}

// Disconnected marks a node as disconnected
func (m *TestManager) Disconnected(nodeID ids.NodeID) {
    m.connected[nodeID] = false
}

// Calculator calculates uptime
type Calculator interface {
    CalculateUptime(nodeID ids.NodeID, subnetID ids.ID) (time.Duration, time.Time, error)
    CalculateUptimePercent(nodeID ids.NodeID, subnetID ids.ID) (float64, error)
}
