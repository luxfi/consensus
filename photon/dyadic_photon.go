// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import "fmt"

// NewDyadicPhoton creates a new dyadic photon instance
func NewDyadicPhoton(choice int) DyadicPhoton {
	return DyadicPhoton{
		preference: choice,
	}
}

// DyadicPhoton is the implementation of a dyadic photon instance
type DyadicPhoton struct {
	// preference is the choice that last had a successful poll. Unless there
	// hasn't been a successful poll, in which case it is the initially provided
	// choice.
	preference int
}

func (dp *DyadicPhoton) Preference() int {
	return dp.preference
}

func (dp *DyadicPhoton) RecordSuccessfulPoll(choice int) {
	dp.preference = choice
}

func (dp *DyadicPhoton) String() string {
	return fmt.Sprintf("DyadicPhoton(Preference = %d)", dp.preference)
}