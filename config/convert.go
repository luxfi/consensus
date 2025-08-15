// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

// ToParameters converts a Config to Parameters
func (c Config) ToParameters() Parameters {
	return Parameters{
		K:                     c.K,
		AlphaPreference:       c.AlphaPreference,
		AlphaConfidence:       c.AlphaConfidence,
		Beta:                  c.Beta,
		ConcurrentPolls:       c.ConcurrentPolls,
		OptimalProcessing:     c.OptimalProcessing,
		MaxOutstandingItems:   c.MaxOutstandingItems,
		MaxItemProcessingTime: c.MaxItemProcessingTime,
		MinRoundInterval:      c.MinRoundInterval,
	}
}
