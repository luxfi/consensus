// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

import (
	"fmt"

	"github.com/luxfi/ids"
)

// polyConfidence implements confidence voting for multiple choices

func newPolyConfidence(alphaPreference int, terminationConditions []terminationCondition, choice ids.ID) polyConfidence {
	return polyConfidence{
		polyThreshold:     newPolyThreshold(alphaPreference, terminationConditions, choice),
		preference:         choice,
		preferenceStrength: make(map[ids.ID]int),
	}
}

// polyConfidence is a naive implementation of a multi-color confidence instance
type polyConfidence struct {
	// wrap the poly threshold logic
	polyThreshold

	// preference is the choice with the largest number of polls which preferred
	// it. Ties are broken by switching choice lazily
	preference ids.ID

	// maxPreferenceStrength is the maximum value stored in [preferenceStrength]
	maxPreferenceStrength int

	// preferenceStrength tracks the total number of network polls which
	// preferred that choice
	preferenceStrength map[ids.ID]int
}

func (sb *polyConfidence) Preference() ids.ID {
	// It is possible, with low probability, that the threshold preference is
	// not equal to the confidence preference when threshold finalizes. However,
	// this case is handled for completion. Therefore, if threshold is
	// finalized, then our finalized threshold choice should be preferred.
	if sb.Finalized() {
		return sb.polyThreshold.Preference()
	}
	return sb.preference
}

func (sb *polyConfidence) RecordPoll(count int, choice ids.ID) {
	if count >= sb.alphaPreference {
		preferenceStrength := sb.preferenceStrength[choice] + 1
		sb.preferenceStrength[choice] = preferenceStrength

		if preferenceStrength > sb.maxPreferenceStrength {
			sb.preference = choice
			sb.maxPreferenceStrength = preferenceStrength
		}
	}
	sb.polyThreshold.RecordPoll(count, choice)
}

func (sb *polyConfidence) String() string {
	return fmt.Sprintf("SB(Preference = %s, PreferenceStrength = %d, %v)",
		sb.preference, sb.maxPreferenceStrength, sb.polyThreshold)
}
