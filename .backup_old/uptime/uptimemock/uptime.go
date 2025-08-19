// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package uptimemock

import (
	"time"
	
	"github.com/luxfi/ids"
)

// Tracker is a mock uptime tracker
type Tracker struct {
	StartTimeFunc    func(ids.NodeID) (time.Time, error)
	ConnectedFunc    func(ids.NodeID) error
	DisconnectedFunc func(ids.NodeID) error
	WeightedFunc     func(ids.NodeID) (time.Duration, error)
}

// StartTime returns the start time for a node
func (t *Tracker) StartTime(nodeID ids.NodeID) (time.Time, error) {
	if t.StartTimeFunc != nil {
		return t.StartTimeFunc(nodeID)
	}
	return time.Now(), nil
}

// Connected marks a node as connected
func (t *Tracker) Connected(nodeID ids.NodeID) error {
	if t.ConnectedFunc != nil {
		return t.ConnectedFunc(nodeID)
	}
	return nil
}

// Disconnected marks a node as disconnected
func (t *Tracker) Disconnected(nodeID ids.NodeID) error {
	if t.DisconnectedFunc != nil {
		return t.DisconnectedFunc(nodeID)
	}
	return nil
}

// Weighted returns the weighted uptime for a node
func (t *Tracker) Weighted(nodeID ids.NodeID) (time.Duration, error) {
	if t.WeightedFunc != nil {
		return t.WeightedFunc(nodeID)
	}
	return time.Hour, nil
}