// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import "testing"

const alphaPreference = 3

var TerminationConditions = []TerminationCondition{
	{
		AlphaConfidence: 3,
		Beta:            4,
	},
	{
		AlphaConfidence: 4,
		Beta:            3,
	},
	{
		AlphaConfidence: 5,
		Beta:            2,
	},
}

type waveTestConstructor[T comparable] func(t *testing.T, alphaPreference int, TerminationConditions []TerminationCondition) waveTest[T]

type waveTest[T comparable] interface {
	RecordPoll(count int, optionalMode T)
	RecordUnsuccessfulPoll()
	AssertEqual(expectedConfidences []int, expectedFinalized bool, expectedPreference T)
}

func executeErrorDrivenTerminatesInBetaPolls[T comparable](t *testing.T, newWaveTest waveTestConstructor[T], choice T) {
	for i, TerminationCondition := range TerminationConditions {
		sfTest := newWaveTest(t, alphaPreference, TerminationConditions)

		for poll := 0; poll < TerminationCondition.Beta; poll++ {
			sfTest.RecordPoll(TerminationCondition.AlphaConfidence, choice)

			expectedConfidences := make([]int, len(TerminationConditions))
			for j := 0; j < i+1; j++ {
				expectedConfidences[j] = poll + 1
			}
			sfTest.AssertEqual(expectedConfidences, poll+1 >= TerminationCondition.Beta, choice)
		}
	}
}

func executeErrorDrivenReset[T comparable](t *testing.T, newWaveTest waveTestConstructor[T], choice T) {
	for i, TerminationCondition := range TerminationConditions {
		sfTest := newWaveTest(t, alphaPreference, TerminationConditions)

		// Accumulate confidence up to 1 less than Beta, reset, and confirm
		// expected behavior from fresh state.
		for poll := 0; poll < TerminationCondition.Beta-1; poll++ {
			sfTest.RecordPoll(TerminationCondition.AlphaConfidence, choice)
		}
		sfTest.RecordUnsuccessfulPoll()
		zeroConfidence := make([]int, len(TerminationConditions))
		sfTest.AssertEqual(zeroConfidence, false, choice)

		for poll := 0; poll < TerminationCondition.Beta; poll++ {
			sfTest.RecordPoll(TerminationCondition.AlphaConfidence, choice)

			expectedConfidences := make([]int, len(TerminationConditions))
			for j := 0; j < i+1; j++ {
				expectedConfidences[j] = poll + 1
			}
			sfTest.AssertEqual(expectedConfidences, poll+1 >= TerminationCondition.Beta, choice)
		}
	}
}

func executeErrorDrivenResetHighestAlphaConfidence[T comparable](t *testing.T, newWaveTest waveTestConstructor[T], choice T) {
	sfTest := newWaveTest(t, alphaPreference, TerminationConditions)

	sfTest.RecordPoll(5, choice)
	sfTest.AssertEqual([]int{1, 1, 1}, false, choice)
	sfTest.RecordPoll(4, choice)
	sfTest.AssertEqual([]int{2, 2, 0}, false, choice)
	sfTest.RecordPoll(3, choice)
	sfTest.AssertEqual([]int{3, 0, 0}, false, choice)
	sfTest.RecordPoll(5, choice)
	sfTest.AssertEqual([]int{4, 0, 0}, true, choice)
}

type waveTestSingleChoice[T comparable] struct {
	name string
	f    func(*testing.T, waveTestConstructor[T], T)
}

func getErrorDrivenWaveSingleChoiceSuite[T comparable]() []waveTestSingleChoice[T] {
	return []waveTestSingleChoice[T]{
		{
			name: "TerminateInBetaPolls",
			f:    executeErrorDrivenTerminatesInBetaPolls[T],
		},
		{
			name: "Reset",
			f:    executeErrorDrivenReset[T],
		},
		{
			name: "ResetHighestAlphaConfidence",
			f:    executeErrorDrivenResetHighestAlphaConfidence[T],
		},
	}
}

func executeErrorDrivenSwitchChoices[T comparable](t *testing.T, newWaveTest waveTestConstructor[T], choice0, choice1 T) {
	sfTest := newWaveTest(t, alphaPreference, TerminationConditions)

	sfTest.RecordPoll(3, choice0)
	sfTest.AssertEqual([]int{1, 0, 0}, false, choice0)

	sfTest.RecordPoll(2, choice1)
	sfTest.AssertEqual([]int{0, 0, 0}, false, choice0)

	sfTest.RecordPoll(3, choice0)
	sfTest.AssertEqual([]int{1, 0, 0}, false, choice0)

	sfTest.RecordPoll(0, choice0)
	sfTest.AssertEqual([]int{0, 0, 0}, false, choice0)

	sfTest.RecordPoll(3, choice1)
	sfTest.AssertEqual([]int{1, 0, 0}, false, choice1)

	sfTest.RecordPoll(5, choice1)
	sfTest.AssertEqual([]int{2, 1, 1}, false, choice1)
	sfTest.RecordPoll(5, choice1)
	sfTest.AssertEqual([]int{3, 2, 2}, true, choice1)
}

type waveTestMultiChoice[T comparable] struct {
	name string
	f    func(*testing.T, waveTestConstructor[T], T, T)
}

func getErrorDrivenWaveMultiChoiceSuite[T comparable]() []waveTestMultiChoice[T] {
	return []waveTestMultiChoice[T]{
		{
			name: "SwitchChoices",
			f:    executeErrorDrivenSwitchChoices[T],
		},
	}
}