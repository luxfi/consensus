// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package threshold

import (
	"github.com/luxfi/consensus/poll"
	"github.com/luxfi/ids"
)

// unaryWrapper adapts unaryThreshold to poll.Unary interface
type unaryWrapper struct {
	*unaryThreshold
}

// RecordPoll implements poll.Unary
func (u *unaryWrapper) RecordPoll(votes []ids.ID) {
	// Convert votes to count for unary consensus
	u.unaryThreshold.RecordPoll(len(votes))
}

// Preference implements poll.Unary
func (u *unaryWrapper) Preference() ids.ID {
	// Unary has no choice, return empty ID
	return ids.Empty
}

// Extend implements poll.Unary
func (u *unaryWrapper) Extend(choice int) poll.Binary {
	binary := u.unaryThreshold.Extend(choice)
	return &binaryWrapper{binaryThreshold: binary}
}

// Clone implements poll.Unary
func (u *unaryWrapper) Clone() poll.Unary {
	cloned := u.unaryThreshold.Clone()
	return &unaryWrapper{unaryThreshold: cloned}
}

// binaryWrapper adapts binaryThreshold to poll.Binary interface
type binaryWrapper struct {
	*binaryThreshold
}

// RecordPoll implements poll.Binary
func (b *binaryWrapper) RecordPoll(votes []ids.ID) {
	// Count votes for choice 0 and 1
	count0 := 0
	count1 := 0
	
	// Binary choice IDs: empty ID = 0, any other = 1
	for _, vote := range votes {
		if vote == ids.Empty {
			count0++
		} else {
			count1++
		}
	}
	
	// Determine winning choice
	if count0 > count1 {
		b.binaryThreshold.RecordPoll(count0, 0)
	} else if count1 > count0 {
		b.binaryThreshold.RecordPoll(count1, 1)
	}
	// If tied, don't record a poll
}

// Preference implements poll.Binary
func (b *binaryWrapper) Preference() ids.ID {
	pref := b.binaryThreshold.Preference()
	if pref == 0 {
		return ids.Empty
	}
	// Return a non-empty ID for choice 1
	return ids.GenerateTestID()
}

// nnaryWrapper adapts multiThreshold to poll.Nnary interface
type nnaryWrapper struct {
	*multiThreshold
}

// RecordPoll implements poll.Nnary
func (n *nnaryWrapper) RecordPoll(votes []ids.ID) {
	// Count votes for each choice
	voteCounts := make(map[ids.ID]int)
	for _, vote := range votes {
		voteCounts[vote]++
	}
	
	// Find the choice with the most votes
	var maxChoice ids.ID
	maxCount := 0
	for choice, count := range voteCounts {
		if count > maxCount {
			maxChoice = choice
			maxCount = count
		}
	}
	
	// Record poll for the winning choice
	if maxCount > 0 {
		n.multiThreshold.RecordPoll(maxCount, maxChoice)
	}
}

// Preference implements poll.Nnary (already returns ids.ID)
func (n *nnaryWrapper) Preference() ids.ID {
	return n.multiThreshold.Preference()
}