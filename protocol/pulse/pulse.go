// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pulse

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// Pulse implements binary consensus protocol
type Pulse struct {
	// params contains all the configuration parameters
	params config.Parameters

	// preference is the currently preferred choice
	preference ids.ID

	// pulsePreference is the preference from the embedded pulse logic
	pulsePreference ids.ID

	// preferenceStrength tracks the total number of polls which preferred each choice
	preferenceStrength map[ids.ID]int

	// confidence tracks consecutive successful polls for the current preference
	confidence int

	// finalized prevents the state from changing after consensus
	finalized bool

	// choices tracks all added choices
	choices []ids.ID
}

// NewPulse creates a new pulse consensus instance
func NewPulse(params config.Parameters) *Pulse {
	return &Pulse{
		params:             params,
		preferenceStrength: make(map[ids.ID]int),
	}
}

// Add adds a new choice to the consensus
func (p *Pulse) Add(choice ids.ID) error {
	if p.finalized {
		return errors.New("cannot add choice after finalization")
	}

	// First choice becomes the initial preference
	if p.preference == ids.Empty {
		p.preference = choice
		p.pulsePreference = choice
	}

	// Track all choices
	for _, existing := range p.choices {
		if existing == choice {
			return nil // Already added
		}
	}
	p.choices = append(p.choices, choice)
	p.preferenceStrength[choice] = 0
	return nil
}

// Preference returns the current preference
func (p *Pulse) Preference() ids.ID {
	// If finalized, return the finalized pulse preference
	if p.finalized {
		return p.pulsePreference
	}
	return p.preference
}

// RecordVotes records a set of votes
func (p *Pulse) RecordVotes(votes bag.Bag[ids.ID]) error {
	if p.finalized {
		return nil
	}

	// Find the choice with the most votes
	var maxVotes int
	var maxChoice ids.ID

	for _, choice := range p.choices {
		count := votes.Count(choice)
		if count > maxVotes {
			maxVotes = count
			maxChoice = choice
		}
	}

	return p.recordPoll(maxVotes, maxChoice)
}

// RecordPrism records votes in prism format
func (p *Pulse) RecordPrism(votes bag.Bag[ids.ID]) error {
	return p.RecordVotes(votes)
}

// recordPoll updates the state based on a poll result
func (p *Pulse) recordPoll(count int, choice ids.ID) error {
	if p.finalized || choice == ids.Empty {
		return nil
	}

	// Pulse logic: track preference strength
	if count >= p.params.AlphaPreference {
		// Reset other preference strengths when voting for a new choice
		if p.preference != choice {
			for c := range p.preferenceStrength {
				if c != choice {
					p.preferenceStrength[c] = 0
				}
			}
		}

		p.preferenceStrength[choice]++

		// Update preference to the strongest choice
		maxStrength := 0
		for c, strength := range p.preferenceStrength {
			if strength > maxStrength {
				maxStrength = strength
				p.preference = c
			}
		}
	}

	// Pulse confidence logic: track confidence
	if count >= p.params.AlphaPreference && choice == p.pulsePreference {
		if count >= p.params.AlphaConfidence {
			p.confidence++
			if p.confidence >= int(p.params.Beta) {
				p.finalized = true
			}
		} else {
			p.confidence = 0
		}
	} else if count >= p.params.AlphaPreference && choice != p.pulsePreference {
		// Switch preference and reset confidence
		p.pulsePreference = choice
		p.confidence = 1
		if p.confidence >= int(p.params.Beta) {
			p.finalized = true
		}
	} else {
		// Unsuccessful poll
		p.confidence = 0
	}

	return nil
}

// RecordUnsuccessfulPoll resets confidence
func (p *Pulse) RecordUnsuccessfulPoll() {
	p.confidence = 0
}

// Finalized returns whether consensus has been reached
func (p *Pulse) Finalized() bool {
	return p.finalized
}

// String returns a string representation
func (p *Pulse) String() string {
	return fmt.Sprintf("Pulse{pref=%s, pulse_pref=%s, conf=%d, finalized=%v}",
		p.preference, p.pulsePreference, p.confidence, p.finalized)
}
