// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/config"
	basepoll "github.com/luxfi/consensus/poll"
	"github.com/luxfi/consensus/utils/bag"
)

// Tree represents a tree-based consensus instance
type Tree interface {
	// Add adds a new item to track
	Add(ids.ID)

	// RecordPoll records the results of a poll, returns true if successful
	RecordPoll(votes bag.Bag[ids.ID]) bool

	// RecordUnsuccessfulPoll records an unsuccessful poll
	RecordUnsuccessfulPoll()

	// Finalized returns true if consensus is reached
	Finalized() bool

	// Preference returns the current preference
	Preference() ids.ID
}

// tree implements Tree interface
type tree struct {
	params          config.Parameters
	pollFactory     basepoll.Factory
	preference      ids.ID
	children        map[ids.ID]bool
	successful      int
	unsuccessful    int
}

// NewTree creates a new tree consensus instance
func NewTree(factory basepoll.Factory, params config.Parameters, initialPreference ids.ID) Tree {
	return &tree{
		params:      params,
		pollFactory: factory,
		preference:  initialPreference,
		children:    make(map[ids.ID]bool),
		successful:  0,
	}
}

func (t *tree) Add(id ids.ID) {
	t.children[id] = true
}

func (t *tree) RecordPoll(votes bag.Bag[ids.ID]) bool {
	// Find the most voted ID
	var bestID ids.ID
	maxVotes := 0
	
	for id := range t.children {
		count := votes.Count(id)
		if count > maxVotes {
			maxVotes = count
			bestID = id
		}
	}
	
	// Check if we have enough votes
	if maxVotes >= t.params.AlphaPreference {
		t.preference = bestID
		t.successful++
		t.unsuccessful = 0
		return true
	}
	
	t.unsuccessful++
	return false
}

func (t *tree) RecordUnsuccessfulPoll() {
	t.unsuccessful++
}

func (t *tree) Finalized() bool {
	return t.successful >= t.params.Beta
}

func (t *tree) Preference() ids.ID {
	return t.preference
}