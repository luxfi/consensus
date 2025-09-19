// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package uptime

import (
	"time"

	"github.com/luxfi/ids"
)

// Manager tracks validator uptime
type Manager interface {
	StartTracking(nodeIDs []ids.NodeID) error
	StopTracking(nodeIDs []ids.NodeID) error
	Connect(nodeID ids.NodeID) error
	Disconnect(nodeID ids.NodeID) error
	IsConnected(nodeID ids.NodeID) bool
	CalculateUptime(nodeID ids.NodeID) (time.Duration, time.Time, error)
	CalculateUptimePercent(nodeID ids.NodeID) (float64, error)
}

// NewManager creates a new uptime manager
func NewManager(state State, clock interface{}) Manager {
	return &NoOpManager{}
}

// NoOpManager is a no-op implementation of Manager
type NoOpManager struct{}

func (m *NoOpManager) StartTracking(nodeIDs []ids.NodeID) error {
	return nil
}

func (m *NoOpManager) StopTracking(nodeIDs []ids.NodeID) error {
	return nil
}

func (m *NoOpManager) Connect(nodeID ids.NodeID) error {
	return nil
}

func (m *NoOpManager) Disconnect(nodeID ids.NodeID) error {
	return nil
}

func (m *NoOpManager) IsConnected(nodeID ids.NodeID) bool {
	return false
}

func (m *NoOpManager) CalculateUptime(nodeID ids.NodeID) (time.Duration, time.Time, error) {
	return 0, time.Time{}, nil
}

func (m *NoOpManager) CalculateUptimePercent(nodeID ids.NodeID) (float64, error) {
	return 0, nil
}

// TestState is a test implementation of State
type TestState struct{}

func NewTestState() State {
	return &TestState{}
}

func (s *TestState) GetStartTime(nodeID ids.NodeID, netID ids.ID) (time.Time, error) {
	return time.Now(), nil
}

func (s *TestState) GetUptime(nodeID ids.NodeID, netID ids.ID) (time.Duration, time.Duration, error) {
	return 0, 0, nil
}

func (s *TestState) SetUptime(nodeID ids.NodeID, netID ids.ID, uptime time.Duration, lastUpdated time.Time) error {
	return nil
}