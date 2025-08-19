// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bag

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBagOf(t *testing.T) {
	tests := []struct {
		name           string
		elements       []int
		expectedCounts map[int]int
	}{
		{
			name:           "nil",
			elements:       nil,
			expectedCounts: map[int]int{},
		},
		{
			name:           "empty",
			elements:       []int{},
			expectedCounts: map[int]int{},
		},
		{
			name:     "unique elements",
			elements: []int{1, 2, 3},
			expectedCounts: map[int]int{
				1: 1,
				2: 1,
				3: 1,
			},
		},
		{
			name:     "duplicate elements",
			elements: []int{1, 2, 3, 1, 2, 3},
			expectedCounts: map[int]int{
				1: 2,
				2: 2,
				3: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			b := Of(tt.elements...)

			require.Equal(len(tt.elements), b.Len())
			for entry, count := range tt.expectedCounts {
				require.Equal(count, b.Count(entry))
			}
		})
	}
}

func TestBagAdd(t *testing.T) {
	require := require.New(t)

	elt0 := 0
	elt1 := 1

	bag := Bag[int]{}

	require.Zero(bag.Count(elt0))
	require.Zero(bag.Count(elt1))
	require.Zero(bag.Len())
	require.Empty(bag.List())
	mode, freq := bag.Mode()
	require.Equal(elt0, mode)
	require.Zero(freq)
	require.Empty(bag.Threshold())

	bag.Add(elt0)

	require.Equal(1, bag.Count(elt0))
	require.Zero(bag.Count(elt1))
	require.Equal(1, bag.Len())
	require.Len(bag.List(), 1)
	mode, freq = bag.Mode()
	require.Equal(elt0, mode)
	require.Equal(1, freq)
	require.Len(bag.Threshold(), 1)

	bag.Add(elt0)

	require.Equal(2, bag.Count(elt0))
	require.Zero(bag.Count(elt1))
	require.Equal(2, bag.Len())
	require.Len(bag.List(), 1)
	mode, freq = bag.Mode()
	require.Equal(elt0, mode)
	require.Equal(2, freq)
	require.Len(bag.Threshold(), 1)

	bag.AddCount(elt1, 3)

	require.Equal(2, bag.Count(elt0))
	require.Equal(3, bag.Count(elt1))
	require.Equal(5, bag.Len())
	require.Len(bag.List(), 2)
	mode, freq = bag.Mode()
	require.Equal(elt1, mode)
	require.Equal(3, freq)
	require.Len(bag.Threshold(), 2)
}

func TestBagSetThreshold(t *testing.T) {
	require := require.New(t)

	bag := Bag[int]{}

	bag.Add(1, 1, 1)
	bag.Add(2, 2)
	bag.Add(3)

	// Default threshold is 0
	require.Equal(3, bag.Threshold().Len())

	// Set threshold to 2
	bag.SetThreshold(2)
	threshold := bag.Threshold()
	require.Equal(2, threshold.Len())
	require.True(threshold.Contains(1))
	require.True(threshold.Contains(2))
	require.False(threshold.Contains(3))

	// Set threshold to 3
	bag.SetThreshold(3)
	threshold = bag.Threshold()
	require.Equal(1, threshold.Len())
	require.True(threshold.Contains(1))
	require.False(threshold.Contains(2))
	require.False(threshold.Contains(3))
}

func TestBagRemove(t *testing.T) {
	require := require.New(t)

	bag := Bag[int]{}
	bag.Add(1, 1, 1)
	bag.Add(2, 2)

	require.Equal(5, bag.Len())
	require.Equal(3, bag.Count(1))
	require.Equal(2, bag.Count(2))

	bag.Remove(1)

	require.Equal(2, bag.Len())
	require.Zero(bag.Count(1))
	require.Equal(2, bag.Count(2))

	// Remove non-existent element
	bag.Remove(3)
	require.Equal(2, bag.Len())
}

func TestBagEquals(t *testing.T) {
	require := require.New(t)

	bag1 := Bag[int]{}
	bag2 := Bag[int]{}

	require.True(bag1.Equals(bag2))

	bag1.Add(1, 2, 3)
	require.False(bag1.Equals(bag2))

	bag2.Add(3, 2, 1)
	require.True(bag1.Equals(bag2))

	bag1.Add(1)
	require.False(bag1.Equals(bag2))

	bag2.Add(1)
	require.True(bag1.Equals(bag2))
}

func TestBagFilter(t *testing.T) {
	require := require.New(t)

	bag := Bag[int]{}
	bag.Add(1, 2, 3, 4, 5)
	bag.Add(2, 4) // Add duplicates

	evenFilter := func(x int) bool { return x%2 == 0 }
	filtered := bag.Filter(evenFilter)

	require.Equal(4, filtered.Len()) // 2 count of 2 + 2 count of 4
	require.Zero(filtered.Count(1))
	require.Equal(2, filtered.Count(2))
	require.Zero(filtered.Count(3))
	require.Equal(2, filtered.Count(4))
	require.Zero(filtered.Count(5))
}

func TestBagSplit(t *testing.T) {
	require := require.New(t)

	bag := Bag[int]{}
	bag.Add(1, 2, 3, 4, 5)
	bag.Add(2, 4) // Add duplicates

	evenSplit := func(x int) bool { return x%2 == 0 }
	split := bag.Split(evenSplit)

	oddBag := split[0]
	evenBag := split[1]

	// Check odd bag
	require.Equal(3, oddBag.Len()) // 1 + 1 + 1
	require.Equal(1, oddBag.Count(1))
	require.Zero(oddBag.Count(2))
	require.Equal(1, oddBag.Count(3))
	require.Zero(oddBag.Count(4))
	require.Equal(1, oddBag.Count(5))

	// Check even bag
	require.Equal(4, evenBag.Len()) // 2 + 2
	require.Zero(evenBag.Count(1))
	require.Equal(2, evenBag.Count(2))
	require.Zero(evenBag.Count(3))
	require.Equal(2, evenBag.Count(4))
	require.Zero(evenBag.Count(5))
}

func TestBagClone(t *testing.T) {
	require := require.New(t)

	bag := Bag[int]{}
	bag.Add(1, 2, 3)
	bag.Add(1)

	clone := bag.Clone()

	require.True(bag.Equals(clone))

	// Modify original
	bag.Add(4)
	require.False(bag.Equals(clone))
	require.Equal(5, bag.Len())
	require.Equal(4, clone.Len())
}

func TestBagAddCountZeroOrNegative(t *testing.T) {
	require := require.New(t)

	bag := Bag[int]{}

	// Adding zero count should be no-op
	bag.AddCount(1, 0)
	require.Zero(bag.Count(1))
	require.Zero(bag.Len())

	// Adding negative count should be no-op
	bag.AddCount(1, -5)
	require.Zero(bag.Count(1))
	require.Zero(bag.Len())

	// Add positive count to verify it works
	bag.AddCount(1, 3)
	require.Equal(3, bag.Count(1))
	require.Equal(3, bag.Len())
}

func TestBagString(t *testing.T) {
	require := require.New(t)

	bag := Bag[int]{}
	bag.Add(1, 2)
	bag.Add(1)

	str := bag.String()
	require.Contains(str, "Bag[int]")
	require.Contains(str, "Size = 3")
	require.Contains(str, "1: 2")
	require.Contains(str, "2: 1")
}

func TestBagPrefixedString(t *testing.T) {
	require := require.New(t)

	bag := Bag[int]{}
	bag.Add(1)

	str := bag.PrefixedString("  ")
	require.Contains(str, "Bag[int]")
	require.Contains(str, "Size = 1")
	require.Contains(str, "  1: 1")
}
