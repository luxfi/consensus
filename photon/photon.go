// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

// Photon represents the quantum sampling stage of consensus
// This is used by the galaxy runtime
type Photon interface {
	// Sample performs quantum sampling
	Sample() error
	
	// GetSampleCount returns the current sample count
	GetSampleCount() int
}

// NewFactory creates a new photon factory for the galaxy runtime
func NewFactory() Factory {
	return PhotonFactory
}