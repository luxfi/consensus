// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils"
	"github.com/luxfi/consensus/photon"
)

// Consensus interface that Flat uses internally
type Consensus interface {
	Parameters() photon.Parameters
	Add(ids.ID)
	Preference() ids.ID
	RecordPoll(*utils.Bag) bool
	RecordUnsuccessfulPoll()
	Finalized() bool
	fmt.Stringer
}

// Flat is a flat consensus implementation that wraps a photon/wave instance
type Flat struct {
	consensus Consensus
}

// NewFlat creates a new flat consensus instance
func NewFlat(factory photon.Factory, params photon.Parameters, choice ids.ID) *Flat {
	consensus := NewTree(factory, params, choice)
	return &Flat{
		consensus: consensus,
	}
}

// Parameters returns the parameters of this consensus instance
func (f *Flat) Parameters() photon.Parameters {
	return f.consensus.Parameters()
}

// Add adds a new choice to the consensus instance
func (f *Flat) Add(choice ids.ID) {
	f.consensus.Add(choice)
}

// Preference returns the current preferred choice
func (f *Flat) Preference() ids.ID {
	return f.consensus.Preference()
}

// RecordPoll records the results of a network poll
func (f *Flat) RecordPoll(votes *utils.Bag) bool {
	return f.consensus.RecordPoll(votes)
}

// RecordUnsuccessfulPoll resets the confidence counters
func (f *Flat) RecordUnsuccessfulPoll() {
	f.consensus.RecordUnsuccessfulPoll()
}

// Finalized returns whether consensus has been reached
func (f *Flat) Finalized() bool {
	return f.consensus.Finalized()
}

// String returns a string representation
func (f *Flat) String() string {
	return fmt.Sprintf("Flat(%s)", f.consensus.String())
}

// Tree is a simple tree consensus implementation
type Tree struct {
	params     photon.Parameters
	preference ids.ID
	choices    *utils.Bag
	finalized  bool
}

// NewTree creates a new Tree consensus instance
func NewTree(factory photon.Factory, params photon.Parameters, choice ids.ID) Consensus {
	return &Tree{
		params:     params,
		preference: choice,
		choices:    utils.NewBag(),
		finalized:  false,
	}
}

// Parameters returns the consensus parameters
func (t *Tree) Parameters() photon.Parameters {
	return t.params
}

// Add adds a new choice to the consensus
func (t *Tree) Add(choice ids.ID) {
	if t.preference == ids.Empty {
		t.preference = choice
	}
}

// Preference returns the current preference
func (t *Tree) Preference() ids.ID {
	return t.preference
}

// RecordPoll records the results of a network poll
func (t *Tree) RecordPoll(votes *utils.Bag) bool {
	if t.finalized {
		return true
	}
	
	// Find the choice with the most votes
	var maxVotes int
	var maxChoice ids.ID
	for _, id := range votes.List() {
		count := votes.Count(id)
		if count > maxVotes {
			maxVotes = count
			maxChoice = id
		}
	}
	
	// Update preference if threshold met
	if maxVotes >= t.params.AlphaPreference {
		t.preference = maxChoice
		t.choices.AddCount(maxChoice, 1)
		
		// Check finalization
		if t.choices.Count(maxChoice) >= t.params.Beta {
			t.finalized = true
			return true
		}
	}
	
	return false
}

// RecordUnsuccessfulPoll resets the confidence counters
func (t *Tree) RecordUnsuccessfulPoll() {
	t.choices = utils.NewBag()
}

// Finalized returns whether consensus has been reached
func (t *Tree) Finalized() bool {
	return t.finalized
}

// String returns a string representation
func (t *Tree) String() string {
	return fmt.Sprintf("Tree{pref=%s, finalized=%v}", t.preference, t.finalized)
}