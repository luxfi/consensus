// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nebula

import (
	"errors"
	"fmt"
	"time"
)

// Parameters configures the Nebula engine
type Parameters struct {
	// K is the sample size
	K int
	
	// AlphaPreference is the preference threshold
	AlphaPreference int
	
	// AlphaConfidence is the confidence threshold
	AlphaConfidence int
	
	// Beta is the finalization threshold
	Beta int
	
	// Advanced parameters
	ConcurrentPolls     int
	OptimalProcessing     int
	MaxOutstandingItems   int
	MaxItemProcessingTime time.Duration
	
	// DAG-specific parameters
	MaxParents      int
	ConflictSetSize int
}

// Valid validates the parameters
func (p Parameters) Valid() error {
	switch {
	case p.K <= 0:
		return errors.New("k must be positive")
	case p.AlphaPreference <= p.K/2:
		return errors.New("alpha preference must be greater than k/2")
	case p.AlphaPreference > p.K:
		return errors.New("alpha preference must be less than or equal to k")
	case p.AlphaConfidence < p.AlphaPreference:
		return errors.New("alpha confidence must be greater than or equal to alpha preference")
	case p.AlphaConfidence > p.K:
		return errors.New("alpha confidence must be less than or equal to k")
	case p.Beta <= 0:
		return errors.New("beta must be positive")
	case p.Beta > p.K:
		return fmt.Errorf("beta (%d) must be <= k (%d)", p.Beta, p.K)
	case p.ConcurrentPolls <= 0:
		return errors.New("concurrent polls must be positive")
	case p.OptimalProcessing <= 0:
		return errors.New("optimal processing must be positive")
	case p.MaxOutstandingItems <= 0:
		return errors.New("max outstanding items must be positive")
	case p.MaxItemProcessingTime <= 0:
		return errors.New("max item processing time must be positive")
	}
	
	// DAG-specific validation
	if p.MaxParents < 0 {
		return errors.New("max parents cannot be negative")
	}
	if p.ConflictSetSize < 0 {
		return errors.New("conflict set size cannot be negative")
	}
	
	return nil
}