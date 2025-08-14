// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

// SamplerType specifies the type of vote sampling
type SamplerType int

// Constants for sampler types
const (
	UniformSampler SamplerType = iota
	StakeSampler
)