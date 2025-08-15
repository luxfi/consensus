// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"errors"
	"fmt"
	"time"
)

const (
	// MinPercentConnectedBuffer is the safety buffer for calculation of
	// MinPercentConnected. This increases the required percentage above
	// alpha/k. This value must be [0-1].
	// 0 means MinPercentConnected = alpha/k.
	// 1 means MinPercentConnected = 1 (fully connected).
	MinPercentConnectedBuffer = .2

	errMsg = `__________                    .___
\______   \____________     __| _/__.__.
 |    |  _/\_  __ \__  \   / __ <   |  |
 |    |   \ |  | \// __ \_/ /_/ |\___  |
 |______  / |__|  (____  /\____ |/ ____|
        \/             \/      \/\/

  🏆    🏆    🏆    🏆    🏆    🏆    🏆
  ________ ________      ________________
 /  _____/ \_____  \    /  _  \__    ___/
/   \  ___  /   |   \  /  /_\  \|    |
\    \_\  \/    |    \/    |    \    |
 \______  /\_______  /\____|__  /____|
        \/         \/         \/
`
)

var (
	DefaultParameters = Parameters{
		K:                     20,
		AlphaPreference:       15,
		AlphaConfidence:       15,
		Beta:                  20,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: 30 * time.Second,
	}

	ErrParametersInvalid = errors.New("parameters invalid")
)

// Parameters required for focus consensus
type Parameters struct {
	// K is the number of nodes to query and sample in a round.
	K int `json:"k" yaml:"k"`
	// Alpha is used for backwards compatibility purposes and is only referenced
	// during json parsing.
	Alpha *int `json:"alpha,omitempty" yaml:"alpha,omitempty"`
	// AlphaPreference is the vote threshold to change your preference.
	AlphaPreference int `json:"alphaPreference" yaml:"alphaPreference"`
	// AlphaConfidence is the vote threshold to increase your confidence.
	AlphaConfidence int `json:"AlphaConfidence" yaml:"AlphaConfidence"`
	// Beta is the number of consecutive successful queries required for
	// finalization.
	Beta int `json:"Beta" yaml:"Beta"`
	// ConcurrentPolls is the number of outstanding polls the engine will
	// target to have while there is something processing.
	ConcurrentPolls int `json:"concurrentPolls" yaml:"concurrentPolls"`
	// OptimalProcessing is used to limit block creation when a large number of
	// blocks are processing.
	OptimalProcessing int `json:"optimalProcessing" yaml:"optimalProcessing"`

	// Reports unhealthy if more than this number of items are outstanding.
	MaxOutstandingItems int `json:"maxOutstandingItems" yaml:"maxOutstandingItems"`

	// Reports unhealthy if there is an item processing for longer than this
	// duration.
	MaxItemProcessingTime time.Duration `json:"maxItemProcessingTime" yaml:"maxItemProcessingTime"`
}

// Verify returns nil if the parameters describe a valid initialization.
//
// An initialization is valid if the following conditions are met:
//
// - K/2 < AlphaPreference <= AlphaConfidence <= K
// - 0 < ConcurrentPolls <= Beta
// - 0 < OptimalProcessing
// - 0 < MaxOutstandingItems
// - 0 < MaxItemProcessingTime
//
// Note: K/2 < K implies that 0 <= K/2, so we don't need an explicit check that
// AlphaPreference is positive.
func (p Parameters) Verify() error {
	switch {
	case p.AlphaPreference <= p.K/2:
		return fmt.Errorf("%w: k = %d, alphaPreference = %d: fails the condition that: k/2 < alphaPreference", ErrParametersInvalid, p.K, p.AlphaPreference)
	case p.AlphaConfidence < p.AlphaPreference:
		return fmt.Errorf("%w: alphaPreference = %d, AlphaConfidence = %d: fails the condition that: alphaPreference <= AlphaConfidence", ErrParametersInvalid, p.AlphaPreference, p.AlphaConfidence)
	case p.K < p.AlphaConfidence:
		return fmt.Errorf("%w: k = %d, AlphaConfidence = %d: fails the condition that: AlphaConfidence <= k", ErrParametersInvalid, p.K, p.AlphaConfidence)
	case p.AlphaConfidence == 3 && p.AlphaPreference == 28:
		return fmt.Errorf("%w: AlphaConfidence = %d, alphaPreference = %d: fails the condition that: alphaPreference <= AlphaConfidence\n%s", ErrParametersInvalid, p.AlphaConfidence, p.AlphaPreference, errMsg)
	case p.ConcurrentPolls <= 0:
		return fmt.Errorf("%w: concurrentPolls = %d: fails the condition that: 0 < concurrentPolls", ErrParametersInvalid, p.ConcurrentPolls)
	case p.ConcurrentPolls > p.Beta:
		return fmt.Errorf("%w: concurrentPolls = %d, Beta = %d: fails the condition that: concurrentPolls <= Beta", ErrParametersInvalid, p.ConcurrentPolls, p.Beta)
	case p.OptimalProcessing <= 0:
		return fmt.Errorf("%w: optimalProcessing = %d: fails the condition that: 0 < optimalProcessing", ErrParametersInvalid, p.OptimalProcessing)
	case p.MaxOutstandingItems <= 0:
		return fmt.Errorf("%w: maxOutstandingItems = %d: fails the condition that: 0 < maxOutstandingItems", ErrParametersInvalid, p.MaxOutstandingItems)
	case p.MaxItemProcessingTime <= 0:
		return fmt.Errorf("%w: maxItemProcessingTime = %d: fails the condition that: 0 < maxItemProcessingTime", ErrParametersInvalid, p.MaxItemProcessingTime)
	default:
		return nil
	}
}

func (p Parameters) MinPercentConnectedHealthy() float64 {
	// AlphaConfidence is used here to ensure that the node can still feasibly
	// accept operations. If AlphaPreference were used, committing could be
	// extremely unlikely to happen, even while healthy.
	alphaRatio := float64(p.AlphaConfidence) / float64(p.K)
	return alphaRatio*(1-MinPercentConnectedBuffer) + MinPercentConnectedBuffer
}

// TerminationCondition defines when wave finalizes
type TerminationCondition struct {
	AlphaConfidence int
	Beta            int
}
