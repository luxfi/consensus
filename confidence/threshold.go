// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

import (
	"github.com/luxfi/consensus/poll"
	"github.com/luxfi/ids"
)

// terminationCondition is imported from termination.go

// binaryThreshold is the binary threshold logic
type binaryThreshold struct {
	poll.BinarySampler
	alphaPreference       int
	terminationConditions []terminationCondition
	confidence            []int
	finalized             bool
}

func newBinaryThreshold(alphaPreference int, terminationConditions []terminationCondition, choice int) binaryThreshold {
	return binaryThreshold{
		BinarySampler:         poll.NewBinarySampler(choice),
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
	}
}

func (bt *binaryThreshold) RecordPoll(count, choice int) {
	if bt.finalized {
		return
	}

	if count < bt.alphaPreference {
		bt.RecordUnsuccessfulPoll()
		return
	}

	if choice != bt.Preference() {
		clear(bt.confidence)
	}
	bt.BinarySampler.RecordSuccessfulPoll(choice)

	for i, terminationCondition := range bt.terminationConditions {
		if count < terminationCondition.AlphaConfidence {
			clear(bt.confidence[i:])
			return
		}

		bt.confidence[i]++
		if bt.confidence[i] >= terminationCondition.Beta {
			bt.finalized = true
			return
		}
	}
}

func (bt *binaryThreshold) RecordUnsuccessfulPoll() {
	clear(bt.confidence)
}

func (bt *binaryThreshold) Finalized() bool {
	return bt.finalized
}

func (bt *binaryThreshold) Extend(choice int) binaryThreshold {
	return newBinaryThreshold(bt.alphaPreference, bt.terminationConditions, choice)
}

// unaryThreshold is the unary threshold logic
type unaryThreshold struct {
	poll.UnarySampler
	alphaPreference       int
	terminationConditions []terminationCondition
	confidence            []int
	finalized             bool
}

func newUnaryThreshold(alphaPreference int, terminationConditions []terminationCondition) unaryThreshold {
	return unaryThreshold{
		UnarySampler:          poll.UnarySampler{},
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
	}
}

func (ut *unaryThreshold) RecordPoll(count int) {
	if ut.finalized {
		return
	}

	if count < ut.alphaPreference {
		ut.RecordUnsuccessfulPoll()
		return
	}

	ut.UnarySampler.RecordSuccessfulPoll()

	for i, terminationCondition := range ut.terminationConditions {
		if count < terminationCondition.AlphaConfidence {
			clear(ut.confidence[i:])
			return
		}

		ut.confidence[i]++
		if ut.confidence[i] >= terminationCondition.Beta {
			ut.finalized = true
			return
		}
	}
}

func (ut *unaryThreshold) RecordUnsuccessfulPoll() {
	clear(ut.confidence)
}

func (ut *unaryThreshold) Finalized() bool {
	return ut.finalized
}

func (ut *unaryThreshold) Extend(choice int) binaryThreshold {
	return newBinaryThreshold(ut.alphaPreference, ut.terminationConditions, choice)
}

func (ut *unaryThreshold) Clone() unaryThreshold {
	newThreshold := *ut
	newThreshold.confidence = make([]int, len(ut.confidence))
	copy(newThreshold.confidence, ut.confidence)
	return newThreshold
}

// polyThreshold is the poly/nnary threshold logic
type polyThreshold struct {
	poll.PolySampler
	alphaPreference       int
	terminationConditions []terminationCondition
	confidence            []int
	finalized             bool
}

// Alias for consistency with poly_confidence.go
type nnaryThreshold = polyThreshold

func newPolyThreshold(alphaPreference int, terminationConditions []terminationCondition, choice ids.ID) polyThreshold {
	return polyThreshold{
		PolySampler:           poll.NewPolySampler(choice),
		alphaPreference:       alphaPreference,
		terminationConditions: terminationConditions,
		confidence:            make([]int, len(terminationConditions)),
	}
}

func (pt *polyThreshold) RecordPoll(count int, choice ids.ID) {
	if pt.finalized {
		return
	}

	if count < pt.alphaPreference {
		pt.RecordUnsuccessfulPoll()
		return
	}

	if choice != pt.Preference() {
		clear(pt.confidence)
	}
	pt.PolySampler.RecordSuccessfulPoll(choice)

	for i, terminationCondition := range pt.terminationConditions {
		if count < terminationCondition.AlphaConfidence {
			clear(pt.confidence[i:])
			return
		}

		pt.confidence[i]++
		if pt.confidence[i] >= terminationCondition.Beta {
			pt.finalized = true
			return
		}
	}
}

func (pt *polyThreshold) RecordUnsuccessfulPoll() {
	clear(pt.confidence)
}

func (pt *polyThreshold) Finalized() bool {
	return pt.finalized
}