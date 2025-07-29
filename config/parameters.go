// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"time"
)

var (
	ErrBetaTooLow                = errors.New("beta value must be greater than 0")
	ErrBetaTooHigh               = errors.New("beta value must be less than k")
	ErrAlphaPreferenceTooLow     = errors.New("alphaPreference value must be greater than k/2")
	ErrAlphaPreferenceTooHigh    = errors.New("alphaPreference value must be at most k")
	ErrAlphaConfidenceTooLow     = errors.New("alphaConfidence value must be greater than k/2")
	ErrAlphaConfidenceTooHigh    = errors.New("alphaConfidence value must be at most k")
	ErrAlphaConfidenceTooSmall   = errors.New("alphaConfidence value must be greater than or equal to alphaPreference")
	ErrKTooLow                   = errors.New("k value must be greater than 0")
	ErrConcurrentRepollsTooLow   = errors.New("concurrentRepolls value must be greater than 0")
	ErrOptimalProcessingTooLow   = errors.New("optimalProcessing value must be greater than 0")
	ErrMaxOutstandingItemsTooLow = errors.New("maxOutstandingItems value must be greater than 0")
	ErrMaxItemProcessingTimeTooLow = errors.New("maxItemProcessingTime must be greater than 0")
	ErrMinRoundIntervalTooLow    = errors.New("minRoundInterval must be greater than 0")
	ErrQThresholdTooLow          = errors.New("qThreshold value must be greater than 0")
	ErrQThresholdTooHigh         = errors.New("qThreshold value must be at most k")
	ErrQuasarTimeoutTooLow       = errors.New("quasarTimeout must be greater than 0")
)

// Parameters contains all configurable values for the Lux consensus engine.
type Parameters struct {
	// Threshold Parameters
	// K is the number of nodes to sample.
	K int
	// AlphaPreference is the vote threshold to change preference.
	AlphaPreference int
	// AlphaConfidence is the vote threshold to increase confidence.
	AlphaConfidence int
	// Beta is the confidence threshold for finalization.
	Beta int

	// Virtuous Parameters
	// ConcurrentRepolls is the maximum number of concurrent re-polls.
	ConcurrentRepolls int
	// OptimalProcessing is the target number of items to process.
	OptimalProcessing int
	// MaxOutstandingItems is the maximum number of outstanding items.
	MaxOutstandingItems int
	// MaxItemProcessingTime is the maximum time to process an item.
	MaxItemProcessingTime time.Duration

	// Timing Parameters
	// MinRoundInterval is the minimum time between consensus rounds.
	// This parameter controls the rate at which rounds are performed
	// and helps prevent excessive network congestion.
	MinRoundInterval time.Duration

	// Performance Parameters
	// BatchSize is the number of items to process in a single batch.
	// This is used for high-throughput scenarios.
	BatchSize int

	// Quantum Consensus Parameters
	// Q is the quantum finality interval - how many seconds between
	// post-quantum Ringtail certificates. This provides quantum-resistant
	// finality on top of the fast metastable BLS consensus.
	Q time.Duration
	
	// QThreshold is the Ringtail threshold (post-quantum) - number of shares
	// required for a valid quantum certificate.
	QThreshold int
	
	// QuasarTimeout is the maximum time allowed for Ringtail certificate
	// aggregation. If the proposer cannot gather the threshold signature
	// within this timeout, the block is invalid and the proposer is penalized.
	QuasarTimeout time.Duration
}

// Valid returns nil if the parameters describe a valid consensus configuration.
func (p Parameters) Valid() error {
	switch {
	case p.K <= 0:
		return ErrKTooLow
	case p.AlphaPreference <= p.K/2:
		return ErrAlphaPreferenceTooLow
	case p.AlphaPreference > p.K:
		return ErrAlphaPreferenceTooHigh
	case p.AlphaConfidence <= p.K/2:
		return ErrAlphaConfidenceTooLow
	case p.AlphaConfidence > p.K:
		return ErrAlphaConfidenceTooHigh
	case p.AlphaConfidence < p.AlphaPreference:
		return ErrAlphaConfidenceTooSmall
	case p.Beta <= 0:
		return ErrBetaTooLow
	case p.Beta > p.K:
		return ErrBetaTooHigh
	case p.ConcurrentRepolls <= 0:
		return ErrConcurrentRepollsTooLow
	case p.OptimalProcessing <= 0:
		return ErrOptimalProcessingTooLow
	case p.MaxOutstandingItems <= 0:
		return ErrMaxOutstandingItemsTooLow
	case p.MaxItemProcessingTime <= 0:
		return ErrMaxItemProcessingTimeTooLow
	case p.MinRoundInterval <= 0:
		return ErrMinRoundIntervalTooLow
	case p.QuasarTimeout > 0 && p.QThreshold <= 0:
		return ErrQThresholdTooLow
	case p.QThreshold < 0:
		return ErrQThresholdTooLow
	case p.QThreshold > p.K:
		return ErrQThresholdTooHigh
	case p.QuasarTimeout < 0:
		return ErrQuasarTimeoutTooLow
	default:
		return nil
	}
}

// Default Configurations

// DefaultParameters defines the default consensus parameters.
// This is set to MainnetParameters for production networks.
var DefaultParameters = MainnetParameters

// MainnetParameters defines consensus parameters for the mainnet (21 validators).
// Optimized for production deployment with higher fault tolerance.
var MainnetParameters = Parameters{
	K:                     21,
	AlphaPreference:       13,                          // tolerate up to 8 failures for liveness
	AlphaConfidence:       18,                          // tolerate up to 3 failures for finality
	Beta:                  8,                           // 8×50 ms + 100 ms = 500 ms finality
	ConcurrentRepolls:     8,                           // pipeline 8 rounds
	OptimalProcessing:     10,
	MaxOutstandingItems:   369,
	MaxItemProcessingTime: 10 * time.Second,           // ~10 s health timeout
	MinRoundInterval:      200 * time.Millisecond,
	Q:                     10 * time.Minute,            // quantum finality every 10 minutes
	QThreshold:            15,                          // quantum threshold for mainnet
	QuasarTimeout:         50 * time.Millisecond,      // quasar timeout
}

// TestnetParameters defines consensus parameters for the testnet (11 validators).
// Balanced between performance and fault tolerance for testing.
var TestnetParameters = Parameters{
	K:                     11,
	AlphaPreference:       8,                           // tolerate up to 3 failures
	AlphaConfidence:       9,                           // tolerate up to 2 failures
	Beta:                  10,                          // 10×50 ms + 100 ms = 600 ms finality
	ConcurrentRepolls:     10,                          // pipeline 10 rounds
	OptimalProcessing:     10,
	MaxOutstandingItems:   256,
	MaxItemProcessingTime: 6900 * time.Millisecond,    // 6.9 s health timeout
	MinRoundInterval:      100 * time.Millisecond,
	Q:                     5 * time.Minute,             // faster quantum finality for testing
	QThreshold:            8,                           // quantum threshold for testnet
	QuasarTimeout:         100 * time.Millisecond,     // quasar timeout
}

// LocalParameters defines consensus parameters for local networks (5 validators).
// Optimized for 10 Gbps network with minimal latency.
var LocalParameters = Parameters{
	K:                     5,
	AlphaPreference:       4,                           // tolerate up to 1 failure
	AlphaConfidence:       4,                           // tolerate up to 1 failure
	Beta:                  3,                           // 3×10 ms + 20 ms ≈ 50 ms finality
	ConcurrentRepolls:     3,                           // pipeline 3 rounds (limited by beta)
	OptimalProcessing:     32,                          // process 32 items in parallel
	MaxOutstandingItems:   256,
	MaxItemProcessingTime: 3690 * time.Millisecond,    // 3.69 s health timeout
	MinRoundInterval:      10 * time.Millisecond,
	Q:                     30 * time.Second,            // rapid quantum finality for testing
	QThreshold:            3,                           // quantum threshold for local
	QuasarTimeout:         20 * time.Millisecond,      // quasar timeout
}

// HighTPSParams defines parameters optimized for maximum throughput.
// Designed for high-performance benchmarking on 10 Gb networks.
var HighTPSParams = Parameters{
	K:                     5,
	AlphaPreference:       4,
	AlphaConfidence:       4,
	Beta:                  4,                           // 20ms finality
	ConcurrentRepolls:     1,                           // keep wire under 10 Gb
	OptimalProcessing:     10,                          // saturate verifier cores
	MaxOutstandingItems:   8192,
	MaxItemProcessingTime: 3 * time.Second,
	MinRoundInterval:      5 * time.Millisecond,
	BatchSize:             4096,                        // maximize throughput
	Q:                     2 * time.Minute,             // quantum cert every 2 min at high TPS
	QThreshold:            3,                           // quantum threshold for high TPS
	QuasarTimeout:         10 * time.Millisecond,      // quasar timeout
}

// GetParametersByName returns the appropriate parameters for the given network name.
func GetParametersByName(network string) (Parameters, error) {
	switch network {
	case "mainnet":
		return MainnetParameters, nil
	case "testnet":
		return TestnetParameters, nil
	case "local", "localnet":
		return LocalParameters, nil
	case "hightps":
		return HighTPSParams, nil
	default:
		return Parameters{}, errors.New("unknown network: " + network)
	}
}

// MinPercentConnectedBuffer is the safety buffer for the minimum percent of
// stake that should be connected.
const MinPercentConnectedBuffer = .2

// OptimalPercentStake returns the percentage of stake that should be
// connected for optimal performance and finality guarantees.
func (p Parameters) OptimalPercentStake() float64 {
	// To finalize blocks, we need alphaConfidence votes. Assuming a uniform
	// sampling of stake, this creates an optimal percent stake of
	// alphaConfidence/k. To avoid potential deadlocks with Byzantine nodes, we
	// add a buffer to ensure we have enough honest stake to make progress.
	alphaRatio := float64(p.AlphaConfidence) / float64(p.K)
	return alphaRatio*(1-MinPercentConnectedBuffer) + MinPercentConnectedBuffer
}

// MinPercentConnectedHealthy returns the minimum percentage of connected stake
// for the node to be considered healthy. This is an alias for OptimalPercentStake.
func (p Parameters) MinPercentConnectedHealthy() float64 {
	return p.OptimalPercentStake()
}