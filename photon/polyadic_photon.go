// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"fmt"

	"github.com/luxfi/ids"
)

// NewPolyadicPhoton creates a new polyadic photon instance
func NewPolyadicPhoton(choice ids.ID) PolyadicPhoton {
	return PolyadicPhoton{
		preference: choice,
	}
}

// PolyadicPhoton is the implementation of a photon instance with an unbounded number
// of choices
type PolyadicPhoton struct {
	// preference is the choice that last had a successful poll. Unless there
	// hasn't been a successful poll, in which case it is the initially provided
	// choice.
	preference ids.ID
}

func (pp *PolyadicPhoton) Preference() ids.ID {
	return pp.preference
}

func (pp *PolyadicPhoton) RecordSuccessfulPoll(choice ids.ID) {
	pp.preference = choice
}

func (pp *PolyadicPhoton) String() string {
	return fmt.Sprintf("PolyadicPhoton(Preference = %s)", pp.preference)
}