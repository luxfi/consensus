// Copyright (C) 2019-2024, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testing

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
)

var (
	Red   = ids.Empty.Prefix(0)
	Blue  = ids.Empty.Prefix(1)
	Green = ids.Empty.Prefix(2)

	_ Consensus = (*Byzantine)(nil)
)

func NewByzantine(_ Factory, _ Parameters, choice ids.ID) Consensus {
	return &Byzantine{
		preference: choice,
	}
}

// Byzantine is a naive implementation of a multi-choice snowball instance
type Byzantine struct {
	// Hardcode the preference
	preference ids.ID
	finalized  bool
}

func (b *Byzantine) Add(choice ids.ID) {
	// Byzantine doesn't change preference based on Add
}

func (b *Byzantine) Preference() ids.ID {
	return b.preference
}

func (b *Byzantine) RecordPoll(votes bag.Bag[ids.ID]) {
	// Byzantine doesn't change based on polls
	// Could mark as finalized after sufficient polls
}

func (b *Byzantine) Finalized() bool {
	return b.finalized
}

func (b *Byzantine) String() string {
	return b.preference.String()
}