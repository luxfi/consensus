// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package set

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOf(t *testing.T) {
	require := require.New(t)

	// Empty set
	s1 := Of[int]()
	require.Equal(0, s1.Len())

	// Set with elements
	s2 := Of(1, 2, 3)
	require.Equal(3, s2.Len())
	require.True(s2.Contains(1))
	require.True(s2.Contains(2))
	require.True(s2.Contains(3))

	// Set with duplicates
	s3 := Of(1, 2, 2, 3, 3, 3)
	require.Equal(3, s3.Len())
}

func TestAdd(t *testing.T) {
	require := require.New(t)

	s := make(Set[string])
	require.Equal(0, s.Len())

	// Add single element
	s.Add("a")
	require.Equal(1, s.Len())
	require.True(s.Contains("a"))

	// Add multiple elements
	s.Add("b", "c")
	require.Equal(3, s.Len())
	require.True(s.Contains("b"))
	require.True(s.Contains("c"))

	// Add duplicate
	s.Add("a")
	require.Equal(3, s.Len())
}

func TestContains(t *testing.T) {
	require := require.New(t)

	s := Of("a", "b", "c")
	require.True(s.Contains("a"))
	require.True(s.Contains("b"))
	require.True(s.Contains("c"))
	require.False(s.Contains("d"))
}

func TestRemove(t *testing.T) {
	require := require.New(t)

	s := Of(1, 2, 3, 4, 5)
	
	// Remove single element
	s.Remove(3)
	require.Equal(4, s.Len())
	require.False(s.Contains(3))

	// Remove multiple elements
	s.Remove(1, 5)
	require.Equal(2, s.Len())
	require.False(s.Contains(1))
	require.False(s.Contains(5))
	require.True(s.Contains(2))
	require.True(s.Contains(4))

	// Remove non-existent element
	s.Remove(10)
	require.Equal(2, s.Len())
}

func TestClear(t *testing.T) {
	require := require.New(t)

	s := Of(1, 2, 3)
	require.Equal(3, s.Len())

	s.Clear()
	require.Equal(0, s.Len())
	require.False(s.Contains(1))
	require.False(s.Contains(2))
	require.False(s.Contains(3))
}

func TestList(t *testing.T) {
	require := require.New(t)

	s := Of(1, 2, 3)
	list := s.List()
	require.Len(list, 3)
	
	// Convert to set to check elements (order is non-deterministic)
	listSet := Of(list...)
	require.True(listSet.Equals(s))
}

func TestEquals(t *testing.T) {
	require := require.New(t)

	s1 := Of(1, 2, 3)
	s2 := Of(1, 2, 3)
	s3 := Of(1, 2)
	s4 := Of(1, 2, 3, 4)
	s5 := Of[int]()
	s6 := Of[int]()

	require.True(s1.Equals(s2))
	require.True(s2.Equals(s1))
	require.False(s1.Equals(s3))
	require.False(s1.Equals(s4))
	require.True(s5.Equals(s6))
}

func TestUnion(t *testing.T) {
	require := require.New(t)

	s1 := Of(1, 2, 3)
	s2 := Of(3, 4, 5)
	
	union := s1.Union(s2)
	require.Equal(5, union.Len())
	for i := 1; i <= 5; i++ {
		require.True(union.Contains(i))
	}

	// Union with empty set
	s3 := Of[int]()
	union2 := s1.Union(s3)
	require.True(union2.Equals(s1))

	// Union of empty sets
	union3 := s3.Union(s3)
	require.Equal(0, union3.Len())
}

func TestIntersection(t *testing.T) {
	require := require.New(t)

	s1 := Of(1, 2, 3, 4)
	s2 := Of(3, 4, 5, 6)
	
	intersection := s1.Intersection(s2)
	require.Equal(2, intersection.Len())
	require.True(intersection.Contains(3))
	require.True(intersection.Contains(4))

	// Intersection with no common elements
	s3 := Of(7, 8, 9)
	intersection2 := s1.Intersection(s3)
	require.Equal(0, intersection2.Len())

	// Intersection with empty set
	s4 := Of[int]()
	intersection3 := s1.Intersection(s4)
	require.Equal(0, intersection3.Len())

	// Test efficiency (smaller set iteration)
	smallSet := Of(1)
	largeSet := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	intersection4 := smallSet.Intersection(largeSet)
	require.Equal(1, intersection4.Len())
	require.True(intersection4.Contains(1))

	intersection5 := largeSet.Intersection(smallSet)
	require.Equal(1, intersection5.Len())
	require.True(intersection5.Contains(1))
}

func TestDifference(t *testing.T) {
	require := require.New(t)

	s1 := Of(1, 2, 3, 4)
	s2 := Of(3, 4, 5, 6)
	
	diff := s1.Difference(s2)
	require.Equal(2, diff.Len())
	require.True(diff.Contains(1))
	require.True(diff.Contains(2))
	require.False(diff.Contains(3))
	require.False(diff.Contains(4))

	// Difference with empty set
	s3 := Of[int]()
	diff2 := s1.Difference(s3)
	require.True(diff2.Equals(s1))

	// Difference of empty set
	diff3 := s3.Difference(s1)
	require.Equal(0, diff3.Len())
}

func TestOverlaps(t *testing.T) {
	require := require.New(t)

	s1 := Of(1, 2, 3)
	s2 := Of(3, 4, 5)
	s3 := Of(4, 5, 6)
	s4 := Of[int]()

	require.True(s1.Overlaps(s2))
	require.True(s2.Overlaps(s1))
	require.False(s1.Overlaps(s3))
	require.False(s1.Overlaps(s4))
	require.False(s4.Overlaps(s1))

	// Test efficiency (smaller set iteration)
	smallSet := Of(1)
	largeSet := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	require.True(smallSet.Overlaps(largeSet))
	require.True(largeSet.Overlaps(smallSet))

	smallSet2 := Of(11)
	require.False(smallSet2.Overlaps(largeSet))
	require.False(largeSet.Overlaps(smallSet2))
}

func TestMarshalJSON(t *testing.T) {
	require := require.New(t)

	s := Of(1, 2, 3)
	data, err := json.Marshal(s)
	require.NoError(err)

	// Unmarshal to verify
	var list []int
	err = json.Unmarshal(data, &list)
	require.NoError(err)
	require.Len(list, 3)

	// Convert back to set to verify elements
	s2 := Of(list...)
	require.True(s.Equals(s2))

	// Empty set
	s3 := Of[string]()
	data2, err := json.Marshal(s3)
	require.NoError(err)
	require.Equal("[]", string(data2))
}

func TestUnmarshalJSON(t *testing.T) {
	require := require.New(t)

	// Normal case
	data := []byte(`[1, 2, 3]`)
	var s Set[int]
	err := json.Unmarshal(data, &s)
	require.NoError(err)
	require.Equal(3, s.Len())
	require.True(s.Contains(1))
	require.True(s.Contains(2))
	require.True(s.Contains(3))

	// Empty array
	data2 := []byte(`[]`)
	var s2 Set[string]
	err = json.Unmarshal(data2, &s2)
	require.NoError(err)
	require.Equal(0, s2.Len())

	// Invalid JSON
	data3 := []byte(`not json`)
	var s3 Set[int]
	err = json.Unmarshal(data3, &s3)
	require.Error(err)
}

func TestString(t *testing.T) {
	require := require.New(t)

	// Empty set
	s1 := Of[int]()
	require.Equal("{}", s1.String())

	// Single element
	s2 := Of(42)
	require.Equal("{42}", s2.String())

	// Multiple elements (order non-deterministic)
	s3 := Of("a", "b")
	str := s3.String()
	require.True(str == "{a, b}" || str == "{b, a}")
}

func TestClone(t *testing.T) {
	require := require.New(t)

	s1 := Of(1, 2, 3)
	s2 := s1.Clone()

	// Should be equal
	require.True(s1.Equals(s2))

	// But independent
	s2.Add(4)
	require.False(s1.Equals(s2))
	require.Equal(3, s1.Len())
	require.Equal(4, s2.Len())

	// Clone empty set
	s3 := Of[string]()
	s4 := s3.Clone()
	require.Equal(0, s4.Len())
}