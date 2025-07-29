// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

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

	elt0 := 0
	elt1 := 1

	bag := Bag[int]{}
	bag.Add(elt0, elt0, elt1)

	bag.SetThreshold(1)

	require.Len(bag.Threshold(), 2)

	bag.SetThreshold(2)

	require.Len(bag.Threshold(), 1)

	bag.SetThreshold(3)

	require.Empty(bag.Threshold())
}

func TestBagCount(t *testing.T) {
	require := require.New(t)

	elt0 := 0
	elt1 := 1

	bag := Bag[int]{}

	require.Zero(bag.Count(elt0))
	require.Zero(bag.Count(elt1))

	bag.Add(elt0)

	require.Equal(1, bag.Count(elt0))
	require.Zero(bag.Count(elt1))

	bag.Add(elt0)

	require.Equal(2, bag.Count(elt0))
	require.Zero(bag.Count(elt1))

	bag.AddCount(elt1, 3)

	require.Equal(2, bag.Count(elt0))
	require.Equal(3, bag.Count(elt1))
}

func TestBagEquals(t *testing.T) {
	require := require.New(t)

	elt0 := 0
	elt1 := 1

	bag0 := Bag[int]{}
	bag1 := Bag[int]{}

	require.True(bag0.Equals(bag1))
	require.True(bag1.Equals(bag0))

	bag0.Add(elt0)

	require.False(bag0.Equals(bag1))
	require.False(bag1.Equals(bag0))

	bag1.Add(elt0)

	require.True(bag0.Equals(bag1))
	require.True(bag1.Equals(bag0))

	bag0.Add(elt0)

	require.False(bag0.Equals(bag1))
	require.False(bag1.Equals(bag0))

	bag1.Add(elt0)

	require.True(bag0.Equals(bag1))
	require.True(bag1.Equals(bag0))

	bag0.Add(elt1)

	require.False(bag0.Equals(bag1))
	require.False(bag1.Equals(bag0))

	bag1.Add(elt1)

	require.True(bag0.Equals(bag1))
	require.True(bag1.Equals(bag0))
}

func TestBagFilter(t *testing.T) {
	require := require.New(t)

	bag := Of(0, 1, 2, 3, 4, 5)
	evens := bag.Filter(func(i int) bool { return i%2 == 0 })

	require.Equal(3, evens.Len())
	require.Equal(1, evens.Count(0))
	require.Equal(1, evens.Count(2))
	require.Equal(1, evens.Count(4))
}

func TestBagSplit(t *testing.T) {
	require := require.New(t)

	bag := Of(0, 1, 2, 3, 4, 5)
	bags := bag.Split(func(i int) bool { return i%2 == 1 })
	evens := bags[0]
	odds := bags[1]

	require.Equal(3, evens.Len())
	require.Equal(1, evens.Count(0))
	require.Equal(1, evens.Count(2))
	require.Equal(1, evens.Count(4))

	require.Equal(3, odds.Len())
	require.Equal(1, odds.Count(1))
	require.Equal(1, odds.Count(3))
	require.Equal(1, odds.Count(5))
}

func TestBagRemove(t *testing.T) {
	require := require.New(t)

	bag := Of(0, 0, 1)

	require.Equal(3, bag.Len())
	require.Equal(2, bag.Count(0))
	require.Equal(1, bag.Count(1))

	bag.Remove(0)

	require.Equal(1, bag.Len())
	require.Zero(bag.Count(0))
	require.Equal(1, bag.Count(1))

	bag.Remove(1)

	require.Zero(bag.Len())
	require.Zero(bag.Count(0))
	require.Zero(bag.Count(1))
}

func TestBagClone(t *testing.T) {
	require := require.New(t)

	bag := Of(0, 0, 1)
	clone := bag.Clone()

	require.True(bag.Equals(clone))

	clone.Add(2)

	require.False(bag.Equals(clone))
	require.Equal(3, bag.Len())
	require.Equal(4, clone.Len())
}