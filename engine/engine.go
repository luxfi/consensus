// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// Engine implements the consensus engine
type Engine struct {
	mu         sync.RWMutex
	params     config.Parameters
	preference ids.ID
	confidence int
	finalized  bool
	
	consecutiveSuccesses int
}

// New creates a new consensus engine
func New(params config.Parameters) *Engine {
	return &Engine{
		params:     params,
		preference: ids.Empty,
		confidence: 0,
		finalized:  false,
	}
}

// RecordPoll records votes from validators
func (e *Engine) RecordPoll(votes []Vote) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if e.finalized {
		return
	}
	
	// Count votes for each choice
	voteCounts := make(map[ids.ID]int)
	for _, vote := range votes {
		voteCounts[vote.Preference] += vote.Confidence
	}
	
	// Find the choice with most votes
	var bestChoice ids.ID
	maxVotes := 0
	for choice, count := range voteCounts {
		if count > maxVotes {
			maxVotes = count
			bestChoice = choice
		}
	}
	
	// Update preference if threshold met
	if maxVotes >= e.params.AlphaPreference {
		if bestChoice != e.preference {
			e.preference = bestChoice
			e.consecutiveSuccesses = 0
		}
		
		// Check confidence threshold
		if maxVotes >= e.params.AlphaConfidence {
			e.consecutiveSuccesses++
			e.confidence = e.consecutiveSuccesses
			
			// Check finalization
			if e.consecutiveSuccesses >= e.params.Beta {
				e.finalized = true
			}
		} else {
			e.consecutiveSuccesses = 0
			e.confidence = 0
		}
	}
}

// Preference returns the current preference
func (e *Engine) Preference() ids.ID {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.preference
}

// Confidence returns the current confidence level
func (e *Engine) Confidence() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.confidence
}

// Finalized returns whether consensus has been reached
func (e *Engine) Finalized() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.finalized
}