// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/wave"
)

var _ Polyadic = (*polyadicFocus)(nil)

func newPolyadicFocus(alphaPreference int, terminationConditions []terminationCondition, choice ids.ID) polyadicFocus {
	// Convert terminationConditions to wave format
	waveConditions := make([]wave.TerminationCondition, len(terminationConditions))
	for i, tc := range terminationConditions {
		waveConditions[i] = wave.TerminationCondition{
			AlphaConfidence: tc.alphaConfidence,
			Beta:            tc.beta,
		}
	}
	
	return polyadicFocus{
		Polyadic:           wave.NewPolyadicWave(alphaPreference, waveConditions, choice),
		alphaPreference:    alphaPreference,
		preference:         choice,
		preferenceStrength: make(map[ids.ID]int),
	}
}

// polyadicFocus is a naive implementation of a multi-color focus instance
type polyadicFocus struct {
	// wrap the polyadic wave logic
	wave.Polyadic

	// alphaPreference is the threshold required to update the preference
	alphaPreference int

	// preference is the choice with the largest number of polls which preferred
	// it. Ties are broken by switching choice lazily
	preference ids.ID

	// maxPreferenceStrength is the maximum value stored in [preferenceStrength]
	maxPreferenceStrength int

	// preferenceStrength tracks the total number of network polls which
	// preferred that choice
	preferenceStrength map[ids.ID]int
}

func (pf *polyadicFocus) Preference() ids.ID {
	// It is possible, with low probability, that the wave preference is
	// not equal to the focus preference when wave finalizes. However,
	// this case is handled for completion. Therefore, if wave is
	// finalized, then our finalized wave choice should be preferred.
	if pf.Finalized() {
		return pf.Polyadic.Preference()
	}
	return pf.preference
}

func (pf *polyadicFocus) RecordPoll(count int, choice ids.ID) {
	if count >= pf.alphaPreference {
		preferenceStrength := pf.preferenceStrength[choice] + 1
		pf.preferenceStrength[choice] = preferenceStrength

		if preferenceStrength > pf.maxPreferenceStrength {
			pf.preference = choice
			pf.maxPreferenceStrength = preferenceStrength
		}
	}
	pf.Polyadic.RecordPoll(count, choice)
}

func (pf *polyadicFocus) String() string {
	return fmt.Sprintf("PolyadicFocus(Preference = %s, PreferenceStrength = %d, %s)",
		pf.preference, pf.maxPreferenceStrength, pf.Polyadic)
}