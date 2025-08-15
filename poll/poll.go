// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/node/utils/bag"
	"github.com/prometheus/client_golang/prometheus"
)

// Set manages a collection of polls
type Set interface {
	Add(requestID uint32, validators []ids.NodeID) bool
	Vote(requestID uint32, nodeID ids.NodeID, vote ids.ID, votes []ids.ID) []bag.Bag[ids.ID]
	Len() int
	String() string
}

// Factory creates new Poll instances
type Factory interface {
	New(vdrs []ids.NodeID) Poll
}

// Poll represents a single poll
type Poll interface {
	Vote(nodeID ids.NodeID, vote ids.ID, votes []ids.ID) []bag.Bag[ids.ID]
	Finished() bool
	Result() []bag.Bag[ids.ID]
	String() string
}

// set implements Set
type set struct {
	log     log.Logger
	factory Factory
	polls   map[uint32]Poll
}

// NewSet creates a new poll set
func NewSet(factory Factory, log log.Logger, registerer prometheus.Registerer) (Set, error) {
	return &set{
		log:     log,
		factory: factory,
		polls:   make(map[uint32]Poll),
	}, nil
}

func (s *set) Add(requestID uint32, validators []ids.NodeID) bool {
	if _, exists := s.polls[requestID]; exists {
		return false
	}
	s.polls[requestID] = s.factory.New(validators)
	return true
}

func (s *set) Vote(requestID uint32, nodeID ids.NodeID, vote ids.ID, votes []ids.ID) []bag.Bag[ids.ID] {
	poll, exists := s.polls[requestID]
	if !exists {
		return nil
	}

	result := poll.Vote(nodeID, vote, votes)
	if poll.Finished() {
		delete(s.polls, requestID)
	}
	return result
}

func (s *set) Len() int {
	return len(s.polls)
}

func (s *set) String() string {
	return "PollSet"
}

// earlyTermFactory implements Factory for early termination polls
type earlyTermFactory struct {
	alphaPreference int
	alphaConfidence int
	consensus       interface{}
}

// NewEarlyTermFactory creates a new early termination poll factory
func NewEarlyTermFactory(
	alphaPreference int,
	alphaConfidence int,
	registerer prometheus.Registerer,
	consensus interface{},
) (Factory, error) {
	return &earlyTermFactory{
		alphaPreference: alphaPreference,
		alphaConfidence: alphaConfidence,
		consensus:       consensus,
	}, nil
}

func (f *earlyTermFactory) New(vdrs []ids.NodeID) Poll {
	return &earlyTermPoll{
		alphaPreference: f.alphaPreference,
		alphaConfidence: f.alphaConfidence,
		validators:      vdrs,
		votes:           make(map[ids.NodeID]ids.ID),
		finished:        false,
	}
}

// earlyTermPoll implements Poll with early termination
type earlyTermPoll struct {
	alphaPreference int
	alphaConfidence int
	validators      []ids.NodeID
	votes           map[ids.NodeID]ids.ID
	finished        bool
	result          []bag.Bag[ids.ID]
}

func (p *earlyTermPoll) Vote(nodeID ids.NodeID, vote ids.ID, votes []ids.ID) []bag.Bag[ids.ID] {
	if p.finished {
		return p.result
	}

	// Record the vote
	p.votes[nodeID] = vote

	// Check if we have enough votes for early termination
	voteCount := make(map[ids.ID]int)
	for _, v := range p.votes {
		voteCount[v]++
	}

	// Check for early termination conditions
	for vote, count := range voteCount {
		if count >= p.alphaConfidence {
			p.finished = true
			b := bag.Bag[ids.ID]{}
			b.AddCount(vote, count)
			p.result = []bag.Bag[ids.ID]{b}
			return p.result
		}
	}

	// Check if all validators have voted
	if len(p.votes) >= len(p.validators) {
		p.finished = true
		b := bag.Bag[ids.ID]{}
		for vote, count := range voteCount {
			b.AddCount(vote, count)
		}
		p.result = []bag.Bag[ids.ID]{b}
		return p.result
	}

	return nil
}

func (p *earlyTermPoll) Finished() bool {
	return p.finished
}

func (p *earlyTermPoll) Result() []bag.Bag[ids.ID] {
	return p.result
}

func (p *earlyTermPoll) String() string {
	return "EarlyTermPoll"
}
