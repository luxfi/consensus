// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils"
)

// Tree implements the Consensus interface for beam
type Tree struct {
	params     Parameters
	preference ids.ID
	choices    map[ids.ID]bool
	finalized  bool
}

// NewTree creates a new tree consensus instance
func NewTree(factory Factory, params Parameters, choice ids.ID) Consensus {
	return &Tree{
		params:     params,
		preference: choice,
		choices:    map[ids.ID]bool{choice: true},
		finalized:  false,
	}
}

// Add adds a new choice to vote on
func (t *Tree) Add(newChoice ids.ID) {
	if t.finalized {
		return
	}
	t.choices[newChoice] = true
}

// Preference returns the currently preferred choice to be finalized
func (t *Tree) Preference() ids.ID {
	return t.preference
}

// RecordPoll records the results of a network poll
func (t *Tree) RecordPoll(votes *utils.Bag) bool {
	if t.finalized {
		return true
	}

	// Simple majority voting
	maxVotes := 0
	var winner ids.ID
	for _, id := range votes.List() {
		count := votes.Count(id)
		if count > maxVotes {
			maxVotes = count
			winner = id
		}
	}

	if maxVotes >= t.params.AlphaPreference {
		t.preference = winner
		return true
	}
	return false
}

// RecordUnsuccessfulPoll resets the wave counters
func (t *Tree) RecordUnsuccessfulPoll() {
	// No-op for simple tree
}

// Finalized returns whether a choice has been finalized
func (t *Tree) Finalized() bool {
	return t.finalized
}

// String returns a string representation
func (t *Tree) String() string {
	return fmt.Sprintf("Tree{preference: %s, finalized: %v}", t.preference, t.finalized)
}