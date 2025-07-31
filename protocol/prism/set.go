// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/linked"
	"github.com/luxfi/log"
	"github.com/luxfi/consensus/utils/metric"
)

var (
	errFailedPrismsMetric         = errors.New("failed to register prisms metric")
	errFailedPollDurationMetrics = errors.New("failed to register poll_duration metrics")
)

type pollHolder interface {
	GetPoll() Poll
	StartTime() time.Time
}

type pollWrapper struct {
	Poll
	start time.Time
}

func (p pollWrapper) GetPoll() Poll {
	return p.Poll
}

func (p pollWrapper) StartTime() time.Time {
	return p.start
}

type set struct {
	log      log.Logger
	numPrisms prometheus.Gauge
	durPrisms metric.Averager
	factory  Factory
	// maps requestID -> poll
	prisms *linked.Hashmap[uint32, pollHolder]
}

// NewSet returns a new empty set of prisms
func NewSet(
	factory Factory,
	log log.Logger,
	reg prometheus.Registerer,
) (Set, error) {
	numPrisms := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prisms",
		Help: "Number of pending network prisms",
	})
	if err := reg.Register(numPrisms); err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedPrismsMetric, err)
	}

	durPrisms, err := metric.NewAverager(
		"poll_duration",
		"time (in ns) this prism took to complete",
		reg,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedPollDurationMetrics, err)
	}

	return &set{
		log:      log,
		numPrisms: numPrisms,
		durPrisms: durPrisms,
		factory:  factory,
		prisms:    linked.NewHashmap[uint32, pollHolder](),
	}, nil
}

// Add to the current set of prisms
// Returns true if the prism was registered correctly and the network sample
// should be made.
func (s *set) Add(requestID uint32, vdrs bag.Bag[ids.NodeID]) bool {
	if _, exists := s.prisms.Get(requestID); exists {
		s.log.Debug("dropping poll",
			zap.String("reason", "duplicated request"),
			zap.Uint32("requestID", requestID),
		)
		return false
	}

	s.log.Debug("creating poll",
		zap.Uint32("requestID", requestID),
		zap.Stringer("validators", &vdrs),
	)

	s.prisms.Put(requestID, pollWrapper{
		Poll:  s.factory.New(vdrs), // create the new poll
		start: time.Now(),
	})
	s.numPrisms.Inc() // increase the metrics
	return true
}

// Vote registers the connections response to a query for [id]. If there was no
// query, or the response has already be registered, nothing is performed.
func (s *set) Vote(requestID uint32, vdr ids.NodeID, vote ids.ID) (bag.Bag[ids.ID], bool) {
	holder, exists := s.prisms.Get(requestID)
	if !exists {
		s.log.Debug("dropping vote",
			zap.String("reason", "unknown poll"),
			zap.Stringer("validator", vdr),
			zap.Uint32("requestID", requestID),
		)
		return bag.Bag[ids.ID]{}, false
	}

	p := holder.GetPoll()

	s.log.Debug("processing vote",
		zap.Stringer("validator", vdr),
		zap.Uint32("requestID", requestID),
		zap.Stringer("vote", vote),
	)

	p.Vote(vdr, vote)
	if !p.Finished() {
		return bag.Bag[ids.ID]{}, false
	}

	// Check if this is the oldest poll
	iter := s.prisms.NewIterator()
	if !iter.Next() {
		return bag.Bag[ids.ID]{}, false
	}
	
	// If this prism is not the oldest one, we can't return results yet
	if iter.Key() != requestID {
		return bag.Bag[ids.ID]{}, false
	}

	results := s.processFinishedPrisms()
	// Return the first result if any, or empty bag
	if len(results) > 0 {
		return results[0], true
	}
	return bag.Bag[ids.ID]{}, true
}

// processFinishedPrisms checks for other dependent finished prisms and returns them all if finished
func (s *set) processFinishedPrisms() []bag.Bag[ids.ID] {
	var results []bag.Bag[ids.ID]

	// iterate from oldest to newest
	iter := s.prisms.NewIterator()
	for iter.Next() {
		holder := iter.Value()
		p := holder.GetPoll()
		if !p.Finished() {
			// since we're iterating from oldest to newest, if the next prism has not finished,
			// we can break and return what we have so far
			break
		}

		s.log.Debug("prism finished",
			zap.Uint32("requestID", iter.Key()),
			zap.String("poll", holder.GetPoll().PrefixedString("  ")),
		)
		s.durPrisms.Observe(float64(time.Since(holder.StartTime())))
		s.numPrisms.Dec() // decrease the metrics

		// Get the result
		result, ok := p.Result()
		// Create a bag with the result
		resultBag := bag.Bag[ids.ID]{}
		if ok && !result.IsZero() {
			// Add the result with its vote count
			voteCount := p.ResultVotes()
			resultBag.AddCount(result, voteCount)
		}
		results = append(results, resultBag)
		s.prisms.Delete(iter.Key())
	}

	// only gets here if the prism has finished
	// results will have values if this and other newer prisms have finished
	return results
}

// Drop registers the connections response to a query for [id]. If there was no
// query, or the response has already be registered, nothing is performed.
func (s *set) Drop(requestID uint32, vdr ids.NodeID) (bag.Bag[ids.ID], bool) {
	holder, exists := s.prisms.Get(requestID)
	if !exists {
		s.log.Debug("dropping vote",
			zap.String("reason", "unknown poll"),
			zap.Stringer("validator", vdr),
			zap.Uint32("requestID", requestID),
		)
		return bag.Bag[ids.ID]{}, false
	}

	s.log.Debug("processing dropped vote",
		zap.Stringer("validator", vdr),
		zap.Uint32("requestID", requestID),
	)

	prism := holder.GetPoll()

	prism.Drop(vdr)
	if !prism.Finished() {
		return bag.Bag[ids.ID]{}, false
	}

	// Check if this is the oldest poll
	iter := s.prisms.NewIterator()
	if !iter.Next() {
		return bag.Bag[ids.ID]{}, false
	}
	
	// If this prism is not the oldest one, we can't return results yet
	if iter.Key() != requestID {
		return bag.Bag[ids.ID]{}, false
	}

	results := s.processFinishedPrisms()
	// Return the first result if any, or empty bag
	if len(results) > 0 {
		return results[0], true
	}
	return bag.Bag[ids.ID]{}, true
}

// Len returns the number of outstanding prisms
func (s *set) Len() int {
	return s.prisms.Len()
}

func (s *set) String() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("current prisms: (Size = %d)", s.prisms.Len()))
	iter := s.prisms.NewIterator()
	for iter.Next() {
		requestID := iter.Key()
		holder := iter.Value()
		prism := holder.GetPoll()
		sb.WriteString(fmt.Sprintf("\n    RequestID %d:\n        %s", requestID, prism.PrefixedString("        ")))
	}
	return sb.String()
}
