// Copyright (C) 2019-2024, Lux Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pulsar

// Parameters configures the Pulsar engine
type Parameters struct {
	// K is the sample size
	K int
	
	// AlphaPreference is the preference threshold
	AlphaPreference int
	
	// AlphaConfidence is the confidence threshold
	AlphaConfidence int
	
	// Beta is the finalization threshold
	Beta int
}

// Valid validates the parameters
func (p Parameters) Valid() error {
	return nil
}