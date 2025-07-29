// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package mocks

import (
	"github.com/luxfi/ids"
)

// ResourceTracker is a mock implementation for testing
type ResourceTracker struct{}

// NewResourceTracker creates a new mock resource tracker
func NewResourceTracker() *ResourceTracker {
	return &ResourceTracker{}
}

// StartProcessing marks the start of processing for a node
func (rt *ResourceTracker) StartProcessing(nodeID ids.NodeID, startTime int64) {}

// StopProcessing marks the end of processing for a node
func (rt *ResourceTracker) StopProcessing(nodeID ids.NodeID, endTime int64) {}

// Add records resource usage
func (rt *ResourceTracker) Add(nodeID ids.NodeID, resource string, amount int64) {}

// Remove removes resource usage
func (rt *ResourceTracker) Remove(nodeID ids.NodeID, resource string, amount int64) {}

// Get retrieves resource usage
func (rt *ResourceTracker) Get(nodeID ids.NodeID, resource string) int64 {
	return 0
}

// GetAll retrieves all resource usage for a node
func (rt *ResourceTracker) GetAll(nodeID ids.NodeID) map[string]int64 {
	return make(map[string]int64)
}