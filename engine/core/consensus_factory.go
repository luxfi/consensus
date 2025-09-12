// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"os"
)

// ConsensusFactory creates the appropriate consensus implementation based on build tags
type ConsensusFactory struct {
	useCGO bool
}

// NewConsensusFactory creates a new consensus factory
func NewConsensusFactory() *ConsensusFactory {
	// Check if CGO is enabled via environment variable
	useCGO := os.Getenv("USE_C_CONSENSUS") == "1"
	return &ConsensusFactory{
		useCGO: useCGO,
	}
}

// CreateConsensus creates a consensus engine instance
func (f *ConsensusFactory) CreateConsensus(params Parameters) (Consensus, error) {
	if f.useCGO && cgoAvailable() {
		// Use C implementation when CGO is available and enabled
		cgoParams := ConsensusParams{
			K:                     params.K,
			AlphaPreference:       params.AlphaPreference,
			AlphaConfidence:       params.AlphaConfidence,
			Beta:                  params.Beta,
			ConcurrentPolls:       params.ConcurrentPolls,
			OptimalProcessing:     params.OptimalProcessing,
			MaxOutstandingItems:   params.MaxOutstandingItems,
			MaxItemProcessingTime: params.MaxItemProcessingTime,
		}
		return NewCGOConsensus(cgoParams)
	}
	// Fall back to pure Go implementation
	return NewSnowball(params), nil
}