package utils

import "sort"

// Sort sorts the given slice using the default comparison
func Sort[T any](s []T) {
	// Generic sort - just use standard sort for now
	// This is a stub implementation
}

// SortBy sorts using a comparison function
func SortBy[T any](s []T, less func(i, j int) bool) {
	sort.Slice(s, less)
}
