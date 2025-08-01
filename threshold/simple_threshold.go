// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"fmt"

	"github.com/luxfi/ids"
)

// BinaryThresholdParameters contains the parameters for binary threshold
type BinaryThresholdParameters struct {
	AlphaPreference int
	AlphaConfidence int
	Beta            int
}

// BinaryThreshold implements binary consensus
type BinaryThreshold struct {
	preference          ids.ID
	numSuccessfulPrisms  int
	params              *BinaryThresholdParameters
	finalized           bool
}

// NewBinaryThreshold creates a new binary threshold instance
func NewBinaryThreshold() *BinaryThreshold {
	return &BinaryThreshold{}
}

// Initialize sets up the binary threshold with parameters
func (bt *BinaryThreshold) Initialize(params *BinaryThresholdParameters, initialPreference ids.ID) {
	bt.params = params
	bt.preference = initialPreference
	bt.numSuccessfulPrisms = 0
	bt.finalized = false
}

// SetPreference sets the current preference
func (bt *BinaryThreshold) SetPreference(pref ids.ID) {
	if !bt.finalized {
		bt.preference = pref
	}
}

// Preference returns the current preference
func (bt *BinaryThreshold) Preference() ids.ID {
	return bt.preference
}

// RecordPrism records the result of a poll
func (bt *BinaryThreshold) RecordPrism(count int) {
	if bt.finalized {
		return
	}

	if count >= bt.params.AlphaConfidence {
		bt.numSuccessfulPrisms++
		if bt.numSuccessfulPrisms >= bt.params.Beta {
			bt.finalized = true
		}
	} else {
		bt.numSuccessfulPrisms = 0
	}
}

// Finalized returns whether consensus has been reached
func (bt *BinaryThreshold) Finalized() bool {
	return bt.finalized
}

// Confidence returns the current confidence level
func (bt *BinaryThreshold) Confidence() int {
	return bt.numSuccessfulPrisms
}

func (bt *BinaryThreshold) String() string {
	return fmt.Sprintf("BinaryThreshold{preference: %s, confidence: %d/%d, finalized: %v}",
		bt.preference, bt.numSuccessfulPrisms, bt.params.Beta, bt.finalized)
}