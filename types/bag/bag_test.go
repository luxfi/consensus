// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bag

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A bag counts votes and answers which choices have reached a quorum. Every test
// here is written against that reading: counts are tallies, the threshold is the
// quorum, and Threshold() is the set of choices a caller may act on.

func TestEmptyBagCountsNothing(t *testing.T) {
	require := require.New(t)

	var zero Bag[int]
	require.Zero(zero.Len())
	require.Zero(zero.Count(1))
	require.Empty(zero.List())

	fresh := New[int]()
	require.Zero(fresh.Len())
	require.Zero(fresh.Count(1))

	mode, freq := zero.Mode()
	require.Zero(mode)
	require.Zero(freq)
}

func TestAddCountsRepeats(t *testing.T) {
	require := require.New(t)

	b := Of(1, 2, 2, 3, 3, 3)
	require.Equal(6, b.Len())
	require.Equal(1, b.Count(1))
	require.Equal(2, b.Count(2))
	require.Equal(3, b.Count(3))
	require.Zero(b.Count(4))
	require.ElementsMatch([]int{1, 2, 3}, b.List())

	mode, freq := b.Mode()
	require.Equal(3, mode)
	require.Equal(3, freq)
}

// TestAddCountIgnoresNonPositive pins the documented no-op. A vote worth minus
// one would otherwise subtract from a tally without removing anything from it,
// leaving Len and the sum of Counts disagreeing.
func TestAddCountIgnoresNonPositive(t *testing.T) {
	require := require.New(t)

	b := Of(1, 1, 1)
	b.AddCount(1, 0)
	b.AddCount(1, -5)
	b.AddCount(2, -1)

	require.Equal(3, b.Len())
	require.Equal(3, b.Count(1))
	require.Zero(b.Count(2))
	require.NotContains(b.List(), 2)
}

// TestThresholdHoldsExactlyTheChoicesThatReachedIt is the property the whole
// type exists for. It is checked as an exact set, both directions: a choice one
// vote short must be absent, and a choice at the line must be present.
func TestThresholdHoldsExactlyTheChoicesThatReachedIt(t *testing.T) {
	require := require.New(t)

	b := New[string]()
	b.SetThreshold(3)
	b.AddCount("quorum", 3)
	b.AddCount("short", 2)
	b.AddCount("over", 9)

	require.ElementsMatch([]string{"quorum", "over"}, b.Threshold().List())
	require.NotContains(b.Threshold(), "short")

	// One more vote carries "short" across, and nothing else moves.
	b.Add("short")
	require.ElementsMatch([]string{"quorum", "over", "short"}, b.Threshold().List())
}

// TestSetThresholdRecountsWhatIsAlreadyThere covers the retroactive half:
// a threshold set after the votes arrived must describe those votes, and
// raising it must take back choices that no longer qualify.
func TestSetThresholdRecountsWhatIsAlreadyThere(t *testing.T) {
	require := require.New(t)

	b := New[string]()
	b.AddCount("a", 1)
	b.AddCount("b", 2)
	b.AddCount("c", 3)

	b.SetThreshold(2)
	require.ElementsMatch([]string{"b", "c"}, b.Threshold().List())

	b.SetThreshold(3)
	require.ElementsMatch([]string{"c"}, b.Threshold().List())

	b.SetThreshold(1)
	require.ElementsMatch([]string{"a", "b", "c"}, b.Threshold().List())

	b.SetThreshold(4)
	require.Empty(b.Threshold().List())
}

// TestRemoveTakesTheChoiceOutOfTheTally covers all three places a removed
// choice is recorded. Leaving it in the threshold set would let a caller act on
// a quorum for a choice the bag no longer holds a single vote for.
func TestRemoveTakesTheChoiceOutOfTheTally(t *testing.T) {
	require := require.New(t)

	b := New[string]()
	b.SetThreshold(2)
	b.AddCount("keep", 2)
	b.AddCount("drop", 5)
	require.Equal(7, b.Len())
	require.Contains(b.Threshold(), "drop")

	b.Remove("drop")
	require.Equal(2, b.Len())
	require.Zero(b.Count("drop"))
	require.NotContains(b.List(), "drop")
	require.NotContains(b.Threshold(), "drop")
	require.Contains(b.Threshold(), "keep")

	// Removing something that was never there changes nothing.
	b.Remove("absent")
	require.Equal(2, b.Len())
}

func TestEqualsComparesCountsNotJustSize(t *testing.T) {
	require := require.New(t)

	equal := func(a, b Bag[int]) bool { return a.Equals(b) }
	require.True(equal(Of(1, 2, 2), Of(2, 1, 2)))
	require.False(equal(Of(1, 2, 2), Of(1, 1, 2)), "same size, different tally")
	require.False(equal(Of(1, 2), Of(1, 2, 2)))
	require.True(equal(New[int](), New[int]()))
}

// TestCloneIsIndependent pins the deep copy. Consensus keeps a bag per round;
// a clone that shared the counts map would let a later round's votes appear in
// an earlier round's tally.
func TestCloneIsIndependent(t *testing.T) {
	require := require.New(t)

	original := New[string]()
	original.SetThreshold(2)
	original.AddCount("a", 2)

	clone := original.Clone()
	require.True(clone.Equals(original))
	require.Contains(clone.Threshold(), "a")

	clone.AddCount("b", 7)
	clone.Remove("a")

	require.Equal(2, original.Len())
	require.Equal(2, original.Count("a"))
	require.Zero(original.Count("b"))
	require.Contains(original.Threshold(), "a")
}

// TestFilterKeepsCountsAndQuorum covers the sub-tally. The counts survive by
// construction; the quorum has to survive too, because a sub-bag that forgot
// the threshold answers Threshold() against zero, and every choice in it clears
// zero. That is the maximally permissive answer to a question about quorum.
func TestFilterKeepsCountsAndQuorum(t *testing.T) {
	require := require.New(t)

	b := New[int]()
	b.SetThreshold(3)
	b.AddCount(1, 5)
	b.AddCount(2, 1)
	b.AddCount(3, 4)
	b.AddCount(4, 2)
	b.AddCount(5, 1) // odd, and one vote short: the entry that tells the two apart

	odd := b.Filter(func(v int) bool { return v%2 == 1 })
	require.Equal(10, odd.Len())
	require.Equal(5, odd.Count(1))
	require.Equal(4, odd.Count(3))
	require.Equal(1, odd.Count(5))
	require.Zero(odd.Count(2))

	// 5 is in the sub-bag and has not reached three. A sub-bag that dropped the
	// threshold would list it here alongside the two that did.
	require.ElementsMatch([]int{1, 3}, odd.Threshold().List())
	require.NotContains(odd.Threshold(), 5)
}

func TestSplitPartitionsEveryVote(t *testing.T) {
	require := require.New(t)

	b := New[int]()
	b.SetThreshold(3)
	b.AddCount(1, 5)
	b.AddCount(2, 1)
	b.AddCount(3, 4)
	b.AddCount(4, 2)

	parts := b.Split(func(v int) bool { return v%2 == 1 })
	require.Equal(b.Len(), parts[0].Len()+parts[1].Len(), "a split loses no votes")
	require.Equal(3, parts[0].Len())
	require.Equal(9, parts[1].Len())
	require.Equal(1, parts[0].Count(2))
	require.Equal(2, parts[0].Count(4))
	require.Equal(5, parts[1].Count(1))
	require.Equal(4, parts[1].Count(3))

	require.ElementsMatch([]int{1, 3}, parts[1].Threshold().List())
	require.Empty(parts[0].Threshold().List(), "neither even choice reached three")
}

func TestStringReportsSizeAndPrefixesEveryLine(t *testing.T) {
	require := require.New(t)

	b := Of("only", "only")
	require.Contains(b.String(), "Size = 2")
	require.Contains(b.String(), "only: 2")
	require.Contains(b.PrefixedString(">> "), "\n>>     only: 2")
}
