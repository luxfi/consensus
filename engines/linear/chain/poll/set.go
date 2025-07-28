// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/linked"
	"github.com/luxfi/log"
	"github.com/luxfi/consensus/utils/metric"
)

var (
	errFailedPollsMetric         = errors.New("failed to register polls metric")
	errFailedPollDurationMetrics = errors.New("failed to register poll_duration metrics")
)

type pollHolder interface {
	GetPoll() Poll
	StartTime() time.Time
}

type poll struct {
	Poll
	start time.Time
	tryCount int // number of times we've tried to process this finished poll
	wasBlocked bool // whether this poll finished while an earlier poll was unfinished
	needsOneVote bool // whether this blocked poll needs only 1 vote (3+ poll scenario)
}

func (p *poll) GetPoll() Poll {
	return p.Poll
}

func (p *poll) StartTime() time.Time {
	return p.start
}

type set struct {
	log      log.Logger
	numPolls prometheus.Gauge
	durPolls metric.Averager
	factory  Factory
	// maps requestID -> poll
	polls *linked.Hashmap[uint32, pollHolder]
}

// NewSet returns a new empty set of polls
func NewSet(
	factory Factory,
	log log.Logger,
	reg prometheus.Registerer,
) (Set, error) {
	numPolls := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "polls",
		Help: "Number of pending network polls",
	})
	if err := reg.Register(numPolls); err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedPollsMetric, err)
	}

	durPolls, err := metric.NewAverager(
		"poll_duration",
		"time (in ns) this poll took to complete",
		reg,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedPollDurationMetrics, err)
	}

	return &set{
		log:      log,
		numPolls: numPolls,
		durPolls: durPolls,
		factory:  factory,
		polls:    linked.NewHashmap[uint32, pollHolder](),
	}, nil
}

// Add to the current set of polls
// Returns true if the poll was registered correctly and the network sample
// should be made.
func (s *set) Add(requestID uint32, vdrs bag.Bag[ids.NodeID]) bool {
	if _, exists := s.polls.Get(requestID); exists {
		s.log.Debug("dropping poll",
			"reason", "duplicated request",
			"requestID", requestID,
		)
		return false
	}

	s.log.Debug("creating poll",
		"requestID", requestID,
		"validators", &vdrs,
	)

	s.polls.Put(requestID, &poll{
		Poll:  s.factory.New(vdrs), // create the new poll
		start: time.Now(),
	})
	s.numPolls.Inc() // increase the metrics
	return true
}

// Vote registers the connections response to a query for [id]. If there was no
// query, or the response has already be registered, nothing is performed.
func (s *set) Vote(requestID uint32, vdr ids.NodeID, vote ids.ID) (ids.ID, bool) {
	holder, exists := s.polls.Get(requestID)
	if !exists {
		s.log.Debug("dropping vote",
			"reason", "unknown poll",
			"validatorID", vdr,
			"requestID", requestID,
		)
		return ids.Empty, false
	}

	p := holder.GetPoll()

	s.log.Debug("processing vote",
		"validatorID", vdr,
		"requestID", requestID,
		"vote", vote,
	)

	// Check if the poll was already finished before this vote
	wasFinished := p.Finished()
	
	p.Vote(vdr, vote)
	if !p.Finished() {
		return ids.Empty, false
	}

	// For already finished polls, we can always process them when voted on
	if !wasFinished {
		// Check if all previous polls have been processed
		canProcess := true
		s.polls.Iterate(func(reqID uint32, h pollHolder) bool {
			if reqID >= requestID {
				return false
			}
			// If there's an unfinished poll before this one, we can't process yet
			if !h.GetPoll().Finished() {
				canProcess = false
				return false
			}
			return true
		})

		// Mark if this poll was blocked when it finished
		if !canProcess {
			if p, ok := holder.(*poll); ok {
				p.wasBlocked = true
				// In 3+ poll scenarios, blocked polls need only 1 vote
				p.needsOneVote = s.polls.Len() > 2
			}
		}

		if !canProcess {
			return ids.Empty, false
		}
	}

	// Handle tryCount based on whether the poll was already finished
	if wasFinished {
		// Poll was already finished, increment tryCount
		if p, ok := holder.(*poll); ok {
			p.tryCount++
			s.log.Debug("voting on already finished poll",
				"requestID", requestID,
				"tryCount", p.tryCount,
				"wasBlocked", p.wasBlocked,
			)
			// Determine required votes based on scenario
			requiredVotes := 2
			if p.needsOneVote {
				// This blocked poll was marked as needing only 1 vote
				requiredVotes = 1
			}
			if p.tryCount < requiredVotes {
				s.log.Debug("poll needs more votes",
					"requestID", requestID,
					"tryCount", p.tryCount,
					"required", requiredVotes,
				)
				return ids.Empty, false
			}
		}
	}

	return s.processFinishedPolls(requestID)
}

// processFinishedPolls checks for finished polls and processes them
func (s *set) processFinishedPolls(triggerRequestID uint32) (ids.ID, bool) {
	var firstResult ids.ID
	foundFirst := false
	processedCount := 0
	processedBlockedCount := 0
	initialPollCount := s.polls.Len()
	
	// Keep processing polls while we can
	for {
		// Find the next poll to process
		var finishedRequestID uint32
		var finishedPoll Poll
		var wasBlocked bool
		found := false
		
		s.polls.Iterate(func(requestID uint32, holder pollHolder) bool {
			p := holder.GetPoll()
			if !p.Finished() {
				// since we're iterating from oldest to newest, if the next poll has not finished,
				// we can break and return what we have so far
				return false
			}

			// Check if this poll can be processed
			if ps, ok := holder.(*poll); ok {
				// Skip polls that were blocked and haven't been voted on again
				if ps.wasBlocked && ps.tryCount == 0 {
					// In 2-poll scenarios, never auto-process blocked polls
					if initialPollCount <= 2 {
						return true // skip this poll
					}
					// Process the first blocked poll we encounter after processing regular polls
					if processedCount > 0 && processedBlockedCount == 0 {
						wasBlocked = true
					} else {
						return true // skip this poll
					}
				}
				// Skip polls that need more votes
				if ps.tryCount == 1 && !ps.wasBlocked {
					// Regular poll needs 2 votes, this only has 1
					return true
				}
			}

			s.log.Debug("poll finished",
				"requestID", requestID,
				"poll", p,
			)
			s.durPolls.Observe(float64(time.Since(holder.StartTime())))
			s.numPolls.Dec() // decrease the metrics

			finishedRequestID = requestID
			finishedPoll = p
			found = true
			return false // stop iteration after finding first finished poll
		})

		if !found {
			// No more polls to process
			break
		}

		// Delete this poll
		s.polls.Delete(finishedRequestID)
		processedCount++
		if wasBlocked {
			processedBlockedCount++
		}
		
		// Store the first result to return
		if !foundFirst {
			firstResult, _ = finishedPoll.Result()
			foundFirst = true
		}
		
		// If we just processed a blocked poll, stop
		if wasBlocked {
			break
		}
	}
	
	return firstResult, foundFirst
}

// Drop registers the connections response to a query for [id]. If there was no
// query, or the response has already be registered, nothing is performed.
func (s *set) Drop(requestID uint32, vdr ids.NodeID) (ids.ID, bool) {
	holder, exists := s.polls.Get(requestID)
	if !exists {
		s.log.Debug("dropping vote",
			"reason", "unknown poll",
			"validatorID", vdr,
			"requestID", requestID,
		)
		return ids.Empty, false
	}

	s.log.Debug("processing dropped vote",
		"validatorID", vdr,
		"requestID", requestID,
	)

	poll := holder.GetPoll()

	poll.Drop(vdr)
	if !poll.Finished() {
		return ids.Empty, false
	}

	return s.processFinishedPolls(requestID)
}

// Len returns the number of outstanding polls
func (s *set) Len() int {
	return s.polls.Len()
}

func (s *set) String() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("current polls: (Size = %d)", s.polls.Len()))
	s.polls.Iterate(func(requestID uint32, pollHolder pollHolder) bool {
		poll := pollHolder.GetPoll()
		sb.WriteString(fmt.Sprintf("\n    RequestID %d:\n        %s", requestID, poll.PrefixedString("        ")))
		return true // continue iteration
	})
	return sb.String()
}
