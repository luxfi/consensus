// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
    "fmt"

    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/utils/bag"
    "github.com/luxfi/ids"
)

// Wave implements n-ary consensus protocol
type Wave struct {
    // params contains all the configuration parameters
    params config.Parameters

    // preference is the currently preferred choice
    preference ids.ID

    // wavePreference is the preference from the embedded wave logic
    wavePreference ids.ID

    // preferenceStrength tracks the total number of polls which preferred each choice
    preferenceStrength map[ids.ID]int

    // confidence tracks consecutive successful polls for the current preference
    confidence int

    // finalized prevents the state from changing after consensus
    finalized bool

    // choices tracks all added choices
    choices []ids.ID

    // numPolls tracks the number of polls this instance has performed
    numPolls int
}

// NewWave creates a new wave consensus instance
func NewWave(params config.Parameters) *Wave {
    return &Wave{
        params:             params,
        preferenceStrength: make(map[ids.ID]int),
    }
}

// Add adds a new choice to the consensus
func (w *Wave) Add(choice ids.ID) error {
    // First choice becomes the initial preference
    if w.preference == ids.Empty {
        w.preference = choice
        w.wavePreference = choice
    }
    
    // Track all choices
    for _, existing := range w.choices {
        if existing == choice {
            return nil // Already added
        }
    }
    w.choices = append(w.choices, choice)
    w.preferenceStrength[choice] = 0
    return nil
}

// Preference returns the current preference
func (w *Wave) Preference() ids.ID {
    // If finalized, return the finalized wave preference
    if w.finalized {
        return w.wavePreference
    }
    return w.preference
}

// RecordVotes records a set of votes
func (w *Wave) RecordVotes(votes bag.Bag[ids.ID]) error {
    if w.finalized {
        return nil
    }

    w.numPolls++

    // Count votes for each choice
    type voteCount struct {
        choice ids.ID
        count  int
    }
    
    var voteCounts []voteCount
    for _, choice := range w.choices {
        count := votes.Count(choice)
        if count > 0 {
            voteCounts = append(voteCounts, voteCount{choice: choice, count: count})
        }
    }

    // Find the choice with the most votes
    var maxVotes int
    var maxChoice ids.ID
    
    for _, vc := range voteCounts {
        if vc.count > maxVotes {
            maxVotes = vc.count
            maxChoice = vc.choice
        }
    }

    return w.recordPoll(maxVotes, maxChoice)
}

// RecordPrism records votes in prism format
func (w *Wave) RecordPrism(votes bag.Bag[ids.ID]) error {
    return w.RecordVotes(votes)
}

// recordPoll updates the state based on a poll result
func (w *Wave) recordPoll(count int, choice ids.ID) error {
    if w.finalized || choice == ids.Empty {
        return nil
    }

    // Wave logic: track preference strength
    if count >= w.params.AlphaPreference {
        // Reset all other preference strengths when voting for a new choice
        for c := range w.preferenceStrength {
            if c != choice {
                w.preferenceStrength[c] = 0
            }
        }
        
        w.preferenceStrength[choice]++
        
        // Update preference to the strongest choice
        maxStrength := 0
        newPreference := w.preference
        
        for c, strength := range w.preferenceStrength {
            if strength > maxStrength {
                maxStrength = strength
                newPreference = c
            }
        }
        w.preference = newPreference
    }

    // Wave confidence logic: track confidence
    if count >= w.params.AlphaPreference && choice == w.wavePreference {
        if count >= w.params.AlphaConfidence {
            w.confidence++
            if w.confidence >= w.params.Beta {
                w.finalized = true
            }
        } else {
            // Reset confidence if we didn't hit alphaConfidence
            w.confidence = 0
        }
    } else if count >= w.params.AlphaPreference && choice != w.wavePreference {
        // Switch preference and reset confidence
        w.wavePreference = choice
        w.confidence = 1
    } else {
        // Unsuccessful poll
        w.confidence = 0
    }

    return nil
}

// RecordUnsuccessfulPoll resets confidence
func (w *Wave) RecordUnsuccessfulPoll() {
    w.confidence = 0
}

// Finalized returns whether consensus has been reached
func (w *Wave) Finalized() bool {
    return w.finalized
}

// NumPolls returns the number of polls performed
func (w *Wave) NumPolls() int {
    return w.numPolls
}

// String returns a string representation
func (w *Wave) String() string {
    return fmt.Sprintf("Wave{pref=%s, wave_pref=%s, conf=%d, finalized=%v, polls=%d}", 
        w.preference, w.wavePreference, w.confidence, w.finalized, w.numPolls)
}