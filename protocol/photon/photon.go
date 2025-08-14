// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
    "fmt"

    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/utils/bag"
    "github.com/luxfi/ids"
)

// Photon implements unary consensus protocol
type Photon struct {
    // preferenceStrength is the total number of polls which have preferred
    // the choice.
    preferenceStrength int

    // confidence is the number of consecutive successful polls for the choice.
    // confidence is reset to 0 if there's a successful poll that doesn't prefer the choice.
    confidence int

    // finalized prevents the state from changing after the required number of
    // consecutive polls has been reached
    finalized bool

    // alphaPreference is the threshold required to update the preference
    alphaPreference int

    // alphaConfidence is the threshold required to increment the confidence counter
    alphaConfidence int

    // beta is the number of consecutive successful polls required for finalization
    beta int

    // choice is the ID we're trying to reach consensus on
    choice ids.ID
}

// NewPhoton creates a new photon consensus instance
func NewPhoton(params config.Parameters) *Photon {
    return &Photon{
        alphaPreference: params.AlphaPreference,
        alphaConfidence: params.AlphaConfidence,
        beta:            params.Beta,
    }
}

// Add adds a choice to the consensus instance
func (p *Photon) Add(choice ids.ID) error {
    if p.choice != ids.Empty && p.choice != choice {
        return fmt.Errorf("photon instance already has a choice")
    }
    p.choice = choice
    return nil
}

// Preference returns the current preference
func (p *Photon) Preference() ids.ID {
    return p.choice
}

// RecordVotes records a set of votes
func (p *Photon) RecordVotes(votes bag.Bag[ids.ID]) error {
    return p.recordPoll(votes.Count(p.choice))
}

// RecordPrism records votes in prism format
func (p *Photon) RecordPrism(votes bag.Bag[ids.ID]) error {
    return p.RecordVotes(votes)
}

// recordPoll records the results of a network poll
func (p *Photon) recordPoll(count int) error {
    if p.finalized {
        return nil // This instance is already decided.
    }

    if count >= p.alphaPreference {
        p.preferenceStrength++
        
        if count >= p.alphaConfidence {
            p.confidence++
        } else {
            // If I didn't hit alphaConfidence, I reset.
            p.confidence = 0
        }
    } else {
        // If I didn't hit alphaPreference, I reset.
        p.confidence = 0
    }

    // I finalize when confidence reaches beta
    p.finalized = p.confidence >= p.beta
    return nil
}

// RecordUnsuccessfulPoll resets confidence
func (p *Photon) RecordUnsuccessfulPoll() {
    p.confidence = 0
}

// Finalized returns whether consensus has been reached
func (p *Photon) Finalized() bool {
    return p.finalized
}

// String returns a string representation
func (p *Photon) String() string {
    return fmt.Sprintf("Photon{choice=%s, pref_strength=%d, conf=%d, finalized=%v}", 
        p.choice, p.preferenceStrength, p.confidence, p.finalized)
}