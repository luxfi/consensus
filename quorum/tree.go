// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/focus"
)

// Tree implements a tree consensus algorithm using focus
type Tree struct {
	params     photon.Parameters
	focus      focus.Polyadic
	choices    map[ids.ID]bool
	preference ids.ID
}

// NewTree creates a new tree consensus instance
func NewTree(params photon.Parameters, choice ids.ID) *Tree {
	focusParams := focus.Parameters{
		AlphaPreference: params.AlphaPreference,
		TerminationConditions: []focus.TerminationCondition{
			{
				AlphaConfidence: params.AlphaConfidence,
				Beta:            params.Beta,
			},
		},
	}
	
	factory := &focus.Factory{}
	polyadic := factory.NewPolyadic(focusParams, choice)
	
	return &Tree{
		params:     params,
		focus:      polyadic,
		choices:    map[ids.ID]bool{choice: true},
		preference: choice,
	}
}

// Add adds a new choice to the consensus
func (t *Tree) Add(choice ids.ID) {
	if !t.choices[choice] {
		t.choices[choice] = true
		t.focus.Add(choice)
	}
}

// Preference returns the current preference
func (t *Tree) Preference() ids.ID {
	return t.focus.Preference()
}

// RecordPoll records a poll result
func (t *Tree) RecordPoll(votes *consensus.Bag[ids.ID]) bool {
	// Set the threshold for this poll
	votes.SetThreshold(t.params.AlphaPreference)
	
	// Get the choice with the most votes
	maxChoice, maxVotes := votes.Mode()
	
	// Check if we have enough votes to record
	if maxVotes >= t.params.AlphaPreference {
		// Record the poll for the winning choice
		t.focus.RecordPoll(maxVotes, maxChoice)
		return true
	}
	
	return false
}

// RecordUnsuccessfulPoll records an unsuccessful poll
func (t *Tree) RecordUnsuccessfulPoll() {
	t.focus.RecordUnsuccessfulPoll()
}

// Finalized returns true if consensus has been reached
func (t *Tree) Finalized() bool {
	return t.focus.Finalized()
}

// String returns a string representation
func (t *Tree) String() string {
	return fmt.Sprintf("Tree{pref=%s, finalized=%v}", t.Preference(), t.Finalized())
}