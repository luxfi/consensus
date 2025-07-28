// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package factories

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/luxfi/consensus/poll"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// Parameters for consensus operations
type Parameters struct {
	K               int
	AlphaPreference int
	AlphaConfidence int
	Beta            int
}

// ConfidenceFactory creates confidence-based polls
type ConfidenceFactory struct {
	log        log.Logger
	registerer prometheus.Registerer
	params     Parameters
}

// NewConfidenceFactory creates a new confidence factory
func NewConfidenceFactory(log log.Logger, registerer prometheus.Registerer, params Parameters) poll.Factory {
	return &ConfidenceFactory{
		log:        log,
		registerer: registerer,
		params:     params,
	}
}

// New creates a new poll
func (f *ConfidenceFactory) New(vdrs bag.Bag[ids.NodeID]) poll.Poll {
	return &confidencePoll{
		vdrs:   vdrs,
		votes:  make(map[ids.ID]int),
		params: f.params,
	}
}

// confidencePoll implements poll.Poll using confidence thresholds
type confidencePoll struct {
	vdrs     bag.Bag[ids.NodeID]
	votes    map[ids.ID]int
	params   Parameters
	finished bool
	result   ids.ID
}

// Vote registers a vote
func (p *confidencePoll) Vote(vdr ids.NodeID, vote ids.ID) {
	if p.finished || p.vdrs.Count(vdr) == 0 {
		return
	}

	weight := p.vdrs.Count(vdr)
	p.vdrs.Remove(vdr)
	p.votes[vote] += weight

	// Check if we have alpha preference
	if p.votes[vote] >= p.params.AlphaPreference {
		p.result = vote
		if p.votes[vote] >= p.params.AlphaConfidence {
			p.finished = true
		}
	}
}

// Drop removes a voter
func (p *confidencePoll) Drop(vdr ids.NodeID) {
	p.vdrs.Remove(vdr)
	if p.vdrs.Len() == 0 {
		p.finished = true
	}
}

// Finished returns true if the poll is complete
func (p *confidencePoll) Finished() bool {
	return p.finished
}

// Result returns the poll result
func (p *confidencePoll) Result() (ids.ID, bool) {
	return p.result, p.finished
}

// PrefixedString returns a prefixed string representation
func (p *confidencePoll) PrefixedString(prefix string) string {
	return fmt.Sprintf("votes: %v", p.votes)
}

// String returns a string representation
func (p *confidencePoll) String() string {
	return p.PrefixedString("")
}

// FlatFactory creates threshold-based polls
type FlatFactory struct {
	log        log.Logger
	registerer prometheus.Registerer
	params     Parameters
}

// NewFlatFactory creates a new flat factory
func NewFlatFactory(log log.Logger, registerer prometheus.Registerer, params Parameters) poll.Factory {
	return &FlatFactory{
		log:        log,
		registerer: registerer,
		params:     params,
	}
}

// New creates a new poll
func (f *FlatFactory) New(vdrs bag.Bag[ids.NodeID]) poll.Poll {
	return &flatPoll{
		vdrs:   vdrs,
		votes:  make(map[ids.ID]int),
		params: f.params,
	}
}

// flatPoll implements poll.Poll using flat thresholds
type flatPoll struct {
	vdrs     bag.Bag[ids.NodeID]
	votes    map[ids.ID]int
	params   Parameters
	finished bool
	result   ids.ID
}

// Vote registers a vote
func (p *flatPoll) Vote(vdr ids.NodeID, vote ids.ID) {
	if p.finished || p.vdrs.Count(vdr) == 0 {
		return
	}

	weight := p.vdrs.Count(vdr)
	p.vdrs.Remove(vdr)
	p.votes[vote] += weight

	// Simple majority check
	totalWeight := len(p.votes) + p.vdrs.Len()
	if p.votes[vote] > totalWeight/2 {
		p.result = vote
		p.finished = true
	}
}

// Drop removes a voter
func (p *flatPoll) Drop(vdr ids.NodeID) {
	p.vdrs.Remove(vdr)
	if p.vdrs.Len() == 0 {
		p.finished = true
	}
}

// Finished returns true if the poll is complete
func (p *flatPoll) Finished() bool {
	return p.finished
}

// Result returns the poll result
func (p *flatPoll) Result() (ids.ID, bool) {
	return p.result, p.finished
}

// PrefixedString returns a prefixed string representation
func (p *flatPoll) PrefixedString(prefix string) string {
	return fmt.Sprintf("votes: %v", p.votes)
}

// String returns a string representation
func (p *flatPoll) String() string {
	return p.PrefixedString("")
}