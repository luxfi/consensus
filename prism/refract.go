// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

// Refract slices vertices by a function
func Refract[V comparable](vertices []V, slice func(V) int) [][]V {
	buckets := make(map[int][]V)

	for _, v := range vertices {
		bucket := slice(v)
		buckets[bucket] = append(buckets[bucket], v)
	}

	// Convert map to slice
	result := make([][]V, 0, len(buckets))
	for _, bucket := range buckets {
		result = append(result, bucket)
	}

	return result
}
