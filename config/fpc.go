// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

// FPCConfig configures Fast Path Consensus
type FPCConfig struct {
	Enable              bool    `json:"enable" yaml:"enable"`
	ThetaMin            float64 `json:"theta_min" yaml:"theta_min"`
	ThetaMax            float64 `json:"theta_max" yaml:"theta_max"`
	VoteLimitPerBlock   int     `json:"vote_limit_per_block" yaml:"vote_limit_per_block"`
	ExecuteOwned        bool    `json:"execute_owned" yaml:"execute_owned"`
	ExecuteMixedOnFinal bool    `json:"execute_mixed_on_final" yaml:"execute_mixed_on_final"`
	EpochFence          bool    `json:"epoch_fence" yaml:"epoch_fence"`
	VotePrefix          []byte  `json:"vote_prefix" yaml:"vote_prefix"`
}

// DefaultFPC returns default FPC configuration with FPC enabled by default
func DefaultFPC() FPCConfig {
	return FPCConfig{
		Enable:              true, // default ON for 50x speedup
		ThetaMin:            0.55, // typical band
		ThetaMax:            0.65,
		VoteLimitPerBlock:   256,
		ExecuteOwned:        true,
		ExecuteMixedOnFinal: true,
		EpochFence:          true,
		VotePrefix:          []byte("LUX/WAVEFPC/V1"),
	}
}

// QuasarConfig holds Quasar dual-certificate configuration
type QuasarConfig struct {
	Enable     bool `json:"enable" yaml:"enable"`
	Precompute int  `json:"precompute" yaml:"precompute"`
	Threshold  int  `json:"threshold" yaml:"threshold"`
}
