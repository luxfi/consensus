// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"fmt"

	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// Cut defines the angle at which votes are evaluated
// Like a prism's cut angle determines which wavelengths pass through,
// this determines when you've collected enough votes for consensus
type Cut struct {
	// Quorum thresholds
	alphaPreference int // α threshold for preference
	alphaConfidence int // α threshold for confidence
	beta            int // β consecutive successes for finalization

	// Current state
	preference  ids.ID
	confidence  int
	finalized   bool
	
	// Vote tracking
	votes       bag.Bag[ids.ID]
	totalWeight uint64
}

// Ensure Cut implements Cutter
var _ Cutter = (*Cut)(nil)

// NewCut creates a new cut with the given thresholds
func NewCut(alphaPreference, alphaConfidence, beta int) *Cut {
	return &Cut{
		alphaPreference: alphaPreference,
		alphaConfidence: alphaConfidence,
		beta:            beta,
		votes:           bag.Bag[ids.ID]{},
	}
}

// RecordVote records a weighted vote from a validator
func (c *Cut) RecordVote(choice ids.ID, weight uint64) {
	c.votes.AddCount(choice, int(weight))
	c.totalWeight += weight
}

// RecordVotes records multiple votes at once
func (c *Cut) RecordVotes(votes bag.Bag[ids.ID]) {
	// Iterate through all choices in the bag
	for _, choice := range votes.List() {
		weight := votes.Count(choice)
		c.RecordVote(choice, uint64(weight))
	}
}

// Refract processes the accumulated votes through our cut angle
// Returns whether the preference changed
func (c *Cut) Refract() bool {
	// Find the choice with the most votes
	var leadingChoice ids.ID
	var leadingWeight uint64
	
	for _, choice := range c.votes.List() {
		weight := c.votes.Count(choice)
		if uint64(weight) > leadingWeight {
			leadingChoice = choice
			leadingWeight = uint64(weight)
		}
	}
	
	// Check if we have a new preference
	oldPreference := c.preference
	
	// Update preference if we meet α threshold
	if leadingWeight >= uint64(c.alphaPreference) {
		c.preference = leadingChoice
	}
	
	// Update confidence based on α confidence threshold
	if c.preference != ids.Empty && leadingChoice == c.preference {
		if leadingWeight >= uint64(c.alphaConfidence) {
			c.confidence++
			// Check for finalization
			if c.confidence >= c.beta {
				c.finalized = true
			}
		} else {
			// Reset confidence if we don't meet threshold
			c.confidence = 0
		}
	} else if c.preference != oldPreference {
		// Reset confidence on preference change
		c.confidence = 0
	}
	
	return c.preference != oldPreference
}

// IsFinalized returns whether this cut has reached finalization
func (c *Cut) IsFinalized() bool {
	return c.finalized
}

// IsAlphaPreferred returns true if the votes meet the alpha preference threshold
func (c *Cut) IsAlphaPreferred(votes int) bool {
	return votes >= c.alphaPreference
}

// IsAlphaConfident returns true if the votes meet the alpha confidence threshold
func (c *Cut) IsAlphaConfident(votes int) bool {
	return votes >= c.alphaConfidence
}

// IsBetaVirtuous returns true if the successful polls meet the beta threshold
func (c *Cut) IsBetaVirtuous(successfulPolls int) bool {
	return successfulPolls >= c.beta
}

// GetPreference returns the current preference
func (c *Cut) GetPreference() ids.ID {
	return c.preference
}

// GetConfidence returns the current confidence level
func (c *Cut) GetConfidence() int {
	return c.confidence
}

// GetVotes returns the current vote distribution
func (c *Cut) GetVotes() bag.Bag[ids.ID] {
	return c.votes
}

// Reset clears the current votes but maintains thresholds
func (c *Cut) Reset() {
	c.votes = bag.Bag[ids.ID]{}
	c.totalWeight = 0
	c.preference = ids.Empty
	c.confidence = 0
	c.finalized = false
}


// String returns a string representation of the cut
func (c *Cut) String() string {
	return fmt.Sprintf("Cut{α_pref=%d, α_conf=%d, β=%d, pref=%s, conf=%d, finalized=%v, votes=%v}",
		c.alphaPreference,
		c.alphaConfidence,
		c.beta,
		c.preference,
		c.confidence,
		c.finalized,
		c.votes,
	)
}

// CutAnalyzer provides analysis of cut behavior across multiple rounds
type CutAnalyzer struct {
	cuts map[ids.ID]*Cut
	
	// Metrics
	roundsToFinalization map[ids.ID]int
	preferenceChanges    map[ids.ID]int
}

// NewCutAnalyzer creates a new analyzer for tracking cut behavior
func NewCutAnalyzer() *CutAnalyzer {
	return &CutAnalyzer{
		cuts:                 make(map[ids.ID]*Cut),
		roundsToFinalization: make(map[ids.ID]int),
		preferenceChanges:    make(map[ids.ID]int),
	}
}

// AddCut adds a cut to track
func (ca *CutAnalyzer) AddCut(id ids.ID, cut *Cut) {
	ca.cuts[id] = cut
	ca.roundsToFinalization[id] = 0
	ca.preferenceChanges[id] = 0
}

// RecordRound records the results of a consensus round
func (ca *CutAnalyzer) RecordRound() {
	for id, cut := range ca.cuts {
		if !cut.IsFinalized() {
			ca.roundsToFinalization[id]++
		}
		
		// Track preference changes
		oldPref := cut.GetPreference()
		if cut.Refract() && oldPref != ids.Empty {
			ca.preferenceChanges[id]++
		}
	}
}

// GetMetrics returns analysis metrics
func (ca *CutAnalyzer) GetMetrics() map[string]interface{} {
	totalFinalized := 0
	totalRounds := 0
	totalChanges := 0
	
	for id, cut := range ca.cuts {
		if cut.IsFinalized() {
			totalFinalized++
			totalRounds += ca.roundsToFinalization[id]
		}
		totalChanges += ca.preferenceChanges[id]
	}
	
	avgRounds := 0.0
	if totalFinalized > 0 {
		avgRounds = float64(totalRounds) / float64(totalFinalized)
	}
	
	return map[string]interface{}{
		"total_cuts":               len(ca.cuts),
		"finalized":               totalFinalized,
		"avg_rounds_to_finalize":  avgRounds,
		"total_preference_changes": totalChanges,
	}
}

// MultiCut handles multiple cuts for different decisions simultaneously
type MultiCut struct {
	cuts map[ids.ID]*Cut
	
	// Default thresholds for new cuts
	defaultAlphaPreference int
	defaultAlphaConfidence int
	defaultBeta            int
}

// NewMultiCut creates a new multi-cut manager
func NewMultiCut(alphaPref, alphaConf, beta int) *MultiCut {
	return &MultiCut{
		cuts:                   make(map[ids.ID]*Cut),
		defaultAlphaPreference: alphaPref,
		defaultAlphaConfidence: alphaConf,
		defaultBeta:            beta,
	}
}

// GetOrCreateCut gets an existing cut or creates a new one
func (mc *MultiCut) GetOrCreateCut(decision ids.ID) *Cut {
	if cut, exists := mc.cuts[decision]; exists {
		return cut
	}
	
	cut := NewCut(
		mc.defaultAlphaPreference,
		mc.defaultAlphaConfidence,
		mc.defaultBeta,
	)
	mc.cuts[decision] = cut
	return cut
}

// RecordVote records a vote for a specific decision
func (mc *MultiCut) RecordVote(decision ids.ID, choice ids.ID, weight uint64) {
	cut := mc.GetOrCreateCut(decision)
	cut.RecordVote(choice, weight)
}

// RefractAll processes all cuts and returns decisions that changed preference
func (mc *MultiCut) RefractAll() []ids.ID {
	var changed []ids.ID
	
	for decision, cut := range mc.cuts {
		if cut.Refract() {
			changed = append(changed, decision)
		}
	}
	
	return changed
}

// GetFinalized returns all finalized decisions
func (mc *MultiCut) GetFinalized() []ids.ID {
	var finalized []ids.ID
	
	for decision, cut := range mc.cuts {
		if cut.IsFinalized() {
			finalized = append(finalized, decision)
		}
	}
	
	return finalized
}

// RemoveFinalized removes all finalized cuts
func (mc *MultiCut) RemoveFinalized() int {
	removed := 0
	
	for decision, cut := range mc.cuts {
		if cut.IsFinalized() {
			delete(mc.cuts, decision)
			removed++
		}
	}
	
	return removed
}