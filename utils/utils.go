package utils

import (
	"sort"
)

// Sort sorts a slice
func Sort[T any](slice []T, less func(i, j int) bool) {
	sort.Slice(slice, less)
}

// SortByID sorts by ID
func SortByID[T interface{ ID() string }](slice []T) {
	sort.Slice(slice, func(i, j int) bool {
		return slice[i].ID() < slice[j].ID()
	})
}

// Contains checks if slice contains element
func Contains[T comparable](slice []T, elem T) bool {
	for _, v := range slice {
		if v == elem {
			return true
		}
	}
	return false
}

// Filter filters a slice
func Filter[T any](slice []T, predicate func(T) bool) []T {
	result := make([]T, 0, len(slice))
	for _, v := range slice {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}
