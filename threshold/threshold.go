// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"github.com/luxfi/ids"
)

// Result represents the result of a quorum check
type Result struct {
	// Achieved indicates if the threshold was met
	Achieved bool
	
	// Count is the number of positive responses
	Count int
	
	// Threshold is the required threshold
	Threshold int
	
	// Participants lists the nodes that responded
	Participants []ids.NodeID
	
	// TotalPolled is the total number of nodes polled
	TotalPolled int
}

// Threshold defines the interface for quorum threshold strategies
type Threshold interface {
	// Add records a response from a node
	Add(nodeID ids.NodeID, response bool)
	
	// Check returns the current quorum status
	Check() Result
	
	// Reset clears all recorded responses
	Reset()
	
	// SetThreshold updates the threshold value
	SetThreshold(threshold int)
	
	// GetThreshold returns the current threshold
	GetThreshold() int
}

// WeightedThreshold extends Threshold with weight support
type WeightedThreshold interface {
	Threshold
	
	// AddWeighted records a weighted response from a node
	AddWeighted(nodeID ids.NodeID, response bool, weight uint64)
	
	// GetWeightedResult returns detailed weighted quorum results
	GetWeightedResult() WeightedResult
}

// WeightedResult provides detailed weighted voting results
type WeightedResult struct {
	Result
	
	// WeightFor is the total weight voting for
	WeightFor uint64
	
	// WeightAgainst is the total weight voting against
	WeightAgainst uint64
	
	// WeightThreshold is the required weight threshold
	WeightThreshold uint64
	
	// TotalWeight is the total weight of all votes
	TotalWeight uint64
}

// DynamicThreshold supports separate preference and confidence thresholds
type DynamicThreshold interface {
	Threshold
	
	// SetThresholds sets both preference and confidence thresholds
	SetThresholds(preference, confidence int)
	
	// CheckPreference checks if preference threshold is met
	CheckPreference() Result
	
	// CheckConfidence checks if confidence threshold is met
	CheckConfidence() Result
	
	// GetThresholds returns both threshold values
	GetThresholds() (preference, confidence int)
}