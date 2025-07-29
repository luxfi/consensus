// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"sync"
	
	"github.com/luxfi/ids"
)

// Dynamic implements separate preference and confidence thresholds
type Dynamic struct {
	mu                 sync.RWMutex
	preferenceThreshold int
	confidenceThreshold int
	responses          map[ids.NodeID]bool
	positive           int
	negative           int
}

// NewDynamic creates a new dynamic threshold
func NewDynamic(preference, confidence int) *Dynamic {
	return &Dynamic{
		preferenceThreshold: preference,
		confidenceThreshold: confidence,
		responses:          make(map[ids.NodeID]bool),
	}
}

// Add records a response from a node
func (d *Dynamic) Add(nodeID ids.NodeID, response bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Check if we already have a response from this node
	if prevResponse, exists := d.responses[nodeID]; exists {
		// Update counters if response changed
		if prevResponse && !response {
			d.positive--
			d.negative++
		} else if !prevResponse && response {
			d.negative--
			d.positive++
		}
	} else {
		// New response
		if response {
			d.positive++
		} else {
			d.negative++
		}
	}
	
	d.responses[nodeID] = response
}

// Check returns the confidence threshold status (stricter)
func (d *Dynamic) Check() Result {
	return d.CheckConfidence()
}

// CheckPreference checks if preference threshold is met
func (d *Dynamic) CheckPreference() Result {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	participants := make([]ids.NodeID, 0, d.positive)
	for nodeID, response := range d.responses {
		if response {
			participants = append(participants, nodeID)
		}
	}
	
	return Result{
		Achieved:     d.positive >= d.preferenceThreshold,
		Count:        d.positive,
		Threshold:    d.preferenceThreshold,
		Participants: participants,
		TotalPolled:  len(d.responses),
	}
}

// CheckConfidence checks if confidence threshold is met
func (d *Dynamic) CheckConfidence() Result {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	participants := make([]ids.NodeID, 0, d.positive)
	for nodeID, response := range d.responses {
		if response {
			participants = append(participants, nodeID)
		}
	}
	
	return Result{
		Achieved:     d.positive >= d.confidenceThreshold,
		Count:        d.positive,
		Threshold:    d.confidenceThreshold,
		Participants: participants,
		TotalPolled:  len(d.responses),
	}
}

// Reset clears all recorded responses
func (d *Dynamic) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.responses = make(map[ids.NodeID]bool)
	d.positive = 0
	d.negative = 0
}

// SetThreshold sets the confidence threshold (for interface compatibility)
func (d *Dynamic) SetThreshold(threshold int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.confidenceThreshold = threshold
}

// GetThreshold returns the confidence threshold (for interface compatibility)
func (d *Dynamic) GetThreshold() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	return d.confidenceThreshold
}

// SetThresholds sets both preference and confidence thresholds
func (d *Dynamic) SetThresholds(preference, confidence int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.preferenceThreshold = preference
	d.confidenceThreshold = confidence
}

// GetThresholds returns both threshold values
func (d *Dynamic) GetThresholds() (preference, confidence int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	return d.preferenceThreshold, d.confidenceThreshold
}

// AdaptiveDynamic implements dynamic thresholds that adapt based on network conditions
type AdaptiveDynamic struct {
	*Dynamic
	
	// Adaptation parameters
	minPreference      int
	maxPreference      int
	minConfidence      int
	maxConfidence      int
	
	// Network metrics
	recentLatencies    []int64 // in milliseconds
	recentSuccessRates []float64
	adaptationWindow   int
}

// NewAdaptiveDynamic creates a new adaptive dynamic threshold
func NewAdaptiveDynamic(initialPref, initialConf int) *AdaptiveDynamic {
	return &AdaptiveDynamic{
		Dynamic: NewDynamic(initialPref, initialConf),
		
		minPreference:    initialPref - 2,
		maxPreference:    initialPref + 2,
		minConfidence:    initialConf - 1,
		maxConfidence:    initialConf + 2,
		
		recentLatencies:    make([]int64, 0, 100),
		recentSuccessRates: make([]float64, 0, 100),
		adaptationWindow:   100,
	}
}

// RecordNetworkMetrics updates network performance metrics
func (ad *AdaptiveDynamic) RecordNetworkMetrics(latencyMs int64, successRate float64) {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	
	// Add new metrics
	ad.recentLatencies = append(ad.recentLatencies, latencyMs)
	ad.recentSuccessRates = append(ad.recentSuccessRates, successRate)
	
	// Maintain window size
	if len(ad.recentLatencies) > ad.adaptationWindow {
		ad.recentLatencies = ad.recentLatencies[1:]
	}
	if len(ad.recentSuccessRates) > ad.adaptationWindow {
		ad.recentSuccessRates = ad.recentSuccessRates[1:]
	}
	
	// Adapt thresholds based on metrics
	ad.adaptThresholds()
}

// adaptThresholds adjusts thresholds based on network conditions
func (ad *AdaptiveDynamic) adaptThresholds() {
	if len(ad.recentLatencies) < 10 {
		return // Not enough data
	}
	
	// Calculate average metrics
	var avgLatency int64
	var avgSuccess float64
	
	for _, lat := range ad.recentLatencies {
		avgLatency += lat
	}
	avgLatency /= int64(len(ad.recentLatencies))
	
	for _, rate := range ad.recentSuccessRates {
		avgSuccess += rate
	}
	avgSuccess /= float64(len(ad.recentSuccessRates))
	
	// Adapt based on conditions
	if avgLatency > 200 && avgSuccess < 0.9 {
		// Network is slow and unreliable - be more conservative
		ad.preferenceThreshold = min(ad.preferenceThreshold+1, ad.maxPreference)
		ad.confidenceThreshold = min(ad.confidenceThreshold+1, ad.maxConfidence)
	} else if avgLatency < 50 && avgSuccess > 0.95 {
		// Network is fast and reliable - can be more aggressive
		ad.preferenceThreshold = max(ad.preferenceThreshold-1, ad.minPreference)
		ad.confidenceThreshold = max(ad.confidenceThreshold-1, ad.minConfidence)
	}
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}