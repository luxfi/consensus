// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

// FPCConfig configures Fast Path Consensus
type FPCConfig struct {
	Enable              bool
	VoteLimitPerBlock   int
	ExecuteOwned        bool
	ExecuteMixedOnFinal bool
	EpochFence          bool
	VotePrefix          []byte
}

// DefaultFPC returns default FPC configuration
func DefaultFPC() FPCConfig {
	return FPCConfig{
		Enable:              true,
		VoteLimitPerBlock:   256,
		ExecuteOwned:        true,
		ExecuteMixedOnFinal: true,
		EpochFence:          true,
		VotePrefix:          []byte("LUX/WAVEFPC/V1"),
	}
}
