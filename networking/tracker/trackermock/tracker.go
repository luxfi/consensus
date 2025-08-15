// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package trackermock

import (
	"sync"

	"github.com/luxfi/ids"
)

// Tracker is a mock implementation of a resource tracker
type Tracker struct {
	mu        sync.RWMutex
	resources map[ids.NodeID]int
}

// New creates a new mock tracker
func New() *Tracker {
	return &Tracker{
		resources: make(map[ids.NodeID]int),
	}
}

// Track adds tracking for a node
func (t *Tracker) Track(nodeID ids.NodeID, resource int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resources[nodeID] = resource
}

// Remove removes tracking for a node
func (t *Tracker) Remove(nodeID ids.NodeID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.resources, nodeID)
}

// GetResource returns the resource for a node
func (t *Tracker) GetResource(nodeID ids.NodeID) (int, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	resource, ok := t.resources[nodeID]
	return resource, ok
}

// GetAllResources returns all resources
func (t *Tracker) GetAllResources() map[ids.NodeID]int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[ids.NodeID]int, len(t.resources))
	for k, v := range t.resources {
		result[k] = v
	}
	return result
}

// Clear removes all tracked resources
func (t *Tracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resources = make(map[ids.NodeID]int)
}

// Size returns the number of tracked nodes
func (t *Tracker) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.resources)
}

// CPUTracker tracks CPU usage
type CPUTracker struct {
	*Tracker
}

// NewCPUTracker creates a new CPU tracker
func NewCPUTracker() *CPUTracker {
	return &CPUTracker{
		Tracker: New(),
	}
}

// StartTracking starts tracking a node's CPU usage
func (c *CPUTracker) StartTracking(nodeID ids.NodeID) {
	c.Track(nodeID, 0)
}

// StopTracking stops tracking a node's CPU usage
func (c *CPUTracker) StopTracking(nodeID ids.NodeID) {
	c.Remove(nodeID)
}

// GetCPUUsage returns CPU usage for a node
func (c *CPUTracker) GetCPUUsage(nodeID ids.NodeID) (float64, bool) {
	usage, ok := c.GetResource(nodeID)
	return float64(usage) / 100.0, ok
}

// SetCPUUsage sets CPU usage for a node
func (c *CPUTracker) SetCPUUsage(nodeID ids.NodeID, usage float64) {
	c.Track(nodeID, int(usage*100))
}

// BandwidthTracker tracks bandwidth usage
type BandwidthTracker struct {
	*Tracker
}

// NewBandwidthTracker creates a new bandwidth tracker
func NewBandwidthTracker() *BandwidthTracker {
	return &BandwidthTracker{
		Tracker: New(),
	}
}

// StartTracking starts tracking a node's bandwidth
func (b *BandwidthTracker) StartTracking(nodeID ids.NodeID) {
	b.Track(nodeID, 0)
}

// StopTracking stops tracking a node's bandwidth
func (b *BandwidthTracker) StopTracking(nodeID ids.NodeID) {
	b.Remove(nodeID)
}

// GetBandwidth returns bandwidth for a node
func (b *BandwidthTracker) GetBandwidth(nodeID ids.NodeID) (int, bool) {
	return b.GetResource(nodeID)
}

// SetBandwidth sets bandwidth for a node
func (b *BandwidthTracker) SetBandwidth(nodeID ids.NodeID, bandwidth int) {
	b.Track(nodeID, bandwidth)
}
