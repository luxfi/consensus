package config

import (
	"errors"
	"time"
)

// Error variables for parameter validation
var (
	ErrParametersInvalid  = errors.New("invalid consensus parameters")
	ErrInvalidK           = errors.New("k must be >= 1")
	ErrInvalidAlpha       = errors.New("alpha must be between 0.5 and 1.0")
	ErrInvalidBeta        = errors.New("beta must be >= 1")
	ErrBlockTimeTooLow    = errors.New("block time must be >= 1ms")
	ErrRoundTimeoutTooLow = errors.New("round timeout must be >= block time")
)

// Parameters defines consensus parameters
type Parameters struct {
	K                     int
	Alpha                 float64 // For compatibility with Quasar
	Beta                  uint32  // For compatibility with Quasar
	AlphaPreference       int
	AlphaConfidence       int
	BetaVirtuous          int
	BetaRogue             int
	ConcurrentPolls       int
	ConcurrentRepolls     int
	OptimalProcessing     int
	MaxOutstandingItems   int
	MaxItemProcessingTime time.Duration
	Parents               int
	BatchSize             int
	BlockTime             time.Duration // For compatibility
	RoundTO               time.Duration // For compatibility
}

// DefaultParams returns default parameters
func DefaultParams() Parameters {
	return Parameters{
		K:                     20,
		Alpha:                 0.8,
		Beta:                  15,
		AlphaPreference:       15,
		AlphaConfidence:       15,
		BetaVirtuous:          20,
		BetaRogue:             20,
		ConcurrentPolls:       4,
		ConcurrentRepolls:     4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   1024,
		MaxItemProcessingTime: 2 * time.Minute,
		Parents:               5,
		BatchSize:             30,
		BlockTime:             100 * time.Millisecond,
		RoundTO:               250 * time.Millisecond,
	}
}

// MainnetParams returns mainnet parameters
func MainnetParams() Parameters {
	p := DefaultParams()
	p.K = 21
	p.BlockTime = 200 * time.Millisecond
	p.RoundTO = 400 * time.Millisecond
	return p
}

// TestnetParams returns testnet parameters
func TestnetParams() Parameters {
	p := DefaultParams()
	p.K = 11
	p.Alpha = 0.7
	p.Beta = 6
	p.BlockTime = 100 * time.Millisecond
	p.RoundTO = 225 * time.Millisecond
	return p
}

// LocalParams returns local parameters
func LocalParams() Parameters {
	p := DefaultParams()
	p.K = 5
	p.Alpha = 0.6
	p.Beta = 3
	p.BlockTime = 10 * time.Millisecond
	p.RoundTO = 45 * time.Millisecond
	return p
}

// XChainParams returns X-Chain parameters
func XChainParams() Parameters {
	p := DefaultParams()
	p.K = 5
	p.Alpha = 0.6
	p.Beta = 3
	p.BlockTime = 1 * time.Millisecond
	p.RoundTO = 5 * time.Millisecond
	return p
}

// WithBlockTime returns a copy of Parameters with updated block time
func (p Parameters) WithBlockTime(blockTime time.Duration) Parameters {
	p.BlockTime = blockTime
	// Adjust round timeout based on block time
	if blockTime <= time.Millisecond {
		p.RoundTO = 5 * time.Millisecond
	} else if blockTime < 10*time.Millisecond {
		p.RoundTO = 25 * time.Millisecond
	} else if blockTime < 100*time.Millisecond {
		p.RoundTO = 250 * time.Millisecond
	} else {
		p.RoundTO = blockTime * 2
	}
	return p
}

// Validate validates parameters (compatibility method)
func (p Parameters) Validate() error {
	return p.Valid()
}

// Valid validates parameters
func (p Parameters) Valid() error {
	// Check K, Alpha, Beta first - these are always required
	if p.K < 1 {
		return ErrInvalidK
	}
	if p.Alpha < 0.5 || p.Alpha > 1.0 {
		return ErrInvalidAlpha
	}
	if p.Beta < 1 {
		return ErrInvalidBeta
	}
	if p.BlockTime > 0 && p.BlockTime < time.Millisecond {
		return ErrBlockTimeTooLow
	}
	if p.BlockTime > 0 && p.RoundTO > 0 && p.RoundTO < p.BlockTime {
		return ErrRoundTimeoutTooLow
	}

	// Only validate other fields if they are set (non-zero)
	if p.AlphaPreference != 0 && (p.AlphaPreference < 0 || p.AlphaPreference > p.K) {
		return ErrParametersInvalid
	}
	if p.AlphaConfidence != 0 && (p.AlphaConfidence < 0 || p.AlphaConfidence > p.K) {
		return ErrParametersInvalid
	}
	if p.BetaVirtuous < 0 {
		return ErrParametersInvalid
	}
	if p.BetaRogue != 0 && p.BetaRogue < p.BetaVirtuous {
		return ErrParametersInvalid
	}
	if p.ConcurrentPolls != 0 && p.ConcurrentPolls < 1 {
		return ErrParametersInvalid
	}
	if p.OptimalProcessing != 0 && p.OptimalProcessing < 1 {
		return ErrParametersInvalid
	}
	if p.MaxOutstandingItems != 0 && p.MaxOutstandingItems < 1 {
		return ErrParametersInvalid
	}
	if p.MaxItemProcessingTime != 0 && p.MaxItemProcessingTime <= 0 {
		return ErrParametersInvalid
	}

	return nil
}
