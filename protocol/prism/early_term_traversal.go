// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
)

var (
	errPollDurationVectorMetrics = errors.New("failed to register poll_duration vector metrics")
	errPollCountVectorMetrics    = errors.New("failed to register poll_count vector metrics")

	terminationReason = "reason"
	exhaustedReason   = "exhausted"
	earlyFailReason   = "early_fail"
	earlyAlphaReason  = "early_alpha"

	exhaustedLabel = prometheus.Labels{
		terminationReason: exhaustedReason,
	}
	earlyFailLabel = prometheus.Labels{
		terminationReason: earlyFailReason,
	}
	earlyAlphaLabel = prometheus.Labels{
		terminationReason: earlyAlphaReason,
	}
)

type earlyTermMetrics struct {
	durExhaustedPrisms  prometheus.Gauge
	durEarlyFailPrisms  prometheus.Gauge
	durEarlyAlphaPrisms prometheus.Gauge

	countExhaustedPrisms  prometheus.Counter
	countEarlyFailPrisms  prometheus.Counter
	countEarlyAlphaPrisms prometheus.Counter
}

func newEarlyTermMetrics(reg prometheus.Registerer) (*earlyTermMetrics, error) {
	pollCountVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "poll_count",
		Help: "Total # of terminated prisms by reason",
	}, []string{terminationReason})
	if err := reg.Register(pollCountVec); err != nil {
		return nil, fmt.Errorf("%w: %w", errPollCountVectorMetrics, err)
	}
	durPrismsVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "poll_duration",
		Help: "time (in ns) prisms took to complete by reason",
	}, []string{terminationReason})
	if err := reg.Register(durPrismsVec); err != nil {
		return nil, fmt.Errorf("%w: %w", errPollDurationVectorMetrics, err)
	}

	return &earlyTermMetrics{
		durExhaustedPrisms:    durPrismsVec.With(exhaustedLabel),
		durEarlyFailPrisms:    durPrismsVec.With(earlyFailLabel),
		durEarlyAlphaPrisms:   durPrismsVec.With(earlyAlphaLabel),
		countExhaustedPrisms:  pollCountVec.With(exhaustedLabel),
		countEarlyFailPrisms:  pollCountVec.With(earlyFailLabel),
		countEarlyAlphaPrisms: pollCountVec.With(earlyAlphaLabel),
	}, nil
}

func (m *earlyTermMetrics) observeExhausted(duration time.Duration) {
	m.durExhaustedPrisms.Add(float64(duration.Nanoseconds()))
	m.countExhaustedPrisms.Inc()
}

func (m *earlyTermMetrics) observeEarlyFail(duration time.Duration) {
	m.durEarlyFailPrisms.Add(float64(duration.Nanoseconds()))
	m.countEarlyFailPrisms.Inc()
}

func (m *earlyTermMetrics) observeEarlyAlpha(duration time.Duration) {
	m.durEarlyAlphaPrisms.Add(float64(duration.Nanoseconds()))
	m.countEarlyAlphaPrisms.Inc()
}

type earlyTermTraversalFactory struct {
	alphaPreference int
	alphaConfidence int
	bt              BlockTraversal
	metrics         *earlyTermMetrics
}

type BlockTraversal interface {
	GetParent(id ids.ID) (ids.ID, bool)
}

// NewEarlyTermFactory returns a factory that returns prisms with early termination.
func NewEarlyTermFactory(
	alphaPreference int,
	alphaConfidence int,
	reg prometheus.Registerer,
	bt BlockTraversal,
) (Factory, error) {
	metrics, err := newEarlyTermMetrics(reg)
	if err != nil {
		return nil, err
	}

	return &earlyTermTraversalFactory{
		bt:              bt,
		alphaPreference: alphaPreference,
		alphaConfidence: alphaConfidence,
		metrics:         metrics,
	}, nil
}

func (f *earlyTermTraversalFactory) New(vdrs bag.Bag[ids.NodeID]) Poll {
	return &earlyTermPrism{
		bt:              f.bt,
		polled:          vdrs,
		alphaPreference: f.alphaPreference,
		alphaConfidence: f.alphaConfidence,
		metrics:         f.metrics,
		start:           time.Now(),
	}
}

// earlyTermPrism finishes when any remaining validators can't change
// the result of the prism for all the votes and transitive votes.
type earlyTermPrism struct {
	votes           bag.Bag[ids.ID]
	polled          bag.Bag[ids.NodeID]
	alphaPreference int
	alphaConfidence int
	bt              BlockTraversal
	metrics         *earlyTermMetrics
	start           time.Time
	finished        bool
}

// Vote registers a response for this poll
func (p *earlyTermPrism) Vote(vdr ids.NodeID, vote ids.ID) {
	count := p.polled.Count(vdr)
	// make sure that a validator can't respond multiple times
	p.polled.Remove(vdr)

	// track the votes the validator responded with
	p.votes.AddCount(vote, count)
}

// Drop any future response for this poll
func (p *earlyTermPrism) Drop(vdr ids.NodeID) {
	p.polled.Remove(vdr)
}

// Finished returns true when one of the following conditions is met.
//
//  1. There are no outstanding votes.
//  2. It is impossible for the prism to achieve an alphaPreference majority
//     after applying transitive voting.
//  3. A single element has achieved an alphaPreference majority and it is
//     impossible for it to achieve an alphaConfidence majority after applying
//     transitive voting.
//  4. A single element has achieved an alphaConfidence majority.
func (p *earlyTermPrism) Finished() bool {
	if p.finished {
		return true
	}

	remaining := p.polled.Len()
	if remaining == 0 {
		p.finished = true
		p.metrics.observeExhausted(time.Since(p.start))
		return true // Case 1
	}

	received := p.votes.Len()
	maxPossibleVotes := received + remaining
	if maxPossibleVotes < p.alphaPreference {
		p.finished = true
		p.metrics.observeEarlyFail(time.Since(p.start))
		return true // Case 2
	}

	//    v
	//   /
	//  u
	// We build a vote graph where each vertex represents a block ID.
	// A vertex 'v' is a parent of vertex 'u' if the ID of 'u' corresponds
	// to a block that is the successive block of the corresponding block for 'v'.
	votesGraph := buildVoteGraph(p.bt.GetParent, p.votes)

	// If vertex 'v' is a parent of vertex 'u', then a vote for the ID of vertex 'u'
	// should also be considered as a vote for the ID of the vertex 'v'.
	transitiveVotes := computeTransitiveVoteCountGraph(&votesGraph, p.votes)

	//     v
	//   /   \
	//  u     w
	// If two competing blocks 'u', 'w' are potential successors to a block 'v',
	// beam would instantiate a monadic wave instance on the prefix of 'u' and 'w'.
	// The prefix inherits the votes for the IDs of 'u' and 'w'.
	// We therefore compute the transitive votes for all prefixes of IDs
	// for each bifurcation in the transitive vote graph.
	transitiveVotesForPrefixes := computeTransitiveVotesForPrefixes(&votesGraph, transitiveVotes)

	// We wish to compute the votes for wave instances, no matter if they correspond to an actual block ID,
	// or a unary wave instance for a shared prefix between a bifurcation of two competing blocks.
	// For that, only the number of votes and existence of such wave instances matters.
	voteCountsForIDsOrPrefixes := aggregateVotesFromPrefixesAndIDs(transitiveVotesForPrefixes, transitiveVotes)

	// Given the aforementioned votes, we wish to see whether there exists a wave instance
	// that can benefit from waiting for more invocations of Vote().
	// We therefore check each amount of votes separately and see if voting for that wave instance
	// should terminate, as it cannot be improved by further voting.

	// If we have no votes, we may be able to improve the prism on some ID.
	weCantImproveVoteForSomeIDOrPrefix := len(voteCountsForIDsOrPrefixes) > 0

	// Consider the votes for each ID or prefix of IDs,
	// if we shouldn't terminate in one of them, then we should not terminate this prism now.
	for _, completedVotes := range voteCountsForIDsOrPrefixes {
		weCantImproveVoteForSomeIDOrPrefix = weCantImproveVoteForSomeIDOrPrefix && p.shouldTerminate(completedVotes, remaining)
		if !weCantImproveVoteForSomeIDOrPrefix {
			break
		}
	}

	// We should terminate the prism only when votes for all IDs or prefixes cannot be improved.
	if weCantImproveVoteForSomeIDOrPrefix {
		p.finished = true
		p.metrics.observeEarlyAlpha(time.Since(p.start))
	}

	return p.finished
}

func (p *earlyTermPrism) shouldTerminate(freq int, remaining int) bool {
	maxPossibleVotes := freq + remaining
	return maxPossibleVotes < p.alphaPreference || // Case 2
		(freq >= p.alphaPreference && maxPossibleVotes < p.alphaConfidence) || // Case 3
		freq >= p.alphaConfidence // Case 4
}

// Result returns the result of this poll
func (p *earlyTermPrism) Result() (ids.ID, bool) {
	// Find the ID with the most votes
	var result ids.ID
	maxVotes := 0
	
	for _, id := range p.votes.List() {
		count := p.votes.Count(id)
		if count > maxVotes {
			result = id
			maxVotes = count
		}
	}
	
	// Return the result and whether we have any votes
	return result, maxVotes > 0
}

// ResultVotes returns the vote count for the winning result
func (p *earlyTermPrism) ResultVotes() int {
	result, ok := p.Result()
	if !ok {
		return 0
	}
	return p.votes.Count(result)
}

func (p *earlyTermPrism) PrefixedString(prefix string) string {
	return fmt.Sprintf(
		"waiting on %s\n%sreceived %s",
		p.polled.PrefixedString(prefix),
		prefix,
		p.votes.PrefixedString(prefix),
	)
}

func (p *earlyTermPrism) String() string {
	return p.PrefixedString("")
}

func aggregateVotesFromPrefixesAndIDs(transitiveVotesForPrefixes []int, transitiveVotes bag.Bag[ids.ID]) []int {
	transitiveVoteIDs := transitiveVotes.List()
	voteCountsForIDsOrPrefixes := make([]int, 0, len(transitiveVoteIDs)+len(transitiveVotesForPrefixes))
	for _, id := range transitiveVoteIDs {
		votesForID := transitiveVotes.Count(id)
		voteCountsForIDsOrPrefixes = append(voteCountsForIDsOrPrefixes, votesForID)
	}
	voteCountsForIDsOrPrefixes = append(voteCountsForIDsOrPrefixes, transitiveVotesForPrefixes...)
	return voteCountsForIDsOrPrefixes
}

func computeTransitiveVotesForPrefixes(votesGraph *voteGraph, transitiveVotes bag.Bag[ids.ID]) []int {
	var votesForPrefix []int
	votesGraph.traverse(func(v *voteVertex) {
		descendantIDs := descendantIDsOfVertex(v)
		pg := longestSharedPrefixes(descendantIDs)
		// Each shared prefix is associated with a bunch of IDs.
		// Sum up all the transitive votes for these blocks,
		// and return all such shared prefixes indexed by the underlying transitive descendant IDs.
		pg.bifurcationsWithCommonPrefix(func(ids []ids.ID) {
			count := sumVotesFromIDs(ids, transitiveVotes)
			votesForPrefix = append(votesForPrefix, count)
		})
	})
	return votesForPrefix
}

func descendantIDsOfVertex(v *voteVertex) []ids.ID {
	descendantIDs := make([]ids.ID, len(v.descendants))
	for i, child := range v.descendants {
		descendantIDs[i] = child.id
	}
	return descendantIDs
}

func sumVotesFromIDs(ids []ids.ID, transitiveVotes bag.Bag[ids.ID]) int {
	var count int
	for _, id := range ids {
		count += transitiveVotes.Count(id)
	}
	return count
}
