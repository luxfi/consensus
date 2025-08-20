package config

import "time"

// Parameters configures consensus behavior
type Parameters struct {
	K         int           // Sample size for voting
	Alpha     float64       // Preference threshold (0.5-1.0)
	Beta      uint32        // Confidence threshold
	RoundTO   time.Duration // Round timeout
	BlockTime time.Duration // Target block time (can be as low as 1ms on 100Gbps networks)
}

// DefaultParams returns standard parameters for typical networks
func DefaultParams() Parameters {
	return Parameters{
		K:         20,
		Alpha:     0.8,
		Beta:      15,
		RoundTO:   250 * time.Millisecond,
		BlockTime: 100 * time.Millisecond,
	}
}

// XChainParams returns ultra-fast parameters for X-Chain on 100Gbps networks
func XChainParams() Parameters {
	return Parameters{
		K:         5,   // Small sample for ultra-low latency
		Alpha:     0.6, // Lower threshold for speed
		Beta:      3,   // Quick confidence
		RoundTO:   5 * time.Millisecond,
		BlockTime: 1 * time.Millisecond, // 1ms blocks on 100Gbps
	}
}

// MainnetParams returns production parameters for 21 validators
func MainnetParams() Parameters {
	return Parameters{
		K:         21,
		Alpha:     0.8,
		Beta:      15,
		RoundTO:   400 * time.Millisecond,
		BlockTime: 200 * time.Millisecond,
	}
}

// TestnetParams returns test network parameters for 11 validators
func TestnetParams() Parameters {
	return Parameters{
		K:         11,
		Alpha:     0.7,
		Beta:      6,
		RoundTO:   225 * time.Millisecond,
		BlockTime: 100 * time.Millisecond,
	}
}

// LocalParams returns local development parameters for 5 validators
func LocalParams() Parameters {
	return Parameters{
		K:         5,
		Alpha:     0.6,
		Beta:      3,
		RoundTO:   45 * time.Millisecond,
		BlockTime: 10 * time.Millisecond,
	}
}

// WithBlockTime returns parameters with custom block time
func (p Parameters) WithBlockTime(blockTime time.Duration) Parameters {
	p.BlockTime = blockTime
	// Adjust round timeout proportionally
	if blockTime < 10*time.Millisecond {
		p.RoundTO = blockTime * 5
	}
	return p
}

// Validate ensures parameters are within safe bounds
func (p Parameters) Validate() error {
	if p.K < 1 {
		return ErrInvalidK
	}
	if p.Alpha < 0.5 || p.Alpha > 1.0 {
		return ErrInvalidAlpha
	}
	if p.Beta < 1 {
		return ErrInvalidBeta
	}
	if p.BlockTime < 1*time.Millisecond {
		return ErrBlockTimeTooLow
	}
	if p.RoundTO < p.BlockTime {
		return ErrRoundTimeoutTooLow
	}
	return nil
}
