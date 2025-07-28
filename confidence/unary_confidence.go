// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

import (
	"fmt"
)

var _ Unary = (*unaryConfidence)(nil)

func newUnaryConfidence(alphaPreference int, terminationConditions []terminationCondition) unaryConfidence {
	return unaryConfidence{
		unaryThreshold: newUnaryThreshold(alphaPreference, terminationConditions),
	}
}

// unaryConfidence is the implementation of a unary confidence instance
type unaryConfidence struct {
	// wrap the unary threshold logic
	unaryThreshold

	// preferenceStrength tracks the total number of polls with a preference
	preferenceStrength int
}

func (sb *unaryConfidence) RecordPoll(count int) {
	if count >= sb.alphaPreference {
		sb.preferenceStrength++
	}
	sb.unaryThreshold.RecordPoll(count)
}

func (sb *unaryConfidence) Extend(choice int) Binary {
	bs := &binaryConfidence{
		binaryThreshold: sb.unaryThreshold.Extend(choice),
		preference:      choice,
	}
	bs.preferenceStrength[choice] = sb.preferenceStrength
	return bs
}

func (sb *unaryConfidence) Clone() Unary {
	newConfidence := *sb
	newConfidence.unaryThreshold = sb.unaryThreshold.Clone()
	return &newConfidence
}

func (sb *unaryConfidence) Preference() int {
	// Unary has no choice, always returns 0
	return 0
}

func (sb *unaryConfidence) String() string {
	return fmt.Sprintf("SB(PreferenceStrength = %d, %v)",
		sb.preferenceStrength,
		sb.unaryThreshold)
}
