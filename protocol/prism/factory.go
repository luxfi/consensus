// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/metric"
	"github.com/luxfi/log"
)


// Factory creates new Polls
type Factory interface {
	New(vdrs bag.Bag[ids.NodeID]) Poll
}

// Poll represents an active poll
type Poll interface {
	Vote(vdr ids.NodeID, vote ids.ID)
	Drop(vdr ids.NodeID)
	Finished() bool
	Result() (ids.ID, bool)
	ResultVotes() int
	PrefixedString(prefix string) string
}

// factory implements Factory
type factory struct {
	log        log.Logger
	registerer prometheus.Registerer
	duration   metric.Averager
}

// NewFactory returns a new Factory
func NewFactory(
	log log.Logger,
	registerer prometheus.Registerer,
	alphaPreference int,
	alphaConfidence int,
) Factory {
	duration, _ := metric.NewAverager("poll", "duration", registerer)
	return &factory{
		log:        log,
		registerer: registerer,
		duration:   duration,
	}
}

// New creates a new Poll
func (f *factory) New(vdrs bag.Bag[ids.NodeID]) Poll {
	return &poll{
		votes:     make(map[ids.ID]int),
		vdrs:      vdrs,
		startTime: time.Now(),
	}
}

// poll implements Poll
type poll struct {
	votes           map[ids.ID]int
	vdrs            bag.Bag[ids.NodeID]
	voted           bag.Bag[ids.NodeID]
	startTime       time.Time
	finished        bool
	result          ids.ID
	resultVotes     int  // Track the vote count for the winning result
	alphaPreference int
	isUnary         bool
	isBinary        bool
	maxChoices      int
}

// Vote registers a vote
func (p *poll) Vote(vdr ids.NodeID, vote ids.ID) {
	if p.finished {
		return
	}

	weight := p.vdrs.Count(vdr)
	if weight == 0 {
		return
	}

	// Remove from remaining voters
	p.vdrs.Remove(vdr)
	p.voted.AddCount(vdr, weight)

	// Add vote
	p.votes[vote] += weight

	// Check if we have enough votes
	if p.vdrs.Len() == 0 {
		p.finished = true
		// Find the result with most votes
		maxVotes := 0
		for id, votes := range p.votes {
			if votes > maxVotes {
				maxVotes = votes
				p.result = id
				p.resultVotes = votes
			}
		}
	}
}

// Drop removes a voter
func (p *poll) Drop(vdr ids.NodeID) {
	if p.finished {
		return
	}

	p.vdrs.Remove(vdr)

	// Check if poll is finished
	if p.vdrs.Len() == 0 {
		p.finished = true
	}
}

// Finished returns true if the poll is complete
func (p *poll) Finished() bool {
	return p.finished
}

// Result returns the poll result
func (p *poll) Result() (ids.ID, bool) {
	if !p.finished {
		return ids.Empty, false
	}
	return p.result, !p.result.IsZero()
}

// ResultVotes returns the vote count for the winning result
func (p *poll) ResultVotes() int {
	return p.resultVotes
}

// PrefixedString returns a string representation with a prefix
func (p *poll) PrefixedString(prefix string) string {
	return fmt.Sprintf(
		"waiting on %s\n%svoted %s",
		p.vdrs.PrefixedString(prefix),
		prefix,
		p.voted.PrefixedString(prefix),
	)
}

// Sampler samples validators for prism sampling
type Sampler interface {
	Sample(validators bag.Bag[ids.NodeID], numToSample int) (bag.Bag[ids.NodeID], error)
}

// uniformSampler samples uniformly
type uniformSampler struct{}

// NewUniformSampler returns a uniform sampler
func NewUniformSampler() Sampler {
	return &uniformSampler{}
}

// Sample returns a uniform sample
func (s *uniformSampler) Sample(validators bag.Bag[ids.NodeID], numToSample int) (bag.Bag[ids.NodeID], error) {
	list := validators.List()
	if numToSample >= len(list) {
		return validators, nil
	}

	sample := bag.Bag[ids.NodeID]{}
	// Simple uniform sampling
	for i := 0; i < numToSample && i < len(list); i++ {
		sample.Add(list[i])
	}
	return sample, nil
}