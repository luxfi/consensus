// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"sync"
	
	"github.com/luxfi/ids"
)

// Static implements a simple static threshold
type Static struct {
	mu         sync.RWMutex
	threshold  int
	responses  map[ids.NodeID]bool
	positive   int
	negative   int
}

// NewStatic creates a new static threshold
func NewStatic(threshold int) *Static {
	return &Static{
		threshold: threshold,
		responses: make(map[ids.NodeID]bool),
	}
}

// Add records a response from a node
func (s *Static) Add(nodeID ids.NodeID, response bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if we already have a response from this node
	if prevResponse, exists := s.responses[nodeID]; exists {
		// Update counters if response changed
		if prevResponse && !response {
			s.positive--
			s.negative++
		} else if !prevResponse && response {
			s.negative--
			s.positive++
		}
	} else {
		// New response
		if response {
			s.positive++
		} else {
			s.negative++
		}
	}
	
	s.responses[nodeID] = response
}

// Check returns the current quorum status
func (s *Static) Check() Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	participants := make([]ids.NodeID, 0, len(s.responses))
	for nodeID, response := range s.responses {
		if response {
			participants = append(participants, nodeID)
		}
	}
	
	return Result{
		Achieved:     s.positive >= s.threshold,
		Count:        s.positive,
		Threshold:    s.threshold,
		Participants: participants,
		TotalPolled:  len(s.responses),
	}
}

// Reset clears all recorded responses
func (s *Static) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.responses = make(map[ids.NodeID]bool)
	s.positive = 0
	s.negative = 0
}

// SetThreshold updates the threshold value
func (s *Static) SetThreshold(threshold int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.threshold = threshold
}

// GetThreshold returns the current threshold
func (s *Static) GetThreshold() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.threshold
}

// WeightedStatic implements weighted voting with a static threshold
type WeightedStatic struct {
	mu              sync.RWMutex
	weightThreshold uint64
	responses       map[ids.NodeID]bool
	weights         map[ids.NodeID]uint64
	weightFor       uint64
	weightAgainst   uint64
}

// NewWeightedStatic creates a new weighted static threshold
func NewWeightedStatic(weightThreshold uint64) *WeightedStatic {
	return &WeightedStatic{
		weightThreshold: weightThreshold,
		responses:       make(map[ids.NodeID]bool),
		weights:         make(map[ids.NodeID]uint64),
	}
}

// Add records an unweighted response (weight = 1)
func (ws *WeightedStatic) Add(nodeID ids.NodeID, response bool) {
	ws.AddWeighted(nodeID, response, 1)
}

// AddWeighted records a weighted response from a node
func (ws *WeightedStatic) AddWeighted(nodeID ids.NodeID, response bool, weight uint64) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	// Remove previous weight if exists
	if prevResponse, exists := ws.responses[nodeID]; exists {
		prevWeight := ws.weights[nodeID]
		if prevResponse {
			ws.weightFor -= prevWeight
		} else {
			ws.weightAgainst -= prevWeight
		}
	}
	
	// Add new weight
	ws.responses[nodeID] = response
	ws.weights[nodeID] = weight
	
	if response {
		ws.weightFor += weight
	} else {
		ws.weightAgainst += weight
	}
}

// Check returns the basic quorum status
func (ws *WeightedStatic) Check() Result {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	
	participants := make([]ids.NodeID, 0)
	positiveCount := 0
	
	for nodeID, response := range ws.responses {
		if response {
			participants = append(participants, nodeID)
			positiveCount++
		}
	}
	
	return Result{
		Achieved:     ws.weightFor >= ws.weightThreshold,
		Count:        positiveCount,
		Threshold:    int(ws.weightThreshold), // Approximation for interface
		Participants: participants,
		TotalPolled:  len(ws.responses),
	}
}

// GetWeightedResult returns detailed weighted voting results
func (ws *WeightedStatic) GetWeightedResult() WeightedResult {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	
	result := ws.Check()
	
	return WeightedResult{
		Result:          result,
		WeightFor:       ws.weightFor,
		WeightAgainst:   ws.weightAgainst,
		WeightThreshold: ws.weightThreshold,
		TotalWeight:     ws.weightFor + ws.weightAgainst,
	}
}

// Reset clears all recorded responses
func (ws *WeightedStatic) Reset() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	ws.responses = make(map[ids.NodeID]bool)
	ws.weights = make(map[ids.NodeID]uint64)
	ws.weightFor = 0
	ws.weightAgainst = 0
}

// SetThreshold updates the threshold value
func (ws *WeightedStatic) SetThreshold(threshold int) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	ws.weightThreshold = uint64(threshold)
}

// GetThreshold returns the current threshold
func (ws *WeightedStatic) GetThreshold() int {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	
	return int(ws.weightThreshold)
}