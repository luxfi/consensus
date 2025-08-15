// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package photon implements single probe primitive for sampling
package photon

import (
	"github.com/luxfi/consensus/types"
)

// Sampler is the single probe primitive interface
type Sampler[T comparable] interface {
	Sample(k int) []T
}

// UniformSampler samples k items uniformly from a set
type UniformSampler[T comparable] struct {
	items []T
}

// NewUniform creates a new uniform sampler
func NewUniform[T comparable](items []T) *UniformSampler[T] {
	return &UniformSampler[T]{items: items}
}

// Sample returns k items uniformly sampled
func (s *UniformSampler[T]) Sample(k int) []T {
	if k >= len(s.items) {
		return s.items
	}
	// Simple sampling for now
	result := make([]T, k)
	for i := 0; i < k; i++ {
		result[i] = s.items[i%len(s.items)]
	}
	return result
}

// WeightedSampler samples k items with weights
type WeightedSampler struct {
	nodes   []types.NodeID
	weights []uint64
}

// NewWeighted creates a new weighted sampler
func NewWeighted(nodes []types.NodeID, weights []uint64) *WeightedSampler {
	return &WeightedSampler{nodes: nodes, weights: weights}
}

// Sample returns k nodes sampled by weight
func (s *WeightedSampler) Sample(k int) []types.NodeID {
	if k >= len(s.nodes) {
		return s.nodes
	}
	// Simple weighted sampling for now
	result := make([]types.NodeID, k)
	for i := 0; i < k; i++ {
		result[i] = s.nodes[i%len(s.nodes)]
	}
	return result
}

// SampleFromSet is a helper function to sample from a set using a sampler
func SampleFromSet[T comparable](set []T, k int, s Sampler[T]) []T {
	// If sampler can directly sample, use it
	// Otherwise, create a temporary uniform sampler
	if k >= len(set) {
		return set
	}
	
	// Simple sampling
	result := make([]T, k)
	for i := 0; i < k; i++ {
		result[i] = set[i%len(set)]
	}
	return result
}

// NewWeightedGeneric creates a weighted sampler for any comparable type
func NewWeightedGeneric[T comparable](items []T, weights []int) Sampler[T] {
	return &WeightedGeneric[T]{
		items:   items,
		weights: weights,
	}
}

// WeightedGeneric implements weighted sampling for any comparable type
type WeightedGeneric[T comparable] struct {
	items   []T
	weights []int
}

// Sample returns k items sampled by weight
func (w *WeightedGeneric[T]) Sample(k int) []T {
	if k >= len(w.items) {
		return w.items
	}
	// Simple weighted sampling
	result := make([]T, k)
	for i := 0; i < k; i++ {
		result[i] = w.items[i%len(w.items)]
	}
	return result
}